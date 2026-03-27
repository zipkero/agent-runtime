# Architecture Overview

agentflow의 전체 실행 흐름과 컴포넌트 간 관계를 기술한다.

---

## 전체 흐름도

```
User Input
    │
    ▼
┌─────────────────────────────────────────────┐
│                   Runtime                   │
│                                             │
│  ┌──────────┐      ┌──────────────────────┐ │
│  │ Planner  │─────▶│      Executor        │ │
│  │          │      │  ┌───────────────┐   │ │
│  │ (LLM or  │      │  │  Tool Router  │   │ │
│  │  Mock)   │      │  │  ┌──────────┐ │   │ │
│  └──────────┘      │  │  │ Registry │ │   │ │
│       ▲            │  │  └──────────┘ │   │ │
│       │            │  └───────────────┘   │ │
│  AgentState        └──────────────────────┘ │
│       │                      │              │
│       │              ToolResult             │
│       │                      │              │
│       └──────── state 반영 ◀─┘              │
│                      │                      │
│               ┌──────▼──────┐               │
│               │   Verifier  │               │
│               └──────┬──────┘               │
│                      │                      │
│            done/retry/fail 판단             │
└─────────────────────────────────────────────┘
    │
    ▼
Response (FinalAnswer)
```

---

## 데이터 흐름 상세

### 1. User Input → Runtime

사용자 입력이 들어오면 Runtime은 `AgentState`를 초기화한다.

```
AgentState {
    RequestID  : 새로 생성된 UUID
    SessionID  : 기존 세션 or 신규 생성
    UserInput  : 사용자 입력 문자열
    Status     : running
    StepCount  : 0
}
```

### 2. Runtime → Planner

Runtime은 현재 `AgentState`를 Planner에 전달한다.
Planner는 상태를 분석하고 다음 행동을 결정해 `PlanResult`를 반환한다.

```
PlanResult {
    ActionType : tool_call | respond_directly | finish
    ToolName   : "calculator" (ActionType이 tool_call일 때)
    ToolInput  : {"expression": "3 * 7"}
    Reasoning  : "사용자가 수식 계산을 요청했으므로 calculator를 호출한다"
}
```

### 3. Planner → Executor → Tool Router → Tool

ActionType이 `tool_call`이면 Executor가 Tool Router를 통해 해당 Tool을 실행한다.

```
Tool Router 처리 흐름:
  PlanResult.ToolName
      │
      ▼
  Registry.Get(name)
      │
      ├── 미등록 → 에러 반환 (loop: retry or fail)
      │
      └── 등록됨 → Tool.Execute(ctx, input)
                        │
                        └── ToolResult 반환
```

### 4. ToolResult → AgentState 반영

Executor가 반환한 `ToolResult`는 `AgentState.ToolResults`에 추가된다.
`StepCount`가 1 증가하고, Verifier가 상태를 평가한다.

### 5. Verifier → finish 판단

Verifier는 현재 AgentState를 보고 세 가지 중 하나를 반환한다.

| 결과 | 조건 | Runtime 행동 |
|------|------|-------------|
| `done` | FinalAnswer가 있고 결과가 충분함 | loop 종료, 응답 반환 |
| `retry` | 결과가 부족하거나 비어있음 | Planner 재호출 |
| `fail` | 복구 불가능한 에러 발생 | loop 종료, 에러 반환 |

### 6. Loop 종료 조건

아래 조건 중 하나라도 충족되면 loop를 종료한다.

1. Planner가 `finish` ActionType 반환
2. Planner가 `respond_directly` ActionType 반환하고 FinalAnswer 작성 완료
3. `StepCount >= MaxStep` (무한 루프 방지)
4. Verifier가 `fail` 반환
5. context deadline 초과

---

## 컴포넌트별 책임 경계

| 컴포넌트 | 하는 것 | 하지 않는 것 |
|----------|---------|-------------|
| Runtime | loop 제어, 종료 판단, retry 조율 | LLM 호출, Tool 실행 |
| Planner | 다음 행동 결정 | 실제 실행, 상태 저장 |
| Executor | PlanResult를 실행으로 연결 | 행동 결정, 상태 관리 |
| Tool Router | Tool 조회 및 실행 위임 | Tool 내부 로직 |
| Tool | 단일 기능 실행 | 다른 Tool 호출, 상태 변경 |
| Verifier | 결과 충분성 평가 | 실행, 재계획 |
| Memory | 상태 영속화 | 행동 결정 |

---

## Phase별 컴포넌트 추가 계획

```
Phase 1  Runtime + MockPlanner + MockExecutor + AgentState
Phase 2  Tool + ToolRegistry + ToolRouter + 구체 Tool 구현
Phase 3  LLMClient + LLMPlanner + Reflector
Phase 4  SessionState + WorkingMemory + LongTermMemory + MemoryManager
Phase 5  Verifier + RetryPolicy + FailureHandler
Phase 6  Task + Workflow + ManagerAgent + WorkerAgent
Phase 7  HTTP API + AsyncTaskQueue + Worker
Phase 8  Timeout + CostPolicy + Observability + PolicyLayer
```

각 Phase는 이전 Phase의 컴포넌트를 교체하거나 확장하는 방식으로 진행된다.
핵심 loop(`internal/agent/runtime.go`)는 Phase 1에 확정되고, 이후엔 부품만 교체된다.

---

## 설계 결정 사항

### AgentState.CurrentPlan 미포함 (Phase 1)

`AgentState`에 `CurrentPlan PlanResult` 필드를 넣으면 패키지 순환 참조가 발생한다.

```
state  → planner (CurrentPlan 타입 때문에)
planner → state  (Planner.Plan 인자 타입 때문에)
```

**Phase 1 선택: PlanResult를 Runtime 지역변수로만 처리**

`PlanResult`는 `planner.Plan()` 호출 직후 `executor.Execute()`로 넘기면 충분하다.
loop 내에서 소비되고 사라지므로 state에 저장할 필요가 없다.

**Phase 3 예정: `internal/types` 패키지로 분리**

LLMPlanner가 도입되면 "이전 step에서 무엇을 결정했는지"를 Planner가 참고해야 한다.
그 시점에 `PlanResult`를 `internal/types`로 이동하면 순환 없이 `AgentState`에 포함 가능하다.

```
internal/types   ← PlanResult, ToolResult 등 공유 타입
internal/state   → types
internal/planner → types, state
internal/executor → types, state
```
