package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

type fileSaveInput struct {
	Path      string  `json:"path"`
	Content   *string `json:"content"`
	Overwrite bool    `json:"overwrite,omitempty"`
}

var fileSaveInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["path", "content"],
  "properties": {
    "path": {"type": "string", "description": "저장할 파일의 base 디렉터리 기준 상대경로"},
    "content": {"type": "string", "description": "파일에 저장할 문자열 내용"},
    "overwrite": {"type": "boolean", "description": "기존 파일 덮어쓰기 허용 여부"}
  }
}`)

// FileSave 는 허용된 base 디렉터리 하위에만 문자열 content를 저장하는 Tool 구현체다.
// 입력 경로는 base 기준 상대경로만 허용하며, 기존 파일은 overwrite=true일 때만 덮어쓴다.
type FileSave struct {
	base string
}

// NewFileSave 는 base 디렉터리를 고정한 FileSave를 생성한다.
func NewFileSave(base string) (*FileSave, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, fmt.Errorf("base 경로 정규화 실패: %w", err)
	}
	return &FileSave{base: abs}, nil
}

// Spec 은 LLM에 노출할 file_save 입력 contract를 반환한다.
func (f *FileSave) Spec() message.ToolSpec {
	return message.ToolSpec{
		Name:        "file_save",
		Description: "허용된 base 디렉터리 하위 상대경로에 문자열 content를 저장한다. base 밖 경로와 명시되지 않은 덮어쓰기는 거부한다.",
		InputSchema: fileSaveInputSchema,
	}
}

// Execute 는 입력을 검증하고, 안전한 대상 경로에 content를 저장한다.
func (f *FileSave) Execute(ctx context.Context, input json.RawMessage) (message.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return message.ToolResult{}, err
	}

	var in fileSaveInput
	if err := json.Unmarshal(input, &in); err != nil {
		return message.ToolResult{}, fmt.Errorf("입력 파싱 실패: %w", err)
	}
	if strings.TrimSpace(in.Path) == "" {
		return message.ToolResult{}, errors.New("입력 검증 실패: path 필드가 필요합니다")
	}
	if in.Content == nil {
		return message.ToolResult{}, errors.New("입력 검증 실패: content 필드가 필요합니다")
	}

	target, rel, err := f.resolvePath(in.Path)
	if err != nil {
		return message.ToolResult{}, err
	}

	if err := ctx.Err(); err != nil {
		return message.ToolResult{}, err
	}
	if err := f.writeFile(target, []byte(*in.Content), in.Overwrite); err != nil {
		return message.ToolResult{}, err
	}

	return message.ToolResult{
		Content: fmt.Sprintf("saved path=%q bytes=%d overwrite=%t", filepath.ToSlash(rel), len([]byte(*in.Content)), in.Overwrite),
	}, nil
}

func (f *FileSave) resolvePath(path string) (string, string, error) {
	if filepath.IsAbs(path) {
		return "", "", fmt.Errorf("경로 거부: 절대경로 %q는 허용되지 않습니다", path)
	}

	abs := filepath.Clean(filepath.Join(f.base, path))
	if abs == f.base || !strings.HasPrefix(abs, f.base+string(filepath.Separator)) {
		return "", "", fmt.Errorf("경로 이탈: %q는 허용 범위 밖입니다", path)
	}

	rel, err := filepath.Rel(f.base, abs)
	if err != nil {
		return "", "", fmt.Errorf("상대 경로 계산 실패: %w", err)
	}
	return abs, rel, nil
}

func (f *FileSave) writeFile(target string, data []byte, overwrite bool) error {
	if info, err := os.Stat(target); err == nil {
		if info.IsDir() {
			return fmt.Errorf("파일 저장 실패: %q는 디렉터리입니다", target)
		}
		if !overwrite {
			return fmt.Errorf("파일 저장 실패: %q가 이미 존재합니다. overwrite=true가 필요합니다", target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("파일 상태 확인 실패: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("상위 디렉터리 생성 실패: %w", err)
	}

	flag := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flag |= os.O_TRUNC
	} else {
		flag |= os.O_EXCL
	}
	file, err := os.OpenFile(target, flag, 0o644)
	if err != nil {
		return fmt.Errorf("파일 열기 실패: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("파일 쓰기 실패: %w", err)
	}
	return nil
}
