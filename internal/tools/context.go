package tools

import "context"

type contextKey string

const (
	ctxRequestID contextKey = "request_id"
	ctxSessionID contextKey = "session_id"
)

// WithRequestID 는 context에 request_id를 저장한다.
// agent runtime이 Route() 호출 전에 context를 준비할 때 사용한다.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxRequestID, id)
}

// WithSessionID 는 context에 session_id를 저장한다.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxSessionID, id)
}

func requestIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestID).(string)
	return v
}

func sessionIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxSessionID).(string)
	return v
}
