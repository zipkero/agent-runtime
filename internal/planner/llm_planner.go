package planner

import (
	"context"
	"encoding/json"
	"fmt"

	"agentflow/internal/llm"
	"agentflow/internal/state"
	"agentflow/internal/tools"
	"agentflow/internal/types"
)

// LLMPlanner 는 LLMClient 를 주입받아 LLM 호출로 PlanResult 를 결정하는 Planner 구현체다.
// Plan() 호출마다 system prompt + user prompt 를 구성해 LLM 에 전달하고,
// 응답 JSON 을 PlanResult 로 파싱해 반환한다.
type LLMPlanner struct {
	client   llm.LLMClient
	registry tools.ToolRegistry
}

// NewLLMPlanner 는 LLMPlanner 를 생성한다.
func NewLLMPlanner(client llm.LLMClient, registry tools.ToolRegistry) *LLMPlanner {
	return &LLMPlanner{
		client:   client,
		registry: registry,
	}
}

// Plan 은 현재 AgentState 를 기반으로 LLM 에게 다음 행동을 묻고 PlanResult 를 반환한다.
// JSON 파싱 실패 시 1회 재시도하며, 재시도도 실패하면 에러를 반환한다.
// LLM 이 존재하지 않는 tool 이름을 반환(hallucination)하면 llm_parse_error 로 분류해 1회 재시도한다.
func (p *LLMPlanner) Plan(ctx context.Context, s state.AgentState) (types.PlanResult, error) {
	toolList := p.registry.List()
	systemPrompt := BuildSystemPrompt(s, toolList)
	userPrompt := BuildUserPrompt(s.UserInput)

	req := llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.0,
	}

	resp, err := p.client.Complete(ctx, req)
	if err != nil {
		return types.PlanResult{}, fmt.Errorf("llm complete: %w", err)
	}

	result, parseErr := p.parseAndValidate(resp.Content, toolList)
	if parseErr == nil {
		return result, nil
	}

	// 1회 재시도
	retryReq := req
	retryReq.Messages = append(retryReq.Messages,
		llm.Message{Role: "assistant", Content: resp.Content},
		llm.Message{Role: "user", Content: "응답이 올바른 JSON 형식이 아니거나 존재하지 않는 tool 이름을 포함합니다. 반드시 지정된 JSON Schema 를 따르는 JSON 객체만 반환하세요."},
	)

	retryResp, err := p.client.Complete(ctx, retryReq)
	if err != nil {
		return types.PlanResult{}, fmt.Errorf("llm retry complete: %w", err)
	}

	result, parseErr = p.parseAndValidate(retryResp.Content, toolList)
	if parseErr != nil {
		return types.PlanResult{}, fmt.Errorf("llm parse error after retry: %w", parseErr)
	}

	return result, nil
}

// parseAndValidate 는 LLM 응답 문자열을 PlanResult 로 파싱하고 유효성을 검증한다.
func (p *LLMPlanner) parseAndValidate(content string, toolList []tools.Tool) (types.PlanResult, error) {
	var result types.PlanResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return types.PlanResult{}, fmt.Errorf("json unmarshal: %w", err)
	}

	if result.ActionType == types.ActionToolCall {
		if result.ToolName == "" {
			return types.PlanResult{}, fmt.Errorf("tool_call 이지만 tool_name 이 비어있음")
		}
		if !p.isRegisteredTool(result.ToolName, toolList) {
			return types.PlanResult{}, fmt.Errorf("hallucinated tool name: %q", result.ToolName)
		}
	}

	return result, nil
}

// isRegisteredTool 은 name 이 toolList 에 존재하는지 확인한다.
func (p *LLMPlanner) isRegisteredTool(name string, toolList []tools.Tool) bool {
	for _, t := range toolList {
		if t.Name() == name {
			return true
		}
	}
	return false
}
