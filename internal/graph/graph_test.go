package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zipkero/agent-runtime/internal/graph"
)

// intState 는 테스트용 단순 정수 state다.
type intState struct {
	value int
	order []string // node 실행 순서를 누적한다.
}

// appendNode 는 실행될 때 order에 이름을 추가하고 value를 delta만큼 증가시키는 stub node다.
type appendNode struct {
	name  string
	delta int
}

func (n *appendNode) Run(_ context.Context, state intState) (graph.NodeResult[intState], error) {
	newOrder := make([]string, len(state.order)+1)
	copy(newOrder, state.order)
	newOrder[len(state.order)] = n.name
	return graph.NodeResult[intState]{
		Update: intState{value: state.value + n.delta, order: newOrder},
	}, nil
}

// TestStaticEdgeExecution 은 두 개 이상의 stub node가 정적 edge 순서대로 실행되고,
// 각 node의 state 변경이 reducer를 거쳐 다음 node와 최종 result에 반영되는지 단언한다.
func TestStaticEdgeExecution(t *testing.T) {
	nodeA := &appendNode{name: "a", delta: 10}
	nodeB := &appendNode{name: "b", delta: 20}
	nodeC := &appendNode{name: "c", delta: 5}

	nodes := map[graph.NodeID]graph.Node[intState]{
		"a": nodeA,
		"b": nodeB,
		"c": nodeC,
	}

	// a → b → c → end 정적 edge
	router := graph.NewStaticRouter[intState](map[graph.NodeID]graph.NodeID{
		"a": "b",
		"b": "c",
		"c": graph.End,
	})

	g, err := graph.New[intState]("a", nodes, router, graph.ReplaceReducer[intState]{}, 10)
	if err != nil {
		t.Fatalf("graph.New 실패: %v", err)
	}

	initial := intState{value: 0, order: nil}
	result := g.Run(context.Background(), initial)

	// 종료 상태 단언
	if result.Status != graph.StatusCompleted {
		t.Errorf("status: got %q, want %q", result.Status, graph.StatusCompleted)
	}

	// step 수 단언 — node a, b, c 세 번 실행
	if result.Steps != 3 {
		t.Errorf("steps: got %d, want 3", result.Steps)
	}

	// 최종 state 단언 — 0 + 10 + 20 + 5 = 35
	if result.State.value != 35 {
		t.Errorf("final state.value: got %d, want 35", result.State.value)
	}

	// 실행 순서 단언
	wantOrder := []string{"a", "b", "c"}
	if len(result.State.order) != len(wantOrder) {
		t.Fatalf("order length: got %d, want %d", len(result.State.order), len(wantOrder))
	}
	for i, name := range wantOrder {
		if result.State.order[i] != name {
			t.Errorf("order[%d]: got %q, want %q", i, result.State.order[i], name)
		}
	}
}

// TestStaticEdgeTwoNodes 는 node가 정확히 두 개인 최소 케이스를 검증한다.
func TestStaticEdgeTwoNodes(t *testing.T) {
	nodeA := &appendNode{name: "first", delta: 1}
	nodeB := &appendNode{name: "second", delta: 2}

	nodes := map[graph.NodeID]graph.Node[intState]{
		"first":  nodeA,
		"second": nodeB,
	}
	router := graph.NewStaticRouter[intState](map[graph.NodeID]graph.NodeID{
		"first":  "second",
		"second": graph.End,
	})

	g, err := graph.New[intState]("first", nodes, router, graph.ReplaceReducer[intState]{}, 10)
	if err != nil {
		t.Fatalf("graph.New 실패: %v", err)
	}

	result := g.Run(context.Background(), intState{value: 0})

	if result.Status != graph.StatusCompleted {
		t.Errorf("status: got %q, want %q", result.Status, graph.StatusCompleted)
	}
	if result.Steps != 2 {
		t.Errorf("steps: got %d, want 2", result.Steps)
	}
	if result.State.value != 3 {
		t.Errorf("final state.value: got %d, want 3", result.State.value)
	}
	if len(result.State.order) != 2 || result.State.order[0] != "first" || result.State.order[1] != "second" {
		t.Errorf("order: got %v, want [first second]", result.State.order)
	}
}

// TestReducerPropagation 은 각 node가 반환한 state 변경이 reducer를 거쳐
// 다음 node의 입력 state에 반영되는지 검증한다.
func TestReducerPropagation(t *testing.T) {
	// custom node: value를 두 배로 만든다.
	var doubleNodeImpl graph.Node[intState] = nodeFunc[intState](func(_ context.Context, s intState) (graph.NodeResult[intState], error) {
		return graph.NodeResult[intState]{Update: intState{value: s.value * 2, order: append(s.order, "double")}}, nil
	})
	// +3 하는 node
	var addNodeImpl graph.Node[intState] = nodeFunc[intState](func(_ context.Context, s intState) (graph.NodeResult[intState], error) {
		return graph.NodeResult[intState]{Update: intState{value: s.value + 3, order: append(s.order, "add")}}, nil
	})

	nodes := map[graph.NodeID]graph.Node[intState]{
		"double": doubleNodeImpl,
		"add":    addNodeImpl,
	}
	router := graph.NewStaticRouter[intState](map[graph.NodeID]graph.NodeID{
		"double": "add",
		"add":    graph.End,
	})
	g, err := graph.New[intState]("double", nodes, router, graph.ReplaceReducer[intState]{}, 10)
	if err != nil {
		t.Fatalf("graph.New 실패: %v", err)
	}

	// initial value = 5 → double → 10 → add → 13
	result := g.Run(context.Background(), intState{value: 5})

	if result.Status != graph.StatusCompleted {
		t.Errorf("status: got %q, want %q", result.Status, graph.StatusCompleted)
	}
	if result.State.value != 13 {
		t.Errorf("final state.value: got %d, want 13", result.State.value)
	}
	if result.Steps != 2 {
		t.Errorf("steps: got %d, want 2", result.Steps)
	}
}

// TestMaxStepsHalt 는 max steps에 도달하면 다음 node를 실행하지 않고 max_steps 상태를 반환하는지 단언한다.
func TestMaxStepsHalt(t *testing.T) {
	// 항상 자기 자신으로 돌아오는 loop node
	selfLoop := nodeFunc[intState](func(_ context.Context, s intState) (graph.NodeResult[intState], error) {
		return graph.NodeResult[intState]{Update: intState{value: s.value + 1}}, nil
	})
	nodes := map[graph.NodeID]graph.Node[intState]{"loop": selfLoop}
	router := graph.NewStaticRouter[intState](map[graph.NodeID]graph.NodeID{
		"loop": "loop",
	})
	g, err := graph.New[intState]("loop", nodes, router, graph.ReplaceReducer[intState]{}, 3)
	if err != nil {
		t.Fatalf("graph.New 실패: %v", err)
	}

	result := g.Run(context.Background(), intState{value: 0})

	if result.Status != graph.StatusMaxSteps {
		t.Errorf("status: got %q, want %q", result.Status, graph.StatusMaxSteps)
	}
	// maxSteps=3이므로 3번 실행된 뒤 멈춘다.
	if result.Steps != 3 {
		t.Errorf("steps: got %d, want 3", result.Steps)
	}
}

// TestNodeErrorHalt 는 node가 error를 반환하면 graph가 error 상태로 중단되는지 단언한다.
func TestNodeErrorHalt(t *testing.T) {
	sentinel := errors.New("node 실패 sentinel")
	errNode := nodeFunc[intState](func(_ context.Context, s intState) (graph.NodeResult[intState], error) {
		return graph.NodeResult[intState]{}, sentinel
	})
	nodes := map[graph.NodeID]graph.Node[intState]{"fail": errNode}
	router := graph.NewStaticRouter[intState](map[graph.NodeID]graph.NodeID{
		"fail": graph.End,
	})
	g, err := graph.New[intState]("fail", nodes, router, graph.ReplaceReducer[intState]{}, 10)
	if err != nil {
		t.Fatalf("graph.New 실패: %v", err)
	}

	result := g.Run(context.Background(), intState{value: 0})

	if result.Status != graph.StatusError {
		t.Errorf("status: got %q, want %q", result.Status, graph.StatusError)
	}
	if !errors.Is(result.Err, sentinel) {
		t.Errorf("Err: got %v, want sentinel", result.Err)
	}
}

// TestContextCanceledHalt 는 취소된 context로 실행하면 canceled 상태를 반환하는지 단언한다.
func TestContextCanceledHalt(t *testing.T) {
	normalNode := nodeFunc[intState](func(_ context.Context, s intState) (graph.NodeResult[intState], error) {
		return graph.NodeResult[intState]{Update: s}, nil
	})
	nodes := map[graph.NodeID]graph.Node[intState]{"n": normalNode}
	router := graph.NewStaticRouter[intState](map[graph.NodeID]graph.NodeID{"n": "n"})
	g, err := graph.New[intState]("n", nodes, router, graph.ReplaceReducer[intState]{}, 100)
	if err != nil {
		t.Fatalf("graph.New 실패: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 실행 전 즉시 취소

	result := g.Run(ctx, intState{value: 0})

	if result.Status != graph.StatusCanceled {
		t.Errorf("status: got %q, want %q", result.Status, graph.StatusCanceled)
	}
}

// TestNewGraphValidation 은 잘못된 인수로 graph.New를 호출하면 error를 반환하는지 단언한다.
func TestNewGraphValidation(t *testing.T) {
	dummyNode := nodeFunc[intState](func(_ context.Context, s intState) (graph.NodeResult[intState], error) {
		return graph.NodeResult[intState]{Update: s}, nil
	})
	router := graph.NewStaticRouter[intState](map[graph.NodeID]graph.NodeID{"a": graph.End})

	// start node가 nodes에 없는 경우
	_, err := graph.New[intState]("missing", map[graph.NodeID]graph.Node[intState]{"a": dummyNode}, router, graph.ReplaceReducer[intState]{}, 10)
	if err == nil {
		t.Error("start node 없을 때 error를 반환해야 한다")
	}

	// router가 nil인 경우
	_, err = graph.New[intState]("a", map[graph.NodeID]graph.Node[intState]{"a": dummyNode}, nil, graph.ReplaceReducer[intState]{}, 10)
	if err == nil {
		t.Error("router가 nil일 때 error를 반환해야 한다")
	}

	// reducer가 nil인 경우
	_, err = graph.New[intState]("a", map[graph.NodeID]graph.Node[intState]{"a": dummyNode}, router, nil, 10)
	if err == nil {
		t.Error("reducer가 nil일 때 error를 반환해야 한다")
	}
}

// nodeFunc 는 함수를 Node[S] 인터페이스로 변환하는 어댑터 타입이다.
type nodeFunc[S any] func(ctx context.Context, state S) (graph.NodeResult[S], error)

func (f nodeFunc[S]) Run(ctx context.Context, state S) (graph.NodeResult[S], error) {
	return f(ctx, state)
}
