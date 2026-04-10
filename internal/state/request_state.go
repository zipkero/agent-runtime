package state

import (
	"github.com/zipkero/gogent/internal/types"
	"time"
)

// RequestState 는 단일 요청 범위의 상태를 담는다.
// AgentState 에 혼재해 있던 요청 단위 데이터를 명시적으로 분리한 구조체다.
type RequestState struct {
	// RequestID 는 요청을 식별하는 고유 ID다.
	RequestID string
	// UserInput 은 사용자가 입력한 원문이다.
	UserInput string
	// ToolResults 는 이 요청에서 실행된 tool 결과 목록이다.
	ToolResults []types.ToolResult
	// ReasoningSteps 는 각 step 에서 LLM 이 수행한 reasoning 요약 목록이다.
	ReasoningSteps []string
	// StartedAt 은 요청이 시작된 시각이다.
	StartedAt time.Time
}
