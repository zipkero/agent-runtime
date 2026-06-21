package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

func TestRunner_PreModelMiddlewareChangesRequest(t *testing.T) {
	const toolName = "lookup"
	ft := &fakeTool{name: toolName, result: "ok"}
	reg := tool.NewRegistry()
	if err := reg.Register(ft); err != nil {
		t.Fatalf("tool 등록 실패: %v", err)
	}

	capturer := &capturingStub{
		seqStub: seqStub{
			responses: []llm.ChatResponse{textResponse("pre 완료")},
		},
	}
	middleware := middlewareStub{
		pre: func(_ context.Context, in agent.PreModelInput) (llm.ChatRequest, error) {
			if in.Step != 0 {
				t.Errorf("PreModel Step 기대값 0, 실제값 %d", in.Step)
			}
			if in.State.Status != agent.StatusRunning {
				t.Errorf("PreModel State.Status 기대값 running, 실제값 %q", in.State.Status)
			}
			if len(in.Request.Tools) != 1 || in.Request.Tools[0].Name != toolName {
				t.Fatalf("PreModel Request.Tools에 %q schema가 없다: %#v", toolName, in.Request.Tools)
			}

			req := in.Request
			req.Model = "middleware-model"
			req.Messages = append(req.Messages, message.Message{
				Role:    message.RoleSystem,
				Content: []message.ContentBlock{message.NewTextBlock("pre middleware system")},
			})
			return req, nil
		},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:     capturer,
		Model:      "original-model",
		MaxSteps:   5,
		Registry:   reg,
		Middleware: []agent.Middleware{middleware},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "pre middleware 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if len(capturer.capturedRequests) != 1 {
		t.Fatalf("ChatRequest 캡처 수 기대값 1, 실제값 %d", len(capturer.capturedRequests))
	}
	gotReq := capturer.capturedRequests[0]
	if gotReq.Model != "middleware-model" {
		t.Errorf("ChatRequest.Model 기대값 %q, 실제값 %q", "middleware-model", gotReq.Model)
	}
	if len(gotReq.Messages) != 2 {
		t.Fatalf("ChatRequest.Messages 수 기대값 2, 실제값 %d", len(gotReq.Messages))
	}
	lastMsg := gotReq.Messages[1]
	if lastMsg.Role != message.RoleSystem || len(lastMsg.Content) == 0 || lastMsg.Content[0].Text != "pre middleware system" {
		t.Errorf("PreModel이 추가한 system message가 전달되지 않았다: %#v", lastMsg)
	}
}

func TestRunner_PostModelMiddlewareChangesResponse(t *testing.T) {
	middleware := middlewareStub{
		post: func(_ context.Context, in agent.PostModelInput) (llm.ChatResponse, error) {
			if in.Step != 0 {
				t.Errorf("PostModel Step 기대값 0, 실제값 %d", in.Step)
			}
			if in.Err != nil {
				t.Errorf("성공 응답 경로의 PostModel Err는 nil이어야 하나 Err=%v", in.Err)
			}
			return textResponse("post middleware 최종 답"), nil
		},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:     &seqStub{responses: []llm.ChatResponse{textResponse("원본 답")}},
		Model:      "stub-model",
		MaxSteps:   5,
		Registry:   tool.NewRegistry(),
		Middleware: []agent.Middleware{middleware},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "post middleware 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if result.FinalText != "post middleware 최종 답" {
		t.Errorf("FinalText 기대값 %q, 실제값 %q", "post middleware 최종 답", result.FinalText)
	}
	if len(result.State.Messages) != 2 {
		t.Fatalf("누적 메시지 수 기대값 2, 실제값 %d", len(result.State.Messages))
	}
	finalMsg, ok := result.State.FinalMessage()
	if !ok {
		t.Fatal("FinalMessage가 ok=false를 반환했다")
	}
	if got := testMessageText(finalMsg); got != "post middleware 최종 답" {
		t.Errorf("AgentState 최종 메시지 text 기대값 %q, 실제값 %q", "post middleware 최종 답", got)
	}
}

// TestRunner_MiddlewareOrderAndChangePropagation 는 여러 middleware의 pre와 post hook이 등록 순서대로
// 실행되고, 앞 hook의 변경값이 뒤 hook과 최종 Runner 결과까지 전달되는지 확인한다.
func TestRunner_MiddlewareOrderAndChangePropagation(t *testing.T) {
	var events []string
	capturer := &capturingStub{
		seqStub: seqStub{
			responses: []llm.ChatResponse{textResponse("원본 응답")},
		},
	}
	first := middlewareStub{
		pre: func(_ context.Context, in agent.PreModelInput) (llm.ChatRequest, error) {
			events = append(events, "pre-1")
			if in.Request.Model != "base-model" {
				t.Errorf("pre-1 Request.Model 기대값 %q, 실제값 %q", "base-model", in.Request.Model)
			}
			req := in.Request
			req.Model = "pre-1-model"
			return req, nil
		},
		post: func(_ context.Context, in agent.PostModelInput) (llm.ChatResponse, error) {
			events = append(events, "post-1")
			if got := testMessageText(in.Response.Message); got != "원본 응답" {
				t.Errorf("post-1 응답 text 기대값 %q, 실제값 %q", "원본 응답", got)
			}
			return textResponse("post-1 응답"), nil
		},
	}
	second := middlewareStub{
		pre: func(_ context.Context, in agent.PreModelInput) (llm.ChatRequest, error) {
			events = append(events, "pre-2")
			if in.Request.Model != "pre-1-model" {
				t.Errorf("pre-2 Request.Model 기대값 %q, 실제값 %q", "pre-1-model", in.Request.Model)
			}
			req := in.Request
			req.Model = "pre-2-model"
			return req, nil
		},
		post: func(_ context.Context, in agent.PostModelInput) (llm.ChatResponse, error) {
			events = append(events, "post-2")
			if got := testMessageText(in.Response.Message); got != "post-1 응답" {
				t.Errorf("post-2 응답 text 기대값 %q, 실제값 %q", "post-1 응답", got)
			}
			return textResponse("post-2 응답"), nil
		},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:     capturer,
		Model:      "base-model",
		MaxSteps:   5,
		Registry:   tool.NewRegistry(),
		Middleware: []agent.Middleware{first, second},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "middleware 순서 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	expectedEvents := []string{"pre-1", "pre-2", "post-1", "post-2"}
	if strings.Join(events, ",") != strings.Join(expectedEvents, ",") {
		t.Fatalf("middleware 실행 순서 기대값 %v, 실제값 %v", expectedEvents, events)
	}
	if len(capturer.capturedRequests) != 1 {
		t.Fatalf("ChatRequest 캡처 수 기대값 1, 실제값 %d", len(capturer.capturedRequests))
	}
	if capturer.capturedRequests[0].Model != "pre-2-model" {
		t.Errorf("최종 ChatRequest.Model 기대값 %q, 실제값 %q", "pre-2-model", capturer.capturedRequests[0].Model)
	}
	if result.FinalText != "post-2 응답" {
		t.Errorf("FinalText 기대값 %q, 실제값 %q", "post-2 응답", result.FinalText)
	}
}
