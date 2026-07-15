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

type FileRead struct {
	root string
}

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

	info, err := os.Lstat(path)
	if err != nil {
		return Result{}, ExecutionErrorf("read file failed: %v", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, ExecutionErrorf("path is not a regular file")
	}

	file, err := os.Open(path)
	if err != nil {
		return Result{}, ExecutionErrorf("read file failed: %v", err)
	}
	defer file.Close()

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

	target, err := filepath.Abs(filepath.Join(f.root, filepath.Clean(inputPath)))
	if err != nil {
		return "", ValidationErrorf("invalid path: %v", err)
	}

	if !isPathInsideRoot(f.root, target) {
		return "", ValidationErrorf("path escapes root")
	}

	return target, nil
}

func isPathInsideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
