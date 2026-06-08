// Package tool 은 tool 실행 런타임 계층을 정의한다.
// 입출력에 internal/message 타입을 그대로 사용한다.
package tool

import (
	"context"
	"encoding/json"

	"github.com/zipkero/agent-runtime/internal/message"
)

// Tool 은 자신의 이름·입력 schema를 노출하고, 입력을 받아 실행되는 단위다.
//
// Execute 가 error를 반환하면 runtime이 IsError ToolResult로 정규화하므로,
// 구현체는 ToolResult의 IsError·ToolCallID를 직접 채우지 않는다.
type Tool interface {
	Spec() message.ToolSpec
	Execute(ctx context.Context, input json.RawMessage) (message.ToolResult, error)
}
