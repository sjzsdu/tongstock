package server

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// recover 必须把堆栈写进日志，否则线上 panic 只剩一行事件。
func TestRecoveryLogsPanicStack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery())
	router.GET("/boom", func(c *gin.Context) { panic("测试 panic") })

	var buf bytes.Buffer
	prevWriter, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("panic was not logged")
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("log line is not JSON: %v: %q", err, line)
	}
	stack, _ := record["stack"].(string)
	if !strings.Contains(stack, "goroutine") || !strings.Contains(stack, "TestRecoveryLogsPanicStack") {
		t.Fatalf("stack = %q, want runtime stack trace", stack)
	}
	if record["panic"] != "测试 panic" {
		t.Fatalf("panic = %v, want recovered value", record["panic"])
	}
}
