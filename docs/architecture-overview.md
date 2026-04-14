# Architecture Overview

agent-runtime의 전체 실행 흐름과 컴포넌트 간 관계를 기술한다.

---

## 전체 흐름도 (Phase 4 기준)

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
│  config.Load()  // .env → Config{OpenAIAPIKey, RedisURL, ...}   │
│  logger = observability.New()  // 단일 logger 인스턴스           │
│                                                                  │
│  MemoryManager = DefaultMemoryManager(                           │
│    InMemorySessionRepository,                                    │
│    InMemoryMemoryRepository,                                     │
│  )                                                               │
│                                                                  │
│  // 모든 컴포넌트에 logger 주입                                  │
│  Runtime = NewRuntime(LLMPlanner, ToolExecutor, MemoryManager,   │
│                       maxStep, logger)                            │
│                                                                  │
│  AgentState{                                                     │
│    Request: RequestState{                                        │
│      RequestID: NewRequestID()                                   │
│      UserInput: "서울 날씨 알려줘"                                │
│    }                                                             │
│    Session: &SessionState{                                       │
│      SessionID: "session-dev"                                    │
│    }                                                             │
│    Status: running                                               │
│  }                                                               │
└───────────────────────────┬─────────────────────────────────────┘
                            │  AgentState
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  Runtime.Run(ctx, AgentState)                   [internal/agent] │
│                                                                  │
│  ┌── Memory Load (1회) ────────────────────────────────────┐    │
│  │  MemoryManager.LoadRelevantMemory(ctx, UserInput)        │    │
│  │  → AgentState.RelevantMemories 에 주입                   │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌──────────────────── LOOP ───────────────────────────────┐    │
│  │                                                          │    │
│  │  ① Planner.Plan(ctx, AgentState)                        │    │
│  │         │                                               │    │
│  │         │  반환: PlanResult{                            │    │
│  │         │    ActionType:       "tool_call"              │    │
│  │         │    ToolName:         "weather_mock"           │    │
│  │         │    ToolInput:        map["city": "Seoul"]     │    │
│  │         │    Reasoning:        "날씨 조회가 필요하다"    │    │
│  │         │    ReasoningSummary: "날씨 조회 호출"          │    │
│  │         │    Confidence:       0.95                     │    │
│  │         │    NextGoal:         "결과를 사용자에게 전달"  │    │
│  │         │  }                                            │    │
│  │         ▼                                               │    │
│  │  ②  respond_directly / summarize / ask_user 이면       │    │
│  │      FinalAnswer = plan.Reasoning                       │    │
│  │         │                                               │    │
│  │         ▼                                               │    │
│  │  ③ IsFinished(PlanResult, AgentState, MaxStep)          │    │
│  │         │                                               │    │
│  │         │  종료 조건:                                   │    │
│  │         │   - ActionType == "finish"                    │    │
│  │         │   - ActionType == "respond_directly"          │    │
│  │         │     + FinalAnswer != ""                       │    │
│  │         │   - ActionType == "summarize"                 │    │
│  │         │     + FinalAnswer != ""                       │    │
│  │         │   - ActionType == "ask_user"                  │    │
│  │         │     + FinalAnswer != ""                       │    │
│  │         │   - StepCount >= MaxStep (기본 10)            │    │
│  │         │   - Status == failed                          │    │
│  │         ▼                                               │    │
│  │  ④ Executor.Execute(ctx, PlanResult)                    │    │
│  │         │                                               │    │
│  │         │  반환: ToolResult{                            │    │
│  │         │    ToolName: "weather_mock"                   │    │
│  │         │    Output:   "도시: Seoul | 날씨: 맑음 | ..."  │    │
│  │         │    IsError:  false                            │    │
│  │         │  }                                            │    │
│  │         ▼                                               │    │
│  │  ⑤ AgentState 반영                                      │    │
│  │     CurrentPlan  = PlanResult                           │    │
│  │     LastToolCall = "weather_mock"                       │    │
│  │     Request.ToolResults = append(..., ToolResult)       │    │
│  │     StepCount++                                         │    │
│  │                                                         │    │
│  └──────────────────────────────────────────────────────┘  │    │
│                                                                  │
│  반환: (AgentState, error)                                       │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  cmd/agent-cli/main.go — Memory Save                             │
│                                                                  │
│  FinalAnswer != "" (정상 완료) 이면:                              │
│    MemoryManager.SaveMemory(ctx, Memory{                         │
│      Content: FinalAnswer + ToolResults 요약                     │
│      Tags:    userInput 에서 추출                                │
│    })                                                            │
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
│  ┌─ ④ 구조화 로그 출력 ──────────────────────────────────────┐  │
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

---

## 데이터 흐름 상세

### 1. User Input → Runtime

사용자 입력이 들어오면 `main.go`에서 `AgentState`를 초기화한다.

```
AgentState {
    Request: RequestState{
        RequestID  : NewRequestID()  // UUID v4
        UserInput  : 사용자 입력 문자열
    }
    Session: &SessionState{
        SessionID  : "session-dev"   // 고정값
    }
    Status     : running
    StepCount  : 0
}
```

`RequestID`는 단일 요청 추적용, `SessionID`는 대화 세션 전체 추적용으로 범위가 다르다.

### 2. Runtime → Memory Load → Planner

Runtime은 `Run()` 시작 시 `MemoryManager.LoadRelevantMemory(ctx, userInput)`를 1회 호출해
`AgentState.RelevantMemories`에 저장한다. prompt_builder는 이 필드를 읽어 system prompt에 반영한다.

이후 현재 `AgentState`를 Planner에 전달한다.
Planner는 상태를 분석하고 다음 행동을 결정해 `PlanResult`를 반환한다.

```
PlanResult {
    ActionType       : tool_call | respond_directly | summarize | ask_user | finish
    ToolName         : "calculator"  (ActionType이 tool_call일 때)
    ToolInput        : {"expression": "3 * 7"}
    Reasoning        : "사용자가 수식 계산을 요청했으므로 calculator를 호출한다"
    ReasoningSummary : "수식 계산 호출"
    Confidence       : 0.95
    NextGoal         : "계산 결과를 사용자에게 전달"
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

  ④ 구조화 로그 출력
       성공: INFO  request_id, session_id, tool_name, input, output_summary, is_error, duration_ms
       실패: ERROR request_id, session_id, tool_name, error_kind, error, duration_ms
```

`request_id` / `session_id`는 `context.WithValue`로 전달된다.
호출 전에 `observability.WithRequestID(ctx, state.RequestID)` 형태로 context에 주입해야 로그에 값이 찍힌다.

### 4. AgentError — 에러 분류 체계

```
AgentError {
    Kind      : ErrorKind  // 에러 유형 식별자
    Retryable : bool       // true → loop에서 재시도 가능
                           // false → 즉시 종료 (fatal)
    Msg       : string
}
```

> 코드 위치: `internal/types/errors.go`

| Kind | Retryable | 이유 |
|------|-----------|------|
| `tool_not_found` | false | tool 이름이 잘못된 것이므로 재시도해도 동일 결과 |
| `input_validation_failed` | false | 외부 시스템 또는 사용자가 보낸 입력 구조 자체가 잘못됨 — 재시도로 해결 불가 |
| `llm_parse_error` | true | LLM이 잘못된 JSON 또는 존재하지 않는 tool 이름을 반환한 경우 — 재요청하면 달라질 수 있음. LLM이 생성한 input이 schema와 맞지 않는 경우도 이 Kind로 분류 |
| `tool_execution_failed` | true | 일시적 오류(네트워크 등) 가능성 있음 |
| `tool_timeout` | true | tool 실행이 deadline을 초과한 경우 — 재시도 시 성공 가능 |

> `input_validation_failed`와 `llm_parse_error`의 구분: LLM output 파싱/검증 단계에서 발생한 오류는 `llm_parse_error`(retryable), 외부 입력 검증 단계에서 발생한 오류는 `input_validation_failed`(fatal).

Phase 5에서 RetryPolicy가 `Retryable` 필드를 기준으로 재시도 여부를 결정한다.

### 5. ToolResult → AgentState 반영

Executor가 반환한 `ToolResult`는 `AgentState.Request.ToolResults`에 추가된다.
`CurrentPlan`에 이번 step의 PlanResult가 저장되고, `StepCount`가 1 증가한다.
다음 loop에서 `IsFinished`로 종료 여부를 판단한다.

> **Phase 5 예정**: Verifier 도입 후 이 시점에 "결과 충분성 평가" 단계가 추가된다.

### 6. Loop 종료 조건

`IsFinished(plan, state, maxStep)`가 판단한다. plan 반환 직후 단 한 번 호출된다.

| 조건 | FinishReason | 결과 Status |
|------|-------------|-------------|
| Planner가 `finish` ActionType 반환 | `action_finish` | `StatusFinished` |
| Planner가 `respond_directly` 반환 + FinalAnswer 채워짐 | `direct_response` | `StatusFinished` |
| Planner가 `summarize` 반환 + FinalAnswer 채워짐 | `summarize` | `StatusFinished` |
| Planner가 `ask_user` 반환 + FinalAnswer 채워짐 | `ask_user` | `StatusWaitingInput` |
| `StepCount >= MaxStep` (기본 10) | `max_step` | `StatusFailed` |
| `Status == StatusFailed` (외부에서 이미 실패 처리됨) | `fatal_error` | `StatusFailed` |

### 7. Memory Save (호출자 책임)

`Runtime.Run()` 반환 후 호출자(`main.go` 또는 Worker)가 정상 완료(`FinalAnswer != ""`)일 때
`MemoryManager.SaveMemory(ctx, Memory)`를 호출해 대화 결과를 Long-term Memory에 저장한다.
실패/중단 시에는 저장하지 않는다.

Runtime 내부가 아닌 호출자에서 저장하는 이유는 Runtime이 MemoryManager에 직접 의존하는 범위를 최소화하기 위함이다 (Load는 Run 시작 시 1회, Save는 호출자 책임).

---

## 컴포넌트별 책임 경계

| 컴포넌트 | 하는 것 | 하지 않는 것 |
|----------|---------|-------------|
| Runtime | loop 제어, 종료 판단, Memory Load, retry 조율 | LLM 호출, Tool 실행, Memory Save |
| Planner (LLMPlanner) | AgentState + ToolRegistry로 prompt 조립, 다음 행동 결정 | 실제 실행, 상태 저장 |
| Executor (ToolExecutor) | PlanResult를 ToolRouter로 위임 | 행동 결정, 상태 관리, Tool 직접 호출 |
| ToolRouter | registry 조회 + input 검증 + 실행 위임 + 에러 분류 + 로그 | Tool 내부 로직, 상태 변경 |
| ToolRegistry | Tool 이름 → 구현체 매핑 저장 | 실행, 검증 |
| Tool | 단일 기능 실행 (calculator / weather_mock / search_mock) | 다른 Tool 호출, 상태 변경 |
| AgentError | 에러 유형 + retryable 여부 전달 | 에러 처리 정책 결정 (Phase 5 RetryPolicy 역할) |
| MemoryManager | Session/Memory 저장소 파사드 (Load/Save 위임) | 행동 결정, loop 제어 |
| Verifier | 결과 충분성 평가 (Phase 5 예정) | 실행, 재계획 |

---

## Phase별 컴포넌트 추가 계획

```
Phase 1  Runtime + MockPlanner + MockExecutor + AgentState
Phase 2  Tool + ToolRegistry + ToolRouter + 구체 Tool 구현 + AgentError
Phase 3  LLMClient + LLMPlanner + ToolExecutor (ToolRouter 실제 연결) + observability
Phase 4  RequestState/SessionState 분리 + MemoryRepository + MemoryManager + Config
Phase 5  Verifier + RetryPolicy + FailureHandler
Phase 6  Task + Workflow + ManagerAgent + WorkerAgent
Phase 7  HTTP API + AsyncTaskQueue + Worker
Phase 8  Timeout + CostPolicy + Observability(OTel) + PolicyLayer
```

각 Phase는 이전 Phase의 컴포넌트를 교체하거나 확장하는 방식으로 진행된다.
핵심 loop(`internal/agent/runtime.go`)는 Phase 1에 확정되고, 이후엔 부품만 교체된다.

---

## 패키지 의존 경로

```
internal/types    ← (다른 internal 패키지 참조 없음)
internal/observability ← (다른 internal 패키지 참조 없음)
internal/state   → types
internal/planner → types, state, tools(ToolRegistry), llm, observability
internal/executor → types, tools
internal/tools   → types, observability
internal/memory  → types, state
internal/agent   → types, state, planner, executor, memory, observability
internal/config  ← (다른 internal 패키지 참조 없음)
```

---

## 설계 결정 사항

### AgentState.CurrentPlan 미포함 (Phase 1) → Phase 2에서 해결

`AgentState`에 `CurrentPlan PlanResult` 필드를 넣으면 패키지 순환 참조가 발생한다.

```
state  → planner (CurrentPlan 타입 때문에)
planner → state  (Planner.Plan 인자 타입 때문에)
```

**Phase 1 선택: PlanResult를 Runtime 지역변수로만 처리**

**Phase 2 완료: `internal/types` 패키지로 분리**

`ActionType`, `PlanResult`, `ToolResult`, `AgentError`를 `internal/types`로 이동했다.
`AgentState.CurrentPlan types.PlanResult` 필드가 순환 참조 없이 가능해졌다.

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

### RequestState / SessionState 분리 (Phase 4)

AgentState의 flat 구조를 Request(단일 Run 범위)와 Session(여러 Run 범위)으로 분리했다.
`RequestState`는 `RequestID`, `UserInput`, `ToolResults`를 보유하고,
`SessionState`는 `SessionID`, `RecentContext`를 보유한다.

### Memory 주입/저장 방식 (Phase 4)

Long-term Memory 조회를 LLM이 tool로 호출하는 방식 대신,
Runtime이 `Run()` 시작 시 1회 주입하는 방식을 채택했다.
UserInput이 이미 확정된 시점에 조회하므로 쿼리 기준이 명확하고 루프 내 반복 DB 조회가 없다.
Save는 Runtime 외부 호출자가 담당하여 Runtime의 MemoryManager 의존을 최소화한다.
