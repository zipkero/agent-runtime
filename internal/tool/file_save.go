package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

// FileSave 구조체는 허용된 루트 아래에 파일을 저장하는 Tool이다.
// 필요한 상위 디렉터리는 만들지만, 실제 파일시스템에서 루트 밖을 가리키는 심볼릭 링크는 거부한다.
// 기존 파일은 overwrite 인수가 true일 때만 덮어쓴다.
type FileSave struct {
	root string
}

// NewFileSave 함수는 존재하는 디렉터리를 쓰기 루트로 고정한 FileSave를 만든다.
func NewFileSave(root string) (FileSave, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return FileSave{}, ConfigurationErrorf("root is required")
	}

	absRoot, err := filepath.Abs(trimmed)
	if err != nil {
		return FileSave{}, ConfigurationErrorf("invalid root: %v", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return FileSave{}, ConfigurationErrorf("invalid root: %v", err)
	}
	if !info.IsDir() {
		return FileSave{}, ConfigurationErrorf("root is not a directory")
	}

	return FileSave{root: filepath.Clean(absRoot)}, nil
}

func (FileSave) Name() string {
	return "save_file"
}

func (FileSave) Description() string {
	return "Save content to a file under an allowed root directory."
}

func (FileSave) Schema() message.ToolSchema {
	return message.ToolSchema{
		Name:        "save_file",
		Description: "Save content to a file under an allowed root directory.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"overwrite":{"type":"boolean"}},"required":["path","content"],"additionalProperties":false}`),
	}
}

func (f FileSave) Validate(args json.RawMessage) error {
	_, _, err := f.resolveSavePath(args)
	return err
}

func (f FileSave) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	target, arguments, err := f.resolveSavePath(args)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, canceledExecutionError("save file", err)
	}

	root, err := os.OpenRoot(f.root)
	if err != nil {
		return Result{}, ExecutionErrorf("open save root failed: %v", err)
	}
	defer root.Close()

	info, err := root.Stat(target)
	overwritten := false
	if err == nil {
		if !info.Mode().IsRegular() {
			return Result{}, ExecutionErrorf("path is not a regular file")
		}
		if !arguments.Overwrite {
			return Result{}, ExecutionErrorf("file already exists and overwrite is false")
		}
		overwritten = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, ExecutionErrorf("inspect file failed: %v", err)
	}

	parent := filepath.Dir(target)
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return Result{}, ExecutionErrorf("create parent directory failed: %v", err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, canceledExecutionError("save file", err)
	}

	contentBytes := []byte(*arguments.Content)
	flags := os.O_WRONLY | os.O_CREATE
	if arguments.Overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := root.OpenFile(target, flags, 0o644)
	if err != nil {
		return Result{}, ExecutionErrorf("write file failed: %v", err)
	}
	_, writeErr := file.Write(contentBytes)
	closeErr := file.Close()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Result{}, canceledExecutionError("save file", ctxErr)
	}
	if writeErr != nil {
		return Result{}, ExecutionErrorf("write file failed: %v", writeErr)
	}
	if closeErr != nil {
		return Result{}, ExecutionErrorf("close file failed: %v", closeErr)
	}

	content := fileSaveContent{
		Path:        arguments.Path,
		Bytes:       len(contentBytes),
		Overwritten: overwritten,
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return Result{}, ExecutionErrorf("encode save result: %v", err)
	}
	return Result{Content: string(encoded)}, nil
}

type fileSaveArguments struct {
	Path      string  `json:"path"`
	Content   *string `json:"content"`
	Overwrite bool    `json:"overwrite"`
}

type fileSaveContent struct {
	Path        string `json:"path"`
	Bytes       int    `json:"bytes"`
	Overwritten bool   `json:"overwritten"`
}

func (f FileSave) resolveSavePath(raw json.RawMessage) (string, fileSaveArguments, error) {
	if f.root == "" {
		return "", fileSaveArguments{}, ConfigurationErrorf("root is required")
	}

	var arguments fileSaveArguments
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return "", fileSaveArguments{}, ValidationErrorf("invalid JSON: %v", err)
	}

	inputPath := strings.TrimSpace(arguments.Path)
	if inputPath == "" {
		return "", fileSaveArguments{}, ValidationErrorf("path is required")
	}
	if filepath.IsAbs(inputPath) {
		return "", fileSaveArguments{}, ValidationErrorf("absolute path is not allowed")
	}
	if arguments.Content == nil {
		return "", fileSaveArguments{}, ValidationErrorf("content is required")
	}

	if !filepath.IsLocal(inputPath) {
		return "", fileSaveArguments{}, ValidationErrorf("path escapes root")
	}

	cleanPath := filepath.Clean(inputPath)
	arguments.Path = cleanPath
	return cleanPath, arguments, nil
}
