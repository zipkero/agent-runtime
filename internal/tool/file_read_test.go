package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// fileReadInput 은 테스트용 file_read 입력 JSON을 만드는 헬퍼다.
func fileReadInput(path string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"path": path})
	return raw
}

// TestFileRead_Spec 은 Spec이 필수 필드를 채워 반환하는지 검증한다.
func TestFileRead_Spec(t *testing.T) {
	base := t.TempDir()
	fr, err := tool.NewFileRead(base)
	if err != nil {
		t.Fatalf("NewFileRead 실패: %v", err)
	}

	spec := fr.Spec()
	if spec.Name != "file_read" {
		t.Errorf("Spec().Name = %q, want %q", spec.Name, "file_read")
	}
	if spec.Description == "" {
		t.Error("Spec().Description이 비어 있습니다")
	}
	if len(spec.InputSchema) == 0 {
		t.Error("Spec().InputSchema가 비어 있습니다")
	}
}

// TestFileRead_ReadSuccess 는 base 하위 파일을 정상적으로 읽는지 검증한다.
func TestFileRead_ReadSuccess(t *testing.T) {
	base := t.TempDir()
	content := "hello, file_read"
	filePath := filepath.Join(base, "hello.txt")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("테스트 파일 생성 실패: %v", err)
	}

	fr, _ := tool.NewFileRead(base)
	result, err := fr.Execute(context.Background(), fileReadInput("hello.txt"))
	if err != nil {
		t.Fatalf("예상치 못한 error: %v", err)
	}
	if result.IsError {
		t.Fatalf("IsError=true, Content=%q", result.Content)
	}
	if result.Content != content {
		t.Errorf("Content = %q, want %q", result.Content, content)
	}
}

// TestFileRead_SubdirSuccess 는 base 하위 서브디렉터리 안의 파일도 읽는지 검증한다.
func TestFileRead_SubdirSuccess(t *testing.T) {
	base := t.TempDir()
	subdir := filepath.Join(base, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("서브디렉터리 생성 실패: %v", err)
	}
	content := "subdir content"
	filePath := filepath.Join(subdir, "data.txt")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("테스트 파일 생성 실패: %v", err)
	}

	fr, _ := tool.NewFileRead(base)
	result, err := fr.Execute(context.Background(), fileReadInput("sub/data.txt"))
	if err != nil {
		t.Fatalf("예상치 못한 error: %v", err)
	}
	if result.Content != content {
		t.Errorf("Content = %q, want %q", result.Content, content)
	}
}

// TestFileRead_TraversalRejected 는 .. traversal 경로가 거부되는지 검증한다.
func TestFileRead_TraversalRejected(t *testing.T) {
	base := t.TempDir()
	fr, _ := tool.NewFileRead(base)

	cases := []string{
		"../secret.txt",
		"sub/../../secret.txt",
		"../",
		"..",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			_, err := fr.Execute(context.Background(), fileReadInput(p))
			if err == nil {
				t.Fatalf("경로 %q 가 거부되어야 하는데 error가 nil을 반환했습니다", p)
			}
		})
	}
}

// TestFileRead_AbsoluteOutsideBaseRejected 는 base 밖 절대경로가 거부되는지 검증한다.
func TestFileRead_AbsoluteOutsideBaseRejected(t *testing.T) {
	base := t.TempDir()
	fr, _ := tool.NewFileRead(base)

	// base와 다른 임시 디렉터리에 파일 생성
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")
	_ = os.WriteFile(outsideFile, []byte("secret"), 0o644)

	_, err := fr.Execute(context.Background(), fileReadInput(outsideFile))
	if err == nil {
		t.Fatalf("base 밖 절대경로 %q 가 거부되어야 하는데 error가 nil을 반환했습니다", outsideFile)
	}
}

// TestFileRead_AbsoluteInsideBaseAllowed 는 base 하위 절대경로는 허용되는지 검증한다.
func TestFileRead_AbsoluteInsideBaseAllowed(t *testing.T) {
	base := t.TempDir()
	content := "absolute path content"
	filePath := filepath.Join(base, "abs.txt")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("테스트 파일 생성 실패: %v", err)
	}

	fr, _ := tool.NewFileRead(base)
	result, err := fr.Execute(context.Background(), fileReadInput(filePath))
	if err != nil {
		t.Fatalf("base 하위 절대경로 읽기 실패: %v", err)
	}
	if result.Content != content {
		t.Errorf("Content = %q, want %q", result.Content, content)
	}
}

// TestFileRead_PrefixSpoofRejected 는 base와 prefix만 같은 위장 경로가 거부되는지 검증한다.
// 예) base=/tmp/foo → /tmp/foobar 는 prefix가 같아 보이지만 base 밖이므로 거부해야 한다.
func TestFileRead_PrefixSpoofRejected(t *testing.T) {
	base := t.TempDir()

	// base 경로 뒤에 "_evil"을 붙여 위장 디렉터리를 구성한다.
	spoofDir := base + "_evil"
	if err := os.MkdirAll(spoofDir, 0o755); err != nil {
		t.Skipf("위장 디렉터리 생성 실패 (OS 제한): %v", err)
	}
	defer os.RemoveAll(spoofDir)

	spoofFile := filepath.Join(spoofDir, "spoof.txt")
	_ = os.WriteFile(spoofFile, []byte("spoof"), 0o644)

	fr, _ := tool.NewFileRead(base)
	_, err := fr.Execute(context.Background(), fileReadInput(spoofFile))
	if err == nil {
		t.Fatalf("위장 경로 %q 가 거부되어야 하는데 error가 nil을 반환했습니다", spoofFile)
	}
}

// TestFileRead_MissingFile 은 존재하지 않는 파일이 error를 반환하는지 검증한다.
func TestFileRead_MissingFile(t *testing.T) {
	base := t.TempDir()
	fr, _ := tool.NewFileRead(base)

	_, err := fr.Execute(context.Background(), fileReadInput("nonexistent.txt"))
	if err == nil {
		t.Fatal("존재하지 않는 파일에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

// TestFileRead_MalformedJSON 은 JSON 파싱 불가 입력이 error를 반환하는지 검증한다.
func TestFileRead_MalformedJSON(t *testing.T) {
	base := t.TempDir()
	fr, _ := tool.NewFileRead(base)

	_, err := fr.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("malformed JSON에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

// TestFileRead_MissingPath 는 path 필드가 빈 문자열인 입력이 error를 반환하는지 검증한다.
func TestFileRead_MissingPath(t *testing.T) {
	base := t.TempDir()
	fr, _ := tool.NewFileRead(base)

	raw, _ := json.Marshal(map[string]any{"path": ""})
	_, err := fr.Execute(context.Background(), raw)
	if err == nil {
		t.Fatal("빈 path 필드에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

// TestFileRead_ViaDispatcher 는 Dispatcher를 통해 file_read가 IsError 정규화를 올바르게 거치는지 검증한다.
func TestFileRead_ViaDispatcher(t *testing.T) {
	base := t.TempDir()
	content := "dispatcher test"
	filePath := filepath.Join(base, "dispatch.txt")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("테스트 파일 생성 실패: %v", err)
	}

	fr, err := tool.NewFileRead(base)
	if err != nil {
		t.Fatalf("NewFileRead 실패: %v", err)
	}

	d := newDispatcher(time.Second, fr)

	t.Run("성공 경로", func(t *testing.T) {
		call := message.ToolCall{ID: "fr-1", Name: "file_read", Input: fileReadInput("dispatch.txt")}
		result := d.Dispatch(context.Background(), call)

		if result.IsError {
			t.Fatalf("IsError=true, Content=%q", result.Content)
		}
		if result.ToolCallID != call.ID {
			t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
		}
		if result.Content != content {
			t.Errorf("Content = %q, want %q", result.Content, content)
		}
	})

	t.Run(".. traversal은 IsError로 정규화", func(t *testing.T) {
		call := message.ToolCall{ID: "fr-2", Name: "file_read", Input: fileReadInput("../secret.txt")}
		result := d.Dispatch(context.Background(), call)

		if !result.IsError {
			t.Fatal("traversal 경로 결과가 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
		}
		if result.ToolCallID != call.ID {
			t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
		}
	})

	t.Run("부재 파일은 IsError로 정규화", func(t *testing.T) {
		call := message.ToolCall{ID: "fr-3", Name: "file_read", Input: fileReadInput("no_such_file.txt")}
		result := d.Dispatch(context.Background(), call)

		if !result.IsError {
			t.Fatal("부재 파일 결과가 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
		}
	})
}
