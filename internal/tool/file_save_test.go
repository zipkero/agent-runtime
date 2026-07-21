package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSaveCreatesRootFile(t *testing.T) {
	root := t.TempDir()
	saveFile, err := NewFileSave(root)
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}
	args := json.RawMessage(`{"path":"nested/note.txt","content":"hello\nworld"}`)

	if err := saveFile.Validate(args); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	got, err := saveFile.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "nested", "note.txt")); err != nil || string(content) != "hello\nworld" {
		t.Fatalf("saved content = %q, error = %v, want hello world", string(content), err)
	}

	var content fileSaveContent
	if err := json.Unmarshal([]byte(got.Content), &content); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if content.Path != filepath.Join("nested", "note.txt") || content.Bytes != len("hello\nworld") || content.Overwritten {
		t.Fatalf("result content = %+v, want saved path, byte count, no overwrite", content)
	}
}

func TestFileSaveCreatesParentDirectories(t *testing.T) {
	root := t.TempDir()
	saveFile, err := NewFileSave(root)
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}

	_, err = saveFile.Execute(context.Background(), json.RawMessage(`{"path":"a/b/c.txt","content":"content"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a", "b", "c.txt")); err != nil {
		t.Fatalf("Stat() error = %v, want created file", err)
	}
}

func TestFileSaveAllowsEmptyContent(t *testing.T) {
	root := t.TempDir()
	saveFile, err := NewFileSave(root)
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}

	got, err := saveFile.Execute(context.Background(), json.RawMessage(`{"path":"empty.txt","content":""}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "empty.txt")); err != nil || len(content) != 0 {
		t.Fatalf("saved content length = %d, error = %v, want empty file", len(content), err)
	}

	var result fileSaveContent
	if err := json.Unmarshal([]byte(got.Content), &result); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if result.Bytes != 0 {
		t.Fatalf("result bytes = %d, want 0", result.Bytes)
	}
}

func TestFileSaveOverwritesOnlyWhenAllowed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	saveFile, err := NewFileSave(root)
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}

	_, err = saveFile.Execute(context.Background(), json.RawMessage(`{"path":"note.txt","content":"new"}`))
	if !IsExecutionError(err) {
		t.Fatalf("Execute() error = %v, want execution error", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "old" {
		t.Fatalf("content after rejected overwrite = %q, error = %v, want old", string(content), err)
	}

	got, err := saveFile.Execute(context.Background(), json.RawMessage(`{"path":"note.txt","content":"new","overwrite":true}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "new" {
		t.Fatalf("content after overwrite = %q, error = %v, want new", string(content), err)
	}

	var result fileSaveContent
	if err := json.Unmarshal([]byte(got.Content), &result); err != nil {
		t.Fatalf("result content is not JSON: %v", err)
	}
	if !result.Overwritten || result.Bytes != len("new") {
		t.Fatalf("result content = %+v, want overwritten true and byte count", result)
	}
}

func TestFileSaveRejectsInvalidArguments(t *testing.T) {
	saveFile, err := NewFileSave(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "invalid json", args: json.RawMessage(`{"path":`)},
		{name: "missing path", args: json.RawMessage(`{"content":"x"}`)},
		{name: "missing content", args: json.RawMessage(`{"path":"note.txt"}`)},
		{name: "empty path", args: json.RawMessage(`{"path":" ","content":"x"}`)},
		{name: "wrong path type", args: json.RawMessage(`{"path":1,"content":"x"}`)},
		{name: "wrong content type", args: json.RawMessage(`{"path":"note.txt","content":1}`)},
		{name: "wrong overwrite type", args: json.RawMessage(`{"path":"note.txt","content":"x","overwrite":"yes"}`)},
		{name: "parent escape", args: json.RawMessage(`{"path":"../secret.txt","content":"x"}`)},
		{name: "absolute path", args: json.RawMessage(`{"path":` + quoteJSON(t, outside) + `,"content":"x"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := saveFile.Validate(tt.args)
			if !IsValidationError(err) {
				t.Fatalf("Validate() error = %v, want validation error", err)
			}
		})
	}
}

func TestFileSaveReturnsExecutionErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "parent-file"), []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	saveFile, err := NewFileSave(root)
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}

	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "directory", args: json.RawMessage(`{"path":"docs","content":"x","overwrite":true}`)},
		{name: "parent is file", args: json.RawMessage(`{"path":"parent-file/note.txt","content":"x"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := saveFile.Validate(tt.args); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			_, err := saveFile.Execute(context.Background(), tt.args)
			if !IsExecutionError(err) {
				t.Fatalf("Execute() error = %v, want execution error", err)
			}
		})
	}
}

func TestFileSaveRejectsSymlinksOutsideRootWithoutSideEffects(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	createDirectoryLink(t, outside, filepath.Join(root, "linked"))
	outsideFile := filepath.Join(outside, "existing.txt")
	if err := os.WriteFile(outsideFile, []byte("original"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	saveFile, err := NewFileSave(root)
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}

	t.Run("intermediate symlink with new parents", func(t *testing.T) {
		args := json.RawMessage(`{"path":"linked/new/parents/note.txt","content":"x"}`)
		if err := saveFile.Validate(args); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		_, err := saveFile.Execute(context.Background(), args)
		if !IsExecutionError(err) {
			t.Fatalf("Execute() error = %v, want execution error", err)
		}
	})

	t.Run("final directory symlink", func(t *testing.T) {
		args := json.RawMessage(`{"path":"linked","content":"changed","overwrite":true}`)
		if err := saveFile.Validate(args); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		_, err := saveFile.Execute(context.Background(), args)
		if !IsExecutionError(err) {
			t.Fatalf("Execute() error = %v, want execution error", err)
		}
	})

	t.Run("final file symlink", func(t *testing.T) {
		if err := os.Symlink(outsideFile, filepath.Join(root, "file-link.txt")); err != nil {
			t.Skipf("Symlink() error = %v", err)
		}
		args := json.RawMessage(`{"path":"file-link.txt","content":"changed","overwrite":true}`)
		if err := saveFile.Validate(args); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		_, err := saveFile.Execute(context.Background(), args)
		if !IsExecutionError(err) {
			t.Fatalf("Execute() error = %v, want execution error", err)
		}
	})

	if _, err := os.Stat(filepath.Join(outside, "new")); !os.IsNotExist(err) {
		t.Fatalf("outside parent Stat() error = %v, want not exist", err)
	}
	if content, err := os.ReadFile(outsideFile); err != nil || string(content) != "original" {
		t.Fatalf("outside file content = %q, error = %v, want original", string(content), err)
	}
}

func TestFileSaveReturnsExecutionErrorWhenContextCanceled(t *testing.T) {
	saveFile, err := NewFileSave(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = saveFile.Execute(ctx, json.RawMessage(`{"path":"note.txt","content":"x"}`))
	if !IsExecutionError(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want canceled execution error", err)
	}
}

func TestFileSaveRejectsInvalidRoot(t *testing.T) {
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
			_, err := NewFileSave(tt.root)
			if !IsConfigurationError(err) {
				t.Fatalf("NewFileSave() error = %v, want configuration error", err)
			}
		})
	}
}

func TestFileSaveSchemaMatchesToolIdentity(t *testing.T) {
	saveFile, err := NewFileSave(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileSave() error = %v", err)
	}
	schema := saveFile.Schema()

	if saveFile.Name() != "save_file" {
		t.Fatalf("Name() = %q, want save_file", saveFile.Name())
	}
	if schema.Name != saveFile.Name() {
		t.Fatalf("Schema().Name = %q, want %q", schema.Name, saveFile.Name())
	}
	if schema.Description == "" || saveFile.Description() == "" {
		t.Fatal("description must not be empty")
	}
	if !json.Valid(schema.InputSchema) {
		t.Fatalf("InputSchema is not valid JSON: %s", schema.InputSchema)
	}
}
