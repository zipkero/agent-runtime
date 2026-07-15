package agent

import (
	"context"
	"errors"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// RunnerOptions 는 Single Agent run에 필요한 provider-neutral 의존성과 실행 제한이다.
type RunnerOptions struct {
	Client             llm.LLMClient
	Model              string
	MaxSteps           int
	ModelTimeout       time.Duration
	Tools              *tool.Registry
	ToolTimeout        time.Duration
	MaxToolCalls       int
	MaxToolResultBytes int
	Middleware         []ModelMiddleware
}

// RunnerResult 는 Runner가 실행한 Agent의 최종 상태와 결과를 보존한다.
type RunnerResult struct {
	State AgentState
}

// Runner 는 실행 의존성을 조립하고 기존 Agent loop를 호출하는 상위 실행 경계다.
type Runner struct {
	agent *Agent
}

// NewRunner 는 주입된 실행 옵션으로 재사용 가능한 Single Agent Runner를 생성한다.
func NewRunner(opts RunnerOptions) (*Runner, error) {
	if opts.Client == nil {
		return nil, errors.New("agent runner client is required")
	}

	agent, err := newAgent(
		Options{
			Client:             opts.Client,
			Model:              opts.Model,
			MaxSteps:           opts.MaxSteps,
			Tools:              opts.Tools,
			ToolTimeout:        opts.ToolTimeout,
			MaxToolCalls:       opts.MaxToolCalls,
			MaxToolResultBytes: opts.MaxToolResultBytes,
		},
		modelCallOptions{
			timeout:    opts.ModelTimeout,
			middleware: opts.Middleware,
		},
	)
	if err != nil {
		return nil, err
	}

	return &Runner{agent: agent}, nil
}

// Run 은 사용자 입력 하나를 기존 Agent loop로 실행한다.
func (r *Runner) Run(ctx context.Context, input string) RunnerResult {
	return RunnerResult{State: r.agent.Run(ctx, input)}
}
