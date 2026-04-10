# Glossary — 핵심 용어 정의

이 문서는 gogent 프로젝트 전반에서 사용하는 핵심 용어의 정의를 기술한다.
코드, 인터페이스, 문서 간 용어가 일치하지 않으면 경계 설계 시 혼란이 생기므로,
새로운 컴포넌트를 추가하기 전에 반드시 이 문서를 기준으로 삼는다.

---

## Agent

사용자 입력을 받아 목표를 달성하기 위한 일련의 행동을 자율적으로 수행하는 주체.
내부적으로 Planner, Executor, Tool Router, Memory를 조합하여 동작한다.
단일 Agent와 다수의 Agent가 협력하는 Multi-Agent 구조 모두를 포괄한다.

> 코드 위치: `internal/agent/`

---

## Runtime

Agent Loop를 구동하는 실행 엔진.
`plan → execute → state 반영 → finish 판단` 사이클을 반복하며,
loop 종료 조건 판단, context deadline 관리, retry 정책 적용을 담당한다.
LLM 호출이나 Tool 실행의 세부 구현은 알지 못한다.

> 코드 위치: `internal/agent/runtime.go`

---

## Planner

현재 AgentState를 입력으로 받아 다음 행동(PlanResult)을 결정하는 컴포넌트.
"무엇을 할지"를 결정하며, "어떻게 실행할지"는 책임지지 않는다.
초기엔 MockPlanner로 시작하고, Phase 3에서 LLMPlanner로 교체된다.

**결정 가능한 행동 유형 (ActionType):**
- `tool_call` — 특정 Tool을 호출하도록 지시
- `respond_directly` — Tool 없이 바로 응답 생성
- `finish` — loop 종료

> 코드 위치: `internal/planner/`

---

## Executor

PlanResult를 입력으로 받아 실제 행동을 수행하고 ToolResult를 반환하는 컴포넌트.
Phase 1에서는 MockExecutor로, Phase 2 이후에는 ToolRouter를 내부에서 사용한다.
"어떻게 실행할지"를 담당하며, 다음 행동을 결정하는 역할은 하지 않는다.

> 코드 위치: `internal/executor/`

---

## Tool

단일 기능을 수행하는 실행 단위. 이름, 설명, 입력 스키마, 실행 메서드로 구성된다.
Tool 자체는 상태를 갖지 않으며, 입력을 받아 결과를 반환하는 순수한 함수에 가깝다.

예시: `calculator`, `weather_mock`, `search_mock`

> 코드 위치: `internal/tools/`

---

## Tool Registry

사용 가능한 Tool 목록을 관리하는 저장소.
Tool을 등록(`Register`)하고, 이름으로 조회(`Get`)하며, 전체 목록을 반환(`List`)한다.
LLM이 선택 가능한 Tool 목록을 알려줄 때도 이 Registry를 기준으로 한다.

> 코드 위치: `internal/tools/registry.go`

---

## Tool Router

PlanResult의 ToolName을 보고 Registry에서 해당 Tool을 찾아 실행을 위임하는 컴포넌트.
Planner와 Tool 구현체 사이의 중재자 역할을 한다.
미등록 Tool 조회, 입력 유효성 검증, 실행 에러를 각각 구분하여 처리한다.

> 코드 위치: `internal/tools/router.go`

---

## Session

사용자와 Agent 사이의 연속된 대화 단위.
단일 요청(Request)이 끝나도 SessionID를 통해 이전 대화 맥락을 복원할 수 있다.
세션은 여러 Request를 포함할 수 있으며, Redis에 직렬화되어 저장된다.

> 코드 위치: `internal/state/session_state.go`

---

## Memory

Agent가 참조할 수 있는 정보 저장소. 범위에 따라 두 가지로 나뉜다.

### Working Memory
현재 요청 처리 중에만 유효한 중간 산출물 (검색 결과, 필터 결과, 요약 등).
요청이 끝나면 소멸한다.

### Long-term Memory
Postgres에 영구 저장되는 기억. 태그 기반으로 검색하며 다음 요청에서도 참조 가능하다.
"지난번에 사용자가 선호한 조건" 같은 정보가 여기에 해당한다.

> 코드 위치: `internal/state/working_memory.go`, `internal/memory/`

---

## Verifier

Executor의 결과(AgentState)를 보고 loop를 계속할지, retry할지, 종료할지 판단하는 컴포넌트.
Planner가 "무엇을 할지" 결정한다면, Verifier는 "결과가 충분한지" 판단한다.
`done`, `retry`, `fail` 세 가지 상태를 반환한다.

> 코드 위치: `internal/verifier/`

---

## Task

Multi-Agent 시나리오에서 분해된 작업 단위.
단일 Agent가 처리하는 최소 실행 단위이며, 다른 Task에 대한 의존성을 가질 수 있다.
`hotel_search`, `filter_by_price` 같은 구체적인 작업이 Task에 해당한다.

> 코드 위치: `internal/orchestration/task.go`

---

## Step

Runtime Loop의 단일 반복(iteration).
한 Step은 `plan → execute → state 반영` 한 사이클을 의미한다.
`AgentState.StepCount`로 몇 번째 step인지 추적하며, 최대 step 초과 시 loop를 종료한다.

> **Phase 5 예정**: Verifier 도입 후 `plan → execute → verify → state 반영` 으로 확장된다.

---

## Workflow

Task들의 의존성 그래프.
독립적인 Task는 병렬로 실행하고, 의존 관계가 있는 Task는 순서를 지켜 실행한다.
Phase 7 ManagerAgent가 이 Workflow를 해석하고 실행 순서를 결정한다.

> 코드 위치: `internal/orchestration/`
