package logger

import (
	"log/slog"
	"os"
)

// InitLogger initializes the global logger with JSON handler.
func InitLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
