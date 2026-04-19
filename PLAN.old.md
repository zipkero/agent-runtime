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

- [x] **Task 0-2-4. Makefile 기본 타겟 추가**
  - **무엇**: `make build`(`go build ./...`), `make test`(`go test ./...`), `make vet`(`go vet ./...`) 타겟 작성
  - **왜**: Phase 1부터 반복 실행하는 명령을 표준화해야 Phase 4에서 `make test-unit`/`make test-integration` 타겟을 추가할 때 기반이 됨. 지금 만들어두지 않으면 Phase 1~3에서 명령을 매번 직접 입력하거나, Phase 4에서 처음부터 Makefile을 작성하게 됨
  - **비고**: Phase 4 Task 4-0-1에서 이 Makefile에 `test-unit`/`test-integration` 타겟만 추가하면 됨
  - **산출물**: `Makefile`

### Step 0-3. 프로젝트 초기화

- [ ] **Task 0-3-1. 디렉터리 구조 생성**
  - **무엇**: 아래 디렉터리 전체를 한 번에 생성
    - `cmd/agent-cli/` — Phase 1 CLI 진입점
    - `cmd/agent-api/` — Phase 7 HTTP API 서버 진입점 (미리 생성, Phase 7까지 빈 stub)
    - `internal/agent/`, `internal/planner/`, `internal/executor/`, `internal/state/`, `internal/tools/`, `internal/types/`
    - `internal/memory/` — Phase 4 Session + Long-term memory
    - `internal/verifier/` — Phase 5 Verifier + Reflector
    - `internal/observability/` — Phase 3 structured logger, Phase 8 OTel
    - `internal/orchestration/` — Phase 6 Multi-agent
    - `internal/api/` — Phase 7 HTTP 핸들러 + AsyncTask
    - `internal/queue/` — Phase 7 TaskQueue + Worker
    - `internal/config/`, `internal/llm/`
    - `testutil/` — 테스트 전용 mock (프로덕션 코드에서 import 금지)
    - `docs/`, `docs/decisions/`, `docs/scenarios/`
  - **왜**: 경계를 디렉터리로 물리적으로 분리해두어야 이후 패키지 간 의존 방향을 강제할 수 있음. 나중에 추가하면 stub 생성 없이 구현부터 작성하게 되어 go build가 이미 깨진 시점에 경계 위반을 발견하게 됨
  - **산출물**: 디렉터리 트리
  - **비고**: Phase 4까지 필요한 디렉터리만 생성 완료. 미생성: `cmd/agent-api/`, `internal/verifier/`, `internal/orchestration/`, `internal/api/`, `internal/queue/`, `docs/scenarios/` — 해당 Phase 시작 시 생성

- [x] **Task 0-3-2. 각 패키지 stub 파일 생성 + go build 통과**
  - **무엇**: Task 0-3-1에서 생성한 모든 디렉터리에 `package` 선언만 있는 빈 `.go` 파일 생성. `testutil/`은 `package testutil`로 선언
  - **왜**: `go build ./...` 통과 여부로 패키지 경계가 올바른지 확인. stub이 없으면 나중에 추가하는 패키지가 경계 규칙을 처음부터 지키는지 검증 불가
  - **산출물**: 각 패키지의 빈 stub 파일

- [x] **Task 0-3-3. go.mod Go 버전 확정**
  - **무엇**: `go.mod`의 Go 버전을 **1.22 이상**으로 명시적으로 고정. `go version` 로컬 확인 후 `go mod tidy` 실행
  - **왜**: Phase 7에서 `net/http` ServeMux의 path parameter(`{id}`) 지원이 Go 1.22부터 가능함. 지금 버전을 고정하지 않으면 Phase 7에서 라우터를 통째로 교체해야 하는 상황이 생길 수 있음. Phase 0에서 확정해야 이후 모든 Phase가 동일한 환경을 전제할 수 있음
  - **산출물**: `go.mod` Go 버전 1.22+ 확인 및 필요 시 업데이트

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
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase1.md`에 기록

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

- [x] **Task 2-2-2. InMemoryToolRegistry 구현**
  - **무엇**: map 기반 ToolRegistry 구현체, 미등록 tool 조회 시 명확한 에러 반환
  - **왜**: 실제 동작하는 registry가 있어야 tool을 등록하고 router가 조회할 수 있음
  - **산출물**: `internal/tools/in_memory_registry.go`

- [x] **Task 2-2-3. calculator tool 구현**
  - **무엇**: 수식 문자열을 받아 계산 결과를 반환하는 tool
  - **왜**: 외부 API 의존 없이 tool 인터페이스와 registry를 검증할 수 있는 가장 단순한 tool
  - **산출물**: `internal/tools/calculator/calculator.go`

- [x] **Task 2-2-4. weather_mock tool 구현**
  - **무엇**: 도시 이름을 받아 고정된 날씨 데이터를 반환하는 mock tool
  - **왜**: planner가 tool을 선택하는 시나리오를 현실적으로 테스트하기 위해
  - **산출물**: `internal/tools/weather_mock/weather_mock.go`

- [x] **Task 2-2-5. search_mock tool 구현**
  - **무엇**: 쿼리 문자열을 받아 고정된 검색 결과를 반환하는 mock tool
  - **왜**: Phase 7 검색 시나리오의 기반이 되며, LLM planner가 search를 선택하는 흐름을 테스트
  - **산출물**: `internal/tools/search_mock/search_mock.go`

- [x] **Task 2-2-6. Registry unit test 작성**
  - **무엇**: 등록 → 조회 성공, 미등록 name 조회 에러 케이스 테스트
  - **왜**: registry는 단순하지만 이후 모든 tool 조회의 기반이므로 에러 케이스 검증 필수
  - **산출물**: `internal/tools/in_memory_registry_test.go`

### Step 2-3. Tool Router

- [x] **Task 2-3-1. ToolRouter 구현**
  - **무엇**: PlanResult를 받아 registry에서 tool을 조회하고 실행하는 컴포넌트. 미등록 tool, input validation 실패, execute 에러를 각각 다르게 처리
  - **왜**: planner와 tool 실행을 직접 연결하면 planner가 tool 구현에 의존하게 됨. router가 그 사이를 중재
  - **산출물**: `internal/tools/router.go`

- [x] **Task 2-3-2. ToolRouter unit test 작성**
  - **무엇**: 유효 tool name 라우팅, 잘못된 tool name 에러, input validation 실패 케이스 테스트
  - **왜**: router의 에러 처리가 loop의 retry 정책에 영향을 주므로 각 케이스가 명확히 구분되어야 함
  - **산출물**: `internal/tools/router_test.go`

### Step 2-4. Tool Spec 문서화

- [x] **Task 2-4-1. docs/tools.md 작성**
  - **무엇**: calculator, weather_mock, search_mock 각각의 name, description, 입력 스키마, 출력 형식, 에러 케이스 정리
  - **왜**: Phase 3에서 LLM system prompt에 tool spec을 넣을 때 이 문서가 기준이 됨
  - **산출물**: `docs/tools.md`

### Step 2-5. Tool 실행 로그

- [x] **Task 2-5-1. Tool 실행 로그 구현**
  - **무엇**: request id, session id, tool name, input, output summary, duration, error 여부를 구조화된 로그로 출력
  - **왜**: 이 로그가 없으면 Phase 3~6에서 LLM이 어떤 tool을 선택했는지 추적 불가능
  - **산출물**: router 또는 executor 내 로그 출력 코드

### Step 2-6. 에러 타입 분류

- [x] **Task 2-6-1. AgentError 타입 정의**
  - **무엇**: `retryable`/`fatal` 구분과 `tool_not_found`, `input_validation_failed`, `tool_execution_failed`, `llm_parse_error` 서브타입을 갖는 에러 타입 정의
  - **왜**: Phase 2 ToolRouter에서 이미 에러 유형을 다르게 처리하고 있음. 상수화된 타입이 없으면 Phase 5 retry 정책에서 "어떤 에러에 재시도할지" 판단 기준이 없음. `tool_not_found`는 fatal, `tool_execution_failed`는 retryable 같은 구분이 이 시점에 고정되어야 함
  - **산출물**: `internal/types/errors.go`

### Step 2-7. 공유 타입 패키지 분리

- [x] **Task 2-7-1. `internal/types` 패키지 생성 및 PlanResult / ToolResult 이동**
  - **무엇**: `PlanResult`를 `internal/planner`에서, `ToolResult`를 `internal/state`에서 `internal/types`로 이동
  - **왜**: Phase 3에서 `AgentState.CurrentPlan PlanResult` 필드를 추가하면 `state → planner → state` 순환 참조가 발생함. LLMPlanner 구현 이전에 타입 분리를 완료해야 Phase 3 전체 빌드가 안정적임. 이 Task를 Phase 3 중간에 두면 LLMPlanner 구현 도중 전체 빌드가 깨지는 시점이 생김
  - **비고**: `internal/state`, `internal/planner`, `internal/executor`가 모두 `internal/types`를 참조. `internal/types`는 다른 internal 패키지를 참조하지 않음. **파급 주의**: `PlanResult`를 참조하는 `router.go`, `executor.go`, `mock_executor.go`, `runtime.go`, `finish.go`, `planner/*.go` 전체 수정 필요. 이 Task 완료 후 `go build ./...` + `go test ./...` 전체 통과를 반드시 확인하고 Phase 3으로 진행한다
  - **산출물**: `internal/types/plan_result.go`, `internal/types/tool_result.go`, 기존 참조 경로 수정

### Phase 2 Exit Criteria

- 미등록 tool 호출 시 `tool_not_found` 에러 반환 확인
- input validation 실패 시 `input_validation_failed` 에러 반환 확인
- `retryable` vs `fatal` 에러 구분 확인
- tool 실행 로그 출력 확인 (request_id, tool_name, duration, error 여부)
- `internal/types` 패키지 분리 후 `go build ./...` + `go test ./...` 전체 통과 확인
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase2.md`에 기록

---

## Phase 3 — Planner 고도화 / LLM 연결

### Step 3-0. Phase 3 사전 준비

- [x] **Task 3-0-1. AgentState에 CurrentPlan 필드 추가**
  - **무엇**: `internal/state/agent_state.go`에 `CurrentPlan types.PlanResult` 필드 추가. Phase 2 Task 2-7-1에서 `PlanResult`가 `internal/types`로 이동되어 있으므로 `state → types` 의존만 발생함(순환 없음)
  - **왜**: Phase 1 Task 1-2-3 비고에서 "순환 참조 방지를 위해 Phase 3에서 internal/types로 해결 예정"이라고 명시됐음. `AgentState.CurrentPlan`이 없으면 Runtime loop가 직전 플래닝 결과를 state에 저장하지 못하고, LLMPlanner의 prompt_builder가 "이미 시도한 action"을 system prompt에 포함할 수 없음
  - **비고**: `go build ./...` + `go test ./...` 전체 통과 확인 후 Task 3-0-2로 진행. `Runtime.Run()` loop의 `④ AgentState 반영` 단계에서 `state.CurrentPlan = plan` 대입 로직도 함께 추가
  - **산출물**: `internal/state/agent_state.go` 수정, `internal/agent/runtime.go` 수정 (CurrentPlan 대입)

- [x] **Task 3-0-2. Phase 3 LLM 연동 테스트 전략 수립**
  - **무엇**: Phase 3에서 실제 OpenAI API를 호출하는 테스트 파일에 `//go:build integration` 태그 적용 규칙을 Phase 4(Task 4-0-1)보다 앞당겨 먼저 수립. `Makefile`의 기존 `make test` 타겟이 integration 테스트를 제외하도록 `-tags integration` 제외 옵션 추가
  - **왜**: Task 3-4-1(OpenAI LLMClient)과 Phase 3 Exit Criteria의 "LLMPlanner → OpenAI API 호출 end-to-end 확인"은 실제 API 키가 필요함. 이를 일반 `go test ./...` 에 포함시키면 API 키 없는 환경(CI, 다른 개발 머신)에서 즉시 실패함. Phase 4 Task 4-0-1보다 먼저 규칙을 적용해야 Phase 3 파일부터 일관성이 생김
  - **비고**: Phase 4 Task 4-0-1은 이 Task에서 수립한 규칙 위에 `make test-integration` 타겟만 추가하면 됨. GitHub Actions CI(Phase 9 Task 9-0-1)는 `make test`(unit only)만 실행
  - **산출물**: `Makefile` 수정 (`make test` 타겟에 `-tags` 제외 옵션 추가)

### Step 3-1. ActionType 확장

- [x] **Task 3-1-1. ActionType 상수 2개 추가**
  - **무엇**: `ask_user`, `summarize` 추가. 기존 3개는 유지
  - **왜**: LLM이 이 타입들을 선택할 수 있어야 더 현실적인 시나리오 대응 가능
  - **비고**: `retry`는 Runtime/RetryPolicy의 루프 제어 정책 (Phase 5에서 별도 구현). `ask_user`는 Phase 3에서 Runtime loop가 만나면 즉시 `respond_directly`로 대체 처리(loop 종료)하며, Phase 8 HTTP API 환경에서의 비동기 사용자 입력 대기 메커니즘은 Phase 8에서 별도 설계한다. Long-term Memory 조회는 Tool이 아닌 Runtime이 Run() 시작 시 UserInput 기반으로 1회 수행하는 방식으로 결정 (Phase 4 Task 4-6-1 참고)
  - **산출물**: `internal/planner/action_type.go` 수정

- [x] **Task 3-1-2. Runtime loop에 `summarize` ActionType 처리 분기 추가**
  - **무엇**: `Runtime.Run()` loop에서 ActionType이 `summarize`일 때의 처리 로직 구현. Executor를 호출하지 않고 `AgentState.ToolResults` 전체를 요약 입력으로 사용해 `respond_directly`와 동일하게 loop를 종료
  - **왜**: ActionType을 추가하면 Runtime loop에서 반드시 처리 분기가 있어야 함. 누락 시 `summarize`를 받은 루프가 정의되지 않은 동작을 함
  - **산출물**: `internal/agent/runtime.go` 수정

- [x] **Task 3-1-3. Runtime loop에 `ask_user` ActionType 처리 분기 추가**
  - **무엇**: `Runtime.Run()` loop에서 ActionType이 `ask_user`일 때, `FinalAnswer`에 LLM이 생성한 질문 문자열을 채우고 `respond_directly`와 동일하게 loop를 즉시 종료하는 분기 추가
  - **왜**: Task 3-1-1 비고에 "CLI 환경에서 ask_user → respond_directly로 대체 처리(loop 종료)"를 정책으로 명시했지만 구현 Task가 없었음. 이 분기가 없으면 LLMPlanner가 `ask_user`를 선택했을 때 loop가 undefined behavior를 보임
  - **비고**: HTTP API 환경에서의 비동기 대기 메커니즘은 Phase 7 Task 7-5-1에서 구현. 이 Task는 CLI 경로만 대상으로 함
  - **산출물**: `internal/agent/runtime.go` 수정

### Step 3-2. PlanResult 스키마 고정

- [x] **Task 3-2-1. PlanResult struct 확장**
  - **무엇**: `ReasoningSummary`, `Confidence`, `NextGoal` 필드 추가, JSON 태그 정의
  - **왜**: LLM이 structured output으로 반환할 때 파싱 기준이 되는 타입. 이 시점에 고정하지 않으면 LLM planner 구현 중 계속 바뀜
  - **산출물**: `internal/types/plan_result.go` 수정 (Phase 2 Task 2-7-1에서 이동됨. `internal/planner/plan_result.go` 아님)

- [x] **Task 3-2-2. PlanResult JSON schema 문자열 작성**
  - **무엇**: system prompt에 삽입할 JSON schema 문자열 상수 또는 생성 함수
  - **왜**: LLM에게 schema를 명시하지 않으면 hallucinated JSON 비율이 높아짐
  - **산출물**: `internal/planner/schema.go`

### Step 3-3. MockLLMClient (테스트 인프라)

- [x] **Task 3-3-1. MockLLMClient 구현**
  - **무엇**: 시나리오 기반으로 LLM 응답을 순서대로 반환하는 mock. 호출 횟수 추적 포함
  - **왜**: LLMPlanner 테스트 시 실제 OpenAI API 호출 없이 응답을 제어할 수 있어야 함. 비용/속도/비결정성 문제를 피하고, 실패 케이스(invalid JSON, hallucinated tool name)를 안정적으로 재현해야 함
  - **비고**: Phase 6 Multi-Agent 테스트에서도 재사용됨
  - **산출물**: `testutil/mock_llm.go`

### Step 3-4. LLM Planner 연결

- [x] **Task 3-4-1. OpenAI LLMClient 구현**
  - **무엇**: `LLMClient` 인터페이스를 구현하는 OpenAI API 클라이언트
  - **왜**: Phase 0에서 정의한 인터페이스의 실제 구현체. 이것이 있어야 LLMPlanner가 동작함
  - **비고**: LLM API 호출 시 `context.WithTimeout`으로 per-call deadline 설정 필수. timeout 없이는 LLM 응답 지연 시 goroutine이 무기한 대기함. Phase 8(Task 8-1-2)의 전체 request deadline과 별개로, 개별 LLM 호출 단위 timeout을 이 시점에 적용
  - **산출물**: `internal/llm/openai_client.go`

- [x] **Task 3-4-2. system prompt 빌더 구현**
  - **무엇**: AgentState와 tool spec 목록을 받아 system prompt 문자열을 생성하는 함수
  - **왜**: prompt 생성 로직이 planner 본체에 인라인으로 있으면 테스트와 수정이 어려움
  - **산출물**: `internal/planner/prompt_builder.go`

- [x] **Task 3-4-3. LLMPlanner 구현**
  - **무엇**: LLMClient를 주입받아 `Plan()` 메서드에서 LLM 호출 → JSON 파싱 → PlanResult 반환
  - **왜**: mock planner를 실제 LLM 기반으로 교체하는 핵심 단계
  - **산출물**: `internal/planner/llm_planner.go`

- [x] **Task 3-4-4. ToolExecutor 구현**
  - **무엇**: `internal/executor/tool_executor.go` 구현. `Execute(ctx, PlanResult)`에서 `ToolRouter.Route()`를 실제로 호출하는 Executor. `cmd/agent-cli/main.go`의 Runtime 조립 시 ToolExecutor를 주입하도록 변경
  - **왜**: `architecture-overview.md`에 "Phase 3: ToolExecutor (ToolRouter 실제 연결)"이 명시되어 있음. LLMPlanner가 tool_call PlanResult를 반환해도 MockExecutor가 그대로라면 실제 tool이 실행되지 않아 end-to-end 검증이 불가능함
  - **비고**: ToolRouter는 Phase 2에서 이미 완성됨. MockExecutor는 삭제하지 않고 테스트용으로 유지한다 — `runtime_test.go`를 포함한 기존 단위 테스트는 MockExecutor를 계속 주입해 사용하며, 운영 경로(`main.go`)에서만 ToolExecutor로 전환함
  - **산출물**: `internal/executor/tool_executor.go`, `cmd/agent-cli/main.go` 수정

- [x] **Task 3-4-5. invalid JSON 재시도 로직 구현**
  - **무엇**: JSON 파싱 실패 시 LLM 재호출 1회 후 에러 반환
  - **왜**: LLM은 간헐적으로 형식 오류를 낼 수 있음. 1회 재시도로 대부분 해결되지만 무한 루프는 금지
  - **구현**: `LLMPlanner.Plan()` 내부에서 `parseAndValidate()` 실패 시 대화 이력(bad response + 수정 요청 메시지)을 포함한 재시도 요청 전송. 재시도도 실패하면 `types.PlanResult{}, error` 반환
  - **산출물**: `internal/planner/llm_planner.go`

- [x] **Task 3-4-6. hallucination 방어 로직 구현**
  - **무엇**: LLMPlanner에서 PlanResult 파싱 직후 ToolName이 registry에 등록된 이름인지 선제 검증. 미등록이면 재시도, 재시도도 실패하면 에러 반환
  - **왜**: ToolRouter의 `tool_not_found` 처리(Phase 2)는 fatal 에러로 즉시 종료. LLM hallucination에 의한 잘못된 tool 이름은 재시도하면 달라질 수 있으므로 retryable로 처리해야 함. 두 검증의 에러 분류가 다르기 때문에 LLMPlanner 레벨의 선제 검증이 별도로 필요
  - **설계 결정**: 파싱/hallucination 실패 시 `PlanResult.ActionType`에 에러 코드를 채우지 않고 `types.PlanResult{}, error`를 반환한다. `ActionType`은 LLM이 선택하는 행동 유형이며, 플래너 내부 에러 분류를 여기에 섞으면 타입의 의미가 오염됨. 에러는 `error` 반환값으로만 전달하는 것이 Go 관용구에 맞음
  - **산출물**: `internal/planner/llm_planner.go` 내 `parseAndValidate()`, `isRegisteredTool()` (ToolRouter는 변경 없음)

- [x] **Task 3-4-7. LLMPlanner unit test 작성**
  - **무엇**: MockLLMClient(Task 3-3-1)를 사용해 6개 케이스 검증
    - 유효 respond_directly 파싱 성공 (1회 호출)
    - 유효 tool_call 파싱 성공 (등록된 tool, 1회 호출)
    - invalid JSON → 재시도 성공 (2회 호출)
    - invalid JSON → 재시도도 실패 → error 반환 (2회 호출)
    - hallucinated tool → 재시도 성공 (2회 호출)
    - hallucinated tool → 재시도도 실패 → error 반환 (2회 호출)
  - **왜**: Phase 5(Task 5-3-4)에서 LLMPlanner 내부 하드코딩 retry를 RetryPolicy로 교체할 때 이 테스트가 회귀 보호 역할을 함
  - **산출물**: `internal/planner/llm_planner_test.go`

### Step 3-5. Token Usage 로깅

- [x] **Task 3-5-1. TokenUsage 타입 정의**
  - **무엇**: prompt tokens, completion tokens, total tokens, 호출 시각, request id를 담는 struct
  - **왜**: 타입이 없으면 로그가 비정형 문자열로 흩어짐. Phase 9 비용 정책의 기반 데이터
  - **산출물**: `internal/llm/token_usage.go`

- [x] **Task 3-5-2. LLM 호출마다 TokenUsage 기록**
  - **무엇**: LLMClient 또는 LLMPlanner에서 응답 수신 후 TokenUsage를 구조화된 로그로 출력
  - **왜**: LLM 연결 이후 소급 추적 불가능하므로 이 시점에 반드시 시작해야 함
  - **산출물**: `openai_client.go` 또는 `llm_planner.go` 수정

### Step 3-6. 기본 Structured Logger 도입

- [x] **Task 3-6-1. Logger 인터페이스 및 기본 구현체 작성**
  - **무엇**: `trace_id`, `session_id`, `request_id`를 기본 필드로 포함하는 structured logger 래퍼. Go 표준 `log/slog` 기반으로 JSON 출력 형식 지원
  - **왜**: Phase 3에서 LLM 호출이 시작되면 어떤 요청이 어떤 플래너 결정을 내렸는지 로그 없이 추적이 불가능하다. Phase 8의 OTel span 연동 전까지의 디버깅 기반을 이 시점에 확보해야 Phase 4~7에서 실질적으로 활용 가능함
  - **비고**: `log/slog`는 Go 1.21 표준 패키지이므로 외부 의존 없음. Phase 8 Task 8-3-2는 이 logger에 OTel span trace ID를 연동하는 것으로 범위가 조정됨 (8-3-1은 OTel SDK 초기화, 8-3-2가 logger 연동). LLMPlanner, ToolRouter, Runtime의 주요 진입/종료 지점에서 이 logger를 사용하도록 교체

### Step 3-7. 설계 결정 문서화

- [x] **Task 3-7-1. Phase 3 설계 결정 기록**
  - **무엇**: LLMPlanner 구현 방식, PlanResult JSON schema 설계 근거, hallucination 방어 전략, structured logger 도입 배경을 `docs/decisions/phase3.md`에 기록
  - **왜**: 코드만으로는 "왜 이렇게 설계했는지"가 드러나지 않음. 특히 LLMPlanner의 retry 정책(Phase 5에서 RetryPolicy로 교체 예정)과 hallucination 방어의 설계 근거는 나중에 되돌아볼 때 중요한 기준점이 됨
  - **산출물**: `docs/decisions/phase3.md`

### Phase 3 Exit Criteria

- LLMPlanner가 OpenAI API 호출 후 유효한 PlanResult 반환 확인
- ToolExecutor가 LLMPlanner의 tool_call 결과를 받아 실제 ToolRouter를 통해 tool 실행 확인 (end-to-end)
- invalid JSON 응답 시 1회 재시도 후 에러 처리 확인
- hallucinated tool name 방어 (registry에 없는 tool 이름 → 에러) 확인
- `ask_user` ActionType 발생 시 loop가 즉시 종료되고 FinalAnswer에 질문 문자열이 채워지는 것 확인 (CLI 경로)
- TokenUsage 로그 출력 확인 (request_id, prompt_tokens, completion_tokens)
- 모든 LLM 호출 및 tool 실행 로그에 trace_id, session_id, request_id 포함 확인 (structured logger)
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase3.md`에 기록 (Task 3-7-1)

---

## Phase 4 — Session / State / Memory 분리

### Step 4-0. 통합 테스트 인프라 준비

- [x] **Task 4-0-1. 통합 테스트 타겟 추가**
  - **무엇**: Phase 0 Task 0-2-4에서 만든 `Makefile`에 `make test-unit`(`go test ./...`, integration 태그 제외)과 `make test-integration`(`go test -tags integration ./...`, `docker-compose up` 전제) 타겟 추가. `README.md`에 로컬 실행 전제 조건 명시
  - **왜**: Phase 4부터 Redis/Postgres 실제 연결이 필요한 통합 테스트가 등장함. `//go:build integration` 태그 규칙 자체는 Phase 3 Task 3-0-2에서 이미 수립됨. 이 Task는 인프라 의존 테스트용 `make test-integration` 타겟 추가에 집중
  - **비고**: Phase 4, 5, 7의 통합 테스트 파일 작성 시마다 파일 상단에 `//go:build integration` 태그 적용. GitHub Actions CI(Phase 9 Task 9-0-1)는 `make test-unit`만 실행
  - **산출물**: `Makefile` 수정 (test-unit/test-integration 타겟 추가), `README.md` 일부 갱신

- [x] **Task 4-0-2. Redis/Postgres 클라이언트 의존성 추가**
  - **무엇**: `go get github.com/redis/go-redis/v9`(또는 동등한 Redis 클라이언트)와 `go get github.com/jackc/pgx/v5`(또는 동등한 Postgres 드라이버) 실행 후 `go mod tidy`
  - **왜**: Task 4-2-4(RedisSessionRepository)와 Task 4-4-3(PostgresMemoryRepository) 구현 전에 의존성이 `go.mod`에 없으면 구현 파일 작성 즉시 빌드가 깨짐. 두 Task 직전에 한 번에 추가하는 것보다 Phase 4 진입 시 먼저 추가해야 이후 모든 Task의 `go build ./...` 확인이 일관됨
  - **비고**: `github.com/redis/go-redis/v9 v9.18.0`, `github.com/jackc/pgx/v5 v5.9.1` 선택
  - **산출물**: `go.mod`, `go.sum` 갱신

### Step 4-1. Request State

- [x] **Task 4-1-1. RequestState struct 정의**
  - **무엇**: RequestID, UserInput, ToolResults, ReasoningSteps, StartedAt 필드를 갖는 struct
  - **왜**: `AgentState`에 섞여 있던 요청 범위 데이터를 명시적으로 분리. 이 경계가 없으면 session 데이터와 혼용됨
  - **산출물**: `internal/state/request_state.go`

- [x] **Task 4-1-2. AgentState aggregator 구조 결정 및 적용**
  - **무엇**: `AgentState`를 `RequestState + SessionState`를 포함하는 aggregator struct로 재정의. `Runtime.Run()` 시그니처(`Run(ctx, AgentState)`)는 유지하되 내부 필드 구조만 변경
  - **왜**: Phase 1에서 확정한 loop 시그니처를 변경하지 않으면서 상태 분리를 달성하는 방법. 시그니처 변경 시 Planner/Executor 인터페이스 전체 연쇄 변경이 발생하므로 aggregator 패턴으로 파급을 최소화
  - **산출물**: `internal/state/agent_state.go` 수정 (RequestState, SessionState 포함 구조로 변경)

- [x] **Task 4-1-3. AgentState 구조 변경에 따른 인터페이스 및 테스트 수정**
  - **무엇**: `AgentState` 필드 구조 변경으로 인해 영향을 받는 Planner 인터페이스, Executor 인터페이스, MockPlanner, MockExecutor, `runtime_test.go` 일괄 수정 및 `go test ./...` 통과 확인
  - **왜**: Phase 1 Exit Criteria를 보호하는 `runtime_test.go`가 AgentState 구조 변경으로 컴파일 오류 또는 동작 오류가 발생할 수 있음. 회귀 검증 없이 넘어가면 Phase 5 이후에 문제가 드러남
  - **산출물**: `internal/planner/planner.go`, `internal/executor/executor.go`, mock 파일들, `internal/agent/runtime_test.go` 수정

### Step 4-2. Session State

- [x] **Task 4-2-1. SessionState struct 정의**
  - **무엇**: SessionID, RecentContext, ActiveGoal, LastUpdated 필드를 갖는 struct
  - **왜**: 연속 대화의 맥락을 담는 단위. Request State와 분리되어야 session ID만으로 이전 대화를 복원할 수 있음
  - **산출물**: `internal/state/session_state.go`

- [x] **Task 4-2-2. SessionRepository 인터페이스 정의**
  - **무엇**: `Load(ctx, sessionID) (SessionState, error)`, `Save(ctx, sessionID, SessionState) error` 인터페이스
  - **왜**: in-memory와 Redis 구현을 교체할 수 있도록 저장소를 인터페이스로 분리
  - **산출물**: `internal/state/session_repository.go`

- [x] **Task 4-2-3. InMemorySessionRepository 구현**
  - **무엇**: map 기반 SessionRepository 구현체
  - **왜**: Redis 연결 전에 동작 검증이 필요. 인터페이스가 같으므로 나중에 Redis로 교체 가능
  - **산출물**: `internal/state/in_memory_session_repository.go`

- [x] **Task 4-2-4. RedisSessionRepository 구현**
  - **무엇**: Redis에 SessionState를 JSON 직렬화하여 저장/조회하는 구현체
  - **왜**: 프로세스 재시작 후에도 세션이 복원되어야 실제 대화 서비스가 가능함
  - **비고**: Phase 4 Exit Criteria의 "Redis 재시작 후 세션 복원" 검증을 위해 `docker-compose.yml`의 Redis 서비스에 `--appendonly yes` 옵션을 추가해 AOF persistence를 활성화해야 함
  - **산출물**: `internal/state/redis_session_repository.go`, `docker-compose.yml` 수정 (AOF 활성화)

- [x] **Task 4-2-5. SessionRepository integration test 작성**
  - **무엇**: InMemorySessionRepository와 RedisSessionRepository에서 동일한 테스트 케이스(저장 → 조회, 없는 ID 조회 에러)를 실행해 인터페이스 호환성 검증. Redis 재시작 후 복원 케이스는 RedisSessionRepository 전용 테스트로 분리
  - **왜**: Phase 4 Exit Criteria의 "Redis 재시작 후 세션 복원 확인"이 테스트 코드로 뒷받침되어야 함. Phase 5에서 AgentState 구조가 변경될 경우 SessionRepository 직렬화 동작의 회귀 보호도 필요
  - **산출물**: `internal/state/session_repository_test.go`

### Step 4-4. Long-term Memory

- [x] **Task 4-4-1. Memory struct 정의**
  - **무엇**: ID, UserID, Content, Tags, CreatedAt 필드를 갖는 struct
  - **왜**: Postgres에 저장할 레코드 단위의 타입 정의
  - **비고**: `Memory` struct는 `internal/types/memory.go`에 정의한다. `internal/state`에서 `AgentState.RelevantMemories []types.Memory` 필드를 사용하려면 `state → memory` 의존이 생기므로 `internal/types`가 유일한 경계 안전 위치임. PlanResult, ToolResult와 동일한 이유 (Phase 2 Task 2-7-1 참고)
  - **산출물**: `internal/types/memory.go` (`internal/memory/memory.go` 아님)

- [x] **Task 4-4-2. MemoryRepository 인터페이스 정의**
  - **무엇**: `Save(ctx, Memory) error`, `LoadByTags(ctx, tags []string, limit int) ([]Memory, error)` 인터페이스
  - **왜**: Postgres 의존을 런타임 코드에서 격리. 테스트 시 in-memory로 교체 가능. 조회 방식을 태그+limit으로 고정해야 나중에 embedding 검색으로 교체할 때 인터페이스 변경 범위가 명확해짐
  - **비고**: `LoadByTags`는 **OR 조건** (태그 중 하나라도 포함된 항목 조회). AND 조건은 결과가 지나치게 좁아져 실용성이 없음. Phase 9에서 embedding 검색으로 교체 시 인터페이스 시그니처는 유지하되 내부 구현만 교체
  - **산출물**: `internal/memory/memory_repository.go`

- [x] **Task 4-4-2-b. InMemoryMemoryRepository 구현**
  - **무엇**: 슬라이스 기반 MemoryRepository 구현체. `Save`는 슬라이스에 append, `LoadByTags`는 OR 조건(`tags` 중 하나라도 일치)으로 필터링 후 limit 적용
  - **왜**: SessionRepository가 InMemory → Redis 순서를 따른 것과 동일한 이유. PostgresMemoryRepository(Task 4-4-3)가 완성되기 전에 MemoryManager(Task 4-5-2)를 단위 테스트하려면 Postgres 없이 동작하는 구현체가 필요함
  - **산출물**: `internal/memory/in_memory_memory_repository.go`

- [x] **Task 4-4-2-c. Postgres 스키마 초기화 코드 작성**
  - **무엇**: 앱 시작 시 `memories` 테이블(`id UUID`, `user_id TEXT`, `content TEXT`, `tags TEXT[]`, `created_at TIMESTAMPTZ`)과 태그 검색용 GIN 인덱스를 `CREATE TABLE IF NOT EXISTS`로 생성하는 `migrate` 함수 작성
  - **왜**: Task 4-4-3에서 PostgresMemoryRepository를 구현하기 전에 테이블이 없으면 실행 자체가 불가능함. 새 마이그레이션 라이브러리 의존 없이 Go 표준 `database/sql`로 처리하는 방식을 사용해 커리큘럼 학습 목표에 집중
  - **비고**: `internal/memory/migrate.go`에 `Migrate(db *sql.DB) error` 함수로 작성. `cmd/agent-cli/main.go` 또는 앱 초기화 경로에서 DB 연결 직후 호출. Phase 9에서 포트폴리오화 시 `golang-migrate` 전환을 검토할 수 있음
  - **산출물**: `internal/memory/migrate.go`

- [x] **Task 4-4-3. PostgresMemoryRepository 구현**
  - **무엇**: Postgres에 Memory를 저장하고 `LoadByTags`를 태그 배열 **OR 조건** (`WHERE tags && $1`) + LIMIT으로 구현하는 구현체
  - **왜**: 장기 기억이 영구 저장소에 없으면 프로세스 재시작마다 소실됨. embedding 검색은 Phase 9 이후 선택 도입
  - **산출물**: `internal/memory/postgres_memory_repository.go`

- [x] **Task 4-4-4. MemoryRepository integration test 작성**
  - **무엇**: `Save` 후 `LoadByTags` OR 조건 검증 (태그 중 하나만 일치해도 반환), 빈 태그 배열 조회, `limit` 초과 시 잘리는지 확인. Postgres 실제 연결 기반 테스트
  - **왜**: OR 조건 쿼리(`WHERE tags && $1`)가 의도대로 동작하는지는 단위 테스트로 검증 불가. Phase 4 Exit Criteria의 "태그 OR 조건 조회 결과 확인"을 코드 수준에서 보장하려면 통합 테스트가 필요
  - **산출물**: `internal/memory/memory_repository_test.go`

### Step 4-5. Memory Manager

- [x] **Task 4-5-1. MemoryManager 인터페이스 정의**
  - **무엇**: `LoadSession`, `SaveSession`, `SaveMemory`, `LoadRelevantMemory` 메서드를 갖는 파사드 인터페이스
  - **왜**: runtime이 session repository와 memory repository를 각각 직접 알면 의존이 넓어짐. 단일 인터페이스로 캡슐화
  - **산출물**: `internal/memory/memory_manager.go`

- [x] **Task 4-5-2. DefaultMemoryManager 구현**
  - **무엇**: SessionRepository + MemoryRepository를 주입받아 MemoryManager 인터페이스를 구현하는 구조체
  - **왜**: runtime은 MemoryManager만 알면 되고 구체 저장소는 주입으로 교체 가능
  - **산출물**: `internal/memory/default_memory_manager.go`

### Step 4-6. Long-term Memory → Planner 피드백 연결

- [x] **Task 4-6-1. Runtime에 Long-term Memory 주입 및 prompt_builder 반영**
  - **무엇**: `Runtime.Run()` 진입 직후(루프 시작 전) `MemoryManager.LoadRelevantMemory(ctx, userInput)`을 1회 호출해 결과를 `AgentState.RelevantMemories`에 저장. `prompt_builder`는 이 필드를 읽어 system prompt의 context 섹션에 포함
  - **왜**: Long-term Memory 조회를 LLM이 tool로 호출하는 방식(A안)은 호출 누락 시 메모리가 활용되지 않는 신뢰성 문제가 있음. 대화형 에이전트는 과거 맥락이 항상 보장되어야 하므로 Runtime이 Run() 시작 시 1회 주입하는 방식(C안)을 채택. UserInput이 이미 확정된 시점에 조회하므로 쿼리 기준이 명확하고 루프 내 반복 DB 조회가 없음
  - **비고**: `AgentState.RelevantMemories []types.Memory` 필드 추가. `Memory` struct가 `internal/types`에 있으므로 `state → types` 의존만 발생하며 패키지 경계 규칙을 위반하지 않음. prompt_builder는 MemoryManager를 직접 알지 않고 AgentState만 참조하므로 패키지 경계 유지. Run() 종료 후 새로 생성된 Memory는 별도 경로(MemoryManager.SaveMemory)로 저장
  - **산출물**: `internal/agent/runtime.go` 수정 (Run() 시작 시 LoadRelevantMemory 호출), `internal/state/agent_state.go` 수정 (RelevantMemories 필드 추가), `internal/planner/prompt_builder.go` 수정 (RelevantMemories → system prompt 반영)

- [x] **Task 4-6-2. Runtime 종료 후 Memory 저장 경로 구현**
  - **무엇**: `Runtime.Run()` 종료 직후 대화 결과(FinalAnswer + ToolResults 요약)를 `MemoryManager.SaveMemory(ctx, Memory)`로 저장하는 로직 구현. 저장 대상은 `FinalAnswer`가 비어있지 않은 정상 완료 케이스만 해당 (실패/중단 시 저장하지 않음)
  - **왜**: Task 4-6-1 비고에 "Run() 종료 후 새로 생성된 Memory는 별도 경로(MemoryManager.SaveMemory)로 저장"이라고 명시했으나, 이를 구현하는 Task가 없었음. 저장 경로가 없으면 LoadRelevantMemory가 항상 빈 결과를 반환하게 되어 Long-term Memory 기능이 실질적으로 동작하지 않음
  - **비고**: Memory 저장 호출 위치는 `Runtime.Run()` 반환 직후 (CLI: `main.go`, HTTP API: Worker goroutine). Runtime 내부가 아닌 호출자에서 저장하는 이유는 Runtime이 MemoryManager에 직접 의존하는 범위를 최소화하기 위함 (Load는 Run 시작 시 1회, Save는 호출자 책임)
  - **산출물**: `cmd/agent-cli/main.go` 수정 (Run 후 SaveMemory 호출), `internal/memory/default_memory_manager.go` 수정 (SaveMemory 구현 확인)

### Step 4-7. 설계 결정 문서화

- [x] **Task 4-7-1. Phase 4 설계 결정 기록**
  - **무엇**: RequestState/SessionState 분리 근거, Memory struct의 `internal/types` 배치 결정, MemoryManager 파사드 패턴 선택 이유, RedisSessionRepository AOF 설정 배경, Long-term Memory 주입 방식으로 C안(Run() 시작 시 1회 주입) 채택 근거를 `docs/decisions/phase4.md`에 기록
  - **왜**: 상태 분리 경계 결정은 Phase 6 multi-agent와 Phase 7 서비스화에서 계속 영향을 미침. 특히 `internal/types` 공유 타입 확장 이력이 문서에 없으면 나중에 경계 위반을 모르고 추가할 수 있음
  - **산출물**: `docs/decisions/phase4.md`

### Phase 4 Exit Criteria

- 동일 SessionID로 재요청 시 이전 RecentContext 복원 확인
- RequestState / SessionState 데이터가 서로 독립적으로 분리 확인
- Redis 재시작 후 세션 복원 확인 (RedisSessionRepository)
- Memory 저장 후 태그 OR 조건 조회 결과 확인
- Long-term Memory 조회 결과가 LLMPlanner system prompt에 반영되어 다음 응답에 영향을 주는 것 확인
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase4.md`에 기록 (Task 4-7-1)

---

## Phase 5 — Verifier / Retry / Concurrency

> **재작성 지점**: Task 5-1-1까지 완료 상태. 이하 구간은 `plan-init` 규칙("무엇이 동작하는가" 기준)으로 재작성되어 있음. 파일 경로·함수명 같은 구현 힌트는 IMPLEMENT.md로 이관 대상.

**회귀 체크**: Phase 4 Exit Criteria (세션 복원, Memory 주입 경로)가 재작성 구간 진입 전에 여전히 통과해야 한다.

### Step 5-1. Concurrency 기초

- [x] **Task 5-1-1. tool 실행 단위 timeout 적용 상태**
  - **목적**: Phase 3에서 LLM 연결 이후 deadline 없이 굴러온 실행 경로에 첫 번째 timeout 경계를 세운다. Phase 8 전체 request deadline과 Phase 6/7 병렬 실행이 전부 이 context 전파 패턴을 전제한다.
  - **입력**: Phase 4까지 통합된 Runtime과 ToolRouter가 context 인자로 호출되는 상태
  - **산출물**: ToolRouter가 호출마다 deadline이 적용된 context를 tool에게 전달하는 경로, 그 동작을 재현하는 테스트
  - **Exit Criteria**: 의도적으로 지연시킨 tool이 `context.DeadlineExceeded` 계열 에러로 중단되는 것이 테스트로 관찰된다

- [ ] **Task 5-1-2. 독립 tool 2개가 errgroup으로 병렬 실행되고 취소가 전파된다**
  - **목적**: Phase 6 Workflow 병렬 실행과 Phase 7 Worker goroutine 관리가 모두 전제하는 "errgroup + context 취소" 패턴을 아주 얕은 범위에서 먼저 손으로 돌려 본다. 이 패턴이 없는 채로 Phase 6에 들어가면 그래프 실행 디버깅과 goroutine leak 디버깅이 한 번에 몰린다.
  - **입력**: Task 5-1-1의 context 전파 패턴이 작동하는 상태
  - **산출물**: 독립 tool 2개를 동시에 실행하는 실행 경로 + 한쪽 실패 시 나머지를 취소하는 동작을 관찰하는 테스트
  - **Exit Criteria**: (1) 두 tool이 동시에 시작되는 것이 테스트 타이밍으로 관찰된다 (2) 한쪽이 에러를 내면 나머지 tool의 context가 Done으로 전이된다 (3) `go test -race`로 경고 없이 통과한다

- [ ] **Task 5-2. Runtime이 Verifier를 거쳐 done/retry/fail 3분기로 종료 판정한다**
  - **목적**: 지금까지 Runtime 내부에 흩어져 있던 종료 판정 로직을 "검증 결과"라는 단일 경계로 모은다. loop가 언제 끝나는지를 Verifier 한 곳에서 설명할 수 있어야 Task 5-3(retry), 5-5(reflection)가 같은 축 위에 붙을 수 있다.
  - **입력**: Phase 1의 Runtime loop, Phase 1의 `IsFinished` 종료 판정, Phase 2의 에러 분류가 동작하는 상태
  - **산출물**: Verifier 경계 (인터페이스 + 기본 구현 + Runtime 주입), Runtime loop 상의 verifier 호출 지점
  - **Exit Criteria**: 다음 3가지 관찰 지점이 모두 테스트로 재현된다 — (1) FinalAnswer가 비어 있는 결과에서 verifier가 `retry`를 내면 loop가 한 step 더 진행, (2) tool 에러가 섞인 결과에서 `fail`을 내면 Status가 `failed`로 전이하며 종료, (3) 정상 결과에서 `done`이면 loop가 정상 종료한다

- [ ] **Task 5-3. RetryPolicy가 retry 결정의 단일 지점이 되고 LLMPlanner 내부 하드코딩 retry는 사라진다**
  - **목적**: Phase 3의 JSON 파싱 재시도(하드코딩 1회)와 Phase 5에서 도입하는 정책 retry가 이중으로 돌지 않도록 retry 진입점을 하나로 모은다. 무한 재시도 방지가 retry 레이어의 존재 이유이므로 상한 검증이 핵심이다.
  - **입력**: Task 5-2가 끝나 verifier가 retry 신호를 낼 수 있는 상태
  - **산출물**: RetryPolicy 경계 (정책 호출 지점이 Runtime 한 곳에 고정), Phase 3의 하드코딩 retry 제거분
  - **Exit Criteria**: (1) 연속 실패 시 설정된 최대 횟수에서 loop가 종료된다, (2) `llm_parse_error`가 RetryPolicy 외의 경로에서 재시도되지 않는다 — Phase 3 LLMPlanner 테스트(Task 3-4-7)가 회귀 없이 통과하는 것으로 확인

- [ ] **Task 5-4. 에러 유형별 loop 제어 신호가 단일 진입점에서 분기된다**
  - **목적**: 에러 분류(Phase 2)를 "그래서 loop는 어떻게 해야 하는가"로 변환하는 단일 함수를 확보한다. 분기가 Runtime 여러 지점에 흩어지면 새 실패 유형 추가 시 누락이 반드시 발생한다.
  - **입력**: Phase 2의 AgentError 분류, Task 5-3의 RetryPolicy 경계
  - **산출물**: Failure 분기 경계 (에러 → {fatal 종료, RetryPolicy에 위임, loop 속행} 매핑)
  - **Exit Criteria**: (1) `tool_not_found` → 즉시 종료, (2) `tool_execution_failed` + timeout → RetryPolicy 경로로 진입, (3) 빈 결과 → loop 속행 (다음 step에서 Planner가 다른 접근을 고를 기회) — 이 3개가 모두 테스트로 관찰된다

- [ ] **Task 5-5. Reflection 결과가 다음 Plan 호출과 loop 속행 판단에 실제로 반영된다**
  - **목적**: LLM 자기검증을 "관찰 가능한 상태 변화"로 고정한다. Reflection을 추가하면서 loop에 아무 영향도 없으면 단순히 prompt 한 번 더 태우는 것에 그치므로, "prompt에 들어가는가"와 "loop가 한 번 더 도는가" 두 관찰 지점이 핵심이다.
  - **입력**: Task 5-2가 완료돼 Verifier 경계가 Runtime에 붙어 있는 상태, Phase 3 prompt_builder
  - **산출물**: Reflector 경계 (인터페이스 + 구현), AgentState의 reflection 보관 슬롯, prompt_builder에서 reflection 반영 경로, Runtime에서 reflection 결과로 loop를 한 번 더 돌리는 분기
  - **Exit Criteria**: 같은 입력을 mock LLM으로 돌렸을 때 (1) 첫 Plan 호출 → 부족 판정 → 두 번째 Plan 호출 prompt에 missing conditions 문자열이 포함된 것이 관찰되고, (2) 두 번째 step에서 FinalAnswer가 채워져 loop가 종료되는 것이 테스트로 재현된다

### Phase 5 Exit Criteria

- Phase 4 Exit Criteria 회귀 통과
- done/retry/fail 3분기가 단일 verifier 경로로 관찰됨
- retry 상한에서 loop가 종료되고, `llm_parse_error` retry가 단일 지점에서만 일어남
- 에러 유형별 제어 신호 분기가 단일 진입점에 모여 있음
- Reflection이 다음 Plan prompt와 loop 속행에 실제 영향을 주는 것 관찰
- `go test -race ./internal/agent/... ./internal/verifier/...` 통과
- Verifier vs Reflector 역할 분리 / RetryPolicy 단일화 / Failure 분기 기준이 `docs/decisions/phase5.md`에 기록

---

## Phase 6 — Multi-Agent Orchestration

**회귀 체크**: Phase 5 Exit Criteria.

### Decision Point 6-D1. orchestration 패키지 의존 방향

> 구현 Task로 진입하기 전에 사용자 승인 필요.

- **선택지 A**: `orchestration → agent` — WorkerAgent가 Runtime을 주입받아 내부에서 호출. Runtime은 multi-agent 존재를 모름
- **선택지 B**: `agent → orchestration` — Runtime이 Manager를 직접 호출. Runtime이 multi-agent를 인식
- **Trade-off**: A는 Runtime 재사용 + 단방향 의존 + 역할 경계 유지. B는 Runtime이 multi-agent 인지 책임을 져 결합 증가 + Phase 7 Worker가 Runtime만 호출해도 되는 단순함이 무너짐
- **기본 권장**: A

- [ ] **Task 6-1. Task DAG가 위상 정렬되고 독립 Task는 병렬로 실행되며 순환은 에러로 감지된다**
  - **목적**: 실행 엔진 이전에 그래프 구조 자체가 옳은지 격리해서 검증한다. 그래프 오류와 goroutine 오류가 디버깅 단계에서 한 덩어리로 나오면 원인 분리가 불가능해진다.
  - **입력**: Task 5-1-2의 errgroup + context 취소 패턴
  - **산출물**: Workflow 경계 (타입 + 정렬 + cycle detection + 병렬 실행 + 실패 전파)
  - **Exit Criteria**: 4개 관찰 지점 — (1) 선형 의존 Task들이 의존 순서대로 실행됨, (2) 독립 Task 2개가 동시에 시작되는 것이 테스트 타이밍으로 관찰됨, (3) 순환 의존 Task 그래프에 cycle 에러 반환, (4) 한 Task 실패 시 나머지에 취소 전파되고 최종 결과에 실패가 병합됨 — 모두 `go test -race` 통과

- [ ] **Task 6-2. "호텔 찾아줘" 입력 하나가 Search → Filter → Ranking → Summary 4단계로 실제 실행되어 결과가 반환된다**
  - **목적**: TaskDecomposer + Manager + Worker + Workflow 조합이 사용자 입력에서 결과까지 흐르는지를 단일 관찰점에 모은다. 각 단계를 개별 Task로 쪼개면 "시나리오가 실제로 돈다"는 핵심 증거가 사라진다.
  - **입력**: Task 6-1의 Workflow 경계, Decision Point 6-D1 결정, Phase 3의 MockLLMClient
  - **산출물**: TaskDecomposer 경계 + Manager/Worker 경계 + Task ↔ AgentState 변환 경로 + Filter/Ranking에 필요한 mock tool + 실행 trace 로그
  - **Exit Criteria**: (1) mock LLM으로 고정된 시나리오 입력을 주면 4개 worker가 정확한 순서(Search → Filter → Ranking → Summary)로 호출되는 것이 trace 로그로 관찰되고, (2) 최종 응답에 요약 문자열이 채워져 반환되며, (3) 순서상 Filter와 Ranking 의존 관계가 Workflow 정렬로 해소된다

### Phase 6 Exit Criteria

- Phase 5 Exit Criteria 회귀 통과
- Workflow 4개 관찰 지점 (정렬/병렬/cycle/실패 전파) 전부 통과
- 호텔 검색 E2E 시나리오가 trace 로그 + 최종 응답으로 관찰됨
- `go test -race ./internal/orchestration/...` 통과
- 의존 방향(6-D1), Manager vs Workflow 역할 분리, Task 간 데이터 전달 방식이 `docs/decisions/phase6.md`에 기록

---

## Phase 7 — Runtime 서비스화

**회귀 체크**: Phase 6 Exit Criteria.

> Kafka 등 외부 브로커는 이 Phase의 범위가 아니다. TaskQueue는 buffered channel 기반 InMemory 구현으로 고정. 인터페이스만 경계로 두고 이후 선택 확장.

### Decision Point 7-D1. HTTP 라우터 선택

- **선택지 A**: 표준 `net/http` ServeMux (Go 1.22+ path parameter)
- **선택지 B**: `chi`/`gorilla/mux` 등 외부 라우터
- **Trade-off**: A는 외부 의존 0 + 기능 최소, B는 미들웨어 생태계
- **기본 권장**: A (Phase 0 Task 0-3-3에서 Go 1.22+ 고정됨)

### Decision Point 7-D2. ask_user 비동기 대기 방식

- **선택지 A**: Runtime.Run()이 `ask_user`에서 반환하고, 사용자 입력 수신 후 새 Run() 호출로 재개 (시그니처 불변)
- **선택지 B**: Runtime loop 내부에서 channel로 대기 (Worker goroutine 차단)
- **Trade-off**: A는 Runtime 시그니처 불변 + Worker 비차단, B는 loop 상태를 그대로 유지하지만 Worker 당 task 병렬성이 줄어듦
- **기본 권장**: A

### Decision Point 7-D3. Admin 엔드포인트 인증 범위

- **결정 내용**: 이 커리큘럼의 목적은 runtime 제어 흐름 학습이며, admin 엔드포인트 인증/인가는 명시적 비목표. `docs/scope.md`에 명시 필요.
- **확인 필요**: "인증 미적용"을 scope 문서에 박는 것에 대한 사용자 승인

- [ ] **Task 7-1. `cmd/agent-api` 프로세스가 기동되고 `/v1/agent/run`, `/v1/tasks/{id}`, `/v1/sessions/{id}`, `/health`가 응답한다**
  - **목적**: API 서버로서 최소한의 기동 경로를 관찰 가능한 상태로 확보한다. 라운드트립이 되는 서버 없이는 이후 Task들이 전부 공중에 뜬다.
  - **입력**: Phase 6 완료, Decision Point 7-D1 결정
  - **산출물**: `cmd/agent-api` 진입점, 4개 엔드포인트, 요청/응답 JSON 경계
  - **Exit Criteria**: (1) `go run ./cmd/agent-api`로 서버 기동, (2) `POST /v1/agent/run`에 JSON 전달 시 200 + task ID 반환, (3) `GET /v1/tasks/{id}` 및 `/v1/sessions/{id}`가 라운드트립, (4) `GET /health`가 의존 서비스 상태 JSON을 반환 — `httptest` 기반 핸들러 테스트로 재현

- [ ] **Task 7-2. HTTP 요청이 Queue를 거쳐 Worker goroutine에서 실행되고, 단일/multi-agent 경로가 분기되며, graceful shutdown이 in-flight task를 잃지 않는다**
  - **목적**: API 계층과 실행 엔진을 물리적으로 분리하고, Phase 6에서 만든 multi-agent 경로를 HTTP 경계에 연결한다. multi-agent가 CLI에서만 돌면 Phase 6의 산출물이 Phase 7 이후 실사용 경로에서 죽는다. 저장소는 이 시점에선 InMemory Repository로 두고, 프로세스 재시작 영속성은 Task 7-3에서 Redis로 교체.
  - **입력**: Task 7-1의 핸들러 경로, Phase 6 Manager 경계
  - **산출물**: TaskQueue 경계 + InMemory 구현 + Worker 루프 + 단일/multi 분기 경로 + graceful shutdown 경로 + AsyncTask 상태 기계(queued/running/succeeded/failed) + InMemory AsyncTaskRepository
  - **Exit Criteria**: (1) POST 직후 task ID 즉시 반환 (동기 실행 아님), (2) Worker가 단일 agent 요청과 multi-agent 요청을 각각 맞는 경로로 처리하는 것이 테스트로 관찰됨, (3) AsyncTask 상태 전이(queued → running → succeeded/failed)가 관찰되고 잘못된 전이는 거부됨, (4) SIGTERM을 보내면 진행 중 task가 완료된 뒤 프로세스가 종료되고 결과는 Repository에 저장됨, (5) `go test -race ./internal/queue/...` 통과

- [ ] **Task 7-3. task 결과가 프로세스 재시작 후에도 `GET /v1/tasks/{id}`로 조회된다**
  - **목적**: 서비스화의 최소 운영 요건인 "결과 휘발 방지"를 관찰 지점으로 고정한다. Task 7-2의 InMemory Repository만으로는 재시작 시 유실되어 Phase 7이 실질적 서비스 상태에 도달하지 못한다.
  - **입력**: Task 7-2 완료, Phase 4의 Redis 연결 인프라
  - **산출물**: RedisAsyncTaskRepository 구현, 기존 InMemory에서 Redis로 주입 교체
  - **Exit Criteria**: (1) POST → task 완료 → API 프로세스 재시작, (2) 재시작 후 동일 task ID 조회 시 이전 결과가 그대로 반환되는 것이 integration test로 관찰됨

- [ ] **Task 7-4. 운영자가 admin API만으로 최근 task / 실패 task / session dump / tool 통계를 조회할 수 있다**
  - **목적**: 로그 grep 없이 운영 신호를 확보한다. Phase 8 timeout 튜닝과 Phase 9 포트폴리오 시나리오 데모에 쓸 최소 관측 채널이 이 단계에서 필요하다.
  - **입력**: Task 7-2 완료, Decision Point 7-D3 결정
  - **산출물**: 4개 admin 엔드포인트 (`/v1/admin/tasks`, `/v1/admin/tasks/failed`, `/v1/admin/sessions/{id}`, `/v1/admin/stats/tools`), tool 호출 통계 집계기
  - **Exit Criteria**: 4개 엔드포인트 각각이 예상 형태의 JSON을 반환하고, tool 통계는 동일 tool 여러 호출 후 호출 횟수와 평균 latency가 증가하는 것이 테스트로 관찰됨

- [ ] **Task 7-5. HTTP 환경에서 `ask_user` 발생 시 task가 대기 상태로 전환되고 사용자 입력 제출 후 재개된다**
  - **목적**: Phase 3에서 CLI 대체 처리로 미뤄둔 ask_user를 HTTP 경계에서 실제로 완성한다. 이 Task가 없으면 ask_user는 영원히 CLI 전용 미완성 ActionType으로 남는다.
  - **입력**: Task 7-2 완료, Decision Point 7-D2 결정
  - **산출물**: AsyncTask에 `waiting_for_user` 상태 추가, 사용자 입력 제출 엔드포인트, Runtime 재개 경로
  - **Exit Criteria**: (1) mock LLM으로 ask_user를 유도 → task가 `waiting_for_user` 상태로 전이되는 것이 `GET /v1/tasks/{id}`로 관찰, (2) 입력 제출 엔드포인트로 값 전달 → task가 재개되어 최종 `succeeded` 상태에 도달

### Phase 7 Exit Criteria

- Phase 6 Exit Criteria 회귀 통과
- Task 7-1 ~ 7-5 각 Task의 Exit Criteria 통과
- `go test -race ./internal/queue/... ./internal/api/...` 통과
- 라우터 선택, ask_user 처리 방식, `orchestration.Task` vs `api.AsyncTask` 개념 분리 근거가 `docs/decisions/phase7.md`에 기록

---

## Phase 8 — 운영 고도화

**회귀 체크**: Phase 7 Exit Criteria.

### Decision Point 8-D1. OTel exporter 선택

- **선택지 A**: stdout exporter (인프라 추가 없음, 시각화 없음)
- **선택지 B**: OTLP → Jaeger/Collector (docker-compose에 컨테이너 추가, trace tree 시각화 가능)
- **Trade-off**: A는 기동 부담 최소 + span 구조 확인은 로그로, B는 trace 시각화가 Phase 9 데모에 직접 쓰임
- **기본 권장**: A로 시작하고 exporter 교체 가능한 구조로 두는 것. 단, Phase 9 포트폴리오에 trace 스크린샷이 필요하면 B.

- [ ] **Task 8-1. tool별 timeout이 config로 주입되고 전체 request deadline도 적용된다**
  - **목적**: Task 5-1-1에서 확립한 context 패턴에 (1) 운영 중 조정 가능한 외부화, (2) 루프 전체 상한을 덧붙인다. 전자는 tool 특성별 튜닝, 후자는 loop 무한 진행 방지가 이유다.
  - **입력**: Task 5-1-1의 context 전파 패턴, Phase 7 완료
  - **산출물**: config의 tool timeout 맵, Runtime 진입 시점의 전체 deadline 적용
  - **Exit Criteria**: (1) config에서 tool A의 timeout을 짧게 주면 A 실행이 `tool_execution_failed` (retryable)로 중단되는 것이 관찰, (2) 전체 deadline 초과 시 loop가 `context.Canceled` 계열로 즉시 종료 관찰

- [ ] **Task 8-2. session 누적 token이 임계값을 넘으면 loop가 중단된다**
  - **목적**: 단일 session이 무제한 비용을 내는 것을 강제로 막는다. Phase 7 이후 Worker가 동시 실행되는 환경에서 tracker는 반드시 동시성 안전해야 한다.
  - **입력**: Phase 3의 TokenUsage 기록 경로, Phase 7의 Worker
  - **산출물**: session 단위 token tracker (동시성 보호), 비용 한도 정책 → loop 중단 연결
  - **Exit Criteria**: (1) 작은 임계값 설정 + 다회 호출 시나리오에서 세션이 임계값 교차 직후 loop가 종료되는 것이 관찰, (2) `go test -race ./internal/llm/...` 통과

- [ ] **Task 8-3. 한 요청이 request → planner → tool → verifier → memory 구간에서 단일 trace로 연결되고 로그의 trace_id가 span TraceID와 일치한다**
  - **목적**: latency 병목과 실패 지점을 trace 하나로 파악 가능하게 만든다. span이 붙어 있어도 서로 연결이 끊기면 Phase 9 시나리오 데모에서 "왜 느렸는지"를 설명할 수 없다.
  - **입력**: Decision Point 8-D1 결정, Phase 3 structured logger
  - **산출물**: OTel SDK 초기화 경로, Runtime/Planner/Tool/Verifier/Memory 각 구간 span, logger trace_id 필드의 span 연동
  - **Exit Criteria**: 하나의 요청에서 (1) exporter 출력(stdout 또는 Jaeger)으로 부모-자식 관계가 연결된 span tree가 확인되고, (2) 같은 요청의 로그 라인들이 모두 동일한 trace_id를 갖는 것이 관찰된다

- [ ] **Task 8-4. tool 사용 제한 + max step + 비용 한도가 Runtime의 단일 `Policy.Check()` 호출로 적용된다**
  - **목적**: 정책 호출 지점이 Runtime 여러 곳에 분산되는 것을 막고 향후 정책 추가 지점을 하나로 고정한다. 파사드 구조가 없으면 Task 8-2의 비용 정책과 기존 max step 처리가 각자 다른 위치에서 호출되어 누락/중복이 생긴다.
  - **입력**: Task 8-2 완료, Phase 1의 max step 처리
  - **산출물**: PolicyLayer 파사드, Runtime 호출 경로 단일화
  - **Exit Criteria**: (1) Runtime 코드에서 개별 정책 호출이 사라지고 `Policy.Check()` 단일 호출만 남은 상태에서, (2) 3개 정책이 모두 유효하게 동작하는 것이 테스트로 확인됨

- [ ] **Task 8-5. 에러가 user / system / provider 분류 레이블을 갖는다**
  - **목적**: Phase 2의 retryable/fatal 분류는 "retry 할지 말지"만 결정할 뿐 운영 단계에서 필요한 "누구 잘못인가"를 구분하지 못한다. 사용자 응답 톤(사과할지 재시도 요청할지), 알림 채널(oncall 호출할지 사용자 공지할지), 메트릭 레이블(provider 장애율 측정)을 결정하려면 책임 주체 기준의 별도 축이 필요하다. 기존 분류와 직교하는 태깅을 덧대 "provider 장애라 사용자 잘못 아님" 같은 판단을 코드가 직접 내릴 수 있게 만든다.
  - **입력**: Phase 2 AgentError
  - **산출물**: 분류 확장 (기존 retryable/fatal과 직교하는 책임 주체 축)
  - **Exit Criteria**: 대표 에러 4종(input_validation_failed, tool_execution_failed, llm_parse_error, tool_not_found)이 각각 예상 분류 레이블(user/system/provider)로 태깅되는 것이 테스트로 관찰됨

### Phase 8 Exit Criteria

- Phase 7 Exit Criteria 회귀 통과
- Task 8-1 ~ 8-5 각 Task Exit Criteria 통과
- `go test -race ./...` 전체 통과
- OTel exporter 선택, PolicyLayer 파사드, TokenTracker 동시성 전략이 `docs/decisions/phase8.md`에 기록

---

## Phase 9 — 문서화 / 포트폴리오

**회귀 체크**: Phase 8 Exit Criteria.

- [ ] **Task 9-1. push/PR마다 build + vet + unit test + race detector가 GitHub Actions에서 자동 실행된다**
  - **목적**: "레포의 코드가 실제로 돌아간다"는 증거를 외부인에게 보여줄 단일 신호를 확보한다. CI 배지 없는 포트폴리오는 신뢰도가 낮다.
  - **입력**: Phase 4 Task 4-0-1의 `make test-unit` 타겟
  - **산출물**: CI 워크플로우, README의 CI 배지
  - **Exit Criteria**: 레포에 푸시 시 CI가 녹색 체크를 반환하고, README 상단 배지가 렌더링된다. integration 테스트는 CI 범위 외임이 워크플로우 코멘트로 명시

- [ ] **Task 9-2. 외부인이 레포 루트 문서만 보고 구조·기동법·대표 시나리오·설계 근거를 파악할 수 있다**
  - **목적**: 포트폴리오로서의 최종 제출물. 코드만으로는 설계 의도가 드러나지 않으므로 "왜 이렇게 나눴는가"를 문서로 연결한다. 개별 문서 작성을 Task로 쪼개는 대신 "외부인이 이해 가능한가"라는 하나의 관찰 기준으로 묶는다.
  - **입력**: Phase 0 ~ Phase 8의 `docs/decisions/*` 누적 기록, Phase 2의 tool spec 문서
  - **산출물**: README (아키텍처 다이어그램 + 기동법 + 대표 시나리오), 컴포넌트별 아키텍처 문서, 시나리오별 실행 로그 (날씨 / 호텔 / 실패 후 retry / multi-agent)
  - **Exit Criteria**: (1) README만 읽어도 기동법과 전체 구조가 파악됨, (2) 4개 시나리오에 실제 실행 로그가 첨부됨, (3) 각 컴포넌트(runtime / planner / memory / tool router / multi-agent)의 경계와 의존 방향이 문서화됨, (4) `go test ./...` 전체 통과

### Phase 9 Exit Criteria

- Phase 8 Exit Criteria 회귀 통과
- CI 녹색 + README 배지 노출
- Task 9-2의 4개 관찰 지점 모두 통과
- `go test ./...` 전체 통과
