package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodeExecutionRunsGoCommand(t *testing.T) {
	root := newGoModule(t, map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("hello from go")
}
`,
	})
	codeExecution, err := NewCodeExecution(root)
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}
	args := json.RawMessage(`{"args":["run","."],"stdin":"ignored"}`)

	if err := codeExecution.Validate(args); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	got, err := codeExecution.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var content codeExecutionContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if !strings.Contains(content.Stdout, "hello from go") || content.Stderr != "" || content.ExitCode != 0 || content.TimedOut {
		t.Fatalf("content = %+v, want stdout, empty stderr, exit 0, no timeout", content)
	}
}

func TestCodeExecutionPassesStdin(t *testing.T) {
	root := newGoModule(t, map[string]string{
		"main.go": `package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	content, _ := io.ReadAll(os.Stdin)
	fmt.Print(string(content))
}
`,
	})
	codeExecution, err := NewCodeExecution(root)
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}

	got, err := codeExecution.Execute(context.Background(), json.RawMessage(`{"args":["run","."],"stdin":"from stdin"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var content codeExecutionContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if content.Stdout != "from stdin" {
		t.Fatalf("stdout = %q, want stdin echo", content.Stdout)
	}
}

func TestCodeExecutionReturnsExecutionErrorWithOutput(t *testing.T) {
	root := newGoModule(t, map[string]string{
		"main_test.go": `package main

import "testing"

func TestFailure(t *testing.T) {
	t.Fatal("expected failure")
}
`,
	})
	codeExecution, err := NewCodeExecution(root)
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}

	got, err := codeExecution.Execute(context.Background(), json.RawMessage(`{"args":["test","./..."]}`))
	if !IsExecutionError(err) {
		t.Fatalf("Execute() error = %v, want execution error", err)
	}

	var content codeExecutionContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if content.ExitCode == 0 || content.TimedOut {
		t.Fatalf("content = %+v, want non-zero exit without timeout", content)
	}
	if !strings.Contains(content.Stdout+content.Stderr, "expected failure") {
		t.Fatalf("content output = stdout:%q stderr:%q, want failure output", content.Stdout, content.Stderr)
	}

	var errorContent codeExecutionContent
	if err := json.Unmarshal([]byte(err.Error()), &errorContent); err != nil {
		t.Fatalf("execution error is not JSON content: %v", err)
	}
	if errorContent.ExitCode != content.ExitCode || !strings.Contains(errorContent.Stdout+errorContent.Stderr, "expected failure") {
		t.Fatalf("execution error content = %+v, want preserved output content", errorContent)
	}
}

func TestCodeExecutionReturnsExecutionErrorOnTimeout(t *testing.T) {
	root := newGoModule(t, map[string]string{
		"main_test.go": `package main

import (
	"testing"
	"time"
)

func TestSleep(t *testing.T) {
	time.Sleep(5 * time.Second)
}
`,
	})
	codeExecution, err := NewCodeExecution(root)
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	got, err := codeExecution.Execute(ctx, json.RawMessage(`{"args":["test","./..."]}`))
	if !IsExecutionError(err) {
		t.Fatalf("Execute() error = %v, want execution error", err)
	}

	var content codeExecutionContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if !content.TimedOut {
		t.Fatalf("content = %+v, want timed_out true", content)
	}
}

func TestCodeExecutionReturnsExecutionErrorWhenContextCanceled(t *testing.T) {
	codeExecution, err := NewCodeExecution(newGoModule(t, nil))
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := codeExecution.Execute(ctx, json.RawMessage(`{"args":["version"]}`))
	if !IsExecutionError(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want canceled execution error", err)
	}

	var content codeExecutionContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if content.TimedOut || content.ExitCode != -1 {
		t.Fatalf("content = %+v, want canceled result", content)
	}
}

func TestCodeExecutionRejectsInvalidArguments(t *testing.T) {
	codeExecution, err := NewCodeExecution(newGoModule(t, nil))
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "main.go")
	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "invalid json", args: json.RawMessage(`{"args":`)},
		{name: "missing args", args: json.RawMessage(`{}`)},
		{name: "empty args", args: json.RawMessage(`{"args":[]}`)},
		{name: "wrong args type", args: json.RawMessage(`{"args":"test"}`)},
		{name: "wrong arg type", args: json.RawMessage(`{"args":[1]}`)},
		{name: "blank arg", args: json.RawMessage(`{"args":["test"," "]}`)},
		{name: "unsupported subcommand", args: json.RawMessage(`{"args":["install","./..."]}`)},
		{name: "workdir flag", args: json.RawMessage(`{"args":["test","-C",".."]}`)},
		{name: "parent escape", args: json.RawMessage(`{"args":["test","../..."]}`)},
		{name: "absolute path", args: json.RawMessage(`{"args":["run",` + quoteJSON(t, outside) + `]}`)},
		{name: "env mutation", args: json.RawMessage(`{"args":["env","-w","GOPATH=/tmp/go"]}`)},
		{name: "explicit executable", args: json.RawMessage(`{"args":["version"],"executable":"go"}`)},
		{name: "explicit workdir", args: json.RawMessage(`{"args":["version"],"workdir":"."}`)},
		{name: "wrong stdin type", args: json.RawMessage(`{"args":["version"],"stdin":1}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := codeExecution.Validate(tt.args)
			if !IsValidationError(err) {
				t.Fatalf("Validate() error = %v, want validation error", err)
			}
		})
	}
}

func TestCodeExecutionLimitsOutput(t *testing.T) {
	root := newGoModule(t, map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Print("0123456789")
}
`,
	})
	codeExecution, err := newCodeExecution(root, "go", 4)
	if err != nil {
		t.Fatalf("newCodeExecution() error = %v", err)
	}

	got, err := codeExecution.Execute(context.Background(), json.RawMessage(`{"args":["run","."]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var content codeExecutionContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if content.Stdout != "0123" || !content.StdoutTruncated {
		t.Fatalf("content = %+v, want truncated stdout", content)
	}
}

func TestCodeExecutionRejectsInvalidRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name string
		root string
	}{
		{name: "empty", root: " "},
		{name: "missing", root: filepath.Join(t.TempDir(), "missing")},
		{name: "file", root: file},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCodeExecution(tt.root)
			if !IsConfigurationError(err) {
				t.Fatalf("NewCodeExecution() error = %v, want configuration error", err)
			}
		})
	}
}

func TestCodeExecutionSchemaMatchesToolIdentity(t *testing.T) {
	codeExecution, err := NewCodeExecution(newGoModule(t, nil))
	if err != nil {
		t.Fatalf("NewCodeExecution() error = %v", err)
	}
	schema := codeExecution.Schema()

	if codeExecution.Name() != "code_execution" {
		t.Fatalf("Name() = %q, want code_execution", codeExecution.Name())
	}
	if schema.Name != codeExecution.Name() {
		t.Fatalf("Schema().Name = %q, want %q", schema.Name, codeExecution.Name())
	}
	if schema.Description == "" || codeExecution.Description() == "" {
		t.Fatal("description must not be empty")
	}
	if !json.Valid(schema.InputSchema) {
		t.Fatalf("InputSchema is not valid JSON: %s", schema.InputSchema)
	}
}

func newGoModule(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/code-execution-test\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	return root
}
