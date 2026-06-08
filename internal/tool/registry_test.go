package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// stubTool 은 테스트용 최소 Tool 구현체다.
type stubTool struct {
	name string
}

func (s *stubTool) Spec() message.ToolSpec {
	return message.ToolSpec{Name: s.name, Description: "stub"}
}

func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (message.ToolResult, error) {
	return message.ToolResult{Content: "ok"}, nil
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := tool.NewRegistry()

	if err := r.Register(&stubTool{name: "alpha"}); err != nil {
		t.Fatalf("첫 번째 등록에서 예상치 못한 에러: %v", err)
	}
	if err := r.Register(&stubTool{name: "alpha"}); err == nil {
		t.Fatal("중복 이름 등록이 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

func TestRegistry_Lookup_Unknown(t *testing.T) {
	r := tool.NewRegistry()
	_ = r.Register(&stubTool{name: "beta"})

	// 등록된 이름 — 찾아야 한다
	if _, ok := r.Lookup("beta"); !ok {
		t.Fatal("등록된 이름 조회가 false를 반환했습니다")
	}

	// 미등록 이름 — unknown으로 구분되어야 한다
	if _, ok := r.Lookup("unknown-tool"); ok {
		t.Fatal("미등록 이름 조회가 true를 반환했습니다")
	}
}

func TestRegistry_Specs(t *testing.T) {
	r := tool.NewRegistry()
	names := []string{"first", "second", "third"}
	for _, n := range names {
		if err := r.Register(&stubTool{name: n}); err != nil {
			t.Fatalf("등록 실패: %v", err)
		}
	}

	specs := r.Specs()
	if len(specs) != len(names) {
		t.Fatalf("Specs() 반환 수 불일치: got %d, want %d", len(specs), len(names))
	}

	// 등록 순서가 보존되어야 한다
	for i, want := range names {
		if specs[i].Name != want {
			t.Errorf("specs[%d].Name = %q, want %q", i, specs[i].Name, want)
		}
	}
}

func TestRegistry_Specs_Empty(t *testing.T) {
	r := tool.NewRegistry()
	specs := r.Specs()
	if len(specs) != 0 {
		t.Fatalf("빈 Registry의 Specs()가 %d 개를 반환했습니다", len(specs))
	}
}
