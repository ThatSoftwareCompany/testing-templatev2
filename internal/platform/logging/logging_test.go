package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestLoggerEmitsJSONWithEnvironment(t *testing.T) {
	var output bytes.Buffer
	logger := newLogger("debug", "test", &output)
	logger.Debug("debug message", "correlation_id", "correlation-1")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("log output is not JSON: %v; output=%q", err, output.String())
	}
	if record["msg"] != "debug message" || record["environment"] != "test" || record["level"] != "DEBUG" {
		t.Fatalf("unexpected structured log: %#v", record)
	}
}

func TestLoggerHonorsConfiguredLevel(t *testing.T) {
	var output bytes.Buffer
	logger := newLogger("warn", "test", &output)
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info should be disabled at warn level")
	}
	if !logger.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("error should be enabled at warn level")
	}
}
