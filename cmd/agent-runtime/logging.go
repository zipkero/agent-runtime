package main

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/config"
)

// newLogger 함수는 설정된 레벨로 stderr에 쓰는 진단 logger를 만든다.
// stdout은 실행 결과 전용이므로 로그와 섞지 않는다.
func newLogger(stderr io.Writer, level string) (*slog.Logger, error) {
	parsed, err := parseLogLevel(level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: parsed})), nil
}

// parseLogLevel 함수는 LOG_LEVEL 값을 slog 레벨로 해석한다.
// 빈 값은 config 기본 레벨로 취급하고, 알 수 없는 값은 조용히 무시하지 않고 오류로 알린다.
func parseLogLevel(level string) (slog.Level, error) {
	value := strings.ToLower(strings.TrimSpace(level))
	if value == "" {
		value = config.DefaultLogLevel
	}

	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL: unsupported level %q", level)
	}
}

// logTrace 함수는 run이 남긴 trace를 debug 레벨로 순서대로 기록한다.
// trace는 메모리 구조라서 이 출력이 없으면 실행 흐름을 확인할 방법이 없다.
func logTrace(logger *slog.Logger, events []agent.TraceEvent) {
	for _, event := range events {
		attrs := []any{
			"step", event.Step,
			"action", string(event.Action),
			"status", string(event.Status),
		}
		if event.ToolName != "" {
			attrs = append(attrs, "tool", event.ToolName)
		}
		if event.ToolCallID != "" {
			attrs = append(attrs, "tool_call_id", event.ToolCallID)
		}
		if event.IsError {
			attrs = append(attrs, "is_error", true)
		}
		if event.Error != nil {
			attrs = append(attrs, "error", event.Error.Error())
		}
		logger.Debug("trace", attrs...)
	}
}
