package tool

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

// FileRead 구조체는 허용된 루트 아래의 일반 파일만 읽는 Tool이다.
// 절대 경로와 실제 파일시스템에서 루트 밖을 가리키는 심볼릭 링크를 거부하며 결과는 DefaultMaxResultBytes를 넘지 않는다.
type FileRead struct {
	root string
}

// NewFileRead 함수는 존재하는 디렉터리를 읽기 루트로 고정한 FileRead를 만든다.
func NewFileRead(root string) (FileRead, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return FileRead{}, ConfigurationErrorf("root is required")
	}

	absRoot, err := filepath.Abs(trimmed)
	if err != nil {
		return FileRead{}, ConfigurationErrorf("invalid root: %v", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return FileRead{}, ConfigurationErrorf("invalid root: %v", err)
	}
	if !info.IsDir() {
		return FileRead{}, ConfigurationErrorf("root is not a directory")
	}

	return FileRead{root: filepath.Clean(absRoot)}, nil
}

func (FileRead) Name() string {
	return "read_file"
}

func (FileRead) Description() string {
	return "Read a file under an allowed root directory."
}

func (FileRead) Schema() message.ToolSchema {
	return message.ToolSchema{
		Name:        "read_file",
		Description: "Read a file under an allowed root directory.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
	}
}

func (f FileRead) Validate(args json.RawMessage) error {
	_, err := f.resolvePath(args)
	return err
}

func (f FileRead) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	path, err := f.resolvePath(args)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, canceledExecutionError("read file", err)
	}

	root, err := os.OpenRoot(f.root)
	if err != nil {
		return Result{}, ExecutionErrorf("open read root failed: %v", err)
	}
	defer root.Close()

	file, err := root.Open(path)
	if err != nil {
		return Result{}, ExecutionErrorf("read file failed: %v", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Result{}, ExecutionErrorf("inspect file failed: %v", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, ExecutionErrorf("path is not a regular file")
	}

	// 상한보다 한 바이트만 더 읽어 초과 여부를 판정하므로 큰 파일 전체를 메모리에 올리지 않는다.
	content, err := io.ReadAll(io.LimitReader(file, DefaultMaxResultBytes+1))
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Result{}, canceledExecutionError("read file", ctxErr)
	}
	if err != nil {
		return Result{}, ExecutionErrorf("read file failed: %v", err)
	}
	if len(content) > DefaultMaxResultBytes {
		return Result{}, ExecutionErrorf("read file result exceeds %d byte limit", DefaultMaxResultBytes)
	}

	return Result{Content: string(content)}, nil
}

type fileReadArguments struct {
	Path string `json:"path"`
}

func (f FileRead) resolvePath(raw json.RawMessage) (string, error) {
	if f.root == "" {
		return "", ConfigurationErrorf("root is required")
	}

	var arguments fileReadArguments
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return "", ValidationErrorf("invalid JSON: %v", err)
	}

	inputPath := strings.TrimSpace(arguments.Path)
	if inputPath == "" {
		return "", ValidationErrorf("path is required")
	}
	if filepath.IsAbs(inputPath) {
		return "", ValidationErrorf("absolute path is not allowed")
	}

	if !filepath.IsLocal(inputPath) {
		return "", ValidationErrorf("path escapes root")
	}

	return filepath.Clean(inputPath), nil
}
