package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	assert.Equal(t, slog.LevelDebug, ParseLevel("DEBUG"))
	assert.Equal(t, slog.LevelInfo, ParseLevel("info"))
	assert.Equal(t, slog.LevelWarn, ParseLevel("warning"))
	assert.Equal(t, slog.LevelError, ParseLevel("error"))
	assert.Equal(t, slog.LevelInfo, ParseLevel("unknown"))
}

func TestLoggerContextAndJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf, "debug").With("request_id", "req-123")

	ctx := IntoContext(context.Background(), l)
	extracted := FromContext(ctx)
	extracted.Debug("hello world", "user_id", "u-1")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	assert.Equal(t, "DEBUG", payload["level"])
	assert.Equal(t, "hello world", payload["msg"])
	assert.Equal(t, "req-123", payload["request_id"])
	assert.Equal(t, "u-1", payload["user_id"])

	assert.Equal(t, slog.Default(), FromContext(context.Background()))
}
