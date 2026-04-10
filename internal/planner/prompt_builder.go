package planner

import (
	"fmt"
	"strings"

	"github.com/zipkero/gogent/internal/state"
	"github.com/zipkero/gogent/internal/tools"
)

// BuildSystemPrompt 는 AgentState 와 등록된 tool 목록을 받아
// LLMPlanner 가 OpenAI 에 전달할 system prompt 문자열을 생성한다.
//
// 구성:
//  1. Agent 역할 선언
//  2. 사용 가능한 tool 명세
//  3. 현재 실행 컨텍스트 (step, 이전 tool 결과)
//  4. 응답 형식 (JSON Schema + 예시)
func BuildSystemPrompt(s state.AgentState, toolList []tools.Tool) string {
	var b strings.Builder

	// 1. Agent 역할
	b.WriteString("당신은 사용자의 요청을 단계적으로 해결하는 AI 에이전트다.\n")
	b.WriteString("매 step마다 아래 JSON Schema를 따르는 JSON 객체 하나만 반환하라.\n\n")

	// 2. Tool 명세
	b.WriteString("## 사용 가능한 Tools\n\n")
	if len(toolList) == 0 {
		b.WriteString("(사용 가능한 tool 없음)\n\n")
	} else {
		for _, t := range toolList {
			b.WriteString(fmt.Sprintf("### %s\n", t.Name()))
			b.WriteString(fmt.Sprintf("%s\n\n", t.Description()))

			schema := t.InputSchema()
			if len(schema.Fields) > 0 {
				b.WriteString("**입력 파라미터:**\n")
				for _, f := range schema.Fields {
					required := ""
					if f.Required {
						required = " (필수)"
					}
					b.WriteString(fmt.Sprintf("- `%s` (%s)%s: %s\n", f.Name, f.Type, required, f.Description))
				}
				b.WriteString("\n")
			}
		}
	}

	// 3. 현재 실행 컨텍스트
	b.WriteString("## 현재 실행 컨텍스트\n\n")
	b.WriteString(fmt.Sprintf("- **현재 step**: %d\n", s.StepCount+1))

	if s.CurrentPlan.NextGoal != "" {
		b.WriteString(fmt.Sprintf("- **이번 step 목표**: %s\n", s.CurrentPlan.NextGoal))
	}

	if len(s.Request.ToolResults) > 0 {
		b.WriteString("\n**이전 tool 실행 결과:**\n")
		for i, tr := range s.Request.ToolResults {
			b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, tr.ToolName, tr.Output))
		}
	}
	b.WriteString("\n")

	// 4. 응답 형식
	b.WriteString(PlanResultSchemaPrompt())

	return b.String()
}

// BuildUserPrompt 는 사용자 입력을 user role 메시지로 감싸 반환한다.
// LLMPlanner 에서 Messages 슬라이스의 마지막 user 메시지로 사용된다.
func BuildUserPrompt(userInput string) string {
	return userInput
}
