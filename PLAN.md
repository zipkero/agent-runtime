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

- [ ] **Task 0-2-4. Makefile 기본 타겟 추가**
  - **무엇**: `make build`(`go build ./...`), `make test`(`go test ./...`), `make vet`(`go vet ./...`) 타겟 작성
  - **왜**: Phase 1부터 반복 실행하는 명령을 표준화해야 Phase 4에서 `make test-unit`/`make test-integration` 타겟을 추가할 때 기반이 됨. 지금 만들어두지 않으면 Phase 1~3에서 명령을 매번 직접 입력하거나, Phase 4에서 처음부터 Makefile을 작성하게 됨
  - **비고**: Phase 4 Task 4-0-1에서 이 Makefile에 `test-unit`/`test-integration` 타겟만 추가하면 됨
  - **산출물**: `Makefile`

### Step 0-3. 프로젝트 초기화

- [x] **Task 0-3-1. 디렉터리 구조 생성**
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

- [x] **Task 0-3-2. 각 패키지 stub 파일 생성 + go build 통과**
  - **무엇**: Task 0-3-1에서 생성한 모든 디렉터리에 `package` 선언만 있는 빈 `.go` 파일 생성. `testutil/`은 `package testutil`로 선언
  - **왜**: `go build ./...` 통과 여부로 패키지 경계가 올바른지 확인. stub이 없으면 나중에 추가하는 패키지가 경계 규칙을 처음부터 지키는지 검증 불가
  - **산출물**: 각 패키지의 빈 stub 파일

- [ ] **Task 0-3-3. go.mod Go 버전 확정**
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
  - **산출물**: `internal/agent/errors.go`

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
  - **비고**: `retry`는 Runtime/RetryPolicy의 루프 제어 정책 (Phase 5에서 별도 구현). `search_memory`는 ActionType이 아닌 Tool로 구현 (Registry에 등록). `ask_user`는 Phase 3에서 Runtime loop가 만나면 즉시 `respond_directly`로 대체 처리(loop 종료)하며, Phase 8 HTTP API 환경에서의 비동기 사용자 입력 대기 메커니즘은 Phase 8에서 별도 설계한다
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
  - **산출물**: `internal/observability/logger.go`

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

- [ ] **Task 4-2-5. SessionRepository integration test 작성**
  - **무엇**: InMemorySessionRepository와 RedisSessionRepository에서 동일한 테스트 케이스(저장 → 조회, 없는 ID 조회 에러)를 실행해 인터페이스 호환성 검증. Redis 재시작 후 복원 케이스는 RedisSessionRepository 전용 테스트로 분리
  - **왜**: Phase 4 Exit Criteria의 "Redis 재시작 후 세션 복원 확인"이 테스트 코드로 뒷받침되어야 함. Phase 5에서 AgentState 구조가 변경될 경우 SessionRepository 직렬화 동작의 회귀 보호도 필요
  - **산출물**: `internal/state/session_repository_test.go`

### Step 4-3. Working Memory

- [ ] **Task 4-3-0. ToolResult에 ToolKind 필드 추가**
  - **무엇**: `internal/types/tool_result.go`의 ToolResult struct에 `Kind string` 필드 추가. 각 Tool 구현체(`search_mock`, `calculator` 등)가 Execute() 반환 시 Kind 값을 채우도록 수정. Kind 상수(`tool_kind_search`, `tool_kind_calculator` 등)는 `internal/types` 패키지에 정의
  - **왜**: Task 4-3-2에서 Runtime이 ToolResult를 WorkingMemory의 `SearchResults / FilteredResults / Summaries` 필드로 분류 저장하려면 ToolResult에 유형 정보가 있어야 함. 이 필드 없이는 Runtime이 어떤 기준으로 분류할지 판단할 수 없음
  - **비고**: ToolResult를 소비하는 runtime.go, router.go, mock_executor.go, 각 테스트 파일에 컴파일 오류가 없는지 `go build ./...`로 확인 후 Task 4-3-1로 진행. (Phase 2 Task 2-7-1에서 `internal/state` → `internal/types`로 이동됨. `internal/state/tool_result.go` 아님)
  - **산출물**: `internal/types/tool_result.go` 수정, 각 Tool 구현체(`internal/tools/*/`) 수정

- [ ] **Task 4-3-1. WorkingMemory struct 정의**
  - **무엇**: SearchResults, FilteredResults, Summaries 필드를 갖는 struct
  - **왜**: tool 실행 중간 산출물이 AgentState에 뭉쳐 있으면 multi-agent 시나리오에서 데이터 경계가 불분명해짐
  - **산출물**: `internal/state/working_memory.go`

- [ ] **Task 4-3-2. WorkingMemory를 AgentState에 통합**
  - **무엇**: `AgentState`에 `Working WorkingMemory` 필드 추가. Runtime의 `④ AgentState 반영` 단계에서 ToolResult의 유형(search/filter/summary)에 따라 `WorkingMemory`의 대응 필드에도 병렬 저장하는 로직 추가
  - **왜**: WorkingMemory struct만 정의하고 AgentState와 연결하지 않으면 Phase 6 WorkerAgent까지 실제로 사용되지 않는 dead code가 됨. Phase 4 내에서 단일 agent가 search_mock → filter → summary 시나리오를 수행할 때 WorkingMemory가 올바르게 채워지는지 검증해야 Phase 6에서 안전하게 재사용 가능
  - **비고**: ToolRouter의 시그니처(`Route(ctx, PlanResult) (ToolResult, error)`)는 변경하지 않는다. WorkingMemory 업데이트 책임은 Runtime에 있음 — ToolRouter는 상태 변경을 알지 못한다
  - **산출물**: `internal/state/agent_state.go` 수정, `internal/agent/runtime.go` 수정 (ToolResult 유형별 WorkingMemory 저장 로직)

### Step 4-4. Long-term Memory

- [ ] **Task 4-4-1. Memory struct 정의**
  - **무엇**: ID, UserID, Content, Tags, CreatedAt 필드를 갖는 struct
  - **왜**: Postgres에 저장할 레코드 단위의 타입 정의
  - **비고**: `Memory` struct는 `internal/types/memory.go`에 정의한다. `internal/state`에서 `AgentState.RelevantMemories []types.Memory` 필드를 사용하려면 `state → memory` 의존이 생기므로 `internal/types`가 유일한 경계 안전 위치임. PlanResult, ToolResult와 동일한 이유 (Phase 2 Task 2-7-1 참고)
  - **산출물**: `internal/types/memory.go` (`internal/memory/memory.go` 아님)

- [ ] **Task 4-4-2. MemoryRepository 인터페이스 정의**
  - **무엇**: `Save(ctx, Memory) error`, `LoadByTags(ctx, tags []string, limit int) ([]Memory, error)` 인터페이스
  - **왜**: Postgres 의존을 런타임 코드에서 격리. 테스트 시 in-memory로 교체 가능. 조회 방식을 태그+limit으로 고정해야 나중에 embedding 검색으로 교체할 때 인터페이스 변경 범위가 명확해짐
  - **비고**: `LoadByTags`는 **OR 조건** (태그 중 하나라도 포함된 항목 조회). AND 조건은 결과가 지나치게 좁아져 실용성이 없음. Phase 9에서 embedding 검색으로 교체 시 인터페이스 시그니처는 유지하되 내부 구현만 교체
  - **산출물**: `internal/memory/memory_repository.go`

- [ ] **Task 4-4-2-b. InMemoryMemoryRepository 구현**
  - **무엇**: 슬라이스 기반 MemoryRepository 구현체. `Save`는 슬라이스에 append, `LoadByTags`는 OR 조건(`tags` 중 하나라도 일치)으로 필터링 후 limit 적용
  - **왜**: SessionRepository가 InMemory → Redis 순서를 따른 것과 동일한 이유. PostgresMemoryRepository(Task 4-4-3)가 완성되기 전에 `search_memory` tool(Task 4-5-3)과 MemoryManager(Task 4-5-2)를 단위 테스트하려면 Postgres 없이 동작하는 구현체가 필요함
  - **산출물**: `internal/memory/in_memory_memory_repository.go`

- [ ] **Task 4-4-2-c. Postgres 스키마 초기화 코드 작성**
  - **무엇**: 앱 시작 시 `memories` 테이블(`id UUID`, `user_id TEXT`, `content TEXT`, `tags TEXT[]`, `created_at TIMESTAMPTZ`)과 태그 검색용 GIN 인덱스를 `CREATE TABLE IF NOT EXISTS`로 생성하는 `migrate` 함수 작성
  - **왜**: Task 4-4-3에서 PostgresMemoryRepository를 구현하기 전에 테이블이 없으면 실행 자체가 불가능함. 새 마이그레이션 라이브러리 의존 없이 Go 표준 `database/sql`로 처리하는 방식을 사용해 커리큘럼 학습 목표에 집중
  - **비고**: `internal/memory/migrate.go`에 `Migrate(db *sql.DB) error` 함수로 작성. `cmd/agent-cli/main.go` 또는 앱 초기화 경로에서 DB 연결 직후 호출. Phase 9에서 포트폴리오화 시 `golang-migrate` 전환을 검토할 수 있음
  - **산출물**: `internal/memory/migrate.go`

- [ ] **Task 4-4-3. PostgresMemoryRepository 구현**
  - **무엇**: Postgres에 Memory를 저장하고 `LoadByTags`를 태그 배열 **OR 조건** (`WHERE tags && $1`) + LIMIT으로 구현하는 구현체
  - **왜**: 장기 기억이 영구 저장소에 없으면 프로세스 재시작마다 소실됨. embedding 검색은 Phase 9 이후 선택 도입
  - **산출물**: `internal/memory/postgres_memory_repository.go`

- [ ] **Task 4-4-4. MemoryRepository integration test 작성**
  - **무엇**: `Save` 후 `LoadByTags` OR 조건 검증 (태그 중 하나만 일치해도 반환), 빈 태그 배열 조회, `limit` 초과 시 잘리는지 확인. Postgres 실제 연결 기반 테스트
  - **왜**: OR 조건 쿼리(`WHERE tags && $1`)가 의도대로 동작하는지는 단위 테스트로 검증 불가. Phase 4 Exit Criteria의 "태그 OR 조건 조회 결과 확인"을 코드 수준에서 보장하려면 통합 테스트가 필요
  - **산출물**: `internal/memory/memory_repository_test.go`

### Step 4-5. Memory Manager

- [ ] **Task 4-5-1. MemoryManager 인터페이스 정의**
  - **무엇**: `LoadSession`, `SaveSession`, `SaveMemory`, `LoadRelevantMemory` 메서드를 갖는 파사드 인터페이스
  - **왜**: runtime이 session repository와 memory repository를 각각 직접 알면 의존이 넓어짐. 단일 인터페이스로 캡슐화
  - **산출물**: `internal/memory/memory_manager.go`

- [ ] **Task 4-5-2. DefaultMemoryManager 구현**
  - **무엇**: SessionRepository + MemoryRepository를 주입받아 MemoryManager 인터페이스를 구현하는 구조체
  - **왜**: runtime은 MemoryManager만 알면 되고 구체 저장소는 주입으로 교체 가능
  - **산출물**: `internal/memory/default_memory_manager.go`

- [ ] **Task 4-5-3. `search_memory` tool 구현**
  - **무엇**: `query` 문자열을 입력받아 `MemoryManager.LoadRelevantMemory()`를 호출하고 결과를 ToolResult로 반환하는 tool 구현 및 Registry 등록
  - **왜**: Task 3-1-1 비고에서 "`search_memory`는 ActionType이 아닌 Tool로 구현(Registry에 등록)"으로 결정했지만 구현 Task가 없었음. Registry에 등록되어야 LLMPlanner가 tool_call로 선택 가능하며, Phase 4의 Long-term Memory 피드백 루프가 실제로 닫힘
  - **비고**: MemoryManager를 tool 생성 시 주입받는 방식으로 구현해 `internal/tools/search_memory` 패키지가 `internal/memory` 패키지에 직접 의존하지 않도록 한다 (인터페이스 주입). `docs/tools.md`에 spec 추가
  - **산출물**: `internal/tools/search_memory/search_memory.go`, `docs/tools.md` 수정

### Step 4-6. Long-term Memory → Planner 피드백 연결

- [ ] **Task 4-6-1. prompt_builder에 Long-term Memory 반영**
  - **무엇**: `internal/planner/prompt_builder.go`를 수정해, `MemoryManager.LoadRelevantMemory()`로 조회한 결과를 system prompt의 context 섹션에 포함하는 로직 추가. Runtime이 Planner 호출 전 `MemoryManager.LoadRelevantMemory()`를 호출하고 결과를 `AgentState`에 임시 저장하거나 prompt_builder에 직접 전달하는 경로 구현
  - **왜**: Phase 4에서 Long-term Memory를 저장하지만 LLMPlanner의 system prompt에 반영되지 않으면 메모리가 "저장은 되지만 활용되지 않는" dead code가 됨. 저장 → 조회 → 프롬프트 반영 경로가 Phase 4 내에서 닫혀야 함
  - **비고**: `AgentState`에 `RelevantMemories []types.Memory` 필드를 추가하고 Runtime에서 채운 뒤 prompt_builder가 참조하는 방식 권장. `Memory` struct가 `internal/types`에 있으므로 `state → types` 의존만 발생하며 패키지 경계 규칙을 위반하지 않음 (Task 4-4-1에서 `internal/types/memory.go`로 정의). prompt_builder가 MemoryManager에 직접 의존하지 않아 패키지 경계가 유지됨
  - **산출물**: `internal/agent/runtime.go` 수정 (MemoryManager 호출 경로 추가), `internal/state/agent_state.go` 수정 (RelevantMemories 필드), `internal/planner/prompt_builder.go` 수정

### Step 4-7. 설계 결정 문서화

- [ ] **Task 4-7-1. Phase 4 설계 결정 기록**
  - **무엇**: RequestState/SessionState/WorkingMemory 분리 근거, Memory struct의 `internal/types` 배치 결정, MemoryManager 파사드 패턴 선택 이유, RedisSessionRepository AOF 설정 배경을 `docs/decisions/phase4.md`에 기록
  - **왜**: 상태 분리 경계 결정은 Phase 6 multi-agent와 Phase 7 서비스화에서 계속 영향을 미침. 특히 `internal/types` 공유 타입 확장 이력이 문서에 없으면 나중에 경계 위반을 모르고 추가할 수 있음
  - **산출물**: `docs/decisions/phase4.md`

### Phase 4 Exit Criteria

- 동일 SessionID로 재요청 시 이전 RecentContext 복원 확인
- RequestState / SessionState / WorkingMemory 데이터가 서로 독립적으로 분리 확인
- search_mock tool 실행 후 `AgentState.Working.SearchResults`에 결과 저장 확인 (WorkingMemory 통합 검증)
- Redis 재시작 후 세션 복원 확인 (RedisSessionRepository)
- Memory 저장 후 태그 OR 조건 조회 결과 확인
- Long-term Memory 조회 결과가 LLMPlanner system prompt에 반영되어 다음 응답에 영향을 주는 것 확인
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase4.md`에 기록 (Task 4-7-1)

---

## Phase 5 — Verifier / Retry / Concurrency

### Step 5-1. Concurrency 기초

- [ ] **Task 5-1-1. context.WithTimeout을 이용한 tool 실행 단위 timeout 적용**
  - **무엇**: ToolRouter.Route() 호출 시 per-tool timeout을 context에 적용하는 구현. context 취소 시 goroutine이 정리되는 패턴 포함
  - **왜**: Phase 3에서 LLM 호출이 시작된 이후 deadline 없이 운용되고 있음. tool 실행 단위부터 timeout을 적용해야 Phase 7 병렬 실행에서 goroutine leak이 발생하지 않으며, 이 패턴을 Phase 9 전체 request deadline(Task 8-1-2)의 기반으로 사용
  - **비고**: Phase 8의 Task 8-1-1(per-tool timeout)과 역할이 겹치지 않도록 이 Task는 "context 전달 패턴 확립"에 집중하고, Phase 8에서 설정값 외부화를 담당
  - **산출물**: `internal/tools/router.go` 수정 (context deadline 전달), `internal/agent/concurrency_test.go` (동작 검증 테스트)

- [ ] **Task 5-1-2. errgroup 의존성 추가 및 병렬 tool 실행 실습**
  - **무엇**: `go get golang.org/x/sync` 실행 후 `go mod tidy`. `errgroup`을 사용해 독립적인 tool 2개를 goroutine으로 병렬 실행하고 결과를 병합하는 예제 구현. context 취소 시 in-flight goroutine 정리 패턴 포함
  - **왜**: Phase 6의 Workflow 병렬 실행 엔진(Task 6-1-4b)과 Phase 7의 Worker goroutine 관리가 errgroup + WaitGroup 패턴을 전제로 함. 이 패턴을 Phase 5에서 단순한 tool 실행으로 먼저 실습하지 않으면 Phase 6에서 처음 마주치게 됨. `golang.org/x/sync`는 외부 의존성이므로 go.mod에 없으면 Task 5-1-2 작성 즉시 빌드가 깨짐
  - **비고**: 결과물은 독립 실행 파일이 아닌 concurrency 패턴 검증용 테스트 코드로 작성. Phase 6 Task 6-1-4b 구현 시 이 패턴을 직접 참조
  - **산출물**: `go.mod`/`go.sum` 갱신, `internal/agent/parallel_example_test.go`

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

- [ ] **Task 5-2-4. Verifier를 Runtime loop에 통합**
  - **무엇**: `Runtime.Run()`의 `④ AgentState 반영` 이후에 `Verifier.Verify(ctx, state)` 호출 추가. `done` → loop 정상 종료, `retry` → loop 속행, `fail` → `Status = StatusFailed` 후 종료
  - **왜**: Verifier 타입과 구현이 있어도 loop에 연결되지 않으면 Phase 5 Exit Criteria("done/retry/fail 분기 확인")를 달성할 수 없음. Reflection(Task 5-5-4)이 "Runtime에 연결"을 별도 Task로 갖는 것과 동일한 이유
  - **비고**: `Runtime`은 `Verifier` 인터페이스만 주입받음. Verifier가 nil이면 skip (기존 Phase 1~4 동작 보존)
  - **산출물**: `internal/agent/runtime.go` 수정

### Step 5-3. Retry Policy

- [ ] **Task 5-3-1. RetryPolicy 인터페이스 정의**
  - **무엇**: `ShouldRetry(err, attempt) bool`, `Delay(attempt) time.Duration` 인터페이스
  - **왜**: retry 로직이 loop에 인라인으로 있으면 유형별로 정책을 다르게 적용하기 어려움
  - **비고**: Phase 3 Task 3-4-5에서 LLMPlanner 내부에 JSON 파싱 실패 시 하드코딩 1회 재시도를 구현했다. RetryPolicy 도입 시 해당 하드코딩 retry를 제거하고 RetryPolicy로 위임하는 정리 작업이 필요함 (이중 retry 방지)
  - **산출물**: `internal/agent/retry_policy.go`

- [ ] **Task 5-3-2. LinearRetryPolicy 구현**
  - **무엇**: 최대 횟수와 고정 대기 시간을 설정할 수 있는 RetryPolicy 구현체
  - **왜**: 가장 단순한 정책으로 먼저 검증. Phase 9에서 더 정교한 정책으로 교체 가능
  - **산출물**: `internal/agent/linear_retry_policy.go`

- [ ] **Task 5-3-3. RetryPolicy unit test 작성**
  - **무엇**: max 3회 초과 시 `ShouldRetry=false` 반환 검증, 각 attempt별 Delay 값 검증
  - **왜**: 무한 재시도 방지가 정책 구현의 핵심이므로 경계 케이스를 반드시 테스트
  - **산출물**: `internal/agent/linear_retry_policy_test.go`

- [ ] **Task 5-3-4. LLMPlanner 하드코딩 retry 제거 및 RetryPolicy 위임**
  - **무엇**: Phase 3(Task 3-4-5)에서 LLMPlanner 내부에 구현한 JSON 파싱 실패 시 하드코딩 1회 재시도 로직을 제거하고, RetryPolicy.ShouldRetry()로 위임하도록 교체
  - **왜**: RetryPolicy 도입 후에도 LLMPlanner 내부 하드코딩 retry가 남아 있으면 `llm_parse_error` 발생 시 LLMPlanner 1회 + RetryPolicy N회로 이중 재시도가 발생함. RetryPolicy가 모든 retry 결정의 단일 지점이 되어야 함
  - **비고**: 이 Task 완료 후 `go test ./internal/planner/...` + `go test ./internal/agent/...` 전체 통과 확인 필수
  - **산출물**: `internal/planner/llm_planner.go` 수정 (하드코딩 retry 제거), `internal/agent/runtime.go` 수정 (RetryPolicy 호출 경로 확인)

### Step 5-4. Failure 분류

- [ ] **Task 5-4-1. 실패 유형별 처리 분기 구현**
  - **무엇**: 에러 유형별 loop 제어 신호를 결정하는 단일 함수 정의
    - `tool_not_found` (fatal) → loop 즉시 종료
    - `llm_parse_error` / `input_validation_failed` (retryable) → planner 재호출 (loop 속행, RetryPolicy 소비)
    - `tool_execution_failed` + timeout → retry (loop 속행, RetryPolicy 소비)
    - empty result → loop 속행 (Planner가 다음 step에서 AgentState의 빈 ToolResult를 보고 스스로 다른 접근을 결정)
  - **왜**: 분기가 여러 곳에 흩어지면 새로운 실패 유형 추가 시 누락이 발생함
  - **비고**: `input_validation_failed`는 Phase 2 정의상 fatal이지만, LLM이 잘못된 input을 생성한 경우(`llm_parse_error`로 분류)는 retryable. FailureHandler에서 에러 Kind를 기준으로 분기. FailureHandler는 "파라미터를 수정"하지 않는다 — 파라미터 결정은 Planner의 책임
  - **산출물**: `internal/agent/failure_handler.go`

### Step 5-5. Reflection

> Verifier와 RetryPolicy가 안정화된 이후 도입. Reflection은 "verifier 판단 전 LLM 자기검증" 역할로, Verifier와 함께 있어야 상호 역할이 명확해진다.

- [ ] **Task 5-5-1. ReflectResult 타입 정의**
  - **무엇**: `Sufficient bool`, `MissingConditions []string`, `Suggestion string` 필드를 갖는 struct
  - **왜**: Reflector 인터페이스 시그니처의 반환 타입
  - **산출물**: `internal/verifier/reflect_result.go`

- [ ] **Task 5-5-1b. AgentState에 ReflectionState 필드 추가**
  - **무엇**: `AgentState`에 `ReflectionState` 필드 추가. `ReflectionState`는 `Sufficient bool`, `MissingConditions []string`, `Suggestion string`을 갖는 struct로 `internal/state` 패키지에 정의
  - **왜**: Task 5-5-4에서 "Sufficient=false일 때 Runtime.Run()에 연결"하려면 AgentState에 reflection 결과를 저장하는 필드가 필요함. 이 필드가 없으면 loop가 reflection 결과를 참조할 수 없음
  - **비고**: `internal/state`는 타입 정의만 담당. `ReflectionState`는 `ReflectResult`와 별개 타입으로 정의해 verifier → state 순환 참조를 방지 (verifier는 ReflectResult 반환, state는 ReflectionState 보관). **이 Task 완료 후 `go build ./...` 통과 확인 후 Task 5-5-2로 진행**
  - **산출물**: `internal/state/reflection_state.go`, `internal/state/agent_state.go` 수정

- [ ] **Task 5-5-2. Reflector 인터페이스 정의**
  - **무엇**: `Reflect(ctx, AgentState) (ReflectResult, error)` 인터페이스
  - **왜**: reflection이 loop에 하드코딩되지 않도록 인터페이스로 분리
  - **산출물**: `internal/verifier/reflector.go`

- [ ] **Task 5-5-3. LLMReflector 구현**
  - **무엇**: reflection 전용 prompt를 사용해 LLM을 호출하고 ReflectResult를 반환하는 구현체
  - **왜**: verifier와 동일한 LLMClient를 재사용하되 prompt가 달라야 함
  - **비고**: **이 Task 완료 후 `go build ./...` 통과 확인 후 Task 5-5-3b로 진행**
  - **산출물**: `internal/verifier/llm_reflector.go`

- [ ] **Task 5-5-3b. prompt_builder에 ReflectionState 반영**
  - **무엇**: `internal/planner/prompt_builder.go`를 수정해 `AgentState.ReflectionState`가 채워져 있을 때 `MissingConditions`와 `Suggestion`을 system prompt에 포함하는 로직 추가
  - **왜**: ReflectionState가 AgentState에 저장되어도 LLMPlanner의 system prompt에 반영되지 않으면 reflection이 다음 Plan 결정에 영향을 주지 못함. loop 제어(Task 5-5-4)와 prompt 반영은 별개 작업
  - **비고**: **이 Task 완료 후 `go build ./...` 통과 확인 후 Task 5-5-4로 진행**
  - **산출물**: `internal/planner/prompt_builder.go` 수정

- [ ] **Task 5-5-4. Reflection 결과를 AgentState에 반영**
  - **무엇**: `Sufficient=false`일 때 loop가 추가 단계를 진행하도록 Runtime.Run()에 연결
  - **왜**: reflection이 state에 반영되지 않으면 loop 제어에 아무 영향도 주지 않음
  - **산출물**: `internal/agent/runtime.go` 수정

### Step 5-6. 설계 결정 문서화

- [ ] **Task 5-6-1. Phase 5 설계 결정 기록**
  - **무엇**: Verifier/Reflector 역할 분리 근거, RetryPolicy 인터페이스 설계 결정, FailureHandler의 에러 유형별 분기 기준, Reflection과 Verification 순서 결정을 `docs/decisions/phase5.md`에 기록
  - **왜**: Verifier(결과 검증)와 Reflector(LLM 자기검증)의 역할 경계는 직관적이지 않음. 이 결정이 문서화되지 않으면 Phase 6~7에서 새 컴포넌트 추가 시 어디에 검증 로직을 넣어야 할지 기준이 없어짐
  - **산출물**: `docs/decisions/phase5.md`

### Phase 5 Exit Criteria

- SimpleVerifier가 `done` / `retry` / `fail` 올바르게 분기 확인
- RetryPolicy max 횟수 초과 시 loop 종료 확인
- `tool_not_found` 에러 → fatal, `tool_execution_failed` 에러 → retry 분기 확인
- `Sufficient=false` reflection 결과 시 loop 추가 진행 확인
- `go test -race ./internal/agent/...` 통과 확인 (concurrency 패턴 검증)
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase5.md`에 기록 (Task 5-6-1)

---


## Phase 6 — Multi-Agent Orchestration

### Step 6-0. orchestration 패키지 설계 결정

- [ ] **Task 6-0-1. orchestration 패키지 의존 방향 및 Runtime 재사용 여부 결정**
  - **무엇**: `internal/orchestration`이 `internal/agent`(Runtime)를 재사용하는지, 별도 실행 경로를 갖는지 결정. 허용되는 의존 방향을 CLAUDE.md 패키지 경계 규칙에 추가
  - **왜**: WorkerAgent가 Runtime.Run()을 내부에서 호출하면 `orchestration → agent` 의존이 생김. 반대로 ManagerAgent를 Runtime이 호출하면 `agent → orchestration` 의존이 생겨 방향이 역전됨. 이 결정 없이 Task 6-3-1부터 구현하면 중간에 패키지 구조를 뜯어야 할 수 있음
  - **비고**: 권장 방향 — WorkerAgent는 Runtime을 직접 주입받아 사용 (`orchestration → agent`). ManagerAgent는 orchestration 내부 조율자. 이 경우 `agent → orchestration` 의존이 없으므로 순환 없음
  - **산출물**: `CLAUDE.md` 패키지 경계 규칙 섹션에 `internal/orchestration` 의존 방향 추가

### Step 6-1. Task Contract

- [ ] **Task 6-1-1. Task / TaskResult 타입 정의**
  - **무엇**: `Task` struct (ID, Type, InputPayload, Dependencies), `TaskResult` struct (TaskID, Output, Error, Latency)
  - **왜**: agent 간 데이터를 주고받는 contract. 이 타입이 없으면 Agent 인터페이스와 Decomposer 인터페이스를 정의할 수 없음
  - **산출물**: `internal/orchestration/task.go`

- [ ] **Task 6-1-2. TaskDecomposer 인터페이스 정의**
  - **무엇**: `Decompose(ctx, userInput) ([]Task, error)` 인터페이스
  - **왜**: Manager가 분해 로직에 의존하지 않도록 경계를 인터페이스로 분리
  - **산출물**: `internal/orchestration/task_decomposer.go`

- [ ] **Task 6-1-3. LLMTaskDecomposer 구현**
  - **무엇**: LLMClient를 사용해 사용자 입력을 Task 목록으로 분해하는 구현체
  - **왜**: 실제 LLM 기반 분해가 있어야 호텔 시나리오 같은 현실적 입력 처리 가능
  - **산출물**: `internal/orchestration/llm_task_decomposer.go`

- [ ] **Task 6-1-3b. MockTaskDecomposer 구현**
  - **무엇**: 고정된 Task 목록을 순서대로 반환하는 테스트용 TaskDecomposer. 호출 횟수 추적 포함
  - **왜**: ManagerAgent 단위 테스트(Task 6-4-2)에서 분해 로직을 LLM 응답 형식에서 분리해야 테스트 결정성을 확보할 수 있음. LLMTaskDecomposer를 MockLLMClient로 간접 제어하면 테스트가 LLM 응답 JSON 형식에 의존하게 되어 TaskDecomposer 인터페이스 계약과 Workflow 실행 로직을 독립적으로 검증하기 어려움
  - **비고**: Phase 3의 MockPlanner, MockLLMClient와 동일한 패턴. `testutil/` 또는 `internal/orchestration/` 에 배치
  - **산출물**: `internal/orchestration/mock_task_decomposer.go`

- [ ] **Task 6-1-4a. Workflow 타입 + topological sort + cycle detection 구현**
  - **무엇**: Task 간 의존 관계를 표현하는 `Workflow` 타입 정의. 위상 정렬(topological sort)로 실행 가능한 순서를 결정하고, 순환 의존(cycle) 감지 시 에러를 반환하는 로직 구현
  - **왜**: 실행 엔진 구현 전에 그래프 정렬과 순환 감지가 먼저 독립적으로 동작해야 함. 이 두 로직을 분리하지 않으면 goroutine 실행 중 발생한 오류와 그래프 구조 오류를 구분할 수 없음
  - **산출물**: `internal/orchestration/workflow.go` (타입 + topological sort + cycle detection)

- [ ] **Task 6-1-4b. Workflow 실행 엔진 구현 — goroutine 병렬 실행 + 실패 전파 + 결과 병합**
  - **무엇**: Task 6-1-4a에서 정렬된 순서를 기반으로, 의존이 없는 Task를 goroutine으로 병렬 실행하는 엔진 구현. 단일 Task 실패 시 나머지 Task에 실패를 전파하고 최종 결과를 병합하는 로직 포함
  - **왜**: 그래프 로직(4a)과 goroutine 관리(4b)를 분리해야 디버깅 지점이 명확해짐. 실패 전파 방식(즉시 전파 vs 나머지 완료 후 전파)은 Task 6-1-4b 구현 시 결정
  - **비고**: `sync.WaitGroup` + `errgroup` 패턴 권장. context 취소로 in-flight goroutine 정리
  - **산출물**: `internal/orchestration/workflow.go` 수정 (실행 엔진 추가)

- [ ] **Task 6-1-5. Workflow unit test 작성**
  - **무엇**: topological sort 순서 검증, 순환 의존 감지 시 에러 반환 검증, 독립 Task 병렬 실행 여부 검증 (goroutine 동시 시작 확인), 단일 Task 실패 시 결과 병합 동작 검증
  - **왜**: topological sort와 cycle detection은 복잡한 로직으로 ManagerAgent 통합 테스트(Task 6-4-2)에서는 세부 케이스를 검증하기 어려움. Workflow 자체를 격리해서 테스트해야 Task 6-4-1 구현 시 의존할 수 있음
  - **산출물**: `internal/orchestration/workflow_test.go`

### Step 6-2. Agent 인터페이스

- [ ] **Task 6-2-1. Agent 인터페이스 정의**
  - **무엇**: `Name() string`, `CanHandle(Task) bool`, `Execute(ctx, Task) (TaskResult, error)` 인터페이스
  - **왜**: Manager가 worker 구현체를 직접 알지 않아도 되도록 경계를 인터페이스로 정의
  - **산출물**: `internal/orchestration/agent.go`

### Step 6-3. Worker Agent 구현

- [ ] **Task 6-3-0. Task → AgentState 변환 어댑터 구현**
  - **무엇**: `orchestration.Task`를 `AgentState`로 변환하는 어댑터 함수 구현. `TaskID`를 `RequestID`로, `InputPayload`를 `UserInput`으로 매핑. 이전 Task의 `TaskResult.Output`(선행 단계 실행 결과)은 `AgentState.Working.SearchResults` 등 WorkingMemory 대응 필드로 주입. WorkerAgent 내부에서 `Runtime.Run()` 호출 전에 사용
  - **왜**: WorkerAgent.Execute(ctx, Task)에서 `Runtime.Run(ctx, AgentState)`를 호출하려면 Task를 AgentState로 변환하는 로직이 필요함. 이 어댑터가 없으면 각 WorkerAgent마다 변환 코드가 중복됨. 특히 FilterAgent는 SearchAgent의 `TaskResult.Output`을 입력으로 받는데, 이를 WorkingMemory에 주입해야 이후 tool 실행 시 데이터를 참조할 수 있음
  - **비고**: 각 WorkerAgent는 독립적인 `AgentState`를 가짐. WorkingMemory 공유가 아닌 `TaskResult.Output` → `AgentState.Working.*` 변환으로 데이터를 전달하는 구조. ManagerAgent가 공유 WorkingMemory를 유지하지 않으며, 데이터 흐름은 항상 `TaskResult → 어댑터 → 다음 AgentState` 경로를 따름
  - **산출물**: `internal/orchestration/task_adapter.go`

- [ ] **Task 6-3-0b. FilterAgent / RankingAgent용 mock tool 구현**
  - **무엇**: `filter_mock` tool (입력: results 배열 + max_price, 출력: 가격 조건에 맞는 결과 배열), `ranking_mock` tool (입력: results 배열, 출력: rating 기준 내림차순 정렬 결과) 구현 및 Registry 등록
  - **왜**: FilterAgent(Task 6-3-2)와 RankingAgent(Task 6-3-3)는 내부에서 Runtime.Run()을 통해 Tool을 실행함. Phase 2 registry에는 `calculator`, `weather_mock`, `search_mock`만 있고 이 두 agent가 사용할 tool이 없으므로 agent 구현 전에 tool이 먼저 있어야 함
  - **비고**: Phase 2의 tool 구현 패턴(`Name()`, `Description()`, `InputSchema()`, `Execute()`)을 그대로 따름. `docs/tools.md`에 두 tool의 spec 추가
  - **산출물**: `internal/tools/filter_mock/filter_mock.go`, `internal/tools/ranking_mock/ranking_mock.go`, `docs/tools.md` 수정

- [ ] **Task 6-3-1. SearchAgent 구현**
  - **무엇**: `hotel_search` task를 처리하는 worker agent
  - **왜**: 시나리오의 첫 번째 단계. search_mock tool을 내부에서 사용
  - **산출물**: `internal/orchestration/search_agent.go`

- [ ] **Task 6-3-2. FilterAgent 구현**
  - **무엇**: `filter_by_price` task를 처리하는 worker agent
  - **왜**: SearchAgent 결과를 입력으로 받아 처리하는 의존 관계가 있는 task 실습
  - **산출물**: `internal/orchestration/filter_agent.go`

- [ ] **Task 6-3-3. RankingAgent 구현**
  - **무엇**: `sort_by_rating` task를 처리하는 worker agent
  - **왜**: FilterAgent 결과를 받아 독립적으로 정렬 처리. 병렬 실행 적합 여부를 판단하는 실습
  - **산출물**: `internal/orchestration/ranking_agent.go`

- [ ] **Task 6-3-4. SummaryAgent 구현**
  - **무엇**: 앞 단계 결과를 받아 LLM으로 요약하는 worker agent
  - **왜**: 마지막 단계에서 LLM 호출이 포함된 task 처리 패턴 실습
  - **산출물**: `internal/orchestration/summary_agent.go`

### Step 6-4. Manager Agent

- [ ] **Task 6-4-1. ManagerAgent 구현**
  - **무엇**: TaskDecomposer와 worker 목록을 주입받아, Task 6-1-4에서 구현한 `Workflow`를 내부적으로 사용해 실행 순서를 결정하고 결과를 병합하는 구조체. ManagerAgent는 분해(Decompose) + Workflow 실행 위임 + 결과 병합 역할
  - **왜**: multi-agent orchestration의 핵심. Workflow가 topological sort와 병렬 실행 엔진을 담당하고, ManagerAgent는 그 조율자 역할. Phase 5에서 익힌 concurrency 패턴을 Workflow 내부에서 실제 적용
  - **비고**: ManagerAgent가 goroutine을 직접 관리하지 않는다 — goroutine 관리는 Workflow가 담당. 역할 분리를 명확히 해야 Task 6-1-5(Workflow unit test)가 독립적으로 의미를 가짐
  - **산출물**: `internal/orchestration/manager_agent.go`

- [ ] **Task 6-4-2. ManagerAgent unit test 작성**
  - **무엇**: worker 선택 로직, Workflow를 통한 병렬/순차 실행 여부, 결과 병합 검증. `testutil/mock_llm.go`(Task 3-3-1)와 표준 `testing.T`를 사용
  - **왜**: manager 로직이 잘못되면 task 순서 오류나 결과 누락이 발생하며 디버깅이 어려움
  - **산출물**: `internal/orchestration/manager_agent_test.go`

### Step 6-5. Multi-Agent 실행 로그

- [ ] **Task 6-5-1. 실행 trace 로그 구현**
  - **무엇**: 호출된 agent 이름, 호출 순서, 각 latency, 실패 지점을 구조화된 로그로 출력
  - **왜**: multi-agent 시나리오는 단일 agent보다 흐름 추적이 복잡하므로 로그가 없으면 디버깅 불가
  - **산출물**: `internal/orchestration/trace.go`

### Step 6-6. 설계 결정 문서화

- [ ] **Task 6-6-1. Phase 6 설계 결정 기록**
  - **무엇**: `orchestration → agent` 의존 방향 결정 근거, ManagerAgent vs Workflow 역할 분리 이유, Task 간 데이터 전달 방식(`TaskResult → 어댑터 → 다음 AgentState`), 실패 전파 방식 결정을 `docs/decisions/phase6.md`에 기록
  - **왜**: orchestration 패키지 의존 방향은 Task 6-0-1에서 결정하지만 코드에는 드러나지 않음. Phase 7 서비스화에서 `orchestration.Task`와 `api.AsyncTask`를 구분해야 할 때 이 문서가 근거가 됨
  - **산출물**: `docs/decisions/phase6.md`

### Phase 6 Exit Criteria

- LLMTaskDecomposer가 사용자 입력을 Task 목록으로 분해 확인
- 의존 관계 없는 Task가 goroutine으로 병렬 실행되는 것 확인
- 의존 관계 있는 Task가 위상 정렬 순서대로 실행되는 것 확인
- SearchAgent → FilterAgent → RankingAgent → SummaryAgent 호텔 검색 시나리오 E2E 통과
- `go test -race ./internal/orchestration/...` 통과 확인 (Workflow 병렬 실행 goroutine race condition 없음)
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase6.md`에 기록 (Task 6-6-1)

---

## Phase 7 — Runtime 서비스화

> **Kafka 도입 여부**: README 기술 스택에 Kafka가 "후반 확장 스택 (Phase 7~)"으로 언급되어 있으나, Phase 8에서는 InMemoryTaskQueue(buffered channel)로 대체한다. Kafka 도입은 Phase 8 완료 후 검증된 큐 인터페이스를 기반으로 선택적으로 추가할 수 있으며, 이 커리큘럼의 필수 범위가 아니다.

### Step 7-0. 서비스화 사전 결정

- [ ] **Task 7-0-1. HTTP 라우터 확인**
  - **무엇**: Phase 0 Task 0-3-3에서 Go 1.22+로 이미 고정됨. 표준 `net/http` ServeMux의 path parameter(`{id}`) 지원이 사용 가능한 상태임을 확인하고 통과
  - **왜**: Task 7-1-2에서 `GET /v1/tasks/{id}` 같은 path parameter 엔드포인트를 구현할 때 라우터 방식을 재확인하는 체크포인트
  - **비고**: Go 버전 변경이 없다면 `go version`으로 확인만 하고 통과. 외부 라우터(`chi` 등)는 도입하지 않음
  - **산출물**: 확인만 (산출물 없음)

- [ ] **Task 7-0-2. cmd/agent-api 진입점 생성**
  - **무엇**: `cmd/agent-api/main.go` 작성. HTTP 서버 초기화(포트 설정, graceful shutdown), 의존성 조립(Redis/Postgres 연결, MemoryManager, TaskQueue, Worker goroutine 시작, HTTP 핸들러 등록)을 담당하는 진입점
  - **왜**: Phase 0 Task 0-3-1에서 `cmd/agent-api/` 디렉터리는 이미 생성됨. 그러나 HTTP 서버 진입점 구현 Task가 없으면 Task 7-1-2(핸들러 구현)를 완성해도 실제로 서버를 띄울 수 없음. `cmd/agent-cli/main.go`(CLI)는 계속 유지되며, 이 Task는 CLI와 독립적인 HTTP 서버 진입점을 추가하는 것
  - **비고**: `cmd/agent-api/main.go`는 `SIGTERM`/`SIGINT` 수신 시 Worker graceful shutdown(Task 7-3-4)을 트리거하는 context cancel을 담당. Worker, TaskQueue, Handler 조립 순서: `InMemoryTaskQueue → Worker(queue, runtime, repo) → handler(queue, repo) → http.Server`
  - **산출물**: `cmd/agent-api/main.go`

- [ ] **Task 7-0-3. Admin 엔드포인트 인증 비목표 명시**
  - **무엇**: `GET /v1/admin/*` 엔드포인트에 인증/인가를 **적용하지 않음**을 `docs/scope.md`에 명시
  - **왜**: Phase 7의 admin API(`GET /v1/admin/tasks`, `GET /v1/admin/sessions/{id}` 등)는 민감한 내용을 반환하지만, 이 커리큘럼의 목적은 runtime 제어 흐름 학습이며 인증/인가는 비목표임. 명시하지 않으면 Phase 7 구현 시 "인증을 달아야 하나"라는 혼동이 생김
  - **비고**: 실제 서비스화 시 반드시 인증이 필요하다는 경고 문구 포함
  - **산출물**: `docs/scope.md` 수정

### Step 7-1. HTTP API

- [ ] **Task 7-1-1. 요청/응답 타입 정의**
  - **무엇**: `RunRequest`, `RunResponse`, `TaskStatusResponse` struct
  - **왜**: JSON 직렬화 기준이 되는 타입 정의 없이는 핸들러 구현 불가
  - **산출물**: `internal/api/types.go`

- [ ] **Task 7-1-2. HTTP 핸들러 구현**
  - **무엇**: `POST /v1/agent/run`, `GET /v1/tasks/{id}`, `GET /v1/sessions/{id}` 엔드포인트
  - **왜**: CLI 입력기를 HTTP 인터페이스로 교체하는 핵심 단계
  - **비고**: `GET /v1/sessions/{id}`는 클라이언트가 자신의 세션 요약(RecentContext, ActiveGoal)을 조회하는 **사용자 facing** 엔드포인트로, 응답에 SessionState 전체가 아닌 요약 필드만 포함. `GET /v1/admin/sessions/{id}`(Task 7-4-3)는 디버깅용으로 SessionState 전체를 반환하는 **관리자 전용** 엔드포인트로 구분
  - **산출물**: `internal/api/handler.go`

- [ ] **Task 7-1-2b. 헬스체크 엔드포인트 구현**
  - **무엇**: `GET /health` 엔드포인트. Redis, Postgres 연결 상태와 Worker 활성 여부를 JSON으로 반환
  - **왜**: 서비스화 목표에서 프로세스가 정상 동작 중인지 확인할 수단이 없으면 운영 자동화(restart policy, load balancer healthcheck)가 불가능함. docker-compose의 `healthcheck` 설정과도 연동 가능
  - **비고**: 응답 형식 예시 — `{"status": "ok", "redis": "ok", "postgres": "ok", "worker": "running"}`. 의존 서비스 연결 실패 시 HTTP 503 반환
  - **산출물**: `internal/api/handler.go` 수정 (`GET /health` 핸들러 추가)

- [ ] **Task 7-1-3. 핸들러 integration test 작성**
  - **무엇**: `httptest`를 사용해 각 엔드포인트의 요청/응답 검증
  - **왜**: API 계층 변경 시 하위 호환성 깨짐을 조기에 감지
  - **산출물**: `internal/api/handler_test.go`

### Step 7-2. Async Task 상태

- [ ] **Task 7-2-1. AsyncTask 타입 및 상태 전이 구현**
  - **무엇**: `queued`, `running`, `succeeded`, `failed` 상태와 상태 전이 검증 로직
  - **왜**: 잘못된 상태 전이(예: queued → succeeded 직접 전환)를 런타임에 잡아야 함
  - **산출물**: `internal/api/async_task.go`, `async_task_test.go`

- [ ] **Task 7-2-2. AsyncTask 결과 저장소 구현**
  - **무엇**: `AsyncTaskRepository` 인터페이스 정의 (`Save`, `Load`, `ListRecent`) + InMemory 구현체. Worker가 task 완료 후 결과를 저장하고, `GET /v1/tasks/{id}` 핸들러가 조회하는 구조
  - **왜**: task 결과를 저장소 없이 in-memory에만 두면 Worker goroutine 종료 시 결과가 소실됨. 또한 `GET /v1/tasks/{id}` 핸들러가 결과를 읽어올 의존 대상이 없으면 핸들러 구현이 불가능함
  - **비고**: Phase 4의 SessionRepository 패턴과 동일한 인터페이스 구조 적용. InMemory 구현 후 필요 시 Redis/Postgres로 교체 가능. **InMemory는 프로세스 재시작 시 소실됨** — Phase 8 Exit Criteria에서 "프로세스 재시작 후 조회 가능"은 아래 Task 7-2-3에서 Redis 구현 후 달성 가능
  - **산출물**: `internal/api/async_task_repository.go`

- [ ] **Task 7-2-3. RedisAsyncTaskRepository 구현**
  - **무엇**: Redis에 AsyncTask 결과를 JSON 직렬화하여 저장/조회하는 구현체. TTL 설정 포함 (완료 task 무기한 보존 방지)
  - **왜**: Phase 8 Exit Criteria의 "프로세스 재시작 후 GET /v1/tasks/{id} 조회 가능"은 InMemory로 달성 불가. Phase 4에서 이미 Redis 연결이 확립되어 있으므로 추가 인프라 없이 구현 가능
  - **비고**: Phase 4 `RedisSessionRepository`와 동일한 패턴. docker-compose에 Redis가 이미 정의되어 있음 (Task 0-2-1)
  - **산출물**: `internal/api/redis_async_task_repository.go`

### Step 7-3. Queue 구조

- [ ] **Task 7-3-0. AsyncTask.Payload → AgentState 변환 어댑터 구현**
  - **무엇**: `POST /v1/agent/run`의 `RunRequest`(Task 7-1-1)를 `AsyncTask.Payload`로 직렬화하는 로직과, Worker goroutine에서 `AsyncTask.Payload`를 역직렬화해 `AgentState`로 변환하는 어댑터 함수 구현
  - **왜**: Task 7-3-3의 Worker 루프가 `AsyncTask.Payload → AgentState` 변환을 전제로 하지만, Payload 타입과 변환 경로가 먼저 정의되지 않으면 Worker 구현 중 타입을 즉석에서 결정해야 함. Phase 6의 `task_adapter.go`(Task 6-3-0)가 같은 이유로 별도 Task였던 것과 동일
  - **비고**: `RunRequest.SessionID`와 `RunRequest.UserInput`을 `AgentState`의 대응 필드로 매핑. Phase 6 `orchestration.Task`와 달리 `AsyncTask`는 HTTP API 요청 단위이므로 WorkingMemory 주입이 필요 없음
  - **산출물**: `internal/queue/payload_adapter.go`

- [ ] **Task 7-3-1. TaskQueue 인터페이스 정의**
  - **무엇**: `Enqueue(task)`, `Dequeue() (task, error)` 인터페이스
  - **왜**: in-memory channel과 Redis Stream을 교체할 수 있도록 인터페이스로 먼저 분리
  - **산출물**: `internal/queue/task_queue.go`

- [ ] **Task 7-3-2. InMemoryTaskQueue 구현**
  - **무엇**: buffered channel 기반 TaskQueue 구현체
  - **왜**: Redis 없이도 API 서버 + worker 분리 구조를 검증할 수 있음
  - **산출물**: `internal/queue/in_memory_task_queue.go`

- [ ] **Task 7-3-3. Worker 루프 구현**
  - **무엇**: queue에서 `AsyncTask`를 꺼내 `AsyncTask.Payload`를 `AgentState`로 변환한 뒤 `runtime.Run()`을 호출하고 결과를 `AsyncTaskRepository`에 저장하는 goroutine. `AsyncTask`(HTTP API 단위 task)와 Phase 6의 `orchestration.Task`(agent 내부 sub-task)는 별도 개념으로, 이 Worker는 HTTP API 요청 단위만 처리함
  - **왜**: API 서버와 실행 엔진을 논리적으로 분리하는 핵심 단계
  - **산출물**: `internal/queue/worker.go`

- [ ] **Task 7-3-4. Worker graceful shutdown 구현**
  - **무엇**: `context.Done()` 신호 수신 시 현재 처리 중인 task를 완료한 뒤 종료하는 로직. `sync.WaitGroup`으로 in-flight task 추적
  - **왜**: Worker가 처리 중인 task가 있을 때 프로세스가 강제 종료되면 task 결과가 저장소에 기록되지 않고 소실됨. `queued` 상태로 남아 있는 task는 재시작 후 재처리 불가 (InMemoryQueue 기준). 이 로직이 없으면 `SIGTERM`이 들어오는 순간 실행 중 데이터가 유실됨
  - **산출물**: `internal/queue/worker.go` 수정

### Step 7-4. Admin / Debug API

- [ ] **Task 7-4-1. 최근 task 목록 엔드포인트 구현**
  - **무엇**: `GET /v1/admin/tasks` — 최근 N개 AsyncTask 목록 반환 (상태, 생성 시각, 소요 시간 포함)
  - **왜**: 운영 중 task 처리 현황을 API로 확인하는 가장 기본적인 수단. 로그 grep 없이 처리량과 상태 분포를 파악할 수 있음
  - **산출물**: `internal/api/admin_handler.go`

- [ ] **Task 7-4-2. 실패 task 조회 엔드포인트 구현**
  - **무엇**: `GET /v1/admin/tasks/failed` — `failed` 상태의 AsyncTask만 필터링해 에러 메시지 포함 반환
  - **왜**: 실패 task를 별도 조회할 수 없으면 모든 task 목록에서 수동으로 찾아야 함. 에러 유형별 패턴 파악이 어려워짐
  - **산출물**: `internal/api/admin_handler.go` 수정

- [ ] **Task 7-4-3. session dump 엔드포인트 구현**
  - **무엇**: `GET /v1/admin/sessions/{id}` — 지정 SessionID의 SessionState 전체 내용 반환
  - **왜**: 특정 session의 상태를 API로 덤프할 수 없으면 session 관련 버그(맥락 누락, 잘못된 복원) 디버깅이 코드 변경 없이 불가능함
  - **산출물**: `internal/api/admin_handler.go` 수정

- [ ] **Task 7-4-4. tool 호출 통계 엔드포인트 구현**
  - **무엇**: `GET /v1/admin/stats/tools` — tool 이름별 호출 횟수, 평균 latency, 에러율 반환. 통계는 in-memory 누적 (프로세스 재시작 시 초기화)
  - **왜**: 어떤 tool이 자주 실패하거나 느린지 확인하지 못하면 Phase 8 per-tool timeout 설정 최적화에 데이터 근거가 없음
  - **비고**: 집계 구조체는 `tool_stats.go`에 분리. Phase 8 OTel 연동 후 이 통계는 메트릭으로 대체될 수 있음
  - **산출물**: `internal/api/admin_handler.go` 수정, `internal/api/tool_stats.go`

### Step 7-5. ask_user 비동기 처리

- [ ] **Task 7-5-0. ask_user 비동기 처리 설계 결정**
  - **무엇**: HTTP API 환경에서 `ask_user` ActionType 발생 시 runtime loop가 차단 없이 대기하는 메커니즘을 설계. `runtime.Run()`이 반환하고 클라이언트 입력 수신 후 재개하는 방식 vs loop 내 channel 대기 방식 중 선택하고 결정 사항을 문서화
  - **왜**: Task 7-5-1은 `runtime.go`, `async_task.go`, `handler.go` 3개 파일에 동시 변경이 필요한 복잡한 작업. 구현 방식을 사전에 확정하지 않으면 구현 도중 `runtime.Run()` 시그니처 변경 여부를 결정짓는 상황이 발생함
  - **비고**: 권장 방향 — `runtime.Run()`이 `ask_user`를 만나면 반환하고, 사용자 입력 수신 후 새 `runtime.Run()` 호출로 재개. 이렇게 하면 기존 loop 시그니처 변경 없이 구현 가능하며 Worker goroutine 차단도 없음
  - **산출물**: `docs/decisions/phase7-ask-user.md`

- [ ] **Task 7-5-1. ask_user ActionType 비동기 대기 메커니즘 구현**
  - **무엇**: HTTP API 환경에서 Runtime이 `ask_user` ActionType을 만났을 때, 즉시 응답 대신 task를 `waiting_for_user` 상태로 전환하고 클라이언트가 사용자 입력을 제출할 수 있는 `POST /v1/tasks/{id}/input` 엔드포인트 구현. 입력 수신 시 해당 task를 재개
  - **왜**: Phase 3(Task 3-1-1)에서 `ask_user`를 "Phase 8 HTTP API 환경에서 별도 설계"로 미뤘음. Phase 7에서 HTTP API와 AsyncTask 상태 기계가 완성되므로 이 시점에 구현하지 않으면 `ask_user`는 CLI에서만 동작하는 미완성 ActionType으로 남음
  - **비고**: `AsyncTask` 상태에 `waiting_for_user` 추가 필요 (Task 7-2-1 `async_task.go` 수정). Phase 3(Task 3-1-1)에서 CLI 환경의 `ask_user → respond_directly 대체` 처리는 그대로 유지하되, HTTP API 환경에서는 이 Task의 메커니즘으로 처리
  - **산출물**: `internal/api/async_task.go` 수정 (`waiting_for_user` 상태 추가), `internal/api/handler.go` 수정 (`POST /v1/tasks/{id}/input` 엔드포인트), `internal/agent/runtime.go` 수정 (ask_user 감지 후 channel 또는 저장소 경유 대기 패턴)

### Step 7-6. 설계 결정 문서화

- [ ] **Task 7-6-1. Phase 7 설계 결정 기록**
  - **무엇**: `orchestration.Task`(Phase 6 sub-task)와 `api.AsyncTask`(HTTP 요청 단위) 개념 분리 근거, ask_user 비동기 대기 메커니즘 선택 이유(runtime.Run() 재호출 방식), HTTP 라우터 선택 결정을 `docs/decisions/phase7.md`에 기록
  - **왜**: 두 Task 타입의 역할 경계가 코드에서는 명확하지 않음. Phase 9 문서화 시 이 경계를 설명할 기반이 됨
  - **산출물**: `docs/decisions/phase7.md`

### Phase 7 Exit Criteria

- `cmd/agent-api/main.go`로 HTTP 서버가 실제로 기동되는 것 확인 (`go run ./cmd/agent-api/`)
- `POST /v1/agent/run` 요청이 task를 queue에 넣고 즉시 task ID 반환 확인
- `GET /v1/tasks/{id}`로 실행 중 / 완료 / 실패 상태 조회 확인
- Worker가 queue에서 task를 꺼내 `runtime.Run()`을 호출하고 결과를 저장소에 영속화 확인
- 프로세스 재시작 후 `GET /v1/tasks/{id}`로 이전 task 결과 조회 가능 확인
- `httptest` 기반 handler integration test 통과 (`go test ./internal/api/...`)
- `GET /health` 엔드포인트가 Redis/Postgres/Worker 상태를 반환하고, 의존 서비스 장애 시 HTTP 503 반환 확인
- `GET /v1/admin/tasks`로 최근 task 목록 조회, `GET /v1/admin/tasks/failed`로 실패 task 필터링 조회 확인
- `GET /v1/admin/stats/tools`로 tool별 호출 횟수 및 에러율 조회 확인
- `ask_user` ActionType 발생 시 task가 `waiting_for_user` 상태로 전환되고, `POST /v1/tasks/{id}/input`으로 입력 제출 후 task가 재개되는 것 확인
- `go test -race ./internal/queue/...` 통과 확인 (Worker goroutine 동시 접근 race condition 없음)
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase7.md`에 기록

---

## Phase 8 — 운영 고도화

### Step 8-1. Timeout / Cancellation

- [ ] **Task 8-1-1. tool별 timeout 설정 외부화**
  - **무엇**: Phase 5(Task 5-1-1)에서 확립한 context deadline 전달 패턴 위에, tool 이름별 timeout 값을 설정 파일(`config`)에서 주입받아 ToolRouter가 사용하도록 변경. 기본값 fallback 포함
  - **왜**: Phase 5에서는 코드에 하드코딩된 timeout으로 패턴만 확립함. tool마다 응답 시간이 다르므로 (예: search tool은 5s, calculator는 1s) 설정으로 분리해야 운영 중 조정 가능
  - **비고**: Phase 5에서 `router.go`에 이미 context deadline 전달이 구현되어 있음. 이 Task는 timeout 값을 config로 외부화하는 것이 목적이며, context 전달 로직 자체를 재구현하지 않음
  - **산출물**: `internal/config/config.go` 수정 (tool timeout 맵 추가), `internal/tools/router.go` 수정 (config에서 timeout 값 읽기)

- [ ] **Task 8-1-2. 전체 request deadline 설정**
  - **무엇**: runtime.Run() 진입 시 전체 요청에 대한 context deadline 설정
  - **왜**: tool 개별 timeout만으로는 loop 자체가 무한히 도는 것을 막을 수 없음
  - **산출물**: `internal/agent/runtime.go` 수정

### Step 8-2. 비용 제어

- [ ] **Task 8-2-1. session별 token 누적 추적**
  - **무엇**: Phase 3의 TokenUsage를 session 단위로 합산하는 집계 로직. TokenTracker는 `map[sessionID]TokenUsage` 형태의 자체 in-memory 저장소를 갖고, SessionRepository에 의존하지 않음
  - **왜**: 요청별 token이 아닌 session 전체 비용이 실제 운영 비용 단위임. `llm → state` 패키지 의존을 만들지 않으려면 TokenTracker가 독립 저장소를 갖는 구조가 필요
  - **비고**: Phase 7에서 Worker goroutine이 동시에 여러 개 실행되므로 map 접근은 반드시 `sync.Mutex`로 보호해야 함. `go test -race`로 race detector 검증 필수
  - **산출물**: `internal/llm/token_tracker.go`

- [ ] **Task 8-2-2. 비용 한도 초과 시 중단 정책 구현**
  - **무엇**: session 누적 token이 임계값 초과 시 loop를 중단하는 정책
  - **왜**: 한도 없이 두면 단일 session이 과도한 비용을 발생시킬 수 있음
  - **산출물**: `internal/agent/cost_policy.go`

### Step 8-3. Observability

- [ ] **Task 8-3-0. OpenTelemetry 의존성 추가**
  - **무엇**: `go get go.opentelemetry.io/otel`, `go get go.opentelemetry.io/otel/sdk`, `go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`(또는 stdout exporter) 실행 후 `go mod tidy`
  - **왜**: Task 8-3-1(OTel SDK 초기화)에서 `go.opentelemetry.io/otel` 패키지를 import하는데, go.mod에 없으면 첫 줄 작성 즉시 빌드가 깨짐. OTel은 관련 패키지가 여러 모듈로 나뉘어 있어 한 번에 추가해야 의존성 충돌 없이 go mod tidy가 통과됨
  - **비고**: stdout exporter부터 시작하는 것을 권장 — OTLP/Jaeger는 exporter 교체만으로 나중에 변경 가능. 선택한 exporter와 버전을 Task 완료 시 기록할 것
  - **산출물**: `go.mod`, `go.sum` 갱신

- [ ] **Task 8-3-1. OpenTelemetry SDK 초기화**
  - **무엇**: `TracerProvider` 초기화, exporter 설정(stdout 또는 OTLP), SDK bootstrap 코드 작성. `docker-compose.yml`에 Jaeger 또는 OTEL Collector 컨테이너 추가
  - **왜**: span을 추가하기 전에 TracerProvider가 없으면 span 데이터가 어디에도 전송되지 않음. exporter 설정 없이는 trace 확인 자체가 불가능
  - **산출물**: `internal/observability/tracer.go` (초기화 함수), `docker-compose.yml` 수정

- [ ] **Task 8-3-2. Logger에 OTel span trace ID 연동**
  - **무엇**: Phase 3(Task 3-6-1)에서 만든 structured logger의 `trace_id` 필드를 OTel span의 TraceID/SpanID로 교체. `context.Context`에서 span을 꺼내 logger 필드에 자동 주입하도록 수정
  - **왜**: Phase 3의 logger는 request_id 기반의 자체 trace_id를 사용함. OTel 연동 이후에는 span TraceID가 trace의 단일 기준이 되어야 Jaeger 등 외부 트레이서와 로그가 같은 ID로 연결됨
  - **비고**: logger 인터페이스는 변경하지 않음 — 내부 구현에서 span을 꺼내는 방식으로만 수정. Task 8-3-1(OTel SDK 초기화) 완료 이후에 진행
  - **산출물**: `internal/observability/logger.go` 수정

- [ ] **Task 8-3-3. OpenTelemetry trace 연결**
  - **무엇**: request → planner → tool → verifier 구간에 OTel span 추가
  - **왜**: latency 병목이 어느 컴포넌트에 있는지 trace 없이는 측정 불가
  - **산출물**: 각 컴포넌트에 OTel span 추가

### Step 8-4. 에러 분류 체계 고도화

- [ ] **Task 8-4-1. 에러 타입 분류 확장**
  - **무엇**: Phase 2에서 정의한 기본 에러 타입에 `user_error`, `system_error`, `provider_error` 분류 추가
  - **왜**: 기본 retryable/fatal 구분은 Phase 2에서 정의됨. 이 단계에서는 알림, 사용자 응답 메시지, 모니터링 레이블에 사용할 운영 관점의 분류를 추가하는 것이 목적
  - **산출물**: `internal/agent/errors.go` 확장

### Step 8-5. Policy Layer

- [ ] **Task 8-5-1. PolicyLayer 구현**
  - **무엇**: tool 사용 제한, 사용자별 max step, 비용 한도를 단일 `Policy` 인터페이스로 묶는 파사드 레이어. 기존 `cost_policy.go`(Task 8-2-2)와 Phase 1의 max step 처리를 PolicyLayer 내부에서 호출하도록 통합
  - **왜**: 정책이 여러 곳에 분산되면 정책 변경 시 누락이 생김. 파사드 구조를 명시하지 않으면 Task 8-2-2의 `cost_policy.go`와 역할이 중복되어 어느 쪽을 수정해야 할지 모호해짐
  - **비고**: `PolicyLayer`는 기존 구현체를 교체하는 것이 아닌 단일 진입점(`PolicyLayer.Check()`)으로 감싸는 파사드 역할. `cost_policy.go`는 그대로 유지하되 `PolicyLayer.Check()`가 내부적으로 호출하는 구조. `runtime.go`에서 기존 개별 정책 호출을 `PolicyLayer.Check()` 단일 호출로 교체
  - **산출물**: `internal/agent/policy.go`, `internal/agent/runtime.go` 수정 (PolicyLayer 호출 경로 추가)

### Step 8-6. 설계 결정 문서화

- [ ] **Task 8-6-1. Phase 8 설계 결정 기록**
  - **무엇**: PolicyLayer 파사드 구조 선택 이유, per-tool timeout 외부화 방식, OTel exporter 선택(stdout vs OTLP vs Jaeger) 근거, TokenTracker 동시성 보호 전략을 `docs/decisions/phase8.md`에 기록
  - **왜**: Phase 8은 운영 고도화 단계로 각 결정이 인프라 변경과 연결됨. Phase 9 포트폴리오 문서에서 "운영 가능성"을 설명할 때 이 결정들이 근거가 됨
  - **산출물**: `docs/decisions/phase8.md`

### Phase 8 Exit Criteria

- per-tool timeout 초과 시 `tool_execution_failed` (retryable) 에러 반환 확인
- 전체 request deadline 초과 시 loop 즉시 종료 및 context.Canceled 에러 반환 확인
- session 누적 token이 임계값 초과 시 loop 중단 확인
- OTel span이 request → planner → tool → verifier 구간에 기록 확인
- PolicyLayer에서 tool 사용 제한 / max step / 비용 한도 단일 인터페이스로 적용 확인
- `go test -race ./internal/llm/...` 통과 확인 (TokenTracker 동시 접근 안전성)
- 해당 Phase의 주요 설계 결정을 `docs/decisions/phase8.md`에 기록 (Task 8-6-1)

---

## Phase 9 — 문서화 / 포트폴리오

### Step 9-0. CI 자동화

- [ ] **Task 9-0-1. GitHub Actions CI 워크플로우 추가**
  - **무엇**: push/PR 시 `go build ./...` + `go vet ./...` + `make test-unit`(`go test ./...`, integration 태그 제외) + `go test -race ./...`(unit 범위)를 실행하는 워크플로우
  - **왜**: 포트폴리오에서 CI 배지가 있는 레포는 코드 신뢰도를 높임. "코드가 실제로 돌아간다"는 증거가 CI 통과로 증명되어야 함. Phase 4-0-1에서 수립한 `make test-unit` 타겟을 여기서 활용
  - **비고**: 통합 테스트(`//go:build integration`)는 인프라(Redis, Postgres) 의존 때문에 CI 실행 대상에서 제외. 추후 필요 시 Docker service를 GitHub Actions에 추가해 확장 가능
  - **산출물**: `.github/workflows/ci.yml`

### Step 9-1. README 고도화

- [ ] **Task 9-1-1. README 핵심 구조 다이어그램 추가**
  - **무엇**: 텍스트 기반 아키텍처 다이어그램 + 실행 방법 + 예시 시나리오 추가
  - **왜**: README만 읽어도 전체 구조를 파악할 수 있어야 포트폴리오로서 가치가 있음
  - **산출물**: `README.md` 갱신

### Step 9-2. 아키텍처 문서

- [ ] **Task 9-2-1. 컴포넌트별 아키텍처 문서 작성**
  - **무엇**: runtime overview, planner, memory, tool router, multi-agent 각각의 설계 의도와 경계를 설명하는 문서
  - **왜**: 코드만 있으면 설계 의도가 드러나지 않음. 왜 이렇게 나눴는지를 설명해야 설계 역량을 보여줄 수 있음
  - **비고**: Phase 0 Task 0-5-1에서 작성한 `docs/architecture-overview.md`는 전체 흐름 다이어그램(개요). 이 Task의 산출물은 컴포넌트별 심화 문서로 역할이 다름. `docs/architecture-overview.md`는 이 Task에서 갱신하거나 "개요는 이 파일 참고"로 상호 링크를 추가해 중복을 방지
  - **산출물**: `docs/01-runtime-overview.md`, `docs/02-planner.md`, `docs/03-memory.md`, `docs/04-tool-router.md`, `docs/05-multi-agent.md`, `docs/architecture-overview.md` 링크 갱신

### Step 9-3. 실행 시나리오 문서

- [ ] **Task 9-3-1. 시나리오별 흐름 문서 작성**
  - **무엇**: 날씨 질의, 호텔 검색, 실패 후 retry, multi-agent 흐름을 단계별로 기술하고 실제 실행 로그 예시 포함
  - **왜**: 실제 동작 증거가 없는 포트폴리오는 신뢰도가 낮음. 시나리오 + 로그 조합이 핵심
  - **산출물**: `docs/scenarios/`

### Phase 9 Exit Criteria

- README만 읽어도 전체 아키텍처와 실행 방법을 파악할 수 있는 수준의 다이어그램 + 예시 포함 확인
- Phase 9까지 모든 컴포넌트의 설계 의도와 경계를 설명하는 아키텍처 문서 완비
- 날씨 질의, 호텔 검색, 실패 후 retry, multi-agent 흐름 4개 시나리오에 실제 실행 로그 첨부 확인
- `go test ./...` 전체 통과
