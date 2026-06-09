package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

// fileReadInput 은 file_read tool이 받는 입력 구조다.
// path: 읽을 파일의 경로 (base 디렉터리 기준 상대경로 또는 base 하위 절대경로).
type fileReadInput struct {
	Path string `json:"path"`
}

// fileReadInputSchema 는 LLM에 전달하는 입력 파라미터 JSON Schema다.
// 런타임 검증에는 쓰이지 않으며, LLM이 올바른 형태로 tool_call을 생성하도록 안내하는 용도다.
var fileReadInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["path"],
  "properties": {
    "path": {"type": "string", "description": "읽을 파일의 경로 (base 디렉터리 기준)"}
  }
}`)

// FileRead 는 허용된 base 디렉터리 하위 파일만 읽는 Tool 구현체다.
// base 디렉터리는 생성 시 고정되며, Execute 호출마다 경로 이탈 여부를 검사한다.
// base 밖 경로(.. traversal, base prefix 이탈, base 밖 절대경로)와 부재 파일은 error를 반환한다.
type FileRead struct {
	base string // 허용 범위의 절대 경로 (filepath.Abs로 정규화됨)
}

// NewFileRead 는 base 디렉터리를 고정한 FileRead를 생성한다.
// base는 절대 경로로 정규화되며, 정규화 실패 시 error를 반환한다.
func NewFileRead(base string) (*FileRead, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, fmt.Errorf("base 경로 정규화 실패: %w", err)
	}
	return &FileRead{base: abs}, nil
}

// Spec 은 tool 이름·설명·입력 schema를 반환한다.
func (f *FileRead) Spec() message.ToolSpec {
	return message.ToolSpec{
		Name:        "file_read",
		Description: "허용된 base 디렉터리 하위의 파일 내용을 반환한다. base 밖 경로나 존재하지 않는 파일은 거부한다.",
		InputSchema: fileReadInputSchema,
	}
}

// Execute 는 입력을 unmarshal·검증한 뒤 파일 내용을 ToolResult로 반환한다.
// 진입 직후 입력 검증 → 경로 이탈 검사 → 파일 읽기 순서로 실행하며,
// 각 단계 실패 시 본체를 실행하지 않고 즉시 error를 반환한다.
func (f *FileRead) Execute(_ context.Context, input json.RawMessage) (message.ToolResult, error) {
	// 입력 unmarshal — JSON 형식 불량이면 즉시 error 반환 (본체 미실행)
	var in fileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return message.ToolResult{}, fmt.Errorf("입력 파싱 실패: %w", err)
	}

	// 필드 검증 — path가 비어 있으면 malformed 입력으로 판단
	if in.Path == "" {
		return message.ToolResult{}, fmt.Errorf("입력 검증 실패: path 필드가 필요합니다")
	}

	// 경로 이탈 검사 — base 밖 접근(.. traversal, 절대경로 이탈, base prefix 위장)을 거부한다
	target, err := f.resolvePath(in.Path)
	if err != nil {
		return message.ToolResult{}, err
	}

	// 파일 읽기 — 부재 파일을 포함한 읽기 실패는 error 반환
	data, err := os.ReadFile(target)
	if err != nil {
		return message.ToolResult{}, fmt.Errorf("파일 읽기 실패: %w", err)
	}

	return message.ToolResult{Content: string(data)}, nil
}

// resolvePath 는 입력 경로를 base 기준으로 정규화하고 base 하위인지 검증한다.
// 절대경로가 입력되면 base 하위 여부를 직접 확인하며,
// 상대경로는 base에 join한 뒤 filepath.Clean으로 정규화한다.
// base를 이탈하면 error를 반환한다.
func (f *FileRead) resolvePath(path string) (string, error) {
	var abs string
	if filepath.IsAbs(path) {
		// 절대경로 입력: Clean으로 .. 제거 후 base prefix 검사
		abs = filepath.Clean(path)
	} else {
		// 상대경로 입력: base에 join한 뒤 Clean으로 정규화
		abs = filepath.Clean(filepath.Join(f.base, path))
	}

	// base 하위인지 검사: base + 경로 구분자 prefix를 요구해 위장 경로를 차단한다.
	// 예) base=/tmp/foo, target=/tmp/foobar → prefix 불일치로 거부
	if abs != f.base && !strings.HasPrefix(abs, f.base+string(filepath.Separator)) {
		return "", fmt.Errorf("경로 이탈: %q는 허용 범위 밖입니다", path)
	}

	return abs, nil
}
