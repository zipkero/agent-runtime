package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// RunnerStatus 는 Runner 실행 결과를 호출자 관점의 상태로 분류한다.
type RunnerStatus string

const (
	// RunnerStatusSuccess 는 Agent가 최종 assistant 응답까지 도달했음을 나타낸다.
	RunnerStatusSuccess RunnerStatus = "success"
	// RunnerStatusMaxSteps 는 Agent가 최종 응답 전에 step 상한에 도달했음을 나타낸다.
	RunnerStatusMaxSteps RunnerStatus = "max_steps"
	// RunnerStatusAgentError 는 LLM 호출, context 취소, graph 구성 오류 같은 Agent 실행 실패를 나타낸다.
	RunnerStatusAgentError RunnerStatus = "agent_error"
)

var (
	// ErrRunnerClientRequired 는 Runner가 실행에 필요한 LLM client 없이 생성될 때 반환된다.
	ErrRunnerClientRequired = errors.New("agent runner client is required")
)

// RunnerConfig 는 한 번의 Single Agent 실행 표면을 구성하는 값이다.
type RunnerConfig struct {
	Client      llm.LLMClient
	Model       string
	MaxSteps    int
	Registry    *tool.Registry
	ToolTimeout time.Duration
	Middleware  []Middleware
	Hook        ReflectionHook
}

// Runner 는 provider-neutral LLM client와 tool registry를 조합해 Single Agent를 실행한다.
type Runner struct {
	client      llm.LLMClient
	model       string
	maxSteps    int
	registry    *tool.Registry
	toolTimeout time.Duration
	middleware  []Middleware
	hook        ReflectionHook
}

// RunnerResult 는 Runner 실행 후 호출자가 확인할 수 있는 최종 결과 표면이다.
type RunnerResult struct {
	State        AgentState
	FinalMessage message.Message
	FinalText    string
	Status       RunnerStatus
	Err          error
}

// NewRunner 는 실행 가능한 Runner를 생성한다.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	if cfg.Client == nil {
		return nil, ErrRunnerClientRequired
	}
	return &Runner{
		client:      cfg.Client,
		model:       cfg.Model,
		maxSteps:    cfg.MaxSteps,
		registry:    cfg.Registry,
		toolTimeout: cfg.ToolTimeout,
		middleware:  append([]Middleware(nil), cfg.Middleware...),
		hook:        cfg.Hook,
	}, nil
}

// Run 은 기존 Agent graph loop를 실행하고 호출자 친화적인 RunnerResult로 매핑한다.
func (r *Runner) Run(ctx context.Context, prompt string) RunnerResult {
	a := NewAgentWithOptions(r.client, r.model, r.maxSteps, r.registry, r.toolTimeout, AgentOptions{
		Hook:       r.hook,
		Middleware: r.middleware,
	})
	state := a.Run(ctx, prompt)

	result := RunnerResult{State: state}
	switch state.Status {
	case StatusFinal:
		finalMsg, ok := state.FinalMessage()
		if !ok {
			result.Status = RunnerStatusAgentError
			result.Err = fmt.Errorf("agent final status without assistant message")
			return result
		}
		result.Status = RunnerStatusSuccess
		result.FinalMessage = finalMsg
		result.FinalText = textFromMessage(finalMsg)
		return result
	case StatusMaxSteps:
		result.Status = RunnerStatusMaxSteps
		return result
	case StatusError:
		result.Status = RunnerStatusAgentError
		result.Err = state.Err
		return result
	default:
		result.Status = RunnerStatusAgentError
		result.Err = fmt.Errorf("agent stopped with unexpected status %q", state.Status)
		return result
	}
}

func textFromMessage(msg message.Message) string {
	var b strings.Builder
	for _, block := range msg.Content {
		if block.Type == message.BlockTypeText {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}
