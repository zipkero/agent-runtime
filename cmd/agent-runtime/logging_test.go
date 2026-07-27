package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    slog.Level
		wantErr bool
	}{
		{name: "empty falls back to config default", level: "", want: slog.LevelInfo},
		{name: "blank falls back to config default", level: "  ", want: slog.LevelInfo},
		{name: "debug", level: "debug", want: slog.LevelDebug},
		{name: "info", level: "info", want: slog.LevelInfo},
		{name: "warn", level: "warn", want: slog.LevelWarn},
		{name: "warning alias", level: "warning", want: slog.LevelWarn},
		{name: "error", level: "error", want: slog.LevelError},
		{name: "case insensitive", level: "DEBUG", want: slog.LevelDebug},
		{name: "unsupported", level: "verbose", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLogLevel(test.level)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseLogLevel(%q) error = nil, want error", test.level)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLogLevel(%q) error = %v", test.level, err)
			}
			if got != test.want {
				t.Fatalf("parseLogLevel(%q) = %v, want %v", test.level, got, test.want)
			}
		})
	}
}

func TestParseLogLevelDefaultMatchesConfig(t *testing.T) {
	fromEmpty, err := parseLogLevel("")
	if err != nil {
		t.Fatalf("parseLogLevel(\"\") error = %v", err)
	}
	fromDefault, err := parseLogLevel(config.DefaultLogLevel)
	if err != nil {
		t.Fatalf("parseLogLevel(%q) error = %v", config.DefaultLogLevel, err)
	}
	if fromEmpty != fromDefault {
		t.Fatalf("empty level = %v, config default %q = %v, want the same level",
			fromEmpty, config.DefaultLogLevel, fromDefault)
	}
}

func TestRunLogsStartupAndFinishToStderr(t *testing.T) {
	t.Chdir(t.TempDir())
	stub := &runStubClient{responses: []llm.ChatResponse{
		{Message: message.Assistant("logged answer"), FinishReason: llm.FinishReasonComplete},
	}}
	cfg := testConfig()
	cfg.LogLevel = "info"
	var stdout, stderr bytes.Buffer

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
	if stdout.String() != "logged answer\n" {
		t.Fatalf("stdout = %q, want final answer only", stdout.String())
	}

	logged := stderr.String()
	for _, want := range []string{
		"level=INFO",
		"msg=starting",
		"provider=ollama",
		"model=ollama-test",
		"tools registered",
		"run finished",
		"status=final",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("stderr = %q, want it to contain %q", logged, want)
		}
	}
	if strings.Contains(logged, "msg=trace") {
		t.Fatalf("stderr = %q, want no trace output at info level", logged)
	}
}

func TestRunLogsTraceAtDebugLevelWithoutSecrets(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	stub := &runStubClient{responses: []llm.ChatResponse{
		{
			Message: message.Assistant("", message.ToolCall{
				ID:        "calculator-1",
				Name:      "calculator",
				Arguments: json.RawMessage(`{"left":1,"operator":"+","right":2}`),
			}),
			FinishReason: llm.FinishReasonToolCall,
		},
		{Message: message.Assistant("three"), FinishReason: llm.FinishReasonComplete},
	}}
	cfg := testConfig()
	cfg.LogLevel = "debug"
	cfg.LLMAPIKey = "secret-key"
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"add", "one", "and", "two"},
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
	if stdout.String() != "three\n" {
		t.Fatalf("stdout = %q, want final answer only", stdout.String())
	}

	logged := stderr.String()
	for _, want := range []string{
		"msg=trace",
		"action=user_message",
		"action=llm_response",
		"action=tool_call",
		"action=tool_result",
		"tool=calculator",
		"action=final",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("stderr = %q, want trace output to contain %q", logged, want)
		}
	}
	if strings.Contains(logged, "secret-key") {
		t.Fatalf("stderr exposed API key: %q", logged)
	}
}

func TestRunRejectsUnsupportedLogLevelBeforeProviderSetup(t *testing.T) {
	cfg := testConfig()
	cfg.LogLevel = "verbose"
	var stdout, stderr bytes.Buffer

	code := run(
		[]string{"hello"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func() (config.Config, error) { return cfg, nil },
		func(config.Config) (llm.LLMClient, error) {
			t.Fatal("buildClient should not be called for an unsupported log level")
			return nil, nil
		},
		func(config.Config, string) (*tool.Registry, error) {
			t.Fatal("buildTools should not be called for an unsupported log level")
			return nil, nil
		},
	)

	if code == 0 || stdout.String() != "" {
		t.Fatalf("code/stdout = %d/%q, want non-zero and empty", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), `LOG_LEVEL: unsupported level "verbose"`) {
		t.Fatalf("stderr = %q, want unsupported log level error", stderr.String())
	}
}
