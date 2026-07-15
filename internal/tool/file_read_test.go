package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReadReadsRootFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "note.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("hello\nworld"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	readFile, err := NewFileRead(root)
	if err != nil {
		t.Fatalf("NewFileRead() error = %v", err)
	}
	args := json.RawMessage(`{"path":"nested/note.txt"}`)

	if err := readFile.Validate(args); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	got, err := readFile.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Content != "hello\nworld" {
		t.Fatalf("Execute() content = %q, want %q", got.Content, "hello\nworld")
	}
}

func TestFileReadLimitsResultBytesWhileReading(t *testing.T) {
	root := t.TempDir()
	readFile, err := NewFileRead(root)
	if err != nil {
		t.Fatalf("NewFileRead() error = %v", err)
	}
	tests := []struct {
		name      string
		size      int
		wantError bool
	}{
		{name: "at limit", size: DefaultMaxResultBytes},
		{name: "over limit", size: DefaultMaxResultBytes + 1, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.name + ".txt"
			if err := os.WriteFile(filepath.Join(root, path), []byte(strings.Repeat("a", tt.size)), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			result, err := readFile.Execute(context.Background(), json.RawMessage(`{"path":`+quoteJSON(t, path)+`}`))
			if tt.wantError {
				if !IsExecutionError(err) || !strings.Contains(err.Error(), "byte limit") {
					t.Fatalf("Execute() error = %v, want result size execution error", err)
				}
				return
			}
			if err != nil || len(result.Content) != tt.size {
				t.Fatalf("Execute() result/error = %d/%v, want %d bytes", len(result.Content), err, tt.size)
			}
		})
	}
}

func TestFileReadRejectsInvalidArguments(t *testing.T) {
	readFile, err := NewFileRead(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRead() error = %v", err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "invalid json", args: json.RawMessage(`{"path":`)},
		{name: "missing path", args: json.RawMessage(`{}`)},
		{name: "empty path", args: json.RawMessage(`{"path":" "}`)},
		{name: "wrong path type", args: json.RawMessage(`{"path":1}`)},
		{name: "parent escape", args: json.RawMessage(`{"path":"../secret.txt"}`)},
		{name: "absolute path", args: json.RawMessage(`{"path":` + quoteJSON(t, outside) + `}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := readFile.Validate(tt.args)
			if !IsValidationError(err) {
				t.Fatalf("Validate() error = %v, want validation error", err)
			}
		})
	}
}

func TestFileReadReturnsExecutionErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	readFile, err := NewFileRead(root)
	if err != nil {
		t.Fatalf("NewFileRead() error = %v", err)
	}
	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "directory", args: json.RawMessage(`{"path":"docs"}`)},
		{name: "missing file", args: json.RawMessage(`{"path":"missing.txt"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := readFile.Validate(tt.args); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			_, err := readFile.Execute(context.Background(), tt.args)
			if !IsExecutionError(err) {
				t.Fatalf("Execute() error = %v, want execution error", err)
			}
		})
	}
}

func TestFileReadReturnsExecutionErrorWhenContextCanceled(t *testing.T) {
	readFile, err := NewFileRead(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRead() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = readFile.Execute(ctx, json.RawMessage(`{"path":"note.txt"}`))
	if !IsExecutionError(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want canceled execution error", err)
	}
}

func TestFileReadRejectsInvalidRoot(t *testing.T) {
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
			_, err := NewFileRead(tt.root)
			if !IsConfigurationError(err) {
				t.Fatalf("NewFileRead() error = %v, want configuration error", err)
			}
		})
	}
}

func TestFileReadSchemaMatchesToolIdentity(t *testing.T) {
	readFile, err := NewFileRead(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRead() error = %v", err)
	}
	schema := readFile.Schema()

	if readFile.Name() != "read_file" {
		t.Fatalf("Name() = %q, want read_file", readFile.Name())
	}
	if schema.Name != readFile.Name() {
		t.Fatalf("Schema().Name = %q, want %q", schema.Name, readFile.Name())
	}
	if schema.Description == "" || readFile.Description() == "" {
		t.Fatal("description must not be empty")
	}
	if !json.Valid(schema.InputSchema) {
		t.Fatalf("InputSchema is not valid JSON: %s", schema.InputSchema)
	}
}

func quoteJSON(t *testing.T, value string) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(encoded)
}
