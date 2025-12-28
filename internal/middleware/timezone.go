package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// TimezoneMiddleware sets the timezone location in the Gin context
func TimezoneMiddleware(timezone string) gin.HandlerFunc {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		slog.Error("Failed to load timezone, defaulting to UTC", "timezone", timezone, "error", err)
		loc = time.UTC
	}

	return func(c *gin.Context) {
		c.Set("timezone", loc)
		c.Next()
	}
}
