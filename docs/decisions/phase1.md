# Phase 1 설계 결정 기록

## 1. AgentState를 값으로 전달

### 결정: Planner.Plan()에 AgentState를 값(value)으로 전달

Planner는 `Plan(ctx, AgentState) (PlanResult, error)` 시그니처를 사용한다. 포인터가 아닌 값 전달.

**왜:**
- Planner는 읽기 전용이다. 상태를 수정하면 안 된다.
- 값 전달이면 Planner 내부에서 AgentState를 변경해도 호출자에 영향이 없다.
- 테스트에서 입력 AgentState가 변조되지 않음을 보장한다.

---

## 2. CurrentPlan 필드 제외 (순환 참조 방지)

### 결정: Phase 1에서 AgentState에 CurrentPlan 필드를 포함하지 않음

`AgentState`가 `PlanResult`를 필드로 가지면 `state → planner → state` 순환 참조가 발생한다.

**Phase 1 선택:** PlanResult를 Runtime 지역변수로만 처리.
**Phase 2에서 해결:** `internal/types` 패키지로 PlanResult를 이동하여 순환 참조 제거.

---

## 3. Finish 조건을 별도 함수로 분리

### 결정: `IsFinished(plan, state, maxStep) FinishResult`

종료 판단 로직을 Runtime.Run() 내부에 인라인으로 작성하지 않고 별도 함수로 추출했다.

**왜:**
- 종료 조건은 `finish` action, max step 초과, fatal error, `respond_directly` 완료 등 여러 가지다.
- 인라인이면 테스트 시 Runtime 전체를 세팅해야 한다. 함수로 분리하면 단위 테스트가 가능하다.
- FinishResult에 Reason을 포함해 종료 원인을 로그로 추적할 수 있다.

---

## 4. MockPlanner / MockExecutor 설계

### 결정: 고정 응답 순서 방식

MockPlanner는 미리 정해진 PlanResult 슬라이스를 순서대로 반환한다. 소진 시 `ActionFinish`를 자동 반환하여 무한루프를 방지한다.

**왜:**
- LLM 없이 loop의 모든 분기(tool_call, respond_directly, finish, max step)를 테스트할 수 있다.
- Phase 3에서 LLM planner로 교체할 때 loop 자체의 회귀를 mock 테스트로 검증한다.
