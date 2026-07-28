package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

type ModuleHealth struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Diagnostics struct {
	Status        string                  `json:"status"`
	Service       string                  `json:"service"`
	SchemaVersion int                     `json:"schema_version,omitempty"`
	Modules       map[string]ModuleHealth `json:"modules"`
	CheckedAt     time.Time               `json:"checked_at"`
}

type DiagnosticsProvider interface {
	Diagnostics(ctx context.Context) Diagnostics
}

type DiagnosticsFunc func(context.Context) Diagnostics

func (fn DiagnosticsFunc) Diagnostics(ctx context.Context) Diagnostics {
	return fn(ctx)
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if id == "" || len(id) > 128 {
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err == nil {
				id = hex.EncodeToString(raw[:])
			} else {
				id = fmt.Sprintf("%d", time.Now().UnixNano())
			}
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func RequestIDFromContext(c *gin.Context) string {
	value, _ := c.Get(requestIDKey)
	id, _ := value.(string)
	return id
}

func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		record := struct {
			Level     string `json:"level"`
			Event     string `json:"event"`
			RequestID string `json:"request_id"`
			Method    string `json:"method"`
			Path      string `json:"path"`
			Status    int    `json:"status"`
			Duration  int64  `json:"duration_ms"`
		}{
			Level:     "info",
			Event:     "http_request",
			RequestID: RequestIDFromContext(c),
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			Status:    c.Writer.Status(),
			Duration:  time.Since(started).Milliseconds(),
		}
		if record.Status >= 500 {
			record.Level = "error"
		}
		data, _ := json.Marshal(record)
		log.Print(string(data))
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				record := struct {
					Level     string `json:"level"`
					Event     string `json:"event"`
					RequestID string `json:"request_id"`
					Path      string `json:"path"`
				}{
					Level: "error", Event: "http_panic",
					RequestID: RequestIDFromContext(c), Path: c.Request.URL.Path,
				}
				data, _ := json.Marshal(record)
				log.Print(string(data))
				WriteError(c, http.StatusInternalServerError, "internal_error", "服务内部错误")
			}
		}()
		c.Next()
	}
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   any    `json:"details,omitempty"`
}

type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

func WriteError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorEnvelope{Error: APIError{
		Code:      code,
		Message:   message,
		RequestID: RequestIDFromContext(c),
	}})
}

type captureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *captureWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *captureWriter) WriteString(value string) (int, error) {
	return w.body.WriteString(value)
}

// ErrorEnvelopeMiddleware provides a compatibility bridge while individual
// handlers migrate to domain errors. It converts legacy {"error":"..."}
// failures into the stable envelope without exposing their internal message.
func ErrorEnvelopeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.Contains(c.GetHeader("Accept"), "text/event-stream") ||
			strings.HasSuffix(c.Request.URL.Path, "/stream") {
			c.Next()
			return
		}
		original := c.Writer
		capture := &captureWriter{ResponseWriter: original}
		c.Writer = capture
		defer func() {
			c.Writer = original
			status := capture.Status()
			body := capture.body.Bytes()
			if len(body) == 0 {
				return
			}
			if status >= 400 && strings.Contains(original.Header().Get("Content-Type"), "application/json") {
				var legacy map[string]any
				if json.Unmarshal(body, &legacy) == nil {
					if _, already := legacy["error"].(map[string]any); !already {
						code, message := statusError(status)
						body, _ = json.Marshal(ErrorEnvelope{Error: APIError{
							Code:      code,
							Message:   message,
							RequestID: RequestIDFromContext(c),
						}})
					}
				}
			}
			original.Header().Del("Content-Length")
			_, _ = original.Write(body)
		}()
		c.Next()
	}
}

func statusError(status int) (string, string) {
	switch status {
	case http.StatusBadRequest:
		return "validation_failed", "请求参数无效"
	case http.StatusUnauthorized:
		return "unauthorized", "需要有效的访问令牌"
	case http.StatusForbidden:
		return "forbidden", "无权执行该操作"
	case http.StatusNotFound:
		return "not_found", "请求的资源不存在"
	case http.StatusConflict:
		return "multiple_matches", "请求匹配到多个结果"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large", "请求体超过允许大小"
	case http.StatusGatewayTimeout:
		return "upstream_timeout", "上游服务响应超时"
	case http.StatusServiceUnavailable:
		return "service_unavailable", "服务暂时不可用"
	default:
		return "internal_error", "服务内部错误"
	}
}
