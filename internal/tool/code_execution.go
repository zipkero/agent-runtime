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
	"sync"

	"github.com/zipkero/agent-runtime/internal/message"
)

// CommandProfile 은 code_execute가 실행할 수 있는 명령 allowlist 항목이다.
// Command는 shell을 거치지 않고 직접 실행되며, Args는 profile이 고정으로 붙이는 인자다.
// 호출자가 넘긴 args는 AllowedArgs 또는 AllowRelativeFileArgs 검증을 통과해야 한다.
type CommandProfile struct {
	Name                  string
	Command               string
	Args                  []string
	Env                   []string
	AllowedArgs           []string
	AllowRelativeFileArgs bool
}

type codeExecutionInput struct {
	Profile string   `json:"profile"`
	Args    []string `json:"args,omitempty"`
}

var codeExecutionInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["profile"],
  "properties": {
    "profile": {"type": "string", "description": "실행할 허용 command profile 이름"},
    "args": {
      "type": "array",
      "items": {"type": "string"},
      "description": "profile별 검증을 통과해야 하는 추가 인자"
    }
  }
}`)

// CodeExecution 은 허용된 command profile만 임시 작업 디렉터리에서 실행하는 Tool 구현체다.
type CodeExecution struct {
	base           string
	profiles       map[string]CommandProfile
	maxOutputBytes int
}

// NewCodeExecution 은 base 하위 임시 작업 디렉터리에서 실행할 code_execute tool을 생성한다.
func NewCodeExecution(base string, profiles []CommandProfile, maxOutputBytes int) (*CodeExecution, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, fmt.Errorf("base 경로 정규화 실패: %w", err)
	}
	if maxOutputBytes <= 0 {
		return nil, errors.New("output cap 설정 실패: maxOutputBytes는 1 이상이어야 합니다")
	}

	index := make(map[string]CommandProfile, len(profiles))
	for _, profile := range profiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			return nil, errors.New("profile 설정 실패: name이 필요합니다")
		}
		if strings.TrimSpace(profile.Command) == "" {
			return nil, fmt.Errorf("profile %q 설정 실패: command가 필요합니다", name)
		}
		if _, exists := index[name]; exists {
			return nil, fmt.Errorf("profile %q 설정 실패: 중복 name입니다", name)
		}
		profile.Name = name
		index[name] = profile
	}

	return &CodeExecution{
		base:           abs,
		profiles:       index,
		maxOutputBytes: maxOutputBytes,
	}, nil
}

// Spec 은 LLM에 노출할 code_execute 입력 contract를 반환한다.
func (c *CodeExecution) Spec() message.ToolSpec {
	return message.ToolSpec{
		Name:        "code_execute",
		Description: "허용된 command profile을 제한된 임시 작업 디렉터리에서 실행하고 stdout, stderr, exit status를 반환한다.",
		InputSchema: codeExecutionInputSchema,
	}
}

// Execute 는 profile allowlist와 인자 검증을 통과한 명령만 실행한다.
func (c *CodeExecution) Execute(ctx context.Context, input json.RawMessage) (message.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return message.ToolResult{}, err
	}

	var in codeExecutionInput
	if err := json.Unmarshal(input, &in); err != nil {
		return message.ToolResult{}, fmt.Errorf("입력 파싱 실패: %w", err)
	}
	profileName := strings.TrimSpace(in.Profile)
	if profileName == "" {
		return message.ToolResult{}, errors.New("입력 검증 실패: profile 필드가 필요합니다")
	}

	profile, ok := c.profiles[profileName]
	if !ok {
		return message.ToolResult{}, fmt.Errorf("입력 검증 실패: 허용되지 않은 profile %q", profileName)
	}

	workDir, err := os.MkdirTemp(c.base, "code-exec-*")
	if err != nil {
		return message.ToolResult{}, fmt.Errorf("임시 작업 디렉터리 생성 실패: %w", err)
	}
	defer os.RemoveAll(workDir)

	if err := validateCodeExecutionArgs(workDir, profile, in.Args); err != nil {
		return message.ToolResult{}, err
	}

	limit := newOutputLimiter(c.maxOutputBytes)
	stdout := limit.writer()
	stderr := limit.writer()

	args := append(append([]string{}, profile.Args...), in.Args...)
	cmd := exec.CommandContext(ctx, profile.Command, args...)
	cmd.Dir = workDir
	cmd.Env = append([]string{}, profile.Env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	if err := ctx.Err(); err != nil {
		return message.ToolResult{}, fmt.Errorf("code execution timeout/cancel: %w", err)
	}
	if limit.exceeded() {
		return message.ToolResult{}, fmt.Errorf("출력 제한 초과: maxOutputBytes=%d", c.maxOutputBytes)
	}

	exitStatus, err := commandExitStatus(cmd, runErr)
	if err != nil {
		return message.ToolResult{}, err
	}

	return message.ToolResult{
		Content: formatCodeExecutionResult(profileName, exitStatus, stdout.String(), stderr.String()),
	}, nil
}

func validateCodeExecutionArgs(workDir string, profile CommandProfile, args []string) error {
	if len(args) == 0 {
		return nil
	}

	allowed := make(map[string]struct{}, len(profile.AllowedArgs))
	for _, arg := range profile.AllowedArgs {
		allowed[arg] = struct{}{}
	}

	for _, arg := range args {
		if _, ok := allowed[arg]; ok {
			continue
		}
		if profile.AllowRelativeFileArgs {
			if err := validateRelativeFileArg(workDir, arg); err == nil {
				continue
			}
		}
		return fmt.Errorf("입력 검증 실패: profile %q에서 허용되지 않은 arg %q", profile.Name, arg)
	}
	return nil
}

func validateRelativeFileArg(workDir, arg string) error {
	if strings.TrimSpace(arg) == "" {
		return errors.New("빈 파일 경로 인자")
	}
	if filepath.IsAbs(arg) {
		return fmt.Errorf("절대 파일 경로 인자 %q는 허용되지 않습니다", arg)
	}

	target := filepath.Clean(filepath.Join(workDir, arg))
	if target == workDir || !strings.HasPrefix(target, workDir+string(filepath.Separator)) {
		return fmt.Errorf("파일 경로 인자 %q는 임시 작업 디렉터리 밖입니다", arg)
	}
	return nil
}

func commandExitStatus(cmd *exec.Cmd, runErr error) (int, error) {
	if runErr == nil {
		if cmd.ProcessState != nil {
			return cmd.ProcessState.ExitCode(), nil
		}
		return 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, fmt.Errorf("명령 실행 실패: %w", runErr)
}

func formatCodeExecutionResult(profile string, exitStatus int, stdout, stderr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "profile=%q exit_status=%d", profile, exitStatus)
	fmt.Fprintf(&b, "\nstdout:\n%s", stdout)
	fmt.Fprintf(&b, "\nstderr:\n%s", stderr)
	return b.String()
}

type outputLimiter struct {
	mu       sync.Mutex
	limit    int
	written  int
	overflow bool
}

type outputLimitWriter struct {
	limiter *outputLimiter
	buf     bytes.Buffer
}

func newOutputLimiter(limit int) *outputLimiter {
	return &outputLimiter{limit: limit}
}

func (l *outputLimiter) writer() *outputLimitWriter {
	return &outputLimitWriter{limiter: l}
}

func (l *outputLimiter) exceeded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.overflow
}

func (w *outputLimitWriter) Write(p []byte) (int, error) {
	w.limiter.mu.Lock()
	defer w.limiter.mu.Unlock()

	remaining := w.limiter.limit - w.limiter.written
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		_, _ = w.buf.Write(p[:n])
	}
	w.limiter.written += len(p)
	if w.limiter.written > w.limiter.limit {
		w.limiter.overflow = true
	}
	return len(p), nil
}

func (w *outputLimitWriter) String() string {
	w.limiter.mu.Lock()
	defer w.limiter.mu.Unlock()
	return w.buf.String()
}
