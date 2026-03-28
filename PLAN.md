# PLAN.md — 구현 Task 목록

Phase별 상세 Task와 진행 상황을 추적한다.
체크박스 기준: `[x]` 완료 / `[ ]` 미완료

---

## Phase 0 — 준비

### Step 0-1. LLM Provider 확정

- [x] **Task 0-1-1. LLMClient 인터페이스 정의**
  - **무엇**: `LLMClient` 인터페이스 파일 1개 작성
  - **왜**: provider를 고정하기 전에 추상화 경계를 먼저 정의해야 이후 planner 설계 시 구현 의존이 없음
  - **산출물**: `internal/llm/client.go`

- [x] **Task 0-1-2. CompletionRequest / CompletionResponse 타입 정의**
  - **무엇**: LLM 요청/응답 구조체 정의
  - **왜**: 인터페이스만으로는 호출부를 작성할 수 없음. 타입이 확정되어야 stub 구현이 가능함
  - **산출물**: `internal/llm/types.go`

### Step 0-2. 환경설정

- [x] **Task 0-2-1. docker-compose.yml 작성** ✓
  - **무엇**: Redis, Postgres 컨테이너 정의
  - **왜**: Phase 4 이전부터 인프라가 실제로 떠 있어야 연결 테스트 가능
  - **산출물**: `docker-compose.yml`

- [x] **Task 0-2-2. .env.example 작성** ✓
  - **무엇**: 환경변수 목록 문서화 + `.gitignore` 설정
  - **왜**: 실제 `.env`를 레포에 올리지 않으면서 필요한 키 목록을 공유
  - **산출물**: `.env.example`

- [x] **Task 0-2-3. 환경변수 로딩 코드 작성**
  - **무엇**: 앱 시작 시 `.env`를 읽고 누락 변수가 있으면 즉시 에러를 내는 config 패키지
  - **왜**: 환경변수가 없을 때 런타임 중간에 터지는 것을 방지
  - **산출물**: `internal/config/config.go`

### Step 0-3. 프로젝트 초기화

- [x] **Task 0-3-1. 디렉터리 구조 생성**
  - **무엇**: `cmd/agent-cli/`, `internal/agent/`, `internal/planner/`, `internal/executor/`, `internal/state/`, `internal/tools/`, `docs/` 생성
  - **왜**: 경계를 디렉터리로 물리적으로 분리해두어야 이후 패키지 간 의존 방향을 강제할 수 있음
  - **산출물**: 디렉터리 트리

- [x] **Task 0-3-2. 각 패키지 stub 파일 생성 + go build 통과**
  - **무엇**: 각 디렉터리에 `package` 선언만 있는 빈 `.go` 파일 생성
  - **왜**: `go build ./...` 통과 여부로 패키지 경계가 올바른지 확인
  - **산출물**: 각 패키지의 빈 stub 파일

### Step 0-4. 용어 정리

- [x] **Task 0-4-1. 핵심 용어 glossary 작성**
  - **무엇**: Agent, Runtime, Planner, Executor, Tool, Tool Router, Session, Memory, Verifier, Task, Step 각각의 정의
  - **왜**: 용어가 코드 간에 달리 쓰이면 인터페이스 경계 설계 시 혼란 발생
  - **산출물**: `docs/glossary.md`

### Step 0-5. 전체 흐름도

- [x] **Task 0-5-1. 아키텍처 개요 문서 작성**
  - **무엇**: `User Request → Runtime → Planner → Tool Router → Executor → Memory Update → Verifier → Response` 흐름을 텍스트 다이어그램으로 기술
  - **왜**: 각 컴포넌트의 위치와 데이터 흐름을 먼저 그려야 인터페이스 설계 시 경계를 잘못 긋지 않음
  - **산출물**: `docs/architecture-overview.md`

### Step 0-6. 범위 고정

- [x] **Task 0-6-1. 범위 문서 작성**
  - **무엇**: 할 것(QA/Search/Planning형)과 하지 않을 것(브라우저 자동조작, 코드 수정형, 자율 배포) 명시
  - **왜**: 나중에 scope creep을 막기 위해 문서로 고정
  - **산출물**: `docs/scope.md`

---

## Phase 1 — 최소 Agent Loop

### Step 1-1. CLI 입력기

- [x] **Task 1-1-1. main.go 진입점 작성**
  - **무엇**: `cmd/agent-cli/main.go` — stdin에서 한 줄 읽어서 `runtime.Run()` 호출
  - **왜**: loop를 실제로 실행할 진입점이 없으면 테스트가 불가능함
  - **산출물**: `cmd/agent-cli/main.go`

- [x] **Task 1-1-2. RequestID / SessionID 생성 로직**
  - **무엇**: UUID 기반 request ID 생성, session ID는 이 단계에서 상수로 고정
  - **왜**: state에 ID가 없으면 로그 추적이 불가능하고 Phase 4 session 연동 시 연결점이 없음
  - **산출물**: `internal/agent/id.go`

### Step 1-2. AgentState 구조

- [x] **Task 1-2-1. AgentStatus 타입 정의**
  - **무엇**: `running`, `finished`, `failed` 등 상태 열거형 정의
  - **왜**: `AgentState.Status` 필드 타입이 먼저 있어야 `AgentState` struct를 완성할 수 있음
  - **산출물**: `internal/state/status.go`

- [x] **Task 1-2-2. ToolResult 타입 정의**
  - **무엇**: tool 실행 결과를 담는 구조체 정의
  - **왜**: `AgentState.ToolResults`의 원소 타입이 필요하고, Phase 2 Tool 인터페이스와도 공유됨
  - **산출물**: `internal/state/tool_result.go`

- [x] **Task 1-2-3. AgentState struct 정의**
  - **무엇**: `AgentState` struct — RequestID, SessionID, UserInput, LastToolCall, ToolResults, FinalAnswer, StepCount, Status
  - **왜**: loop의 모든 컴포넌트가 이 구조체를 통해 상태를 주고받음. 이것이 없으면 planner/executor 인터페이스 시그니처를 확정할 수 없음
  - **비고**: `CurrentPlan` 필드 제외 — 순환 참조 방지 (Phase 3에서 `internal/types`로 해결 예정, `docs/architecture-overview.md` 참고)
  - **산출물**: `internal/state/agent_state.go`

### Step 1-3. Planner 인터페이스

- [x] **Task 1-3-1. ActionType 상수 정의**
  - **무엇**: `tool_call`, `respond_directly`, `finish` 3개 상수
  - **왜**: PlanResult 타입 정의에 앞서 ActionType이 먼저 있어야 함
  - **산출물**: `internal/planner/action_type.go`

- [x] **Task 1-3-2. PlanResult 타입 정의**
  - **무엇**: action type, selected tool name, tool input, reasoning summary 필드를 갖는 struct
  - **왜**: Planner 인터페이스 시그니처의 반환 타입
  - **산출물**: `internal/planner/plan_result.go`

- [x] **Task 1-3-3. Planner 인터페이스 정의**
  - **무엇**: `Plan(ctx, AgentState) (PlanResult, error)` 인터페이스
  - **왜**: loop가 planner 구현체에 의존하지 않도록 경계를 인터페이스로 정의
  - **비고**: `AgentState`를 값으로 전달 — 읽기 전용 보장, Planner는 상태를 수정하지 않음
  - **산출물**: `internal/planner/planner.go`

- [x] **Task 1-3-4. MockPlanner 구현**
  - **무엇**: 고정된 PlanResult를 순서대로 반환하는 테스트용 planner
  - **왜**: LLM 없이도 loop 동작을 검증하려면 교체 가능한 구현체가 필요함
  - **비고**: Steps 소진 시 `ActionFinish` 자동 반환 — 무한루프 방지
  - **산출물**: `internal/planner/mock_planner.go`

### Step 1-4. Executor 인터페이스

- [x] **Task 1-4-1. Executor 인터페이스 정의**
  - **무엇**: `Execute(ctx, PlanResult) (ToolResult, error)` 인터페이스
  - **왜**: loop가 실행 구현체에 의존하지 않도록 경계를 인터페이스로 정의
  - **비고**: `AgentState`를 받지 않음 — `PlanResult`만으로 실행에 충분, Executor는 Tool 실행 위임 역할
  - **산출물**: `internal/executor/executor.go`

- [x] **Task 1-4-2. MockExecutor 구현**
  - **무엇**: 고정된 ToolResult를 반환하는 테스트용 executor
  - **왜**: Phase 2 Tool Registry 없이도 loop 단위 테스트가 가능해야 함
  - **비고**: Results 소진 시 빈 ToolResult 반환 — 종료 결정은 Planner 역할이므로 Executor는 관여하지 않음
  - **산출물**: `internal/executor/mock_executor.go`

### Step 1-5. Finish 조건 + Runtime Loop

- [x] **Task 1-5-1. Finish 조건 정의**
  - **무엇**: `finish` action / max step 초과 / fatal error / `respond_directly` 완료 4개 조건을 판별 함수로 정의
  - **왜**: 루프 종료 로직이 loop 코드에 인라인으로 흩어지면 테스트와 유지보수가 어려움
  - **비고**: `IsFinished(plan, state, maxStep) FinishResult` — 종료 여부와 이유를 함께 반환. Runtime이 이 결과로 Status 전이를 결정함
  - **산출물**: `internal/agent/finish.go`

- [x] **Task 1-5-2. Runtime.Run() 루프 구현**
  - **무엇**: `plan → execute → state 반영 → finish 판단`을 반복하는 메인 루프
  - **왜**: 이것이 전체 커리큘럼의 핵심 골격. 이후 모든 Phase는 이 루프의 부품을 교체하거나 확장하는 것
  - **산출물**: `internal/agent/runtime.go`

- [x] **Task 1-5-3. Loop 단위 테스트 작성**
  - **무엇**: mock planner + mock executor 조합으로 `tool_call → finish`, `max step 초과` 케이스 테스트
  - **왜**: planner 교체 시에도 loop가 동작하는지 검증. 이 테스트가 없으면 Phase 3에서 LLM planner로 교체 시 회귀 확인 불가
  - **산출물**: `internal/agent/runtime_test.go`

### Phase 1 Exit Criteria

- MockPlanner + MockExecutor 조합으로 `tool_call → finish` 흐름 동작 확인
- max step 초과 시 loop 종료 확인
- AgentState에 StepCount 누적 및 Status 전이(`running` → `finished`/`failed`) 확인
- `go test ./internal/agent/...` 통과

---

## Phase 2 — Tool Registry + Tool Router

### Step 2-1. Tool 인터페이스

- [x] **Task 2-1-1. Schema 타입 정의**
  - **무엇**: tool 입력 스키마를 표현하는 타입 (필드명, 타입, 필수 여부)
  - **왜**: Tool 인터페이스의 `InputSchema()` 반환 타입이 필요하고, Phase 3에서 LLM에게 tool spec을 전달할 때도 사용됨
  - **산출물**: `internal/tools/schema.go`

- [x] **Task 2-1-2. Tool 인터페이스 정의**
  - **무엇**: `Name()`, `Description()`, `InputSchema()`, `Execute(ctx, map[string]any) (ToolResult, error)` 인터페이스
  - **왜**: 모든 tool이 이 인터페이스를 구현하면 registry가 구현체를 몰라도 됨
  - **산출물**: `internal/tools/tool.go`

### Step 2-2. Tool Registry

- [x] **Task 2-2-1. ToolRegistry 인터페이스 정의**
  - **무엇**: `Register(Tool)`, `Get(name) (Tool, error)`, `List() []Tool` 인터페이스
  - **왜**: router가 registry 구현에 의존하지 않도록 경계를 인터페이스로 먼저 정의
  - **산출물**: `internal/tools/registry.go`

- [ ] **Task 2-2-2. InMemoryToolRegistry 구현**
  - **무엇**: map 기반 ToolRegistry 구현체, 미등록 tool 조회 시 명확한 에러 반환
  - **왜**: 실제 동작하는 registry가 있어야 tool을 등록하고 router가 조회할 수 있음
  - **산출물**: `internal/tools/in_memory_registry.go`

- [ ] **Task 2-2-3. calculator tool 구현**
  - **무엇**: 수식 문자열을 받아 계산 결과를 반환하는 tool
  - **왜**: 외부 API 의존 없이 tool 인터페이스와 registry를 검증할 수 있는 가장 단순한 tool
  - **산출물**: `internal/tools/calculator/calculator.go`

- [ ] **Task 2-2-4. weather_mock tool 구현**
  - **무엇**: 도시 이름을 받아 고정된 날씨 데이터를 반환하는 mock tool
  - **왜**: planner가 tool을 선택하는 시나리오를 현실적으로 테스트하기 위해
  - **산출물**: `internal/tools/weather_mock/weather_mock.go`

- [ ] **Task 2-2-5. search_mock tool 구현**
  - **무엇**: 쿼리 문자열을 받아 고정된 검색 결과를 반환하는 mock tool
  - **왜**: Phase 7 검색 시나리오의 기반이 되며, LLM planner가 search를 선택하는 흐름을 테스트
  - **산출물**: `internal/tools/search_mock/search_mock.go`

- [ ] **Task 2-2-6. Registry unit test 작성**
  - **무엇**: 등록 → 조회 성공, 미등록 name 조회 에러 케이스 테스트
  - **왜**: registry는 단순하지만 이후 모든 tool 조회의 기반이므로 에러 케이스 검증 필수
  - **산출물**: `internal/tools/in_memory_registry_test.go`

### Step 2-3. Tool Router

- [ ] **Task 2-3-1. ToolRouter 구현**
  - **무엇**: PlanResult를 받아 registry에서 tool을 조회하고 실행하는 컴포넌트. 미등록 tool, input validation 실패, execute 에러를 각각 다르게 처리
  - **왜**: planner와 tool 실행을 직접 연결하면 planner가 tool 구현에 의존하게 됨. router가 그 사이를 중재
  - **산출물**: `internal/tools/router.go`

- [ ] **Task 2-3-2. ToolRouter unit test 작성**
  - **무엇**: 유효 tool name 라우팅, 잘못된 tool name 에러, input validation 실패 케이스 테스트
  - **왜**: router의 에러 처리가 loop의 retry 정책에 영향을 주므로 각 케이스가 명확히 구분되어야 함
  - **산출물**: `internal/tools/router_test.go`

### Step 2-4. Tool Spec 문서화

- [ ] **Task 2-4-1. docs/tools.md 작성**
  - **무엇**: calculator, weather_mock, search_mock 각각의 name, description, 입력 스키마, 출력 형식, 에러 케이스 정리
  - **왜**: Phase 3에서 LLM system prompt에 tool spec을 넣을 때 이 문서가 기준이 됨
  - **산출물**: `docs/tools.md`

### Step 2-5. Tool 실행 로그

- [ ] **Task 2-5-1. Tool 실행 로그 구현**
  - **무엇**: request id, session id, tool name, input, output summary, duration, error 여부를 구조화된 로그로 출력
  - **왜**: 이 로그가 없으면 Phase 3~6에서 LLM이 어떤 tool을 선택했는지 추적 불가능
  - **산출물**: router 또는 executor 내 로그 출력 코드

### Step 2-6. 에러 타입 분류

- [ ] **Task 2-6-1. AgentError 타입 정의**
  - **무엇**: `retryable`/`fatal` 구분과 `tool_not_found`, `input_validation_failed`, `tool_execution_failed`, `llm_parse_error` 서브타입을 갖는 에러 타입 정의
  - **왜**: Phase 2 ToolRouter에서 이미 에러 유형을 다르게 처리하고 있음. 상수화된 타입이 없으면 Phase 5 retry 정책에서 "어떤 에러에 재시도할지" 판단 기준이 없음. `tool_not_found`는 fatal, `tool_execution_failed`는 retryable 같은 구분이 이 시점에 고정되어야 함
  - **산출물**: `internal/agent/errors.go`

### Phase 2 Exit Criteria

- 미등록 tool 호출 시 `tool_not_found` 에러 반환 확인
- input validation 실패 시 `input_validation_failed` 에러 반환 확인
- `retryable` vs `fatal` 에러 구분 확인
- tool 실행 로그 출력 확인 (request_id, tool_name, duration, error 여부)

---

## Phase 3 — Planner 고도화 / LLM 연결

### Step 3-1. ActionType 확장

- [ ] **Task 3-1-1. ActionType 상수 4개 추가**
  - **무엇**: `ask_user`, `summarize`, `search_memory`, `retry` 추가. 기존 3개는 유지
  - **왜**: LLM이 이 타입들을 선택할 수 있어야 더 현실적인 시나리오 대응 가능
  - **산출물**: `internal/planner/action_type.go` 수정

### Step 3-2. PlanResult 스키마 고정

- [ ] **Task 3-2-1. PlanResult struct 확장**
  - **무엇**: `ReasoningSummary`, `Confidence`, `NextGoal` 필드 추가, JSON 태그 정의
  - **왜**: LLM이 structured output으로 반환할 때 파싱 기준이 되는 타입. 이 시점에 고정하지 않으면 LLM planner 구현 중 계속 바뀜
  - **산출물**: `internal/planner/plan_result.go` 수정

- [ ] **Task 3-2-2. PlanResult JSON schema 문자열 작성**
  - **무엇**: system prompt에 삽입할 JSON schema 문자열 상수 또는 생성 함수
  - **왜**: LLM에게 schema를 명시하지 않으면 hallucinated JSON 비율이 높아짐
  - **산출물**: `internal/planner/schema.go`

### Step 3-3. LLM Planner 연결

- [ ] **Task 3-3-1. OpenAI LLMClient 구현**
  - **무엇**: `LLMClient` 인터페이스를 구현하는 OpenAI API 클라이언트
  - **왜**: Phase 0에서 정의한 인터페이스의 실제 구현체. 이것이 있어야 LLMPlanner가 동작함
  - **산출물**: `internal/llm/openai_client.go`

- [ ] **Task 3-3-2. system prompt 빌더 구현**
  - **무엇**: AgentState와 tool spec 목록을 받아 system prompt 문자열을 생성하는 함수
  - **왜**: prompt 생성 로직이 planner 본체에 인라인으로 있으면 테스트와 수정이 어려움
  - **산출물**: `internal/planner/prompt_builder.go`

- [ ] **Task 3-3-3. LLMPlanner 구현**
  - **무엇**: LLMClient를 주입받아 `Plan()` 메서드에서 LLM 호출 → JSON 파싱 → PlanResult 반환
  - **왜**: mock planner를 실제 LLM 기반으로 교체하는 핵심 단계
  - **산출물**: `internal/planner/llm_planner.go`

- [ ] **Task 3-3-4. invalid JSON 재시도 로직 구현**
  - **무엇**: JSON 파싱 실패 시 LLM 재호출 1회 후 에러 반환
  - **왜**: LLM은 간헐적으로 형식 오류를 낼 수 있음. 1회 재시도로 대부분 해결되지만 무한 루프는 금지
  - **산출물**: `LLMPlanner.parseResult()` 내부 또는 별도 retry 함수

- [ ] **Task 3-3-5. hallucination 방어 로직 구현**
  - **무엇**: PlanResult의 ToolName이 registry에 없을 경우 에러 처리
  - **왜**: LLM이 존재하지 않는 tool 이름을 반환할 수 있으며 이를 그대로 실행하면 런타임 에러
  - **산출물**: LLMPlanner 또는 ToolRouter 내 검증 코드

### Step 3-4. Token Usage 로깅

- [ ] **Task 3-4-1. TokenUsage 타입 정의**
  - **무엇**: prompt tokens, completion tokens, total tokens, 호출 시각, request id를 담는 struct
  - **왜**: 타입이 없으면 로그가 비정형 문자열로 흩어짐. Phase 9 비용 정책의 기반 데이터
  - **산출물**: `internal/llm/token_usage.go`

- [ ] **Task 3-4-2. LLM 호출마다 TokenUsage 기록**
  - **무엇**: LLMClient 또는 LLMPlanner에서 응답 수신 후 TokenUsage를 구조화된 로그로 출력
  - **왜**: LLM 연결 이후 소급 추적 불가능하므로 이 시점에 반드시 시작해야 함
  - **산출물**: `openai_client.go` 또는 `llm_planner.go` 수정

### Phase 3 Exit Criteria

- LLMPlanner가 OpenAI API 호출 후 유효한 PlanResult 반환 확인
- invalid JSON 응답 시 1회 재시도 후 에러 처리 확인
- hallucinated tool name 방어 (registry에 없는 tool 이름 → 에러) 확인
- TokenUsage 로그 출력 확인 (request_id, prompt_tokens, completion_tokens)

---

## Phase 4 — Session / State / Memory 분리

### Step 4-1. Request State

- [ ] **Task 4-1-1. RequestState struct 정의**
  - **무엇**: RequestID, UserInput, ToolResults, ReasoningSteps, StartedAt 필드를 갖는 struct
  - **왜**: `AgentState`에 섞여 있던 요청 범위 데이터를 명시적으로 분리. 이 경계가 없으면 session 데이터와 혼용됨
  - **산출물**: `internal/state/request_state.go`

### Step 4-2. Session State

- [ ] **Task 4-2-1. SessionState struct 정의**
  - **무엇**: SessionID, RecentContext, ActiveGoal, LastUpdated 필드를 갖는 struct
  - **왜**: 연속 대화의 맥락을 담는 단위. Request State와 분리되어야 session ID만으로 이전 대화를 복원할 수 있음
  - **산출물**: `internal/state/session_state.go`

- [ ] **Task 4-2-2. SessionRepository 인터페이스 정의**
  - **무엇**: `Load(ctx, sessionID) (SessionState, error)`, `Save(ctx, sessionID, SessionState) error` 인터페이스
  - **왜**: in-memory와 Redis 구현을 교체할 수 있도록 저장소를 인터페이스로 분리
  - **산출물**: `internal/state/session_repository.go`

- [ ] **Task 4-2-3. InMemorySessionRepository 구현**
  - **무엇**: map 기반 SessionRepository 구현체
  - **왜**: Redis 연결 전에 동작 검증이 필요. 인터페이스가 같으므로 나중에 Redis로 교체 가능
  - **산출물**: `internal/state/in_memory_session_repository.go`

- [ ] **Task 4-2-4. RedisSessionRepository 구현**
  - **무엇**: Redis에 SessionState를 JSON 직렬화하여 저장/조회하는 구현체
  - **왜**: 프로세스 재시작 후에도 세션이 복원되어야 실제 대화 서비스가 가능함
  - **산출물**: `internal/state/redis_session_repository.go`

### Step 4-3. Working Memory

- [ ] **Task 4-3-1. WorkingMemory struct 정의**
  - **무엇**: SearchResults, FilteredResults, Summaries 필드를 갖는 struct
  - **왜**: tool 실행 중간 산출물이 AgentState에 뭉쳐 있으면 multi-agent 시나리오에서 데이터 경계가 불분명해짐
  - **산출물**: `internal/state/working_memory.go`

### Step 4-4. Long-term Memory

- [ ] **Task 4-4-1. Memory struct 정의**
  - **무엇**: ID, UserID, Content, Tags, CreatedAt 필드를 갖는 struct
  - **왜**: Postgres에 저장할 레코드 단위의 타입 정의
  - **산출물**: `internal/memory/memory.go`

- [ ] **Task 4-4-2. MemoryRepository 인터페이스 정의**
  - **무엇**: `Save(ctx, Memory) error`, `LoadByTags(ctx, tags []string, limit int) ([]Memory, error)` 인터페이스
  - **왜**: Postgres 의존을 런타임 코드에서 격리. 테스트 시 in-memory로 교체 가능. 조회 방식을 태그+limit으로 고정해야 나중에 embedding 검색으로 교체할 때 인터페이스 변경 범위가 명확해짐
  - **산출물**: `internal/memory/memory_repository.go`

- [ ] **Task 4-4-3. PostgresMemoryRepository 구현**
  - **무엇**: Postgres에 Memory를 저장하고 `LoadByTags`를 태그 배열 AND 조건 + LIMIT으로 구현하는 구현체
  - **왜**: 장기 기억이 영구 저장소에 없으면 프로세스 재시작마다 소실됨. embedding 검색은 Phase 9 이후 선택 도입
  - **산출물**: `internal/memory/postgres_memory_repository.go`

### Step 4-5. Memory Manager

- [ ] **Task 4-5-1. MemoryManager 인터페이스 정의**
  - **무엇**: `LoadSession`, `SaveSession`, `SaveMemory`, `LoadRelevantMemory` 메서드를 갖는 파사드 인터페이스
  - **왜**: runtime이 session repository와 memory repository를 각각 직접 알면 의존이 넓어짐. 단일 인터페이스로 캡슐화
  - **산출물**: `internal/memory/memory_manager.go`

- [ ] **Task 4-5-2. DefaultMemoryManager 구현**
  - **무엇**: SessionRepository + MemoryRepository를 주입받아 MemoryManager 인터페이스를 구현하는 구조체
  - **왜**: runtime은 MemoryManager만 알면 되고 구체 저장소는 주입으로 교체 가능
  - **산출물**: `internal/memory/default_memory_manager.go`

### Phase 4 Exit Criteria

- 동일 SessionID로 재요청 시 이전 RecentContext 복원 확인
- RequestState / SessionState / WorkingMemory 데이터가 서로 독립적으로 분리 확인
- Redis 재시작 후 세션 복원 확인 (RedisSessionRepository)
- Memory 저장 후 태그 기반 조회 결과 확인

---

## Phase 5 — Verifier / Retry / Concurrency

### Step 5-1. Concurrency 기초

- [ ] **Task 5-1-1. context.WithTimeout 패턴 실습 코드 작성**
  - **무엇**: timeout이 발생했을 때 goroutine이 정리되는 패턴을 단독 테스트로 작성
  - **왜**: Phase 7 병렬 실행에서 goroutine leak이 발생하지 않으려면 이 패턴을 먼저 이해해야 함
  - **산출물**: `internal/agent/concurrency_test.go`

### Step 5-2. Verifier 인터페이스

- [ ] **Task 5-2-1. VerifyStatus / VerifyResult 타입 정의**
  - **무엇**: `done`, `retry`, `fail` 상태를 갖는 열거형과 VerifyResult struct
  - **왜**: Verifier 인터페이스 반환 타입이 먼저 있어야 인터페이스를 정의할 수 있음
  - **산출물**: `internal/verifier/verify_result.go`

- [ ] **Task 5-2-2. Verifier 인터페이스 정의**
  - **무엇**: `Verify(ctx, AgentState) (VerifyResult, error)` 인터페이스
  - **왜**: loop가 verifier 구현에 의존하지 않도록 경계를 인터페이스로 정의
  - **산출물**: `internal/verifier/verifier.go`

- [ ] **Task 5-2-3. SimpleVerifier 구현 및 테스트**
  - **무엇**: FinalAnswer가 비어있으면 `retry`, tool 에러 있으면 `fail`, 그 외 `done` 반환하는 구현체와 unit test
  - **왜**: loop에 verifier를 연결하기 전에 각 케이스가 올바르게 분기되는지 검증
  - **산출물**: `internal/verifier/simple_verifier.go`, `simple_verifier_test.go`

### Step 5-3. Retry Policy

- [ ] **Task 5-3-1. RetryPolicy 인터페이스 정의**
  - **무엇**: `ShouldRetry(err, attempt) bool`, `Delay(attempt) time.Duration` 인터페이스
  - **왜**: retry 로직이 loop에 인라인으로 있으면 유형별로 정책을 다르게 적용하기 어려움
  - **산출물**: `internal/agent/retry_policy.go`

- [ ] **Task 5-3-2. LinearRetryPolicy 구현**
  - **무엇**: 최대 횟수와 고정 대기 시간을 설정할 수 있는 RetryPolicy 구현체
  - **왜**: 가장 단순한 정책으로 먼저 검증. Phase 9에서 더 정교한 정책으로 교체 가능
  - **산출물**: `internal/agent/linear_retry_policy.go`

- [ ] **Task 5-3-3. RetryPolicy unit test 작성**
  - **무엇**: max 3회 초과 시 `ShouldRetry=false` 반환 검증, 각 attempt별 Delay 값 검증
  - **왜**: 무한 재시도 방지가 정책 구현의 핵심이므로 경계 케이스를 반드시 테스트
  - **산출물**: `internal/agent/linear_retry_policy_test.go`

### Step 5-4. Failure 분류

- [ ] **Task 5-4-1. 실패 유형별 처리 분기 구현**
  - **무엇**: tool not found → 종료, malformed params → planner 재호출, timeout → retry, llm parse error → 재생성, empty result → 파라미터 변경 후 재시도 분기를 단일 함수로 정의
  - **왜**: 분기가 여러 곳에 흩어지면 새로운 실패 유형 추가 시 누락이 발생함
  - **산출물**: `internal/agent/failure_handler.go`

### Step 5-5. Reflection

> Verifier와 RetryPolicy가 안정화된 이후 도입. Reflection은 "verifier 판단 전 LLM 자기검증" 역할로, Verifier와 함께 있어야 상호 역할이 명확해진다.

- [ ] **Task 5-5-1. ReflectResult 타입 정의**
  - **무엇**: `Sufficient bool`, `MissingConditions []string`, `Suggestion string` 필드를 갖는 struct
  - **왜**: Reflector 인터페이스 시그니처의 반환 타입
  - **산출물**: `internal/planner/reflect_result.go`

- [ ] **Task 5-5-2. Reflector 인터페이스 정의**
  - **무엇**: `Reflect(ctx, AgentState) (ReflectResult, error)` 인터페이스
  - **왜**: reflection이 loop에 하드코딩되지 않도록 인터페이스로 분리
  - **산출물**: `internal/planner/reflector.go`

- [ ] **Task 5-5-3. LLMReflector 구현**
  - **무엇**: reflection 전용 prompt를 사용해 LLM을 호출하고 ReflectResult를 반환하는 구현체
  - **왜**: planner와 동일한 LLMClient를 재사용하되 prompt가 달라야 함
  - **산출물**: `internal/planner/llm_reflector.go`

- [ ] **Task 5-5-4. Reflection 결과를 AgentState에 반영**
  - **무엇**: `Sufficient=false`일 때 loop가 추가 단계를 진행하도록 Runtime.Run()에 연결
  - **왜**: reflection이 state에 반영되지 않으면 loop 제어에 아무 영향도 주지 않음
  - **산출물**: `internal/agent/runtime.go` 수정

### Phase 5 Exit Criteria

- SimpleVerifier가 `done` / `retry` / `fail` 올바르게 분기 확인
- RetryPolicy max 횟수 초과 시 loop 종료 확인
- `tool_not_found` 에러 → fatal, `tool_execution_failed` 에러 → retry 분기 확인
- `Sufficient=false` reflection 결과 시 loop 추가 진행 확인

---

## Phase 6 — Test Harness Engineering

### Step 6-1. testharness 패키지 구성

- [ ] **Task 6-1-1. Scenario / ExpectedState 타입 정의**
  - **무엇**: 시나리오 이름, 입력, PlanStep 목록, ExecuteStep 목록, 기대 결과를 선언적으로 표현하는 타입
  - **왜**: 테스트마다 setup 코드를 반복하는 대신 시나리오를 데이터로 표현해야 Phase 7 multi-agent 테스트까지 재사용 가능함
  - **산출물**: `testharness/scenario.go`

- [ ] **Task 6-1-2. MockLLMClient 구현**
  - **무엇**: 시나리오 기반으로 LLM 응답을 순서대로 반환하는 mock. 호출 횟수 추적 포함
  - **왜**: Phase 3 LLMPlanner 테스트 시 실제 API 호출 없이 응답을 제어할 수 있어야 함
  - **산출물**: `testharness/mock_llm.go`

- [ ] **Task 6-1-3. AgentHarness 구현**
  - **무엇**: Scenario를 받아 MockPlanner + MockExecutor + Runtime을 조합해 E2E 실행하고 결과를 반환하는 조율자
  - **왜**: 각 테스트에서 Runtime 조합 코드를 중복 작성하지 않도록 단일 진입점 제공
  - **산출물**: `testharness/harness.go`

### Step 6-2. 검증 헬퍼

- [ ] **Task 6-2-1. assert 헬퍼 구현**
  - **무엇**: `AssertState`, `AssertNoError`, `AssertError`, `AssertPlannerCalledTimes`, `AssertToolCalled` 함수 모음. 표준 `testing.T`만 사용
  - **왜**: testify 없이도 명확한 에러 메시지를 제공해야 하고, 외부 의존성을 추가하지 않는 원칙을 유지
  - **산출물**: `testharness/assert.go`

### Step 6-3. 기존 테스트 이관 및 시나리오 검증

- [ ] **Task 6-3-1. runtime_test.go 시나리오를 하네스 기반으로 재작성**
  - **무엇**: Phase 1에서 작성한 `tool_call → finish`, `max step 초과` 케이스를 AgentHarness + Scenario 타입으로 이관
  - **왜**: 하네스 자체가 동작하는지 기존 케이스로 검증하고, 이후 Phase 7 multi-agent 테스트의 패턴을 확립
  - **산출물**: `testharness/harness_test.go`, `internal/agent/runtime_test.go` 정리

### Phase 6 Exit Criteria

- AgentHarness에 Scenario 하나 추가만으로 새 E2E 테스트 케이스 실행 가능 확인
- Phase 1 runtime 테스트가 하네스 기반으로 동일하게 통과 확인
- `AssertToolCalled`, `AssertPlannerCalledTimes` 등 assert 헬퍼로 결과 검증 가능 확인
- `go test ./testharness/...` 통과

---

## Phase 7 — Multi-Agent Orchestration

### Step 7-1. Task Contract

- [ ] **Task 7-1-1. Task / TaskResult 타입 정의**
  - **무엇**: `Task` struct (ID, Type, InputPayload, Dependencies), `TaskResult` struct (TaskID, Output, Error, Latency)
  - **왜**: agent 간 데이터를 주고받는 contract. 이 타입이 없으면 Agent 인터페이스와 Decomposer 인터페이스를 정의할 수 없음
  - **산출물**: `internal/orchestration/task.go`

- [ ] **Task 7-1-2. TaskDecomposer 인터페이스 정의**
  - **무엇**: `Decompose(ctx, userInput) ([]Task, error)` 인터페이스
  - **왜**: Manager가 분해 로직에 의존하지 않도록 경계를 인터페이스로 분리
  - **산출물**: `internal/orchestration/task_decomposer.go`

- [ ] **Task 7-1-3. LLMTaskDecomposer 구현**
  - **무엇**: LLMClient를 사용해 사용자 입력을 Task 목록으로 분해하는 구현체
  - **왜**: 실제 LLM 기반 분해가 있어야 호텔 시나리오 같은 현실적 입력 처리 가능
  - **산출물**: `internal/orchestration/llm_task_decomposer.go`

### Step 7-2. Agent 인터페이스

- [ ] **Task 7-2-1. Agent 인터페이스 정의**
  - **무엇**: `Name() string`, `CanHandle(Task) bool`, `Execute(ctx, Task) (TaskResult, error)` 인터페이스
  - **왜**: Manager가 worker 구현체를 직접 알지 않아도 되도록 경계를 인터페이스로 정의
  - **산출물**: `internal/orchestration/agent.go`

### Step 7-3. Worker Agent 구현

- [ ] **Task 7-3-1. SearchAgent 구현**
  - **무엇**: `hotel_search` task를 처리하는 worker agent
  - **왜**: 시나리오의 첫 번째 단계. search_mock tool을 내부에서 사용
  - **산출물**: `internal/orchestration/search_agent.go`

- [ ] **Task 7-3-2. FilterAgent 구현**
  - **무엇**: `filter_by_price` task를 처리하는 worker agent
  - **왜**: SearchAgent 결과를 입력으로 받아 처리하는 의존 관계가 있는 task 실습
  - **산출물**: `internal/orchestration/filter_agent.go`

- [ ] **Task 7-3-3. RankingAgent 구현**
  - **무엇**: `sort_by_rating` task를 처리하는 worker agent
  - **왜**: FilterAgent 결과를 받아 독립적으로 정렬 처리. 병렬 실행 적합 여부를 판단하는 실습
  - **산출물**: `internal/orchestration/ranking_agent.go`

- [ ] **Task 7-3-4. SummaryAgent 구현**
  - **무엇**: 앞 단계 결과를 받아 LLM으로 요약하는 worker agent
  - **왜**: 마지막 단계에서 LLM 호출이 포함된 task 처리 패턴 실습
  - **산출물**: `internal/orchestration/summary_agent.go`

### Step 7-4. Manager Agent

- [ ] **Task 7-4-1. ManagerAgent 구현**
  - **무엇**: TaskDecomposer와 worker 목록을 주입받아, task를 분해하고 실행 순서를 결정하며 독립 task는 goroutine으로 병렬 실행하고 결과를 병합하는 구조체
  - **왜**: multi-agent orchestration의 핵심. Phase 6에서 구축한 test harness로 검증하고, Phase 5에서 익힌 concurrency 패턴을 여기서 실제 적용
  - **산출물**: `internal/orchestration/manager_agent.go`

- [ ] **Task 7-4-2. ManagerAgent unit test 작성**
  - **무엇**: worker 선택 로직, 병렬 실행 여부, 결과 병합 검증
  - **왜**: manager 로직이 잘못되면 task 순서 오류나 결과 누락이 발생하며 디버깅이 어려움
  - **산출물**: `internal/orchestration/manager_agent_test.go`

### Step 7-5. Multi-Agent 실행 로그

- [ ] **Task 7-5-1. 실행 trace 로그 구현**
  - **무엇**: 호출된 agent 이름, 호출 순서, 각 latency, 실패 지점을 구조화된 로그로 출력
  - **왜**: multi-agent 시나리오는 단일 agent보다 흐름 추적이 복잡하므로 로그가 없으면 디버깅 불가
  - **산출물**: `internal/orchestration/trace.go`

---

## Phase 8 — Runtime 서비스화

### Step 8-1. HTTP API

- [ ] **Task 8-1-1. 요청/응답 타입 정의**
  - **무엇**: `RunRequest`, `RunResponse`, `TaskStatusResponse` struct
  - **왜**: JSON 직렬화 기준이 되는 타입 정의 없이는 핸들러 구현 불가
  - **산출물**: `internal/api/types.go`

- [ ] **Task 8-1-2. HTTP 핸들러 구현**
  - **무엇**: `POST /v1/agent/run`, `GET /v1/tasks/{id}`, `GET /v1/sessions/{id}` 엔드포인트
  - **왜**: CLI 입력기를 HTTP 인터페이스로 교체하는 핵심 단계
  - **산출물**: `internal/api/handler.go`

- [ ] **Task 8-1-3. 핸들러 integration test 작성**
  - **무엇**: `httptest`를 사용해 각 엔드포인트의 요청/응답 검증
  - **왜**: API 계층 변경 시 하위 호환성 깨짐을 조기에 감지
  - **산출물**: `internal/api/handler_test.go`

### Step 8-2. Async Task 상태

- [ ] **Task 8-2-1. AsyncTask 타입 및 상태 전이 구현**
  - **무엇**: `queued`, `running`, `succeeded`, `failed` 상태와 상태 전이 검증 로직
  - **왜**: 잘못된 상태 전이(예: queued → succeeded 직접 전환)를 런타임에 잡아야 함
  - **산출물**: `internal/api/async_task.go`, `async_task_test.go`

### Step 8-3. Queue 구조

- [ ] **Task 8-3-1. TaskQueue 인터페이스 정의**
  - **무엇**: `Enqueue(task)`, `Dequeue() (task, error)` 인터페이스
  - **왜**: in-memory channel과 Redis Stream을 교체할 수 있도록 인터페이스로 먼저 분리
  - **산출물**: `internal/queue/task_queue.go`

- [ ] **Task 8-3-2. InMemoryTaskQueue 구현**
  - **무엇**: buffered channel 기반 TaskQueue 구현체
  - **왜**: Redis 없이도 API 서버 + worker 분리 구조를 검증할 수 있음
  - **산출물**: `internal/queue/in_memory_task_queue.go`

- [ ] **Task 8-3-3. Worker 루프 구현**
  - **무엇**: queue에서 task를 꺼내 runtime.Run()을 호출하고 결과를 저장하는 goroutine
  - **왜**: API 서버와 실행 엔진을 논리적으로 분리하는 핵심 단계
  - **산출물**: `internal/queue/worker.go`

### Step 8-4. Admin / Debug API

- [ ] **Task 8-4-1. Admin 엔드포인트 구현**
  - **무엇**: 최근 task 목록, 실패 task 조회, session dump, tool 호출 통계 엔드포인트
  - **왜**: 운영 중 문제를 API로 조회할 수 없으면 디버깅이 로그 grep에만 의존하게 됨
  - **산출물**: `internal/api/admin_handler.go`

---

## Phase 9 — 운영 고도화

### Step 9-1. Timeout / Cancellation

- [ ] **Task 9-1-1. tool별 timeout 설정 구현**
  - **무엇**: ToolRouter에 per-tool timeout 설정 추가
  - **왜**: tool마다 응답 시간이 다르므로 단일 deadline으로는 과도하게 느리거나 빠르게 종료됨
  - **산출물**: `internal/tools/router.go` 수정

- [ ] **Task 9-1-2. 전체 request deadline 설정**
  - **무엇**: runtime.Run() 진입 시 전체 요청에 대한 context deadline 설정
  - **왜**: tool 개별 timeout만으로는 loop 자체가 무한히 도는 것을 막을 수 없음
  - **산출물**: `internal/agent/runtime.go` 수정

### Step 9-2. 비용 제어

- [ ] **Task 9-2-1. session별 token 누적 추적**
  - **무엇**: Phase 3의 TokenUsage를 session 단위로 합산하는 집계 로직
  - **왜**: 요청별 token이 아닌 session 전체 비용이 실제 운영 비용 단위임
  - **산출물**: `internal/llm/token_tracker.go`

- [ ] **Task 9-2-2. 비용 한도 초과 시 중단 정책 구현**
  - **무엇**: session 누적 token이 임계값 초과 시 loop를 중단하는 정책
  - **왜**: 한도 없이 두면 단일 session이 과도한 비용을 발생시킬 수 있음
  - **산출물**: `internal/agent/cost_policy.go`

### Step 9-3. Observability

- [ ] **Task 9-3-1. structured logging + trace ID 적용**
  - **무엇**: 모든 로그에 trace ID를 포함하는 logger 래퍼
  - **왜**: trace ID 없이는 multi-agent 시나리오에서 요청 단위 로그 추적 불가
  - **산출물**: `internal/observability/logger.go`

- [ ] **Task 9-3-2. OpenTelemetry trace 연결**
  - **무엇**: request → planner → tool → verifier 구간에 OTel span 추가
  - **왜**: latency 병목이 어느 컴포넌트에 있는지 trace 없이는 측정 불가
  - **산출물**: 각 컴포넌트에 OTel span 추가

### Step 9-4. 에러 분류 체계 고도화

- [ ] **Task 9-4-1. 에러 타입 분류 확장**
  - **무엇**: Phase 2에서 정의한 기본 에러 타입에 `user_error`, `system_error`, `provider_error` 분류 추가
  - **왜**: 기본 retryable/fatal 구분은 Phase 2에서 정의됨. 이 단계에서는 알림, 사용자 응답 메시지, 모니터링 레이블에 사용할 운영 관점의 분류를 추가하는 것이 목적
  - **산출물**: `internal/agent/errors.go` 확장

### Step 9-5. Policy Layer

- [ ] **Task 9-5-1. PolicyLayer 구현**
  - **무엇**: tool 사용 제한, 사용자별 max step, 비용 한도를 단일 Policy 인터페이스로 묶는 레이어
  - **왜**: 정책이 여러 곳에 분산되면 정책 변경 시 누락이 생김
  - **산출물**: `internal/agent/policy.go`

---

## Phase 10 — 문서화 / 포트폴리오

### Step 10-1. README 고도화

- [ ] **Task 10-1-1. README 핵심 구조 다이어그램 추가**
  - **무엇**: 텍스트 기반 아키텍처 다이어그램 + 실행 방법 + 예시 시나리오 추가
  - **왜**: README만 읽어도 전체 구조를 파악할 수 있어야 포트폴리오로서 가치가 있음
  - **산출물**: `README.md` 갱신

### Step 10-2. 아키텍처 문서

- [ ] **Task 10-2-1. 컴포넌트별 아키텍처 문서 작성**
  - **무엇**: runtime overview, planner, memory, tool router, multi-agent 각각의 설계 의도와 경계를 설명하는 문서
  - **왜**: 코드만 있으면 설계 의도가 드러나지 않음. 왜 이렇게 나눴는지를 설명해야 설계 역량을 보여줄 수 있음
  - **산출물**: `docs/01-runtime-overview.md`, `docs/02-planner.md`, `docs/03-memory.md`, `docs/04-tool-router.md`, `docs/05-multi-agent.md`

### Step 10-3. 실행 시나리오 문서

- [ ] **Task 10-3-1. 시나리오별 흐름 문서 작성**
  - **무엇**: 날씨 질의, 호텔 검색, 실패 후 retry, multi-agent 흐름을 단계별로 기술하고 실제 실행 로그 예시 포함
  - **왜**: 실제 동작 증거가 없는 포트폴리오는 신뢰도가 낮음. 시나리오 + 로그 조합이 핵심
  - **산출물**: `docs/scenarios/`
