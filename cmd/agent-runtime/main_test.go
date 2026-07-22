package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

type runStubClient struct {
	responses []llm.ChatResponse
	err       error
	requests  []llm.ChatRequest
	deadlines []time.Time
	delay     time.Duration
}

func (c *runStubClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.requests = append(c.requests, req)
	deadline, _ := ctx.Deadline()
	c.deadlines = append(c.deadlines, deadline)
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	if len(c.responses) == 0 {
		return llm.ChatResponse{}, errors.New("stub response is required")
	}
	index := len(c.requests) - 1
	if index >= len(c.responses) {
		index = len(c.responses) - 1
	}
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	return c.responses[index], nil
}

func testConfig() config.Config {
	return config.Config{
		LLMProvider: "ollama",
		LLMModel:    "ollama-test",
		LLMHost:     "http://localhost:11434",
		LLMTimeout:  2 * time.Second,
	}
}

func TestRunExecutesToolLoopFromCurrentWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello tool"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	t.Chdir(root)

	stub := &runStubClient{delay: 20 * time.Millisecond, responses: []llm.ChatResponse{
		{
			Message: message.Assistant("", message.ToolCall{
				ID:        "read-1",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}),
			FinishReason: llm.FinishReasonToolCall,
		},
		{Message: message.Assistant("read complete"), FinishReason: llm.FinishReasonComplete},
	}}
	var stdout, stderr bytes.Buffer
	started := time.Now()

	code := run(
		[]string{"read", "the", "note"},
		strings.NewReader("ignored"),
		&stdout,
		&stderr,
		func() (config.Config, error) { return testConfig(), nil },
		func(config.Config) (llm.LLMClient, error) { return stub, nil },
		newConfiguredTools,
	)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "read complete\n" || stderr.String() != "" {
		t.Fatalf("stdout/stderr = %q/%q, want final answer on stdout only", stdout.String(), stderr.String())
	}
	if len(stub.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(stub.requests))
	}
	if got, want := schemaNames(stub.requests[0]), []string{"calculator", "read_file", "web_search", "save_file"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tool schema names = %v, want %v", got, want)
	}
	secondMessages := stub.requests[1].Messages
	if len(secondMessages) != 3 || secondMessages[2].ToolResult == nil {
		t.Fatalf("second request messages = %+v, want user, assistant, and tool result", secondMessages)
	}
	if result := secondMessages[2].ToolResult; result.IsError || result.Name != "read_file" || result.Content != "hello tool" {
		t.Fatalf("tool result = %+v, want current-directory file content", result)
	}
	if len(stub.deadlines) != 2 || stub.deadlines[0].IsZero() || stub.deadlines[1].IsZero() {
		t.Fatalf("model deadlines = %v, want one deadline per call", stub.deadlines)
	}
	if !stub.deadlines[1].After(stub.deadlines[0]) {
		t.Fatalf("model deadlines = %v, want a fresh timeout for each call", stub.deadlines)
	}
	latestAllowed := started.Add(testConfig().LLMTimeout + time.Second)
	for _, deadline := range stub.deadlines {
		if deadline.After(latestAllowed) {
			t.Fatalf("model deadline = %v, want LLM_TIMEOUT per call", deadline)
		}
	}
}

func TestRunUsesCurrentWorkingDirectoryForCodeExecution(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/cli-root\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	t.Chdir(root)

	stub := &runStubClient{responses: []llm.ChatResponse{
		{
			Message: message.Assistant("", message.ToolCall{
				ID:        "code-1",
				Name:      "code_execution",
				Arguments: json.RawMessage(`{"args":["list","-m"]}`),
			}),
			FinishReason: llm.FinishReasonToolCall,
		},
		{Message: message.Assistant("code complete"), FinishReason: llm.FinishReasonComplete},
	}}
	cfg := testConfig()
	cfg.EnableCodeExecution = true
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"inspect", "module"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) { return cfg, nil },
		func(config.Config) (llm.LLMClient, error) { return stub, nil },
		newConfiguredTools,
	)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := schemaNames(stub.requests[0]), []string{"calculator", "read_file", "web_search", "save_file", "code_execution"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tool schema names = %v, want %v", got, want)
	}
	toolResult := stub.requests[1].Messages[2].ToolResult
	if toolResult == nil || toolResult.IsError {
		t.Fatalf("code execution result = %+v, want success", toolResult)
	}
	var content struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(toolResult.Content), &content); err != nil {
		t.Fatalf("decode code execution result: %v", err)
	}
	if content.Stdout != "example.com/cli-root\n" {
		t.Fatalf("code stdout = %q, want module from current working directory", content.Stdout)
	}
}

func TestRunReadsPromptFromStdin(t *testing.T) {
	t.Chdir(t.TempDir())
	stub := &runStubClient{responses: []llm.ChatResponse{{Message: message.Assistant("stdin answer")}}}
	var stdout, stderr bytes.Buffer

	code := run(
		nil,
		strings.NewReader("from stdin\n"),
		&stdout,
		&stderr,
		func() (config.Config, error) { return testConfig(), nil },
		func(config.Config) (llm.LLMClient, error) { return stub, nil },
		newConfiguredTools,
	)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stub.requests[0].Messages[0].Text != "from stdin" {
		t.Fatalf("prompt = %q, want stdin text", stub.requests[0].Messages[0].Text)
	}
	if stdout.String() != "stdin answer\n" || stderr.String() != "" {
		t.Fatalf("stdout/stderr = %q/%q, want response on stdout only", stdout.String(), stderr.String())
	}
}

func TestRunAppliesOverallDeadline(t *testing.T) {
	t.Chdir(t.TempDir())
	stub := &runStubClient{responses: []llm.ChatResponse{{Message: message.Assistant("answer")}}}
	cfg := testConfig()
	cfg.LLMTimeout = 15 * time.Minute
	var stdout, stderr bytes.Buffer
	started := time.Now()

	code := run(
		[]string{"hello"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) { return cfg, nil },
		func(config.Config) (llm.LLMClient, error) { return stub, nil },
		newConfiguredTools,
	)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if len(stub.deadlines) != 1 || stub.deadlines[0].IsZero() {
		t.Fatalf("model deadlines = %v, want overall run deadline", stub.deadlines)
	}
	if deadline := stub.deadlines[0]; deadline.Before(started.Add(cliRunTimeout-time.Second)) || deadline.After(started.Add(cliRunTimeout+time.Second)) {
		t.Fatalf("deadline = %v, want CLI run deadline near %v", deadline, started.Add(cliRunTimeout))
	}
}

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
		func(config.Config, string) (*tool.Registry, error) {
			t.Fatal("buildTools should not be called for empty prompt")
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

func TestRunWritesAgentErrorsToStderr(t *testing.T) {
	t.Chdir(t.TempDir())
	stub := &runStubClient{err: errors.New("provider unavailable")}
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"hello"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) {
			cfg := testConfig()
			cfg.LLMAPIKey = "secret-key"
			return cfg, nil
		},
		func(config.Config) (llm.LLMClient, error) { return stub, nil },
		newConfiguredTools,
	)

	if code == 0 {
		t.Fatal("run() code = 0, want non-zero")
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "agent error: provider unavailable") {
		t.Fatalf("stderr = %q, want agent error", stderr.String())
	}
	if strings.Contains(stderr.String(), "secret-key") {
		t.Fatalf("stderr exposed API key: %q", stderr.String())
	}
}

func TestRunReportsRunnerCreationFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"hello"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) { return testConfig(), nil },
		func(config.Config) (llm.LLMClient, error) { return nil, nil },
		newConfiguredTools,
	)

	if code == 0 || stdout.String() != "" {
		t.Fatalf("code/stdout = %d/%q, want non-zero and empty", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "runner error: agent runner client is required") {
		t.Fatalf("stderr = %q, want runner creation error", stderr.String())
	}
}

func TestRunReportsNeedsAction(t *testing.T) {
	t.Chdir(t.TempDir())
	stub := &runStubClient{responses: []llm.ChatResponse{{
		Message: message.Assistant("", message.ToolCall{
			ID:        "read-1",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"note.txt"}`),
		}),
		FinishReason: llm.FinishReasonToolCall,
	}}}
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"read"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) { return testConfig(), nil },
		func(config.Config) (llm.LLMClient, error) { return stub, nil },
		func(config.Config, string) (*tool.Registry, error) { return tool.NewRegistry(), nil },
	)

	if code == 0 || stdout.String() != "" {
		t.Fatalf("code/stdout = %d/%q, want non-zero and empty", code, stdout.String())
	}
	if stderr.String() != "agent stopped: needs_action\n" {
		t.Fatalf("stderr = %q, want needs_action status", stderr.String())
	}
}

func TestRunReportsMaxSteps(t *testing.T) {
	t.Chdir(t.TempDir())
	stub := &runStubClient{responses: []llm.ChatResponse{{
		Message: message.Assistant("", message.ToolCall{
			ID:        "calculator-1",
			Name:      "calculator",
			Arguments: json.RawMessage(`{"left":1,"operator":"+","right":1}`),
		}),
		FinishReason: llm.FinishReasonToolCall,
	}}}
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"keep", "calculating"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) { return testConfig(), nil },
		func(config.Config) (llm.LLMClient, error) { return stub, nil },
		newConfiguredTools,
	)

	if code == 0 || stdout.String() != "" {
		t.Fatalf("code/stdout = %d/%q, want non-zero and empty", code, stdout.String())
	}
	if stderr.String() != "agent stopped: max_steps\n" {
		t.Fatalf("stderr = %q, want max_steps status", stderr.String())
	}
	if len(stub.requests) != cliMaxSteps {
		t.Fatalf("request count = %d, want max steps %d", len(stub.requests), cliMaxSteps)
	}
}

func schemaNames(req llm.ChatRequest) []string {
	names := make([]string, 0, len(req.Tools))
	for _, schema := range req.Tools {
		names = append(names, schema.Name)
	}
	return names
}
