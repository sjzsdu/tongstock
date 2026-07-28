package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestIsLoopback(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"127.0.0.1:0", true},
		{"localhost", true},
		{"::1", true},
		{"[::1]", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
	}
	for _, c := range cases {
		if got := IsLoopback(c.in); got != c.want {
			t.Errorf("IsLoopback(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeBindAddress(t *testing.T) {
	cases := map[string]string{
		"":              "127.0.0.1",
		"127.0.0.1":     "127.0.0.1",
		"127.0.0.1:123": "127.0.0.1",
		"::1":           "::1",
		"[::1]":         "::1",
		"[::1]:123":     "::1",
	}
	for input, want := range cases {
		if got := NormalizeBindAddress(input); got != want {
			t.Errorf("NormalizeBindAddress(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidateBindSecurity(t *testing.T) {
	if _, _, err := ValidateBindSecurity("0.0.0.0", ""); err == nil {
		t.Fatal("non-loopback bind without token was accepted")
	}
	bind, loopback, err := ValidateBindSecurity("0.0.0.0", "secret")
	if err != nil {
		t.Fatalf("non-loopback bind with token error = %v", err)
	}
	if bind != "0.0.0.0" || loopback {
		t.Fatalf("bind=%q loopback=%v, want non-loopback 0.0.0.0", bind, loopback)
	}
	if _, loopback, err := ValidateBindSecurity("[::1]", ""); err != nil || !loopback {
		t.Fatalf("IPv6 loopback rejected: loopback=%v err=%v", loopback, err)
	}
}

func TestAccessTokenAuth_LoopbackIsNoop(t *testing.T) {
	r := gin.New()
	r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	// Loopback bind with no token: middleware is bypassed.
	r.Use(AccessTokenAuth("127.0.0.1", ""))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAccessTokenAuth_NonLoopbackWithoutTokenRejected(t *testing.T) {
	r := gin.New()
	r.Use(AccessTokenAuth("0.0.0.0", "secret"))
	r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body=%s", w.Code, w.Body.String())
	}
}

func TestAccessTokenAuth_WrongTokenRejected(t *testing.T) {
	r := gin.New()
	r.Use(AccessTokenAuth("0.0.0.0", "secret"))
	r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAccessTokenAuth_CorrectTokenAccepted(t *testing.T) {
	r := gin.New()
	r.Use(AccessTokenAuth("0.0.0.0", "secret"))
	r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer secret")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
}

func TestMaxRequestBodyRejectsKnownOversizeBody(t *testing.T) {
	r := gin.New()
	r.Use(MaxRequestBody())
	r.POST("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, maxRequestBodyBytes+1)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestAccessTokenCanProtectAPIWithoutBlockingSPA(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "spa") })
	api := r.Group("/api", AccessTokenAuth("0.0.0.0", "secret"))
	api.GET("/quote", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	spaReq := httptest.NewRequest(http.MethodGet, "/", nil)
	spaRec := httptest.NewRecorder()
	r.ServeHTTP(spaRec, spaReq)
	if spaRec.Code != http.StatusOK {
		t.Fatalf("SPA status = %d, want %d", spaRec.Code, http.StatusOK)
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
	apiRec := httptest.NewRecorder()
	r.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusUnauthorized {
		t.Fatalf("API status = %d, want %d", apiRec.Code, http.StatusUnauthorized)
	}
}
