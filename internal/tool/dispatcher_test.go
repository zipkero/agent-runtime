package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// errorTool 은 Execute 가 항상 error를 반환하는 테스트용 Tool이다.
type errorTool struct {
	name string
}

func (e *errorTool) Spec() message.ToolSpec {
	return message.ToolSpec{Name: e.name, Description: "error tool"}
}

func (e *errorTool) Execute(_ context.Context, _ json.RawMessage) (message.ToolResult, error) {
	return message.ToolResult{}, errors.New("execute 실패")
}

// hangTool 은 ctx가 취소될 때까지 매달리는 테스트용 Tool이다.
type hangTool struct {
	name string
}

func (h *hangTool) Spec() message.ToolSpec {
	return message.ToolSpec{Name: h.name, Description: "hang tool"}
}

func (h *hangTool) Execute(ctx context.Context, _ json.RawMessage) (message.ToolResult, error) {
	// ctx가 취소될 때까지 블로킹
	<-ctx.Done()
	return message.ToolResult{}, ctx.Err()
}

// successTool 은 항상 성공 결과를 반환하는 테스트용 Tool이다.
type successTool struct {
	name    string
	content string
}

func (s *successTool) Spec() message.ToolSpec {
	return message.ToolSpec{Name: s.name, Description: "success tool"}
}

func (s *successTool) Execute(_ context.Context, _ json.RawMessage) (message.ToolResult, error) {
	return message.ToolResult{Content: s.content}, nil
}

// newDispatcher 는 테스트용 Registry·Dispatcher를 생성하는 헬퍼다.
func newDispatcher(timeout time.Duration, tools ...tool.Tool) *tool.Dispatcher {
	r := tool.NewRegistry()
	for _, t := range tools {
		if err := r.Register(t); err != nil {
			panic(err)
		}
	}
	return tool.NewDispatcher(r, timeout)
}

func TestDispatcher_UnknownTool(t *testing.T) {
	d := newDispatcher(time.Second)

	call := message.ToolCall{ID: "call-1", Name: "nonexistent", Input: json.RawMessage(`{}`)}
	result := d.Dispatch(context.Background(), call)

	if !result.IsError {
		t.Fatal("unknown tool 이름이 IsError=true 결과를 반환해야 하는데 false를 반환했습니다")
	}
	if result.ToolCallID != call.ID {
		t.Errorf("ToolCallID 불일치: got %q, want %q", result.ToolCallID, call.ID)
	}
}

func TestDispatcher_ErrorTool(t *testing.T) {
	d := newDispatcher(time.Second, &errorTool{name: "err-tool"})

	call := message.ToolCall{ID: "call-2", Name: "err-tool", Input: json.RawMessage(`{}`)}
	result := d.Dispatch(context.Background(), call)

	if !result.IsError {
		t.Fatal("error 반환 tool이 IsError=true 결과를 반환해야 하는데 false를 반환했습니다")
	}
	if result.ToolCallID != call.ID {
		t.Errorf("ToolCallID 불일치: got %q, want %q", result.ToolCallID, call.ID)
	}
}

func TestDispatcher_TimeoutTool(t *testing.T) {
	// 짧은 timeout을 주입해 hang tool이 deadline 초과로 흡수되도록 한다
	d := newDispatcher(10*time.Millisecond, &hangTool{name: "hang-tool"})

	call := message.ToolCall{ID: "call-3", Name: "hang-tool", Input: json.RawMessage(`{}`)}
	result := d.Dispatch(context.Background(), call)

	if !result.IsError {
		t.Fatal("timeout이 IsError=true 결과로 흡수되어야 하는데 false를 반환했습니다")
	}
	if result.ToolCallID != call.ID {
		t.Errorf("ToolCallID 불일치: got %q, want %q", result.ToolCallID, call.ID)
	}
}

func TestDispatcher_SuccessTool(t *testing.T) {
	d := newDispatcher(time.Second, &successTool{name: "ok-tool", content: "결과 문자열"})

	call := message.ToolCall{ID: "call-4", Name: "ok-tool", Input: json.RawMessage(`{}`)}
	result := d.Dispatch(context.Background(), call)

	if result.IsError {
		t.Fatalf("성공 tool이 IsError=false를 반환해야 하는데 true를 반환했습니다: %s", result.Content)
	}
	if result.ToolCallID != call.ID {
		t.Errorf("ToolCallID 불일치: got %q, want %q", result.ToolCallID, call.ID)
	}
	if result.Content != "결과 문자열" {
		t.Errorf("Content 불일치: got %q, want %q", result.Content, "결과 문자열")
	}
}
