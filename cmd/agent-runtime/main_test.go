package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

type runStubClient struct {
	response    llm.ChatResponse
	err         error
	request     llm.ChatRequest
	hasDeadline bool
}

func (c *runStubClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.request = req
	_, c.hasDeadline = ctx.Deadline()
	return c.response, c.err
}

// TestRunUsesArgsPromptAndWritesOnlyResponseText 는 positional argument prompt가 LLM 요청으로 전달되고 stdout에는 응답 text만 쓰이는지 확인한다.
func TestRunUsesArgsPromptAndWritesOnlyResponseText(t *testing.T) {
	stub := &runStubClient{response: llm.ChatResponse{Message: message.Assistant("answer text")}}
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"hello", "runtime"},
		strings.NewReader("ignored"),
		&stdout,
		&stderr,
		func() (config.Config, error) {
			return config.Config{
				LLMProvider: "ollama",
				LLMModel:    "ollama-test",
				LLMHost:     "http://localhost:11434",
				LLMTimeout:  time.Second,
			}, nil
		},
		func(config.Config) (llm.LLMClient, error) {
			return stub, nil
		},
	)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "answer text\n" {
		t.Fatalf("stdout = %q, want response text only", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if stub.request.Model != "ollama-test" {
		t.Fatalf("request Model = %q, want ollama-test", stub.request.Model)
	}
	if len(stub.request.Messages) != 1 || stub.request.Messages[0].Role != message.RoleUser || stub.request.Messages[0].Text != "hello runtime" {
		t.Fatalf("request Messages = %+v, want single user prompt", stub.request.Messages)
	}
	if !stub.hasDeadline {
		t.Fatal("Chat context has no deadline, want LLM_TIMEOUT deadline")
	}
}

// TestRunReadsPromptFromStdin 는 positional argument가 없을 때 stdin 전체를 단발 prompt로 사용하는지 확인한다.
func TestRunReadsPromptFromStdin(t *testing.T) {
	stub := &runStubClient{response: llm.ChatResponse{Message: message.Assistant("stdin answer")}}
	var stdout, stderr bytes.Buffer

	code := run(
		nil,
		strings.NewReader("from stdin\n"),
		&stdout,
		&stderr,
		func() (config.Config, error) {
			return config.Config{
				LLMProvider: "ollama",
				LLMModel:    "ollama-test",
				LLMHost:     "http://localhost:11434",
				LLMTimeout:  time.Second,
			}, nil
		},
		func(config.Config) (llm.LLMClient, error) {
			return stub, nil
		},
	)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stub.request.Messages[0].Text != "from stdin" {
		t.Fatalf("prompt = %q, want stdin text", stub.request.Messages[0].Text)
	}
	if stdout.String() != "stdin answer\n" || stderr.String() != "" {
		t.Fatalf("stdout/stderr = %q/%q, want response on stdout only", stdout.String(), stderr.String())
	}
}

// TestRunRejectsEmptyPromptWithoutCallingProvider 는 빈 prompt를 외부 호출 전에 사용 오류로 종료하는지 확인한다.
func TestRunRejectsEmptyPromptWithoutCallingProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false

	code := run(
		nil,
		strings.NewReader(" \n\t"),
		&stdout,
		&stderr,
		func() (config.Config, error) {
			called = true
			return config.Config{}, nil
		},
		func(config.Config) (llm.LLMClient, error) {
			t.Fatal("buildClient should not be called for empty prompt")
			return nil, nil
		},
	)

	if code == 0 {
		t.Fatal("run() code = 0, want non-zero")
	}
	if called {
		t.Fatal("loadConfig was called, want prompt validation before config/provider setup")
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "prompt is required") {
		t.Fatalf("stderr = %q, want prompt error", stderr.String())
	}
}

// TestRunWritesLLMErrorsToStderr 는 provider 호출 실패가 stderr와 non-zero exit로 분리되는지 확인한다.
func TestRunWritesLLMErrorsToStderr(t *testing.T) {
	stub := &runStubClient{err: errors.New("provider unavailable")}
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"hello"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) {
			return config.Config{
				LLMProvider: "claude",
				LLMModel:    "claude-test",
				LLMAPIKey:   "secret-key",
				LLMTimeout:  time.Second,
			}, nil
		},
		func(config.Config) (llm.LLMClient, error) {
			return stub, nil
		},
	)

	if code == 0 {
		t.Fatal("run() code = 0, want non-zero")
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "llm error: provider unavailable") {
		t.Fatalf("stderr = %q, want provider error", stderr.String())
	}
	if strings.Contains(stderr.String(), "secret-key") {
		t.Fatalf("stderr exposed API key: %q", stderr.String())
	}
}
