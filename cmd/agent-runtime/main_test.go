package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

// captureStdout 은 f 실행 동안 os.Stdout으로 쓴 내용을 캡처한다.
// 전역 os.Stdout을 잠시 바꿔치기하므로 이 helper를 쓰는 테스트는 t.Parallel()과 함께 쓰면 안 된다.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// captureStderr 은 f 실행 동안 os.Stderr으로 쓴 내용을 캡처한다.
// captureStdout과 마찬가지로 전역을 바꿔치기하므로 병렬 테스트에서 쓰면 안 된다.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	f()
	w.Close()
	os.Stderr = orig
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// TestRun_TextResponse_Final 은 text 응답이 오면 final 상태로 stdout에 출력되고 종료코드 0을 반환하는지 검증한다.
func TestRun_TextResponse_Final(t *testing.T) {
	stub := llm.NewStubClient(llm.ChatResponse{
		Message: message.Message{
			Role:    message.RoleAssistant,
			Content: []message.ContentBlock{message.NewTextBlock("안녕하세요")},
		},
	})

	var code int
	out := captureStdout(t, func() {
		code = run(context.Background(), stub, "claude-3-5-haiku-20241022", "안녕", 5)
	})

	if code != 0 {
		t.Fatalf("종료코드 0 기대, 실제: %d", code)
	}
	if out == "" {
		t.Fatal("stdout 출력이 없다")
	}
	if !contains(out, "안녕하세요") {
		t.Errorf("응답 텍스트가 stdout에 없다: %q", out)
	}
}

// TestRun_MaxSteps_WritesToStderrAndExitsNonZero 는 tool_call만 반복 반환하는 stub으로
// max step 초과 시 stderr에 원인이 출력되고 비정상 종료코드로 끝나는지 검증한다.
// 기존 TestRun_ToolCall_WritesDistinctToStdout을 loop 의미에 맞게 재정의한 케이스다.
func TestRun_MaxSteps_WritesToStderrAndExitsNonZero(t *testing.T) {
	// tool_call 응답을 계속 반환해 max step 초과를 유도한다.
	toolCallResp := llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "tc-1",
					Name:  "search",
					Input: json.RawMessage(`{"query":"test"}`),
				}),
			},
		},
	}
	stub := llm.NewStubClient(toolCallResp)

	var code int
	errOut := captureStderr(t, func() {
		// maxSteps=2로 빠르게 소진한다.
		code = run(context.Background(), stub, "claude-3-5-haiku-20241022", "검색해줘", 2)
	})

	if code == 0 {
		t.Fatal("max step 초과 시 비정상 종료코드(non-zero) 기대")
	}
	// max step 초과임을 드러내는 문구가 stderr에 있어야 한다.
	if !contains(errOut, "max step") {
		t.Errorf("max step 초과 문구가 stderr에 없다: %q", errOut)
	}
}

func TestRun_ChatError_WritesToStderrAndExitsNonZero(t *testing.T) {
	// client 에러의 원인 텍스트가 stderr로 전파되는지만 확인한다.
	causeErr := errors.New("chat 호출 실패")
	stub := llm.NewErrorStubClient(causeErr)

	var code int
	errOut := captureStderr(t, func() {
		code = run(context.Background(), stub, "claude-3-5-haiku-20241022", "안녕", 5)
	})

	if code == 0 {
		t.Fatal("에러 시 비정상 종료코드(non-zero) 기대")
	}
	if !contains(errOut, causeErr.Error()) {
		t.Errorf("에러 메시지가 stderr에 없다: %q", errOut)
	}
}

func TestRun_ContextCanceled_WritesToStderrAndExitsNonZero(t *testing.T) {
	stub := llm.NewStubClient(llm.ChatResponse{
		Message: message.Message{
			Role:    message.RoleAssistant,
			Content: []message.ContentBlock{message.NewTextBlock("응답")},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 이미 취소된 ctx를 전달해 ctx 취소 상황을 재현한다.

	var code int
	errOut := captureStderr(t, func() {
		code = run(ctx, stub, "claude-3-5-haiku-20241022", "안녕", 5)
	})

	if code == 0 {
		t.Fatal("ctx 취소 시 비정상 종료코드(non-zero) 기대")
	}
	if errOut == "" {
		t.Fatal("ctx 취소 에러가 stderr에 없다")
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
