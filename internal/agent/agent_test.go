package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

type stubClient struct {
	response llm.ChatResponse
	request  llm.ChatRequest
	calls    int
}

func (c *stubClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
	c.request = llm.ChatRequest{
		Model:    req.Model,
		Messages: append([]message.Message(nil), req.Messages...),
	}
	return c.response, nil
}

// TestRunStoresUserMessageAndFinalAssistantResponse 는 Task 001의 final 정상 경로 contract를 고정한다.
func TestRunStoresUserMessageAndFinalAssistantResponse(t *testing.T) {
	client := &stubClient{
		response: llm.ChatResponse{
			Message: message.Assistant("answer text"),
		},
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "hello runtime")

	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	if client.request.Model != "test-model" {
		t.Fatalf("request Model = %q, want test-model", client.request.Model)
	}
	if len(client.request.Messages) != 1 {
		t.Fatalf("len(request Messages) = %d, want 1", len(client.request.Messages))
	}
	if client.request.Messages[0].Role != message.RoleUser || client.request.Messages[0].Text != "hello runtime" {
		t.Fatalf("request user message = %+v, want role=user text=hello runtime", client.request.Messages[0])
	}

	if state.Status != StatusFinal {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusFinal)
	}
	if state.FinalAnswer != "answer text" {
		t.Fatalf("FinalAnswer = %q, want answer text", state.FinalAnswer)
	}
	if state.Step != 1 {
		t.Fatalf("Step = %d, want 1", state.Step)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("len(state Messages) = %d, want 2", len(state.Messages))
	}
	if state.Messages[0].Role != message.RoleUser || state.Messages[0].Text != "hello runtime" {
		t.Fatalf("state user message = %+v, want role=user text=hello runtime", state.Messages[0])
	}
	if state.Messages[1].Role != message.RoleAssistant || state.Messages[1].Text != "answer text" {
		t.Fatalf("state assistant message = %+v, want role=assistant text=answer text", state.Messages[1])
	}
}

// TestRunStopsWithNeedsActionWhenAssistantRequestsTool 는 tool 실행 없는 대기 상태 contract를 고정한다.
func TestRunStopsWithNeedsActionWhenAssistantRequestsTool(t *testing.T) {
	toolCall := message.ToolCall{
		ID:        "call-1",
		Name:      "read_file",
		Arguments: json.RawMessage(`{"path":"README.md"}`),
	}
	client := &stubClient{
		response: llm.ChatResponse{
			Message: message.Assistant("need tool", toolCall),
		},
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "read README")

	if state.Status != StatusNeedsAction {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusNeedsAction)
	}
	if state.FinalAnswer != "" {
		t.Fatalf("FinalAnswer = %q, want empty", state.FinalAnswer)
	}
	if len(state.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(state.ToolCalls))
	}
	call := state.ToolCalls[0]
	if call.ID != "call-1" || call.Name != "read_file" || string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("ToolCalls[0] = %+v, want call-1/read_file args", call)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("len(state Messages) = %d, want 2", len(state.Messages))
	}
	assistant := state.Messages[1]
	if assistant.Role != message.RoleAssistant || assistant.Text != "need tool" {
		t.Fatalf("assistant message = %+v, want role=assistant text=need tool", assistant)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("len(assistant ToolCalls) = %d, want 1", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].ID != call.ID ||
		assistant.ToolCalls[0].Name != call.Name ||
		string(assistant.ToolCalls[0].Arguments) != string(call.Arguments) {
		t.Fatalf("assistant ToolCalls = %+v, want same state call %+v", assistant.ToolCalls[0], call)
	}
	if assistant.ToolResult != nil {
		t.Fatalf("assistant ToolResult = %+v, want nil", assistant.ToolResult)
	}
}
