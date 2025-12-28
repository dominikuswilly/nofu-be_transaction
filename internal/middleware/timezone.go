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
		slog.Error("Failed to load timezone, trying fallback", "timezone", timezone, "error", err)
		// Fallback for Asia/Jakarta (UTC+7)
		if timezone == "Asia/Jakarta" {
			loc = time.FixedZone("Asia/Jakarta", 7*60*60)
			slog.Info("Using fallback Asia/Jakarta (UTC+7)")
		} else {
			loc = time.UTC
			slog.Info("Using fallback UTC")
		}
	}

	return func(c *gin.Context) {
		slog.Info("Setting timezone in context", "location", loc.String())
		c.Set("timezone", loc)
		c.Next()
	}
}
