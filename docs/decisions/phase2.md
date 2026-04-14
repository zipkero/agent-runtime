# Phase 2 설계 결정 기록

## 1. ToolRouter와 ToolRegistry 분리

### 결정: Registry는 저장만, Router가 조회 + 검증 + 실행 + 에러 분류 + 로그를 담당

`ToolRegistry.Get()`은 `map[string]Tool` 조회만 수행한다. input 검증, 에러 분류, 로그 출력은 ToolRouter가 담당한다.

**왜:**
- Executor에 이 로직을 두면 Executor의 책임이 과중해진다.
- 나중에 tool 실행 경로가 여러 개 생기면(HTTP handler 등) 중복 구현이 발생한다.
- Router가 단일 실행 게이트웨이로 캡슐화하면 교체/확장 지점이 하나로 유지된다.

---

## 2. AgentError 에러 분류 체계

### 결정: ErrorKind + Retryable 2축 분류

| Kind | Retryable | 이유 |
|------|-----------|------|
| `tool_not_found` | false | tool 이름 자체가 잘못됨 — 재시도해도 동일 결과 |
| `input_validation_failed` | false | 입력 구조가 잘못됨 — 재시도로 해결 불가 |
| `tool_execution_failed` | true | 일시적 오류(네트워크 등) 가능성 |
| `llm_parse_error` | true | LLM 재요청 시 달라질 수 있음 |

**왜:**
- Phase 5 RetryPolicy가 `Retryable` 필드를 기준으로 재시도 여부를 결정한다.
- `input_validation_failed`와 `llm_parse_error`의 구분: LLM output 파싱/검증 단계 오류는 retryable, 외부 입력 검증 오류는 fatal.

---

## 3. PlanResult를 internal/types로 이동

### 결정: 공유 타입을 `internal/types` 패키지로 분리

Phase 1에서 `AgentState.CurrentPlan`을 넣지 못했던 순환 참조 문제를 해결하기 위해 `ActionType`, `PlanResult`, `ToolResult`, `AgentError`를 `internal/types`로 이동했다.

**왜:**
- `state → types ← planner` 방향으로 의존하면 순환이 발생하지 않는다.
- `AgentState.CurrentPlan types.PlanResult` 필드 추가가 가능해진다.

---

## 4. request_id vs session_id

### 결정: 두 ID 모두 UUID이지만 범위가 다름

- `session_id`: 사용자 세션 전체. 여러 요청에 걸쳐 동일한 값 유지.
- `request_id`: 단일 요청 1회. 같은 세션에서 요청마다 새로 생성.

**왜:**
- 로그에서 `session_id`로 대화 전체를, `request_id`로 특정 요청의 tool 실행 흐름만 필터링할 수 있다.
- Phase 4에서 SessionState 분리 시 이 구분이 저장소 키 설계의 기반이 된다.

---

## 5. Input Validation을 Router에서 수행

### 결정: Tool.InputSchema()를 기준으로 Router가 검증

Tool이 `InputSchema() Schema`를 반환하면 Router가 required 필드 누락, 타입 불일치를 검증한다.

**왜:**
- 각 Tool.Execute() 내부에서 개별적으로 검증하면 중복 코드가 발생한다.
- Schema 기반 검증을 Router에 중앙화하면 모든 tool에 일관된 검증이 적용된다.
- Phase 3에서 LLM에게 tool spec을 전달할 때도 동일한 Schema를 사용한다.
