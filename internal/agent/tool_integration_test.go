package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	runtimetool "github.com/zipkero/agent-runtime/internal/tool"
)

// TestRunExecutesRegisteredToolBundle 은 등록된 Tool 묶음이 기존 registry와 Agent loop를 통해 함께 동작하는지 확인한다.
func TestRunExecutesRegisteredToolBundle(t *testing.T) {
	root := t.TempDir()
	saveFile, err := runtimetool.NewFileSave(root)
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}
	codeExecution, err := runtimetool.NewCodeExecution(root)
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}

	registry := runtimetool.NewRegistry()
	for _, registeredTool := range []runtimetool.Tool{
		runtimetool.NewWebSearch(""),
		saveFile,
		codeExecution,
	} {
		if err := registry.Register(registeredTool); err != nil {
			t.Fatalf("Register(%s) error = %v", registeredTool.Name(), err)
		}
	}

	client := &stubClient{
		responses: []llm.ChatResponse{
			{Message: message.Assistant("using tools",
				message.ToolCall{ID: "call-search", Name: "web_search", Arguments: json.RawMessage(`{"query":"go agent runtime"}`)},
				message.ToolCall{ID: "call-save", Name: "save_file", Arguments: json.RawMessage(`{"path":"output.txt","content":"saved by agent"}`)},
				message.ToolCall{ID: "call-code", Name: "code_execution", Arguments: json.RawMessage(`{"args":["version"]}`)},
			)},
			{Message: message.Assistant("final after tools")},
		},
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
		Tools:    registry,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "use registered tools")

	if state.Status != StatusFinal || state.FinalAnswer != "final after tools" || state.LastError != nil {
		t.Fatalf("state = %+v, want final result without Agent error", state)
	}
	if len(client.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(client.requests))
	}
	wantToolNames := []string{"web_search", "save_file", "code_execution"}
	for requestIndex, request := range client.requests {
		if len(request.Tools) != len(wantToolNames) {
			t.Fatalf("request[%d] len(Tools) = %d, want %d", requestIndex, len(request.Tools), len(wantToolNames))
		}
		for toolIndex, wantName := range wantToolNames {
			if request.Tools[toolIndex].Name != wantName || len(request.Tools[toolIndex].InputSchema) == 0 {
				t.Fatalf("request[%d] Tools[%d] = %+v, want %s schema", requestIndex, toolIndex, request.Tools[toolIndex], wantName)
			}
		}
	}

	secondRequest := client.requests[1]
	if len(secondRequest.Messages) != 5 {
		t.Fatalf("second request len(Messages) = %d, want user, assistant, and three tool results", len(secondRequest.Messages))
	}
	assertToolResult(t, secondRequest.Messages[2], "call-search", "web_search", true)
	assertToolResult(t, secondRequest.Messages[3], "call-save", "save_file", false)
	assertToolResult(t, secondRequest.Messages[4], "call-code", "code_execution", false)

	if secondRequest.Messages[2].ToolResult.Content != "Tavily API key is required" {
		t.Fatalf("web search error content = %q, want missing API key error", secondRequest.Messages[2].ToolResult.Content)
	}
	saved, err := os.ReadFile(root + string(os.PathSeparator) + "output.txt")
	if err != nil {
		t.Fatalf("ReadFile(output.txt) error = %v", err)
	}
	if string(saved) != "saved by agent" {
		t.Fatalf("saved content = %q, want saved by agent", saved)
	}
	if !strings.Contains(secondRequest.Messages[4].ToolResult.Content, `"exit_code":0`) {
		t.Fatalf("code execution content = %q, want exit_code 0", secondRequest.Messages[4].ToolResult.Content)
	}
}

func assertToolResult(t *testing.T, got message.Message, wantCallID, wantName string, wantError bool) {
	t.Helper()

	if got.Role != message.RoleTool || got.ToolResult == nil {
		t.Fatalf("message = %+v, want tool result", got)
	}
	if got.ToolResult.ToolCallID != wantCallID || got.ToolResult.Name != wantName || got.ToolResult.IsError != wantError {
		t.Fatalf("ToolResult = %+v, want call=%s name=%s error=%v", got.ToolResult, wantCallID, wantName, wantError)
	}
	if got.ToolResult.Content == "" {
		t.Fatalf("ToolResult content is empty for %s", wantName)
	}
}
