package server

import (
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

// WriteErrorWithDetails writes the error envelope plus preserved top-level
// keys, mirroring exactly what the retired ErrorEnvelopeMiddleware produced for a legacy
// mixed-key body: each extra value is round-tripped through json so structs
// re-marshal with sorted key order and numbers keep float64 formatting, the
// envelope is marshaled from a map so top-level keys stay sorted, and an
// extra "error" key is dropped in favor of the envelope (the middleware
// never surfaces the handler's own error string). Callers pass
// code, message := statusError(status), like every plain WriteError site.
func WriteErrorWithDetails(c *gin.Context, status int, code, message string, extra map[string]any) {
	envelope := map[string]any{"error": APIError{
		Code:      code,
		Message:   message,
		RequestID: RequestIDFromContext(c),
	}}
	for key, value := range extra {
		if key == "error" {
			continue
		}
		envelope[key] = normalizeJSONValue(value)
	}
	c.AbortWithStatusJSON(status, envelope)
}

// normalizeJSONValue round-trips a value through json.Marshal /
// json.Unmarshal into any. The middleware re-marshals values parsed from the
// legacy body, so this round-trip is what keeps converted responses
// byte-identical; a marshal failure returns the original value.
func normalizeJSONValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return value
	}
	return out
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
