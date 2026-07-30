package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestFileSaveCreatesRootFile 은 루트 아래에 파일을 만들고 저장 경로, 바이트 수, 덮어쓰기 여부를 JSON 결과로 돌려주는지 확인한다.
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

// TestFileSaveCreatesParentDirectories 는 루트 안에 없는 상위 디렉터리를 만들어 저장을 이어가는지 확인한다.
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

// TestFileSaveAllowsEmptyContent 는 빈 문자열 content를 필드 누락으로 보지 않고 빈 파일로 저장하는지 확인한다.
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

// TestFileSaveOverwritesOnlyWhenAllowed 는 overwrite가 false면 기존 내용을 그대로 두고 실행 오류로 끊고,
// true면 덮어쓴 뒤 결과에 그 사실을 남기는지 확인한다.
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

// TestFileSaveRejectsInvalidArguments 는 JSON 오류, path·content 누락과 타입 불일치, 상위 경로 탈출, 절대 경로를 검증 단계에서 거절하는지 확인한다.
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

// TestFileSaveReturnsExecutionErrors 는 대상이 디렉터리이거나 상위 경로가 파일인 경우를 실행 오류로 구분하는지 확인한다.
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

// TestFileSaveRejectsSymlinksOutsideRootWithoutSideEffects 는 루트 밖을 가리키는 심볼릭 링크 경로를 거절하면서
// 루트 밖에 디렉터리를 만들거나 기존 파일을 바꾸지 않는지 확인한다.
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

// TestFileSaveReturnsExecutionErrorWhenContextCanceled 은 취소된 ctx에서 파일을 만들지 않고 취소 원인을 보존한 실행 오류를 반환하는지 확인한다.
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

// TestFileSaveRejectsInvalidRoot 는 빈 루트, 없는 경로, 디렉터리가 아닌 경로를 Tool 생성 시점에 설정 오류로 거절하는지 확인한다.
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

// TestFileSaveSchemaMatchesToolIdentity 는 Tool 이름과 schema의 이름·설명·InputSchema가 서로 어긋나지 않는지 확인한다.
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
