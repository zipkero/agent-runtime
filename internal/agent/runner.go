package agent

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// RunnerOptions 구조체는 단일 Agent 실행에 필요한 공급자 중립 의존성과 실행 제한이다.
// ModelTimeout 값이 0이면 공급자 호출에 별도 제한 시간을 추가하지 않고, 나머지 기본값은 Options 구조체와 동일하다.
// Timeout과 Tool 호출·result 제한은 음수를 허용하지 않는다.
// OutputSchema가 nil이 아니면 Runner 생성 시 self-contained JSON Schema로 compile하며 빈 값과 외부 참조를 거부한다.
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
	OutputSchema       json.RawMessage
}

// RunnerResult 구조체는 Runner가 실행한 Agent의 최종 상태와 결과를 보존한다.
// StructuredOutput은 지정된 schema를 만족한 최종 JSON의 앞뒤 공백을 제거한 원문이다.
type RunnerResult struct {
	State            AgentState
	StructuredOutput json.RawMessage
}

// Runner 구조체는 실행 의존성을 조립하고 Agent 반복 흐름을 호출하는 상위 실행 경계다.
type Runner struct {
	agent                     *Agent
	structuredOutputValidator *structuredOutputValidator
}

// NewRunner 함수는 주입된 실행 옵션으로 재사용 가능한 단일 Agent Runner를 생성한다.
func NewRunner(opts RunnerOptions) (*Runner, error) {
	if opts.Client == nil {
		return nil, errors.New("agent runner client is required")
	}
	var validator *structuredOutputValidator
	if opts.OutputSchema != nil {
		var err error
		validator, err = newStructuredOutputValidator(opts.OutputSchema)
		if err != nil {
			return nil, err
		}
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

	return &Runner{
		agent:                     agent,
		structuredOutputValidator: validator,
	}, nil
}

// Run 메서드는 사용자 입력 하나를 Agent 반복 흐름으로 실행한다.
func (r *Runner) Run(ctx context.Context, input string) RunnerResult {
	result := RunnerResult{State: r.agent.Run(ctx, input)}
	if r.structuredOutputValidator == nil || result.State.Status != StatusFinal {
		return result
	}

	output, err := r.structuredOutputValidator.Validate(result.State.FinalAnswer)
	if err != nil {
		result.State.stopStructuredOutput(err)
		return result
	}
	result.StructuredOutput = output
	return result
}

// RunnerStreamEventKind 타입은 RunStream이 내보내는 event 종류를 구분한다.
type RunnerStreamEventKind string

const (
	// RunnerStreamEventTextDelta 상수는 생성 중인 model text 조각을 나타낸다.
	RunnerStreamEventTextDelta RunnerStreamEventKind = "text_delta"
	// RunnerStreamEventFinal 상수는 Agent가 StatusFinal로 끝난 성공 결과를 나타낸다.
	RunnerStreamEventFinal RunnerStreamEventKind = "final"
	// RunnerStreamEventError 상수는 최종 오류 결과를 나타낸다.
	RunnerStreamEventError RunnerStreamEventKind = "error"
)

// RunnerStreamEvent 구조체는 RunStream이 내보내는 event 하나를 나타낸다.
// Kind가 RunnerStreamEventTextDelta이면 Step과 TextDelta만 채워지고, RunnerStreamEventFinal 또는
// RunnerStreamEventError이면 Result만 채워진다.
type RunnerStreamEvent struct {
	Kind      RunnerStreamEventKind
	Step      int
	TextDelta string
	Result    *RunnerResult
}

// RunStream 메서드는 사용자 입력 하나를 streaming Agent 반복 흐름으로 실행한다. Iterator는 생성 순서대로 0개
// 이상의 text delta event를 내보낸 뒤 정확히 한 번의 final 또는 error event로 끝난다. Client가
// llm.StreamingLLMClient를 구현하지 않으면 공급자 호출이나 Tool 실행 없이 error event 한 번으로 끝난다. Consumer가
// 순회를 중단하면 그 시점 이후 producer는 provider 요청과 Tool 실행을 포함해 추가 event를 만들지 않는다.
func (r *Runner) RunStream(ctx context.Context, input string) iter.Seq[RunnerStreamEvent] {
	return func(yield func(RunnerStreamEvent) bool) {
		streamingClient, ok := r.agent.client.(llm.StreamingLLMClient)
		if !ok {
			state := AgentState{Status: StatusError, LastError: unsupportedStreamError()}
			yield(RunnerStreamEvent{Kind: RunnerStreamEventError, Result: &RunnerResult{State: state}})
			return
		}

		onText := func(step int, textDelta string) bool {
			return yield(RunnerStreamEvent{Kind: RunnerStreamEventTextDelta, Step: step, TextDelta: textDelta})
		}

		state, aborted := r.agent.runStream(ctx, input, streamingClient, onText)
		if aborted {
			return
		}

		result := RunnerResult{State: state}
		if state.Status == StatusFinal {
			yield(RunnerStreamEvent{Kind: RunnerStreamEventFinal, Result: &result})
			return
		}
		yield(RunnerStreamEvent{Kind: RunnerStreamEventError, Result: &result})
	}
}
