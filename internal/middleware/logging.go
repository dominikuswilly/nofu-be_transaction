package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// RequestLogger logs request and response details
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Generate or get request ID
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("request_id", requestID)

		// Read and restore request body
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Log request
		slog.Info("Incoming request",
			"request_id", requestID,
			"method", c.Request.Method,
			"uri", c.Request.RequestURI,
			"remote_addr", c.ClientIP(),
			"req_headers", c.Request.Header,
			"req_body", string(requestBody),
		)

		// Wrap response writer to capture response body
		blw := &responseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = blw

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Log response
		slog.Info("Outgoing response",
			"request_id", requestID,
			"status", c.Writer.Status(),
			"duration_ms", duration.Milliseconds(),
			"resp_headers", c.Writer.Header(),
			"resp_body", blw.body.String(),
		)
	}
}
