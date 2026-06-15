package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

func fileSaveInput(path, content string, overwrite bool) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"path":      path,
		"content":   content,
		"overwrite": overwrite,
	})
	return raw
}

func TestFileSave_Spec(t *testing.T) {
	base := t.TempDir()
	fs, err := tool.NewFileSave(base)
	if err != nil {
		t.Fatalf("NewFileSave 실패: %v", err)
	}

	spec := fs.Spec()
	if spec.Name != "file_save" {
		t.Errorf("Spec().Name = %q, want %q", spec.Name, "file_save")
	}
	if spec.Description == "" {
		t.Error("Spec().Description이 비어 있습니다")
	}
	if len(spec.InputSchema) == 0 {
		t.Error("Spec().InputSchema가 비어 있습니다")
	}
}

func TestFileSave_SaveSuccess(t *testing.T) {
	base := t.TempDir()
	fs, _ := tool.NewFileSave(base)

	result, err := fs.Execute(context.Background(), fileSaveInput("hello.txt", "hello, file_save", false))
	if err != nil {
		t.Fatalf("예상치 못한 error: %v", err)
	}
	if result.IsError {
		t.Fatalf("IsError=true, Content=%q", result.Content)
	}
	if !strings.Contains(result.Content, `path="hello.txt"`) {
		t.Errorf("Content가 저장 경로를 포함해야 합니다: %q", result.Content)
	}
	if !strings.Contains(result.Content, "bytes=16") {
		t.Errorf("Content가 byte 수를 포함해야 합니다: %q", result.Content)
	}

	data, err := os.ReadFile(filepath.Join(base, "hello.txt"))
	if err != nil {
		t.Fatalf("저장 파일 읽기 실패: %v", err)
	}
	if string(data) != "hello, file_save" {
		t.Errorf("저장 내용 = %q, want %q", string(data), "hello, file_save")
	}
}

func TestFileSave_CreateSubdir(t *testing.T) {
	base := t.TempDir()
	fs, _ := tool.NewFileSave(base)

	_, err := fs.Execute(context.Background(), fileSaveInput("nested/data.txt", "nested content", false))
	if err != nil {
		t.Fatalf("하위 디렉터리 저장 실패: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(base, "nested", "data.txt"))
	if err != nil {
		t.Fatalf("저장 파일 읽기 실패: %v", err)
	}
	if string(data) != "nested content" {
		t.Errorf("저장 내용 = %q, want %q", string(data), "nested content")
	}
}

func TestFileSave_OverwritePolicy(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "same.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("테스트 파일 생성 실패: %v", err)
	}
	fs, _ := tool.NewFileSave(base)

	if _, err := fs.Execute(context.Background(), fileSaveInput("same.txt", "new", false)); err == nil {
		t.Fatal("overwrite=false 기존 파일 저장에서 error를 반환해야 하는데 nil을 반환했습니다")
	}

	result, err := fs.Execute(context.Background(), fileSaveInput("same.txt", "new", true))
	if err != nil {
		t.Fatalf("overwrite=true 저장 실패: %v", err)
	}
	if !strings.Contains(result.Content, "overwrite=true") {
		t.Errorf("Content가 overwrite=true를 포함해야 합니다: %q", result.Content)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("저장 파일 읽기 실패: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("저장 내용 = %q, want %q", string(data), "new")
	}
}

func TestFileSave_RejectedPaths(t *testing.T) {
	base := t.TempDir()
	fs, _ := tool.NewFileSave(base)
	outside := filepath.Join(t.TempDir(), "outside.txt")

	tests := []struct {
		name string
		path string
	}{
		{"empty path", ""},
		{"parent traversal", "../secret.txt"},
		{"nested parent traversal", "sub/../../secret.txt"},
		{"base directory", "."},
		{"absolute outside base", outside},
		{"absolute inside base", filepath.Join(base, "inside.txt")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fs.Execute(context.Background(), fileSaveInput(tt.path, "content", false))
			if err == nil {
				t.Fatalf("경로 %q 가 거부되어야 하는데 error가 nil을 반환했습니다", tt.path)
			}
		})
	}
}

func TestFileSave_DirectoryTargetRejected(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "dir"), 0o755); err != nil {
		t.Fatalf("테스트 디렉터리 생성 실패: %v", err)
	}
	fs, _ := tool.NewFileSave(base)

	_, err := fs.Execute(context.Background(), fileSaveInput("dir", "content", true))
	if err == nil {
		t.Fatal("디렉터리 대상 쓰기에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

func TestFileSave_InvalidInput(t *testing.T) {
	base := t.TempDir()
	fs, _ := tool.NewFileSave(base)

	tests := []struct {
		name  string
		input json.RawMessage
	}{
		{"malformed JSON", json.RawMessage(`not json`)},
		{"missing content", func() json.RawMessage {
			raw, _ := json.Marshal(map[string]any{"path": "data.txt"})
			return raw
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fs.Execute(context.Background(), tt.input)
			if err == nil {
				t.Fatal("잘못된 입력에서 error를 반환해야 하는데 nil을 반환했습니다")
			}
		})
	}
}

func TestFileSave_ViaDispatcher(t *testing.T) {
	base := t.TempDir()
	fs, err := tool.NewFileSave(base)
	if err != nil {
		t.Fatalf("NewFileSave 실패: %v", err)
	}
	d := newDispatcher(time.Second, fs)

	t.Run("성공 경로", func(t *testing.T) {
		call := message.ToolCall{ID: "save-1", Name: "file_save", Input: fileSaveInput("dispatch.txt", "dispatch", false)}
		result := d.Dispatch(context.Background(), call)

		if result.IsError {
			t.Fatalf("IsError=true, Content=%q", result.Content)
		}
		if result.ToolCallID != call.ID {
			t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
		}
	})

	t.Run("traversal은 IsError로 정규화", func(t *testing.T) {
		call := message.ToolCall{ID: "save-2", Name: "file_save", Input: fileSaveInput("../secret.txt", "secret", false)}
		result := d.Dispatch(context.Background(), call)

		if !result.IsError {
			t.Fatal("traversal 경로 결과가 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
		}
		if result.ToolCallID != call.ID {
			t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
		}
	})
}
