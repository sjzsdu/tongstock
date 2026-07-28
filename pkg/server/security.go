package server

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// maxRequestBodyBytes caps the size of request bodies to prevent abuse.
	// 4 MiB covers typical POST payloads for watchlist/trade/agent calls.
	maxRequestBodyBytes int64 = 4 << 20

	// headerAuthorization is the canonical Authorization header name.
	headerAuthorization = "Authorization"

	// headerBearerPrefix is the expected prefix of the Authorization header.
	headerBearerPrefix = "Bearer "
)

// NormalizeBindAddress converts a configured bind address into the host form
// expected by net.JoinHostPort. It accepts bracketed IPv6 and legacy host:port
// values, although the configured server.port remains authoritative.
func NormalizeBindAddress(addr string) string {
	host := strings.TrimSpace(addr)
	if host == "" {
		return "127.0.0.1"
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return strings.Trim(host, "[]")
}

// IsLoopback reports whether the configured bind address is a loopback host.
func IsLoopback(addr string) bool {
	host := NormalizeBindAddress(addr)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateBindSecurity normalizes a configured bind address and enforces the
// rule that any non-loopback listener requires an access token.
func ValidateBindSecurity(addr, accessToken string) (bind string, loopback bool, err error) {
	bind = NormalizeBindAddress(addr)
	loopback = IsLoopback(bind)
	if !loopback && strings.TrimSpace(accessToken) == "" {
		return bind, false, fmt.Errorf(
			"server.bind_address is %q (non-loopback) but server.access_token is empty: refusing to start without an access token",
			bind,
		)
	}
	return bind, loopback, nil
}

// SecurityHeaders adds a minimal set of security-related response headers.
// It is intentionally conservative and does not break the SPA or SSE.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		c.Next()
	}
}

// MaxRequestBody limits the request body size. gin's built-in
// MaxMultipartMemory already caps multipart bodies; this caps generic ones.
func MaxRequestBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxRequestBodyBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "request body exceeds 4 MiB limit",
			})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
		c.Next()
	}
}

// AccessTokenAuth returns a middleware that requires a valid Bearer token
// when the server is bound to a non-loopback address. When bound to
// loopback, or when no token is configured, the middleware is a no-op so
// that local development remains frictionless.
//
// When access is required the middleware:
//   - Returns 401 if the Authorization header is missing or malformed.
//   - Returns 401 if the token does not match (constant-time compare).
func AccessTokenAuth(bindAddress, accessToken string) gin.HandlerFunc {
	active := accessToken != "" && !IsLoopback(bindAddress)
	if !active {
		return func(c *gin.Context) { c.Next() }
	}
	expected := []byte(accessToken)
	return func(c *gin.Context) {
		h := c.GetHeader(headerAuthorization)
		if h == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required when accessing from non-loopback",
			})
			return
		}
		if !strings.HasPrefix(h, headerBearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header must start with 'Bearer '",
			})
			return
		}
		got := strings.TrimSpace(strings.TrimPrefix(h, headerBearerPrefix))
		if got == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "empty bearer token",
			})
			return
		}
		// Constant-time comparison to mitigate timing attacks.
		if subtle.ConstantTimeCompare([]byte(got), expected) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid access token",
			})
			return
		}
		c.Next()
	}
}
