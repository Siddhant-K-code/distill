package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "info", Format: FormatJSON, Output: &buf})
	logger.Info("test message", "key", "value")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if record["msg"] != "test message" {
		t.Errorf("expected msg=test message, got %v", record["msg"])
	}
	if record["key"] != "value" {
		t.Errorf("expected key=value, got %v", record["key"])
	}
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "info", Format: FormatText, Output: &buf})
	logger.Info("hello world")

	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("expected 'hello world' in output, got: %s", buf.String())
	}
}

func TestNew_DebugFiltered(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "info", Format: FormatJSON, Output: &buf})
	logger.Debug("should be filtered")

	if buf.Len() > 0 {
		t.Errorf("debug message should be filtered at info level, got: %s", buf.String())
	}
}

func TestNew_DebugVisible(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "debug", Format: FormatJSON, Output: &buf})
	logger.Debug("debug visible")

	if buf.Len() == 0 {
		t.Error("debug message should be visible at debug level")
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}
	for _, c := range cases {
		got := parseLevel(c.input)
		if got != c.want {
			t.Errorf("parseLevel(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestNewDefault(t *testing.T) {
	logger := NewDefault()
	if logger == nil {
		t.Error("NewDefault returned nil")
	}
}

func TestNewDebug(t *testing.T) {
	logger := NewDebug()
	if logger == nil {
		t.Error("NewDebug returned nil")
	}
}

func TestWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "info", Format: FormatJSON, Output: &buf})
	l := WithRequestID(logger, "req-123")
	l.Info("with request id")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if record["request_id"] != "req-123" {
		t.Errorf("expected request_id=req-123, got %v", record["request_id"])
	}
}

func TestWithTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "info", Format: FormatJSON, Output: &buf})
	l := WithTraceID(logger, "trace-abc")
	l.Info("with trace id")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if record["trace_id"] != "trace-abc" {
		t.Errorf("expected trace_id=trace-abc, got %v", record["trace_id"])
	}
}

func TestWithComponent(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "info", Format: FormatJSON, Output: &buf})
	l := WithComponent(logger, "dedup")
	l.Info("component log")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if record["component"] != "dedup" {
		t.Errorf("expected component=dedup, got %v", record["component"])
	}
}

func TestNew_DefaultOutput(t *testing.T) {
	// Should not panic when Output is nil (defaults to stderr).
	logger := New(Config{Level: "info"})
	if logger == nil {
		t.Error("expected non-nil logger")
	}
}
