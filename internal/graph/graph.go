// Package graph 는 generic graph 실행 엔진을 정의한다.
// Node, Reducer, Router를 조합해 상태를 가진 실행 흐름을 표현한다.
// LLM, tool, CLI 등 agent 도메인 타입을 import하지 않는다.
package graph

import (
	"context"
	"errors"
	"fmt"
)

// NodeID 는 graph 안에서 node를 식별하는 이름이다.
type NodeID string

// End 는 graph 실행 종료를 나타내는 예약된 NodeID다.
// Router가 이 값을 반환하면 graph는 completed 상태로 종료된다.
const End NodeID = "__end__"

// Status 는 graph 실행이 어떤 이유로 멈췄는지를 나타낸다.
type Status string

const (
	// StatusCompleted 는 router가 End를 선택해 정상 종료한 상태다.
	StatusCompleted Status = "completed"
	// StatusMaxSteps 는 node 실행 횟수가 max steps에 도달해 안전 종료한 상태다.
	StatusMaxSteps Status = "max_steps"
	// StatusError 는 node 실행이 error를 반환해 중단된 상태다. 원인은 Result.Err에 담긴다.
	StatusError Status = "error"
	// StatusCanceled 는 context 취소 또는 deadline 초과로 중단된 상태다.
	StatusCanceled Status = "canceled"
)

// Result 는 graph 실행이 끝난 뒤 호출자가 관찰하는 최종 값이다.
type Result[S any] struct {
	State   S
	Status  Status
	Steps   int      // 실행된 node 횟수
	Current NodeID   // 마지막으로 실행된 node (또는 실행 중단 시점의 node)
	Err     error
}

// NodeResult 는 node 실행의 출력이다.
// Update에 담긴 값이 Reducer를 통해 graph state에 반영된다.
type NodeResult[S any] struct {
	Update S
}

// Node 는 graph의 실행 단위다.
// 현재 state를 받아 변경 사항을 NodeResult로 반환하거나, error를 반환해 실행을 중단시킨다.
type Node[S any] interface {
	Run(ctx context.Context, state S) (NodeResult[S], error)
}

// Reducer 는 node 결과를 현재 state에 반영하는 규칙이다.
// graph core에서 state 갱신은 항상 이 인터페이스를 통한다.
type Reducer[S any] interface {
	Reduce(current S, result NodeResult[S]) (S, error)
}

// Router 는 현재 node와 state를 기준으로 다음 node를 결정한다.
// End를 반환하면 graph가 completed 상태로 종료된다.
type Router[S any] interface {
	Next(current NodeID, state S) (NodeID, error)
}

// ReplaceReducer 는 node 결과의 Update를 그대로 다음 state로 삼는 기본 reducer다.
// node가 반환한 state 전체로 교체하므로 별도 merge 로직이 필요 없는 경우에 사용한다.
type ReplaceReducer[S any] struct{}

func (ReplaceReducer[S]) Reduce(_ S, result NodeResult[S]) (S, error) {
	return result.Update, nil
}

// StaticRouter 는 고정 mapping으로 다음 node를 결정하는 router다.
// map에 현재 node 이름이 없으면 error를 반환한다.
type StaticRouter[S any] struct {
	edges map[NodeID]NodeID
}

// NewStaticRouter 는 정적 edge mapping으로 StaticRouter를 생성한다.
func NewStaticRouter[S any](edges map[NodeID]NodeID) *StaticRouter[S] {
	return &StaticRouter[S]{edges: edges}
}

func (r *StaticRouter[S]) Next(current NodeID, _ S) (NodeID, error) {
	next, ok := r.edges[current]
	if !ok {
		return "", fmt.Errorf("graph: no edge from node %q", current)
	}
	return next, nil
}

// Graph 는 시작 node, node 집합, router, reducer, max steps를 가진 실행 가능한 graph다.
type Graph[S any] struct {
	start    NodeID
	nodes    map[NodeID]Node[S]
	router   Router[S]
	reducer  Reducer[S]
	maxSteps int
}

// New 는 Graph를 생성한다.
// start가 nodes에 없거나 router/reducer가 nil이면 error를 반환한다.
// maxSteps가 0 이하면 기본값 100을 사용한다.
func New[S any](start NodeID, nodes map[NodeID]Node[S], router Router[S], reducer Reducer[S], maxSteps int) (*Graph[S], error) {
	if _, ok := nodes[start]; !ok {
		return nil, fmt.Errorf("graph: start node %q not found in nodes", start)
	}
	if router == nil {
		return nil, errors.New("graph: router must not be nil")
	}
	if reducer == nil {
		return nil, errors.New("graph: reducer must not be nil")
	}
	if maxSteps <= 0 {
		maxSteps = 100
	}
	return &Graph[S]{
		start:    start,
		nodes:    nodes,
		router:   router,
		reducer:  reducer,
		maxSteps: maxSteps,
	}, nil
}

// Run 은 initial state를 시작으로 graph 실행 loop를 돌고 Result를 반환한다.
// error는 두 번째 반환값으로 던지지 않고 Result에 흡수된다.
func (g *Graph[S]) Run(ctx context.Context, initial S) Result[S] {
	state := initial
	current := g.start
	steps := 0

	for {
		// max steps 선검사 — node 실행 전에 상한 도달 여부를 판정한다.
		if steps >= g.maxSteps {
			return Result[S]{State: state, Status: StatusMaxSteps, Steps: steps, Current: current}
		}

		// context 취소 선검사
		if err := ctx.Err(); err != nil {
			return Result[S]{State: state, Status: StatusCanceled, Steps: steps, Current: current, Err: err}
		}

		// node 실행
		node, ok := g.nodes[current]
		if !ok {
			err := fmt.Errorf("graph: node %q not found", current)
			return Result[S]{State: state, Status: StatusError, Steps: steps, Current: current, Err: err}
		}

		nodeResult, err := node.Run(ctx, state)
		if err != nil {
			// context 취소로 인한 node error는 canceled로 분류한다.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return Result[S]{State: state, Status: StatusCanceled, Steps: steps, Current: current, Err: err}
			}
			return Result[S]{State: state, Status: StatusError, Steps: steps, Current: current, Err: err}
		}

		// reducer로 state 갱신
		newState, err := g.reducer.Reduce(state, nodeResult)
		if err != nil {
			return Result[S]{State: state, Status: StatusError, Steps: steps, Current: current, Err: err}
		}
		state = newState
		steps++

		// router로 다음 node 선택
		next, err := g.router.Next(current, state)
		if err != nil {
			return Result[S]{State: state, Status: StatusError, Steps: steps, Current: current, Err: err}
		}

		// End이면 completed 상태로 종료
		if next == End {
			return Result[S]{State: state, Status: StatusCompleted, Steps: steps, Current: current}
		}

		current = next
	}
}
