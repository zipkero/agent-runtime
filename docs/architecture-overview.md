# Architecture Overview

agentflow의 전체 실행 흐름과 컴포넌트 간 관계를 기술한다.

---

## 전체 흐름도 (Phase 2 기준)

```
┌─────────────────────────────────────────────────────────────────┐
│  사용자 입력 (stdin)                                              │
│  "서울 날씨 알려줘"                                               │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  cmd/agent-cli/main.go                                           │
│                                                                  │
│  AgentState{                                                     │
│    RequestID:  NewRequestID()   // uuid 생성                     │
│    SessionID:  "session-dev"    // 고정값 (Phase 4에서 변경)     │
│    UserInput:  "서울 날씨 알려줘"                                 │
│    Status:     running                                           │
│    StepCount:  0                                                 │
│  }                                                               │
└───────────────────────────┬─────────────────────────────────────┘
                            │  AgentState
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  Runtime.Run(ctx, AgentState)                   [internal/agent] │
│                                                                  │
│  ┌──────────────────── LOOP ───────────────────────────────┐    │
│  │                                                          │    │
│  │  ① Planner.Plan(ctx, AgentState)                        │    │
│  │         │                                               │    │
│  │         │  반환: PlanResult{                            │    │
│  │         │    ActionType: "tool_call"                    │    │
│  │         │    ToolName:   "weather_mock"                 │    │
│  │         │    ToolInput:  map["city": "Seoul"]           │    │
│  │         │    Reasoning:  "날씨 조회가 필요하다"          │    │
│  │         │  }                                            │    │
│  │         ▼                                               │    │
│  │  ② IsFinished(PlanResult, AgentState, MaxStep)          │    │
│  │         │                                               │    │
│  │         │  반환: FinishResult{Finished: false}          │    │
│  │         │  (종료 조건 미충족이면 계속)                   │    │
│  │         │                                               │    │
│  │         │  종료 조건:                                   │    │
│  │         │   - ActionType == "finish"                    │    │
│  │         │   - ActionType == "respond_directly"          │    │
│  │         │     + FinalAnswer != ""                       │    │
│  │         │   - StepCount >= MaxStep (기본 10)            │    │
│  │         │   - Status == failed                          │    │
│  │         ▼                                               │    │
│  │  ③ Executor.Execute(ctx, PlanResult)                    │    │
│  │         │                                               │    │
│  │         │  반환: ToolResult{                            │    │
│  │         │    ToolName: "weather_mock"                   │    │
│  │         │    Output:   "도시: Seoul | 날씨: 맑음 | ..."  │    │
│  │         │    IsError:  false                            │    │
│  │         │  }                                            │    │
│  │         ▼                                               │    │
│  │  ④ AgentState 반영                                      │    │
│  │     LastToolCall = "weather_mock"                       │    │
│  │     ToolResults  = append(..., ToolResult)              │    │
│  │     StepCount++                                         │    │
│  │                                                         │    │
│  └──────────────────────────────────────────────────────┘  │    │
│                                                                  │
│  반환: (AgentState, error)                                       │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  Executor.Execute(ctx, PlanResult)          [internal/executor]  │
│                                                                  │
│  PlanResult{                                                     │
│    ToolName:  "weather_mock"                                     │
│    ToolInput: map["city": "Seoul"]                               │
│  }                                                               │
│             │                                                    │
│             ▼                                                    │
│  ToolRouter.Route(ctx, PlanResult)              [internal/tools] │
│                                                                  │
│  ┌─ ① registry.Get("weather_mock") ──────────────────────────┐  │
│  │     없으면 → AgentError{                                   │  │
│  │               Kind:      "tool_not_found"                  │  │
│  │               Retryable: false  (fatal)                    │  │
│  │             }                                              │  │
│  └────────────────────────────────────────────────────────────┘  │
│  ┌─ ② validateInput(Schema, ToolInput) ──────────────────────┐  │
│  │     Schema.Fields = [{Name:"city", Type:string, Required}] │  │
│  │     실패 시 → AgentError{                                  │  │
│  │               Kind:      "input_validation_failed"         │  │
│  │               Retryable: false  (fatal)                    │  │
│  │             }                                              │  │
│  └────────────────────────────────────────────────────────────┘  │
│  ┌─ ③ tool.Execute(ctx, map["city":"Seoul"]) ────────────────┐  │
│  │     에러 시 → AgentError{                                  │  │
│  │               Kind:      "tool_execution_failed"           │  │
│  │               Retryable: true   (retryable)                │  │
│  │             }                                              │  │
│  └────────────────────────────────────────────────────────────┘  │
│  ┌─ ④ slog 로그 출력 ─────────────────────────────────────── ┐  │
│  │     request_id, session_id, tool_name, input,              │  │
│  │     output_summary, is_error, duration_ms                  │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  반환: ToolResult{ ToolName, Output, IsError, ErrMsg }           │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  ToolRegistry (InMemoryToolRegistry)            [internal/tools] │
│                                                                  │
│  map[string]Tool{                                                │
│    "calculator"   → Calculator{}                                 │
│    "weather_mock" → WeatherMock{}                                │
│    "search_mock"  → SearchMock{}                                 │
│  }                                                               │
│                                                                  │
│  각 Tool.InputSchema():                                          │
│   calculator   → Fields: [{expression, string, required}]       │
│   weather_mock → Fields: [{city,       string, required}]       │
│   search_mock  → Fields: [{query,      string, required}]       │
└─────────────────────────────────────────────────────────────────┘
```

> **현재 미연결 (Phase 3에서 연결 예정)**
> - `Planner` = MockPlanner (실제 LLM 미연결)
> - `Executor` = MockExecutor (ToolRouter 미연결)
> - `ctx`에 `WithRequestID` / `WithSessionID` 주입 미완료 → 로그에 request_id 빈 값

---

## 데이터 흐름 상세

### 1. User Input → Runtime

사용자 입력이 들어오면 Runtime은 `AgentState`를 초기화한다.

```
AgentState {
    RequestID  : NewRequestID()  // UUID v4
    SessionID  : "session-dev"   // Phase 4에서 실제 세션으로 교체
    UserInput  : 사용자 입력 문자열
    Status     : running
    StepCount  : 0
}
```

`RequestID`는 단일 요청 추적용, `SessionID`는 대화 세션 전체 추적용으로 범위가 다르다.

### 2. Runtime → Planner

Runtime은 현재 `AgentState`를 Planner에 전달한다.
Planner는 상태를 분석하고 다음 행동을 결정해 `PlanResult`를 반환한다.

```
PlanResult {
    ActionType : tool_call | respond_directly | finish
    ToolName   : "calculator"  (ActionType이 tool_call일 때)
    ToolInput  : {"expression": "3 * 7"}
    Reasoning  : "사용자가 수식 계산을 요청했으므로 calculator를 호출한다"
}
```

### 3. Planner → Executor → ToolRouter → Tool

ActionType이 `tool_call`이면 Executor가 ToolRouter를 통해 Tool을 실행한다.

```
ToolRouter.Route(ctx, PlanResult):

  ① registry.Get(ToolName)
       실패 → AgentError{Kind: "tool_not_found", Retryable: false}

  ② validateInput(tool.InputSchema(), ToolInput)
       - required 필드 누락 여부 확인
       - 필드 타입 일치 여부 확인 (string / number / boolean)
       실패 → AgentError{Kind: "input_validation_failed", Retryable: false}

  ③ tool.Execute(ctx, ToolInput)
       실패 → AgentError{Kind: "tool_execution_failed", Retryable: true}

  ④ slog 구조화 로그 출력
       성공: INFO  request_id, session_id, tool_name, input, output_summary, is_error, duration_ms
       실패: ERROR request_id, session_id, tool_name, error_kind, error, duration_ms
```

`request_id` / `session_id`는 `context.WithValue`로 전달된다.
호출 전에 `tools.WithRequestID(ctx, state.RequestID)` 형태로 context에 주입해야 로그에 값이 찍힌다.

### 4. AgentError — 에러 분류 체계

```
AgentError {
    Kind      : ErrorKind  // 에러 유형 식별자
    Retryable : bool       // true → loop에서 재시도 가능
                           // false → 즉시 종료 (fatal)
    Msg       : string
}
```

| Kind | Retryable | 이유 |
|------|-----------|------|
| `tool_not_found` | false | tool 이름이 잘못된 것이므로 재시도해도 동일 결과 |
| `input_validation_failed` | false | 입력 구조 자체가 잘못됨 |
| `tool_execution_failed` | true | 일시적 오류(네트워크, 타임아웃 등) 가능성 있음 |
| `llm_parse_error` | true | LLM에게 다시 요청하면 달라질 수 있음 |

Phase 5에서 RetryPolicy가 `Retryable` 필드를 기준으로 재시도 여부를 결정한다.

### 5. ToolResult → AgentState 반영

Executor가 반환한 `ToolResult`는 `AgentState.ToolResults`에 추가된다.
`StepCount`가 1 증가하고, 다음 loop에서 `IsFinished`로 종료 여부를 판단한다.

> **Phase 5 예정**: Verifier 도입 후 이 시점에 "결과 충분성 평가" 단계가 추가된다.

### 6. Loop 종료 조건

`IsFinished(plan, state, maxStep)`가 판단한다. plan 반환 직후 단 한 번 호출된다.

| 조건 | FinishReason | 결과 Status |
|------|-------------|-------------|
| Planner가 `finish` ActionType 반환 | `action_finish` | `StatusFinished` |
| Planner가 `respond_directly` 반환 + FinalAnswer 채워짐 | `direct_response` | `StatusFinished` |
| `StepCount >= MaxStep` (기본 10) | `max_step` | `StatusFailed` |
| `Status == StatusFailed` (외부에서 이미 실패 처리됨) | `fatal_error` | `StatusFailed` |

---

## 컴포넌트별 책임 경계

| 컴포넌트 | 하는 것 | 하지 않는 것 |
|----------|---------|-------------|
| Runtime | loop 제어, 종료 판단, retry 조율 | LLM 호출, Tool 실행 |
| Planner | 다음 행동 결정 (ActionType + ToolName + ToolInput) | 실제 실행, 상태 저장 |
| Executor | PlanResult를 ToolRouter로 위임 | 행동 결정, 상태 관리, Tool 직접 호출 |
| ToolRouter | registry 조회 + input 검증 + 실행 위임 + 에러 분류 + 로그 | Tool 내부 로직, 상태 변경 |
| ToolRegistry | Tool 이름 → 구현체 매핑 저장 | 실행, 검증 |
| Tool | 단일 기능 실행 (calculator / weather_mock / search_mock) | 다른 Tool 호출, 상태 변경 |
| AgentError | 에러 유형 + retryable 여부 전달 | 에러 처리 정책 결정 (Phase 5 RetryPolicy 역할) |
| Verifier | 결과 충분성 평가 (Phase 5 예정) | 실행, 재계획 |
| Memory | 상태 영속화 (Phase 4 예정) | 행동 결정 |

---

## Phase별 컴포넌트 추가 계획

```
Phase 1  Runtime + MockPlanner + MockExecutor + AgentState
Phase 2  Tool + ToolRegistry + ToolRouter + 구체 Tool 구현 + AgentError
Phase 3  LLMClient + LLMPlanner + ToolExecutor (ToolRouter 실제 연결)
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

### ToolRouter와 ToolRegistry 분리 (Phase 2)

`ToolRegistry.Get()`은 저장소 역할만 한다.
input 검증, 에러 분류, 로그 출력을 Executor에 직접 두면 Executor의 책임이 과중해지고,
나중에 tool 실행 경로가 여러 개 생길 때 중복 구현이 발생한다.
ToolRouter가 "조회 + 검증 + 실행 위임 + 에러 분류 + 로그"를 하나의 실행 게이트웨이로 캡슐화한다.

### request_id vs session_id (Phase 2)

두 ID 모두 UUID지만 범위가 다르다.

- `session_id`: 사용자 세션 전체. 여러 요청에 걸쳐 동일한 값 유지.
- `request_id`: 단일 요청 1회. 같은 세션에서 요청마다 새로 생성.

로그에서 `session_id`로 대화 전체를, `request_id`로 특정 요청의 tool 실행 흐름만 필터링할 수 있다.
현재 `session_id`는 `"session-dev"` 고정값이며, Phase 4 memory 연동 시 실제 세션으로 교체된다.
