package tool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

func codeExecutionInput(profile string, args ...string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"profile": profile,
		"args":    args,
	})
	return raw
}

func helperProfile(name string, allowedArgs ...string) tool.CommandProfile {
	return tool.CommandProfile{
		Name:        name,
		Command:     os.Args[0],
		Args:        []string{"-test.run=TestCodeExecution_HelperProcess", "--"},
		Env:         []string{"GO_WANT_CODE_EXEC_HELPER=1"},
		AllowedArgs: allowedArgs,
	}
}

func newTestCodeExecution(t *testing.T, maxOutputBytes int, profiles ...tool.CommandProfile) *tool.CodeExecution {
	t.Helper()
	ce, err := tool.NewCodeExecution(t.TempDir(), profiles, maxOutputBytes)
	if err != nil {
		t.Fatalf("NewCodeExecution 실패: %v", err)
	}
	return ce
}

func TestCodeExecution_HelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODE_EXEC_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	args = args[1:]
	if len(args) == 0 {
		os.Exit(2)
	}

	switch args[0] {
	case "stdout":
		fmt.Print("hello stdout")
	case "stderr":
		fmt.Fprint(os.Stderr, "hello stderr")
	case "exit7":
		fmt.Fprint(os.Stderr, "failed with seven")
		os.Exit(7)
	case "sleep":
		time.Sleep(500 * time.Millisecond)
	case "spam":
		fmt.Print(strings.Repeat("x", 2048))
	case "pwd":
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Print(wd)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q", args[0])
		os.Exit(2)
	}
	os.Exit(0)
}

func TestCodeExecution_Spec(t *testing.T) {
	ce := newTestCodeExecution(t, 1024, helperProfile("helper", "stdout"))
	spec := ce.Spec()

	if spec.Name != "code_execute" {
		t.Errorf("Spec().Name = %q, want %q", spec.Name, "code_execute")
	}
	if spec.Description == "" {
		t.Error("Spec().Description이 비어 있습니다")
	}
	if len(spec.InputSchema) == 0 {
		t.Error("Spec().InputSchema가 비어 있습니다")
	}
}

func TestCodeExecution_RunSuccess(t *testing.T) {
	ce := newTestCodeExecution(t, 1024, helperProfile("helper", "stdout", "stderr"))

	result, err := ce.Execute(context.Background(), codeExecutionInput("helper", "stdout"))
	if err != nil {
		t.Fatalf("예상치 못한 error: %v", err)
	}
	if result.IsError {
		t.Fatalf("IsError=true, Content=%q", result.Content)
	}
	for _, want := range []string{`profile="helper"`, "exit_status=0", "hello stdout", "stderr:"} {
		if !strings.Contains(result.Content, want) {
			t.Errorf("Content가 %q를 포함해야 합니다: %q", want, result.Content)
		}
	}

	result, err = ce.Execute(context.Background(), codeExecutionInput("helper", "stderr"))
	if err != nil {
		t.Fatalf("stderr 실행 실패: %v", err)
	}
	if !strings.Contains(result.Content, "hello stderr") {
		t.Errorf("Content가 stderr 출력을 포함해야 합니다: %q", result.Content)
	}
}

func TestCodeExecution_NonZeroExitObserved(t *testing.T) {
	ce := newTestCodeExecution(t, 1024, helperProfile("helper", "exit7"))

	result, err := ce.Execute(context.Background(), codeExecutionInput("helper", "exit7"))
	if err != nil {
		t.Fatalf("non-zero exit은 error가 아니라 결과로 관찰되어야 합니다: %v", err)
	}
	if !strings.Contains(result.Content, "exit_status=7") {
		t.Errorf("Content가 exit status를 포함해야 합니다: %q", result.Content)
	}
	if !strings.Contains(result.Content, "failed with seven") {
		t.Errorf("Content가 stderr를 포함해야 합니다: %q", result.Content)
	}
}

func TestCodeExecution_RejectsUnknownProfileAndArgs(t *testing.T) {
	ce := newTestCodeExecution(t, 1024, helperProfile("helper", "stdout"))

	tests := []struct {
		name  string
		input json.RawMessage
	}{
		{"unknown profile", codeExecutionInput("unknown", "stdout")},
		{"missing profile", codeExecutionInput("")},
		{"disallowed arg", codeExecutionInput("helper", "sleep")},
		{"malformed JSON", json.RawMessage(`not json`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ce.Execute(context.Background(), tt.input)
			if err == nil {
				t.Fatal("잘못된 실행 요청에서 error를 반환해야 하는데 nil을 반환했습니다")
			}
		})
	}
}

func TestCodeExecution_TimeoutViaDispatcher(t *testing.T) {
	ce := newTestCodeExecution(t, 1024, helperProfile("helper", "sleep"))
	d := newDispatcher(20*time.Millisecond, ce)
	call := message.ToolCall{
		ID:    "exec-timeout-1",
		Name:  "code_execute",
		Input: codeExecutionInput("helper", "sleep"),
	}

	result := d.Dispatch(context.Background(), call)
	if !result.IsError {
		t.Fatal("timeout이 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
	}
	if result.ToolCallID != call.ID {
		t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
	}
}

func TestCodeExecution_OutputCapExceeded(t *testing.T) {
	ce := newTestCodeExecution(t, 32, helperProfile("helper", "spam"))
	d := newDispatcher(time.Second, ce)
	call := message.ToolCall{
		ID:    "exec-cap-1",
		Name:  "code_execute",
		Input: codeExecutionInput("helper", "spam"),
	}

	result := d.Dispatch(context.Background(), call)
	if !result.IsError {
		t.Fatal("output cap 초과가 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
	}
	if !strings.Contains(result.Content, "출력 제한 초과") {
		t.Errorf("Content가 출력 제한 초과를 포함해야 합니다: %q", result.Content)
	}
}

func TestCodeExecution_RejectsFileArgsOutsideWorkDir(t *testing.T) {
	profile := helperProfile("path-helper")
	profile.AllowRelativeFileArgs = true
	ce := newTestCodeExecution(t, 1024, profile)

	tests := []string{
		"../outside.txt",
		filepath.Join("nested", "..", "..", "outside.txt"),
		filepath.Join(t.TempDir(), "absolute.txt"),
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			_, err := ce.Execute(context.Background(), codeExecutionInput("path-helper", path))
			if err == nil {
				t.Fatalf("제한 범위 밖 파일 인자 %q가 거부되어야 하는데 nil을 반환했습니다", path)
			}
		})
	}
}

func TestCodeExecution_RunViaDispatcher(t *testing.T) {
	ce := newTestCodeExecution(t, 1024, helperProfile("helper", "stdout"))
	d := newDispatcher(time.Second, ce)
	call := message.ToolCall{
		ID:    "exec-1",
		Name:  "code_execute",
		Input: codeExecutionInput("helper", "stdout"),
	}

	result := d.Dispatch(context.Background(), call)
	if result.IsError {
		t.Fatalf("IsError=true, Content=%q", result.Content)
	}
	if result.ToolCallID != call.ID {
		t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
	}
	if !strings.Contains(result.Content, "hello stdout") {
		t.Errorf("Content가 stdout을 포함해야 합니다: %q", result.Content)
	}
}
