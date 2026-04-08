package observability

import (
	"context"
	"log/slog"
	"os"
)

const (
	KeyTraceID   = "trace_id"
	KeySessionID = "session_id"
	KeyRequestID = "request_id"
)

// New returns a *slog.Logger writing JSON to stdout.
func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

// FromContext returns a *slog.Logger with trace_id, session_id, request_id fields
// populated from ctx. Fields with empty values are omitted.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	var attrs []slog.Attr
	if v := TraceIDFromCtx(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyTraceID, v))
	}
	if v := SessionIDFromCtx(ctx); v != "" {
		attrs = append(attrs, slog.String(KeySessionID, v))
	}
	if v := RequestIDFromCtx(ctx); v != "" {
		attrs = append(attrs, slog.String(KeyRequestID, v))
	}
	if len(attrs) == 0 {
		return base
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return base.With(args...)
}
