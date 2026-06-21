// Package agent 는 ReAct loop와 그 실행 상태를 정의한다.
// llm.LLMClient(interface)와 message 타입에만 의존하며, provider 구현체에는 의존하지 않는다.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/zipkero/agent-runtime/internal/graph"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// Status 는 AgentState가 놓인 종료 종류를 하나의 명시적 값으로 구분한다.
// running을 제외한 final/max steps/error 세 값이 종료 상태이며,
// loop는 종료 상태에 도달하면 더 이상 LLM을 호출하지 않는다.
type Status string

const (
	// StatusRunning 은 아직 최종 답에 도달하지 않았고 step 여유가 남아 loop가 도는 중인 상태다.
	StatusRunning Status = "running"
	// StatusFinal 은 tool_call이 없는 assistant 응답을 받아 최종 답으로 판정해 종료한 상태다.
	StatusFinal Status = "final"
	// StatusMaxSteps 는 step counter가 상한에 도달했는데도 최종 답에 못 닿아 안전 종료한 상태다.
	StatusMaxSteps Status = "max_steps"
	// StatusError 는 LLM 호출이 에러를 반환해 안전 종료한 상태다. 원인은 Err에 담는다.
	StatusError Status = "error"
)

// AgentState 는 한 번의 Agent 실행 동안 누적되는 값이자 종료 후 호출자가 결과를 관찰하는 표면이다.
type AgentState struct {
	// Messages 는 user 입력부터 매 step의 assistant 응답까지 누적된 대화다.
	Messages []message.Message
	Steps    int
	Status   Status
	// Err 는 Status==StatusError일 때만 채워지는 원인 에러다.
	Err error
}

// FinalMessage 는 final 상태일 때 최종 답 메시지(마지막 assistant 응답)와 true를 반환한다.
// final이 아니거나 assistant 응답이 없으면 zero 값과 false를 반환한다.
func (s AgentState) FinalMessage() (message.Message, bool) {
	if s.Status != StatusFinal {
		return message.Message{}, false
	}
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == message.RoleAssistant {
			return s.Messages[i], true
		}
	}
	return message.Message{}, false
}

// ReflectionHook 은 매 step 경계에서 현재 step 번호와 누적 state를 받아 관찰하는 콜백 타입이다.
// nil을 주입하면 no-op으로 동작하며, loop가 항상 정상 진행된다.
type ReflectionHook func(step int, state AgentState)

// Middleware 는 Agent의 model 호출 전후에서 요청과 응답을 관찰하거나 변경하는 hook이다.
type Middleware interface {
	PreModel(ctx context.Context, in PreModelInput) (llm.ChatRequest, error)
	PostModel(ctx context.Context, in PostModelInput) (llm.ChatResponse, error)
}

// MiddlewareStage 는 middleware 실패가 발생한 model 호출 전후 위치를 나타낸다.
type MiddlewareStage string

const (
	MiddlewareStagePreModel  MiddlewareStage = "pre_model"
	MiddlewareStagePostModel MiddlewareStage = "post_model"
)

// MiddlewareError 는 실패한 middleware의 stage와 등록 위치를 호출자에게 전달한다.
type MiddlewareError struct {
	Stage MiddlewareStage
	Index int
	Err   error
}

func (e *MiddlewareError) Error() string {
	return fmt.Sprintf("agent middleware %s[%d]: %v", e.Stage, e.Index, e.Err)
}

func (e *MiddlewareError) Unwrap() error {
	return e.Err
}

// PreModelInput 은 LLM 호출 직전에 middleware가 관찰하는 실행 context다.
type PreModelInput struct {
	Step    int
	State   AgentState
	Request llm.ChatRequest
}

// PostModelInput 은 LLM 호출 직후 middleware가 관찰하는 실행 context다.
type PostModelInput struct {
	Step     int
	State    AgentState
	Request  llm.ChatRequest
	Response llm.ChatResponse
	Err      error
}

// AgentOptions 는 기존 NewAgent 생성자 밖에서 확장 실행 옵션을 전달한다.
type AgentOptions struct {
	Hook       ReflectionHook
	Middleware []Middleware
}

// Agent 는 주입된 LLMClient를 들고 AgentState 위에서 ReAct loop를 실행하는 단위다.
type Agent struct {
	client      llm.LLMClient
	model       string
	maxSteps    int
	hook        ReflectionHook
	middleware  []Middleware
	registry    *tool.Registry   // tool 이름 조회와 schema 수집에 사용한다. nil이면 tool 미사용.
	toolTimeout time.Duration    // per-tool 실행 deadline. 0이면 context 상속만 따른다.
	dispatcher  *tool.Dispatcher // registry가 non-nil일 때 생성되며 tool_call 실행을 위임한다.
}

const (
	agentLLMNodeID  graph.NodeID = "llm_node"
	agentToolNodeID graph.NodeID = "tool_node"
)

// NewAgent 는 Agent를 생성한다.
// hook이 nil이면 step 경계에서 아무 동작도 하지 않는다(no-op).
// registry가 nil이면 tool schema를 LLM에 싣지 않고 tool_call 결과도 처리하지 않는다.
// toolTimeout이 0이면 per-tool deadline을 별도로 적용하지 않는다(loop ctx 상속만).
func NewAgent(client llm.LLMClient, model string, maxSteps int, hook ReflectionHook, registry *tool.Registry, toolTimeout time.Duration) *Agent {
	return NewAgentWithOptions(client, model, maxSteps, registry, toolTimeout, AgentOptions{Hook: hook})
}

// NewAgentWithOptions 는 middleware 같은 확장 옵션을 포함해 Agent를 생성한다.
func NewAgentWithOptions(client llm.LLMClient, model string, maxSteps int, registry *tool.Registry, toolTimeout time.Duration, options AgentOptions) *Agent {
	a := &Agent{
		client:      client,
		model:       model,
		maxSteps:    maxSteps,
		hook:        options.Hook,
		middleware:  append([]Middleware(nil), options.Middleware...),
		registry:    registry,
		toolTimeout: toolTimeout,
	}
	// registry가 non-nil인 경우에만 dispatcher를 초기화한다.
	// nil registry는 기존 동작(tool 미사용)을 그대로 보존한다.
	if registry != nil {
		a.dispatcher = tool.NewDispatcher(registry, toolTimeout)
	}
	return a
}

// Run 은 사용자 입력(prompt)을 첫 메시지로 ReAct loop를 실행하고,
// final/max steps/error 중 하나로 종료된 AgentState를 반환한다.
// 에러는 두 번째 반환값으로 던지지 않고 state에 흡수된다(ctx 취소 포함).
func (a *Agent) Run(ctx context.Context, prompt string) AgentState {
	initial := AgentState{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock(prompt)},
			},
		},
		Steps:  0,
		Status: StatusRunning,
	}

	nodes := map[graph.NodeID]graph.Node[AgentState]{
		agentLLMNodeID:  &llmNode{agent: a},
		agentToolNodeID: &toolNode{agent: a},
	}
	router := graph.NewConditionalRouter[AgentState](func(current graph.NodeID, state AgentState) (graph.NodeID, error) {
		switch current {
		case agentLLMNodeID:
			if state.Status != StatusRunning {
				return graph.End, nil
			}
			if lastAssistantHasToolCalls(state) {
				return agentToolNodeID, nil
			}
			return graph.End, nil
		case agentToolNodeID:
			return agentLLMNodeID, nil
		default:
			return graph.End, nil
		}
	})

	g, err := graph.New[AgentState](agentLLMNodeID, nodes, router, graph.ReplaceReducer[AgentState]{}, a.graphMaxSteps())
	if err != nil {
		initial.Status = StatusError
		initial.Err = err
		return initial
	}

	result := g.Run(ctx, initial)
	state := result.State
	switch result.Status {
	case graph.StatusCompleted:
		return state
	case graph.StatusMaxSteps:
		state.Status = StatusMaxSteps
		return state
	case graph.StatusError, graph.StatusCanceled:
		state.Status = StatusError
		state.Err = result.Err
		return state
	default:
		state.Status = StatusError
		return state
	}
}

func (a *Agent) graphMaxSteps() int {
	if a.maxSteps <= 0 {
		return 1
	}
	return a.maxSteps*2 + 1
}

type llmNode struct {
	agent *Agent
}

func (n *llmNode) Run(ctx context.Context, state AgentState) (graph.NodeResult[AgentState], error) {
	a := n.agent
	if a.hook != nil {
		a.hook(state.Steps, state)
	}
	if state.Steps >= a.maxSteps {
		state.Status = StatusMaxSteps
		return graph.NodeResult[AgentState]{Update: state}, nil
	}

	req := llm.ChatRequest{
		Model:    a.model,
		Messages: state.Messages,
	}
	if a.registry != nil {
		req.Tools = a.registry.Specs()
	}
	for i, middleware := range a.middleware {
		var err error
		req, err = middleware.PreModel(ctx, PreModelInput{
			Step:    state.Steps,
			State:   state,
			Request: req,
		})
		if err != nil {
			return graph.NodeResult[AgentState]{}, &MiddlewareError{
				Stage: MiddlewareStagePreModel,
				Index: i,
				Err:   err,
			}
		}
	}

	resp, err := a.client.Chat(ctx, req)
	if err != nil {
		for i, middleware := range a.middleware {
			_, middlewareErr := middleware.PostModel(ctx, PostModelInput{
				Step:    state.Steps,
				State:   state,
				Request: req,
				Err:     err,
			})
			if middlewareErr != nil {
				return graph.NodeResult[AgentState]{}, &MiddlewareError{
					Stage: MiddlewareStagePostModel,
					Index: i,
					Err:   middlewareErr,
				}
			}
		}
		return graph.NodeResult[AgentState]{}, err
	}
	for i, middleware := range a.middleware {
		resp, err = middleware.PostModel(ctx, PostModelInput{
			Step:     state.Steps,
			State:    state,
			Request:  req,
			Response: resp,
		})
		if err != nil {
			return graph.NodeResult[AgentState]{}, &MiddlewareError{
				Stage: MiddlewareStagePostModel,
				Index: i,
				Err:   err,
			}
		}
	}

	state.Messages = append(state.Messages, resp.Message)
	state.Steps++
	if !resp.Message.HasToolCalls() {
		state.Status = StatusFinal
	}
	return graph.NodeResult[AgentState]{Update: state}, nil
}

type toolNode struct {
	agent *Agent
}

func (n *toolNode) Run(ctx context.Context, state AgentState) (graph.NodeResult[AgentState], error) {
	if n.agent.dispatcher == nil {
		return graph.NodeResult[AgentState]{Update: state}, nil
	}

	var resultBlocks []message.ContentBlock
	for _, block := range lastAssistantMessage(state).Content {
		if block.Type == message.BlockTypeToolCall && block.ToolCall != nil {
			result := n.agent.dispatcher.Dispatch(ctx, *block.ToolCall)
			resultBlocks = append(resultBlocks, message.NewToolResultBlock(result))
		}
	}
	if len(resultBlocks) > 0 {
		state.Messages = append(state.Messages, message.Message{
			Role:    message.RoleTool,
			Content: resultBlocks,
		})
	}
	return graph.NodeResult[AgentState]{Update: state}, nil
}

func lastAssistantHasToolCalls(state AgentState) bool {
	msg := lastAssistantMessage(state)
	return msg.HasToolCalls()
}

func lastAssistantMessage(state AgentState) message.Message {
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Role == message.RoleAssistant {
			return state.Messages[i]
		}
	}
	return message.Message{}
}
