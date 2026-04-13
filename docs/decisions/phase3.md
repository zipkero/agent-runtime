# Phase 3 설계 결정 기록

## 1. LLMPlanner 구현 방식

### 결정: stateless, per-call prompt 조립

LLMPlanner는 매 `Plan()` 호출마다 AgentState와 ToolRegistry로부터 system/user prompt를 동적으로 구성한다. 상태를 내부에 보유하지 않는다.

**왜:**
- Planner 인터페이스 계약(`Plan(ctx, AgentState) → PlanResult`)이 stateless를 강제한다.
- 과거 tool 실행 결과(`ToolResults`)와 다음 step 목표(`NextGoal`)는 AgentState에 있으므로 Planner가 별도로 상태를 관리할 필요가 없다.
- 테스트 시 AgentState 값만 바꾸면 어떤 시나리오든 재현할 수 있다.

**대안으로 고려한 방식:**
- 대화 history를 Planner 내부에서 누적하는 방식 → Phase 4에서 SessionState로 분리할 예정이므로 지금은 도입하지 않는다.

---

## 2. PlanResult JSON Schema 설계 근거

### 필드 구성

| 필드 | 이유 |
|---|---|
| `action_type` | Loop의 분기 기준. `tool_call` / `respond_directly` / `summarize` / `ask_user` / `finish` 5가지로 모든 행동을 표현한다. |
| `tool_name` / `tool_input` | `action_type == tool_call`일 때만 유효. 나머지 경우에는 omit. |
| `reasoning` | LLM 추론 전문. `respond_directly` / `summarize` / `ask_user`에서 FinalAnswer로 사용된다. |
| `reasoning_summary` | 한 줄 요약. 다음 step system prompt에 삽입해 연속성 유지. |
| `confidence` | LLM 자기 평가 점수(0.0~1.0). Phase 5 RetryPolicy에서 재시도 판단에 활용 예정. |
| `next_goal` | 다음 step 목표. prompt_builder가 "이번 step 목표"로 포함해 LLM이 목표를 잃지 않도록 한다. |

### Temperature = 0.0

결정적 JSON 출력이 필요하므로 temperature를 0으로 고정한다. 창의적 응답이 필요한 상황은 현재 scope 밖이다.

---

## 3. Hallucination 방어 전략

### 결정: ToolRegistry 대조 검증 → 파싱 에러로 분류 → 1회 재시도

`action_type == tool_call`일 때, LLM이 반환한 `tool_name`이 ToolRegistry에 등록되지 않은 경우 파싱 에러로 간주한다.

**흐름:**
```
parseAndValidate → tool_name not in registry → error("hallucinated tool name: ...")
→ 1회 재시도 (에러 메시지를 assistant 응답 직후 user 메시지로 삽입)
→ 재시도도 실패 → 에러 반환
```

**왜:**
- LLM이 존재하지 않는 tool을 호출하면 Executor에서 `unknown tool` 에러가 발생한다. 이를 Planner 단계에서 차단하면 에러 경로가 명확해진다.
- 재시도 시 "존재하지 않는 tool 이름을 포함한다"는 지시를 conversation에 추가하므로 LLM이 원인을 파악하고 수정할 수 있다.
- 재시도 횟수를 1회로 제한한 이유: 과도한 재시도는 비용·지연을 키운다. Phase 5에서 RetryPolicy 인터페이스로 교체할 예정이므로 지금은 단순하게 유지한다.

---

## 4. Retry 정책 현황 및 Phase 5 교체 계획

현재 retry는 `LLMPlanner.Plan()` 내부에 하드코딩(최대 1회)되어 있다.

**Phase 5 계획:**
- `RetryPolicy` 인터페이스를 정의하고 LLMPlanner에 주입.
- `confidence` 필드와 결합해 낮은 신뢰도의 응답에 대한 재시도 여부를 정책으로 제어.
- 지수 백오프 등 전략을 교체 가능하게 만든다.

---

## 5. Structured Logger 도입 배경

### 결정: `log/slog` + `observability` 패키지 중앙화

모든 LLM 호출, tool 실행, runtime step에서 `trace_id` / `session_id` / `request_id`를 JSON 로그에 포함한다.

**왜:**
- `log/slog`는 Go 1.21 표준 패키지로 외부 의존이 없다.
- `observability.FromContext(ctx, base)`로 context에서 추적 ID를 자동으로 꺼내 logger에 주입하므로, 각 호출 지점에서 ID를 수동으로 전달하지 않아도 된다.
- `log/slog` 직접 사용을 `observability` 패키지로만 허용하는 규칙은 logger 생성 방식이 나중에 바뀌더라도 (OTel 연동 등) 변경 지점이 하나임을 보장한다.

**Phase 8 계획:**
- 8-3-1: OTel SDK 초기화.
- 8-3-2: `observability` logger에 OTel span의 trace ID를 연동.

---

## 6. ask_user ActionType 처리

CLI 환경에서 `action_type == ask_user`이면 `s.FinalAnswer = plan.Reasoning`으로 질문 문자열을 채운 뒤 즉시 loop를 종료한다.

HTTP API 비동기 대기 메커니즘(사용자 응답을 기다리며 loop를 suspend)은 Phase 7 Task 7-5-1에서 구현한다.

---

## 7. summarize vs respond_directly

두 ActionType 모두 `Reasoning`을 FinalAnswer로 사용하고 Executor를 호출하지 않는다. 의미 차이:

- `respond_directly`: tool 결과 없이 LLM이 직접 답변.
- `summarize`: 이전 ToolResults를 종합해 최종 답변 생성. 다음 step에서 별도 요약 호출 없이 Reasoning 자체가 요약문이 됨.

현재 runtime에서 두 경로의 처리 코드는 동일하나, 나중에 summarize에 별도 prompt를 적용하거나 결과 포맷을 다르게 할 때 분기 기준이 된다.
