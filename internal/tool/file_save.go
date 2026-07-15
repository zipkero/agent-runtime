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

type FileSave struct {
	root string
}

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

	info, err := os.Lstat(target)
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
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Result{}, ExecutionErrorf("create parent directory failed: %v", err)
	}
	if err := ensureNoSymlinkPath(f.root, parent); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, canceledExecutionError("save file", err)
	}

	contentBytes := []byte(*arguments.Content)
	err = os.WriteFile(target, contentBytes, 0o644)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Result{}, canceledExecutionError("save file", ctxErr)
	}
	if err != nil {
		return Result{}, ExecutionErrorf("write file failed: %v", err)
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

	cleanPath := filepath.Clean(inputPath)
	target, err := filepath.Abs(filepath.Join(f.root, cleanPath))
	if err != nil {
		return "", fileSaveArguments{}, ValidationErrorf("invalid path: %v", err)
	}
	if !isPathInsideRoot(f.root, target) {
		return "", fileSaveArguments{}, ValidationErrorf("path escapes root")
	}

	arguments.Path = cleanPath
	return target, arguments, nil
}

func ensureNoSymlinkPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return ExecutionErrorf("inspect parent directory failed: %v", err)
	}
	if rel == "." {
		return nil
	}

	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return ExecutionErrorf("inspect parent directory failed: %v", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ExecutionErrorf("parent directory contains a symlink")
		}
		if !info.IsDir() {
			return ExecutionErrorf("parent path is not a directory")
		}
	}
	return nil
}
