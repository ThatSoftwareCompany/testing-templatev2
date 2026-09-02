package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func New(level, environment string) *slog.Logger {
	return newLogger(level, environment, os.Stdout)
}

func newLogger(level, environment string, writer io.Writer) *slog.Logger {
	configuredLevel := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		configuredLevel = slog.LevelDebug
	case "warn":
		configuredLevel = slog.LevelWarn
	case "error":
		configuredLevel = slog.LevelError
	}

	return slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: configuredLevel,
	})).With("environment", environment)
}
