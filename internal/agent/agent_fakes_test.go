package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// capturingStub 은 seqStub과 동일하게 순서대로 응답하되,
// 각 Chat 호출 시 전달된 ChatRequest를 순서대로 캡처한다.
type capturingStub struct {
	seqStub
	capturedRequests []llm.ChatRequest
}

func (s *capturingStub) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	s.capturedRequests = append(s.capturedRequests, req)
	return s.seqStub.Chat(ctx, req)
}

type middlewareStub struct {
	pre  func(context.Context, agent.PreModelInput) (llm.ChatRequest, error)
	post func(context.Context, agent.PostModelInput) (llm.ChatResponse, error)
}

func (m middlewareStub) PreModel(ctx context.Context, in agent.PreModelInput) (llm.ChatRequest, error) {
	if m.pre == nil {
		return in.Request, nil
	}
	return m.pre(ctx, in)
}

func (m middlewareStub) PostModel(ctx context.Context, in agent.PostModelInput) (llm.ChatResponse, error) {
	if m.post == nil {
		return in.Response, nil
	}
	return m.post(ctx, in)
}

// fakeTool 은 테스트용 Tool 구현체다. Execute가 호출되면 calls를 증가시키고 지정한 결과를 반환한다.
type fakeTool struct {
	name   string
	result string
	calls  int
}

func (f *fakeTool) Spec() message.ToolSpec {
	return message.ToolSpec{
		Name:        f.name,
		Description: f.name + " 테스트용 tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (message.ToolResult, error) {
	f.calls++
	return message.ToolResult{Content: f.result}, nil
}

// seqStub 은 step마다 다른 응답을 순서대로 반환하는 다단계 LLMClient 구현체다.
// 실제 client처럼 ctx 취소를 먼저 존중하고 호출 횟수를 기록한다.
type seqStub struct {
	responses []llm.ChatResponse
	err       error // err != nil이면 모든 호출에서 이 에러를 반환한다
	calls     int   // 실제로 응답을 반환한 호출 횟수
}

// Chat 은 ctx 취소를 먼저 확인하고, 순서대로 응답을 반환한다.
// 시퀀스를 소진하면 마지막 응답을 반복 반환해 step을 계속 진행하게 한다.
func (s *seqStub) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	// 실제 client처럼 취소된 ctx를 먼저 존중한다
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	if s.err != nil {
		s.calls++
		return llm.ChatResponse{}, s.err
	}
	idx := s.calls
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	s.calls++
	return s.responses[idx], nil
}

// 컴파일 타임에 llm.LLMClient 인터페이스를 구현하는지 확인한다.
var _ llm.LLMClient = (*seqStub)(nil)

// toolCallResponse 는 tool_call ContentBlock을 포함한 assistant 응답을 생성하는 헬퍼다.
func toolCallResponse() llm.ChatResponse {
	return llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_001",
					Name:  "lookup",
					Input: json.RawMessage(`{"query":"test"}`),
				}),
			},
		},
	}
}

// textResponse 는 text ContentBlock만 포함한 assistant 응답을 생성하는 헬퍼다.
func textResponse(text string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewTextBlock(text),
			},
		},
	}
}

func phase51ToolBundleResponse() llm.ChatResponse {
	return llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_web",
					Name:  "web_search",
					Input: json.RawMessage(`{"query":"agent runtime"}`),
				}),
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_save",
					Name:  "file_save",
					Input: json.RawMessage(`{"path":"agent-output.txt","content":"saved by agent regression"}`),
				}),
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_code",
					Name:  "code_execute",
					Input: json.RawMessage(`{"profile":"helper","args":["stdout"]}`),
				}),
			},
		},
	}
}

func newPhase51TestRegistry(t *testing.T, workDir string) *tool.Registry {
	t.Helper()

	reg := tool.NewRegistry()
	if err := reg.Register(tool.NewWebSearch("", nil)); err != nil {
		t.Fatalf("web_search 등록 실패: %v", err)
	}
	fs, err := tool.NewFileSave(workDir)
	if err != nil {
		t.Fatalf("file_save 생성 실패: %v", err)
	}
	if err := reg.Register(fs); err != nil {
		t.Fatalf("file_save 등록 실패: %v", err)
	}
	ce, err := tool.NewCodeExecution(workDir, []tool.CommandProfile{
		{
			Name:        "helper",
			Command:     os.Args[0],
			Args:        []string{"-test.run=TestRun_Phase51ToolBundle_HelperProcess", "--"},
			Env:         []string{"GO_WANT_AGENT_PHASE51_HELPER=1"},
			AllowedArgs: []string{"stdout"},
		},
	}, 1024)
	if err != nil {
		t.Fatalf("code_execute 생성 실패: %v", err)
	}
	if err := reg.Register(ce); err != nil {
		t.Fatalf("code_execute 등록 실패: %v", err)
	}
	return reg
}

// TestRun_Phase51ToolBundle_HelperProcess 는 code_execute가 테스트 바이너리를 하위 프로세스로 다시
// 실행할 때 사용하는 helper 진입점이다.
func TestRun_Phase51ToolBundle_HelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_AGENT_PHASE51_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 || args[1] != "stdout" {
		fmt.Fprint(os.Stderr, "unexpected helper args")
		os.Exit(2)
	}
	fmt.Print("agent code ok")
	os.Exit(0)
}

func assertToolSpecs(t *testing.T, specs []message.ToolSpec, expected ...string) {
	t.Helper()
	names := make(map[string]bool)
	for _, spec := range specs {
		names[spec.Name] = true
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("ChatRequest.Tools에 %q schema가 없다", name)
		}
	}
}

func testMessageText(msg message.Message) string {
	var b strings.Builder
	for _, block := range msg.Content {
		if block.Type == message.BlockTypeText {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}
