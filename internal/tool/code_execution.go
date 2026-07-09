package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

const defaultCodeExecutionOutputLimit = 64 * 1024

type CodeExecution struct {
	root        string
	executable  string
	outputLimit int
}

func NewCodeExecution(root string) (CodeExecution, error) {
	return newCodeExecution(root, "go", defaultCodeExecutionOutputLimit)
}

func newCodeExecution(root, executable string, outputLimit int) (CodeExecution, error) {
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		return CodeExecution{}, ConfigurationErrorf("root is required")
	}

	absRoot, err := filepath.Abs(trimmedRoot)
	if err != nil {
		return CodeExecution{}, ConfigurationErrorf("invalid root: %v", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return CodeExecution{}, ConfigurationErrorf("invalid root: %v", err)
	}
	if !info.IsDir() {
		return CodeExecution{}, ConfigurationErrorf("root is not a directory")
	}

	trimmedExecutable := strings.TrimSpace(executable)
	if trimmedExecutable == "" {
		return CodeExecution{}, ConfigurationErrorf("executable is required")
	}
	resolvedExecutable, err := exec.LookPath(trimmedExecutable)
	if err != nil {
		return CodeExecution{}, ConfigurationErrorf("go executable not found: %v", err)
	}

	if outputLimit <= 0 {
		outputLimit = defaultCodeExecutionOutputLimit
	}

	return CodeExecution{
		root:        filepath.Clean(absRoot),
		executable:  resolvedExecutable,
		outputLimit: outputLimit,
	}, nil
}

func (CodeExecution) Name() string {
	return "code_execution"
}

func (CodeExecution) Description() string {
	return "Run allowed Go commands under a root directory."
}

func (CodeExecution) Schema() message.ToolSchema {
	return message.ToolSchema{
		Name:        "code_execution",
		Description: "Run allowed Go commands under a root directory.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"args":{"type":"array","items":{"type":"string"},"minItems":1},"stdin":{"type":"string"}},"required":["args"],"additionalProperties":false}`),
	}
}

func (c CodeExecution) Validate(args json.RawMessage) error {
	_, err := decodeCodeExecutionArguments(args)
	return err
}

func (c CodeExecution) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	arguments, err := decodeCodeExecutionArguments(args)
	if err != nil {
		return Result{}, err
	}
	if c.root == "" {
		return Result{}, ConfigurationErrorf("root is required")
	}
	if c.executable == "" {
		return Result{}, ConfigurationErrorf("go executable is required")
	}
	if err := ctx.Err(); err != nil {
		content := codeExecutionContent{ExitCode: -1, TimedOut: errors.Is(err, context.DeadlineExceeded)}
		content.Error = codeExecutionContextMessage(err, content.TimedOut)
		result := resultWithExecutionContent(content)
		return result, codeExecutionError(result.Content, err)
	}

	stdout := &limitedOutput{limit: c.outputLimit}
	stderr := &limitedOutput{limit: c.outputLimit}
	cmd := exec.CommandContext(ctx, c.executable, arguments.Args...)
	cmd.Dir = c.root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = strings.NewReader(arguments.Stdin)

	err = cmd.Run()
	content := codeExecutionContent{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		ExitCode:        commandExitCode(cmd, err),
		TimedOut:        errors.Is(ctx.Err(), context.DeadlineExceeded),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			content.Error = codeExecutionContextMessage(ctxErr, content.TimedOut)
			result := resultWithExecutionContent(content)
			return result, codeExecutionError(result.Content, ctxErr)
		}
		content.Error = fmt.Sprintf("go execution failed with exit code %d", content.ExitCode)
		result := resultWithExecutionContent(content)
		return result, codeExecutionError(result.Content, err)
	}

	result := resultWithExecutionContent(content)
	return result, nil
}

type codeExecutionArguments struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
}

type codeExecutionContent struct {
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	TimedOut        bool   `json:"timed_out"`
	Error           string `json:"error,omitempty"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
}

func decodeCodeExecutionArguments(raw json.RawMessage) (codeExecutionArguments, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var arguments codeExecutionArguments
	if err := decoder.Decode(&arguments); err != nil {
		return codeExecutionArguments{}, ValidationErrorf("invalid JSON: %v", err)
	}
	if len(arguments.Args) == 0 {
		return codeExecutionArguments{}, ValidationErrorf("args is required")
	}
	if err := validateCodeExecutionArgs(arguments.Args); err != nil {
		return codeExecutionArguments{}, err
	}
	return arguments, nil
}

func validateCodeExecutionArgs(args []string) error {
	if strings.TrimSpace(args[0]) == "" {
		return ValidationErrorf("arg must not be empty")
	}
	if !isAllowedGoSubcommand(args[0]) {
		return ValidationErrorf("unsupported go subcommand %q", args[0])
	}
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			return ValidationErrorf("arg must not be empty")
		}
		if isDisallowedGoArg(arg) {
			return ValidationErrorf("arg escapes root or changes working directory")
		}
	}
	if args[0] == "env" {
		for _, arg := range args[1:] {
			if arg == "-w" || strings.HasPrefix(arg, "-w=") || arg == "-u" || strings.HasPrefix(arg, "-u=") {
				return ValidationErrorf("go env mutation is not allowed")
			}
		}
	}
	return nil
}

func isAllowedGoSubcommand(subcommand string) bool {
	switch subcommand {
	case "env", "list", "run", "test", "version":
		return true
	default:
		return false
	}
}

func isDisallowedGoArg(arg string) bool {
	if arg == "-C" || strings.HasPrefix(arg, "-C=") {
		return true
	}
	if filepath.IsAbs(arg) || pathEscapesRoot(arg) {
		return true
	}

	if before, after, ok := strings.Cut(arg, "="); ok && before != "" {
		return filepath.IsAbs(after) || pathEscapesRoot(after)
	}
	return false
}

func pathEscapesRoot(value string) bool {
	clean := filepath.Clean(value)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func commandExitCode(cmd *exec.Cmd, err error) int {
	if cmd != nil && cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func resultWithExecutionContent(content codeExecutionContent) Result {
	encoded, err := json.Marshal(content)
	if err != nil {
		return Result{Content: `{"stdout":"","stderr":"","exit_code":-1,"timed_out":false}`}
	}
	return Result{Content: string(encoded)}
}

func codeExecutionContextMessage(err error, timedOut bool) string {
	message := "go execution canceled: %s"
	if timedOut {
		message = "go execution timed out: %s"
	}
	return fmt.Sprintf(message, err.Error())
}

func codeExecutionError(message string, err error) error {
	return &Error{
		Kind:    ErrorKindExecution,
		Message: message,
		Err:     err,
	}
}

type limitedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (o *limitedOutput) Write(p []byte) (int, error) {
	if o.limit <= 0 {
		return len(p), nil
	}

	remaining := o.limit - o.buffer.Len()
	if remaining <= 0 {
		o.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = o.buffer.Write(p[:remaining])
		o.truncated = true
		return len(p), nil
	}
	_, _ = o.buffer.Write(p)
	return len(p), nil
}

func (o *limitedOutput) String() string {
	return o.buffer.String()
}

func (o *limitedOutput) Truncated() bool {
	return o.truncated
}
