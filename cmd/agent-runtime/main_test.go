package main

import (
	"bytes"
	"context"
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

func TestRun_TextResponse_WritesToStdout(t *testing.T) {
	stub := llm.NewStubClient(llm.ChatResponse{
		Message: message.Message{
			Role:    message.RoleAssistant,
			Content: []message.ContentBlock{message.NewTextBlock("안녕하세요")},
		},
	})

	var code int
	out := captureStdout(t, func() {
		code = run(context.Background(), stub, "claude-3-5-haiku-20241022", "안녕")
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

func TestRun_ToolCall_WritesDistinctToStdout(t *testing.T) {
	stub := llm.NewStubClient(llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "tc-1",
					Name:  "search",
					Input: []byte(`{"query":"test"}`),
				}),
			},
		},
	})

	var code int
	out := captureStdout(t, func() {
		code = run(context.Background(), stub, "claude-3-5-haiku-20241022", "검색해줘")
	})

	if code != 0 {
		t.Fatalf("종료코드 0 기대, 실제: %d", code)
	}
	// tool call 구분 표시([tool_call] 접두사)가 stdout에 있어야 한다.
	if !contains(out, "[tool_call]") {
		t.Errorf("tool call 구분 표시가 stdout에 없다: %q", out)
	}
	if !contains(out, "search") {
		t.Errorf("tool name이 stdout에 없다: %q", out)
	}
}

func TestRun_ChatError_WritesToStderrAndExitsNonZero(t *testing.T) {
	stub := llm.NewErrorStubClient(errors.New("인증 실패"))

	var code int
	errOut := captureStderr(t, func() {
		code = run(context.Background(), stub, "claude-3-5-haiku-20241022", "안녕")
	})

	if code == 0 {
		t.Fatal("에러 시 비정상 종료코드(non-zero) 기대")
	}
	if !contains(errOut, "인증 실패") {
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
		code = run(ctx, stub, "claude-3-5-haiku-20241022", "안녕")
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
