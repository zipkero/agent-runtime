package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
)

// Dispatcher 는 tool_call 하나를 받아 검증·timeout·정규화·unknown을 모두 흡수해
// ToolResult로 돌려준다. 어떤 실패 경로에서도 error를 throw하지 않는다.
//
// timeout 은 per-tool 실행의 최대 허용 시간이다. loop ctx에서 파생한 per-tool ctx에
// 이 deadline이 적용되어, 상위 취소·전체 timeout도 함께 전파된다.
type Dispatcher struct {
	registry *Registry
	timeout  time.Duration
}

// NewDispatcher 는 주어진 registry와 per-tool timeout으로 Dispatcher를 생성한다.
func NewDispatcher(r *Registry, timeout time.Duration) *Dispatcher {
	return &Dispatcher{
		registry: r,
		timeout:  timeout,
	}
}

// Dispatch 는 tool_call 하나를 받아 ToolResult를 반환한다.
// 처리 순서:
//  1. 이름 lookup 실패 → 본체 미실행, IsError=true "unknown tool" 결과
//  2. loop ctx에서 per-tool ctx 파생 (context.WithTimeout)
//  3. Execute 호출, 반환 error가 non-nil이면 IsError=true 결과로 정규화
//     (검증 실패·실행 에러·timeout이 모두 같은 경로로 흡수됨)
//  4. 성공 시 tool이 채운 Content를 그대로 두고 IsError=false
//
// 모든 경로에서 ToolCallID 는 call.ID로 채워진다.
func (d *Dispatcher) Dispatch(ctx context.Context, call message.ToolCall) message.ToolResult {
	// (1) 이름 lookup
	t, ok := d.registry.Lookup(call.Name)
	if !ok {
		return message.ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("unknown tool: %q", call.Name),
			IsError:    true,
		}
	}

	// (2) per-tool ctx 파생
	toolCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// (3) Execute 호출 — error는 모두 IsError ToolResult로 정규화
	result, err := t.Execute(toolCtx, call.Input)
	if err != nil {
		return message.ToolResult{
			ToolCallID: call.ID,
			Content:    err.Error(),
			IsError:    true,
		}
	}

	// (4) 성공 — tool이 채운 Content를 그대로 두고 ToolCallID만 결합
	result.ToolCallID = call.ID
	result.IsError = false
	return result
}
