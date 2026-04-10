package tools

import (
	"context"

	"github.com/zipkero/gogent/internal/types"
)

// Tool 은 Agent가 실행할 수 있는 단일 기능 단위의 인터페이스다.
//
// Name 과 Description 은 Registry 조회와 LLM system prompt 생성에 사용된다.
// InputSchema 는 ToolRouter 의 input validation 과 LLM 파라미터 명세에 사용된다.
// Execute 는 LLM 이 결정한 input map 을 받아 실제 tool 로직을 실행한다.
type Tool interface {
	// Name 은 tool 의 고유 식별자다. Registry 에서 이름으로 조회할 때 키로 사용된다.
	Name() string

	// Description 은 이 tool 이 하는 일을 LLM 이 이해할 수 있게 설명한다.
	Description() string

	// InputSchema 는 Execute 가 받을 input map 의 구조를 기술한다.
	InputSchema() Schema

	// Execute 는 input map 을 받아 tool 로직을 실행하고 결과를 반환한다.
	// input 의 키와 값 타입은 InputSchema 를 따라야 한다.
	Execute(ctx context.Context, input map[string]any) (types.ToolResult, error)
}
