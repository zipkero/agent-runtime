package tools_test

import (
	"testing"

	"github.com/zipkero/gogent/internal/tools"
	"github.com/zipkero/gogent/internal/tools/calculator"
	"github.com/zipkero/gogent/internal/tools/search_mock"
	"github.com/zipkero/gogent/internal/tools/weather_mock"
)

func TestInMemoryToolRegistry_RegisterAndGet(t *testing.T) {
	r := tools.NewInMemoryToolRegistry()
	calc := calculator.New()

	r.Register(calc)

	got, err := r.Get("calculator")
	if err != nil {
		t.Fatalf("등록된 tool 조회 실패: %v", err)
	}
	if got.Name() != "calculator" {
		t.Errorf("tool 이름 불일치: got %q", got.Name())
	}
}

func TestInMemoryToolRegistry_GetUnregistered(t *testing.T) {
	r := tools.NewInMemoryToolRegistry()

	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("미등록 tool 조회 시 error 를 반환해야 한다")
	}
}

func TestInMemoryToolRegistry_List(t *testing.T) {
	r := tools.NewInMemoryToolRegistry()
	r.Register(calculator.New())
	r.Register(weather_mock.New())
	r.Register(search_mock.New())

	list := r.List()
	if len(list) != 3 {
		t.Errorf("List 길이 불일치: got %d, want 3", len(list))
	}
}

func TestInMemoryToolRegistry_RegisterOverwrite(t *testing.T) {
	r := tools.NewInMemoryToolRegistry()
	r.Register(calculator.New())
	r.Register(calculator.New()) // 동일 이름 재등록

	list := r.List()
	if len(list) != 1 {
		t.Errorf("덮어쓰기 후 List 길이 불일치: got %d, want 1", len(list))
	}
}
