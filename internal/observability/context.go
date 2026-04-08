package observability

import "context"

type ctxKey string

const (
	ctxTraceID   ctxKey = "trace_id"
	ctxSessionID ctxKey = "session_id"
	ctxRequestID ctxKey = "request_id"
)

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxTraceID, id)
}

func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxSessionID, id)
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxRequestID, id)
}

func TraceIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxTraceID).(string)
	return v
}

func SessionIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxSessionID).(string)
	return v
}

func RequestIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestID).(string)
	return v
}
