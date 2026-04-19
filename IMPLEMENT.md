# IMPLEMENT.md — 구현 체크리스트

PLAN.md(Phase 5 Task 5-1-2 이후 ~ Phase 9)의 실행 순서와 진행 상태를 추적한다.
각 Unit은 PLAN.md의 특정 Exit Criteria에 매핑된다. 설계 근거와 Decision Point는 PLAN.md에만 존재한다.

---

## Phase 5 — Verifier / Retry / Concurrency

- [ ] **Unit 5-0. Phase 4 회귀 체크**
  - **Purpose**: → PLAN: Phase 5 > Task 5-0 (Session 복원 + Long-term Memory 주입 관찰)
  - **Approach**: 기존 통합 테스트 재실행으로 세션 복원과 LoadRelevantMemory → prompt 반영 경로를 확인하고, `go test ./...` 전체를 돌려 기준선을 잡는다.

- [ ] **Unit 5-1-2. 독립 tool 2개 errgroup 병렬 실행 + 취소 전파**
  - **Purpose**: → PLAN: Phase 5 > Task 5-1-2 (동시 시작 / 실패 시 취소 / race 통과)
  - **Approach**: ToolRouter 상위에 errgroup 기반 병렬 실행 경로를 얕게 추가하고, 의도적 지연·실패 tool을 주입하는 `-race` 테스트로 3개 관찰 지점을 재현한다.
  - **Prerequisite**: Unit 5-0 통과 (5-1-1 context 전파 패턴이 유효해야 errgroup이 그 위에 얹힘)

- [ ] **Unit 5-2. Verifier 경계 + Runtime done/retry/fail 3분기**
  - **Purpose**: → PLAN: Phase 5 > Task 5-2 (3분기 관찰)
  - **Approach**: `internal/verifier`에 Verifier 인터페이스와 기본 구현을 두고 Runtime loop 종료 지점 직전에 호출, 반환 값으로 Status 전이와 loop 계속 여부를 분기한다.
  - **Prerequisite**: Unit 5-0 통과 (Phase 5 전체의 기준선)

- [ ] **Unit 5-3. RetryPolicy 단일 지점 + LLMPlanner 하드코딩 retry 제거**
  - **Purpose**: → PLAN: Phase 5 > Task 5-3 (상한 동작 + llm_parse_error 재시도 단일 경로)
  - **Approach**: RetryPolicy를 Runtime 쪽에 두고 verifier의 retry 신호와 결합한 뒤, LLMPlanner의 기존 1회 재시도 로직을 제거하고 Phase 3 Task 3-4-7 회귀 테스트로 보호한다.
  - **Prerequisite**: Unit 5-2 통과 (verifier가 retry 신호를 낼 수 있어야 정책이 물릴 지점이 생김)

- [ ] **Unit 5-4. 에러 유형 → loop 제어 신호 단일 분기**
  - **Purpose**: → PLAN: Phase 5 > Task 5-4 (tool_not_found 즉시 종료 / tool_execution_failed+timeout retry / 빈 결과 속행)
  - **Approach**: AgentError 타입을 입력으로 받아 `{fatal 종료, RetryPolicy 위임, loop 속행}` 중 하나를 반환하는 단일 함수를 도입하고, Runtime에 흩어진 에러 분기를 이 함수 호출로 치환한다.
  - **Prerequisite**: Unit 5-3 통과 (RetryPolicy 경계가 있어야 "위임" 분기를 실제로 연결 가능)

- [ ] **Unit 5-5. Reflector 경계 + Reflection이 다음 Plan에 반영**
  - **Purpose**: → PLAN: Phase 5 > Task 5-5 (두 번째 prompt에 missing conditions 포함 / 두 번째 step에서 FinalAnswer 채워짐)
  - **Approach**: `internal/verifier`에 Reflector 인터페이스를 추가하고 AgentState에 reflection 보관 슬롯, prompt_builder에서 해당 필드 반영, Runtime에서 reflection 신호 시 loop 속행 분기를 연결한다.
  - **Prerequisite**: Unit 5-2 통과 (Verifier 경계가 Runtime에 붙어 있어야 Reflector가 같은 축에 부착됨)

- [ ] **Unit 5-D. Phase 5 설계 결정 기록**
  - **Purpose**: → PLAN: Phase 5 Exit Criteria (`docs/decisions/phase5.md` 기록)
  - **Approach**: Verifier vs Reflector 역할 분리, RetryPolicy 단일화, Failure 분기 기준을 Phase 5 구현 종료 시점에 `docs/decisions/phase5.md`로 정리한다.
  - **Prerequisite**: Unit 5-1-2 ~ 5-5 전부 통과

---

## Phase 6 — Multi-Agent Orchestration

> Decision Point 6-D1 확정: **선택지 A (`orchestration → agent`)**. Phase 6 전체 Unit이 이 방향을 전제로 구현됨.

- [ ] **Unit 6-0. Phase 5 회귀 체크**
  - **Purpose**: → PLAN: Phase 6 > Task 6-0 (done/retry/fail 3분기 / retry 상한 / verifier 테스트 통과)
  - **Approach**: `go test -race ./internal/agent/... ./internal/verifier/...` 와 Phase 5 Exit Criteria 시나리오를 재실행해 기준선을 잡는다.

- [ ] **Unit 6-1. Workflow DAG 정렬·병렬·cycle·실패 전파**
  - **Purpose**: → PLAN: Phase 6 > Task 6-1 (4개 관찰 지점 + race 통과)
  - **Approach**: `internal/orchestration`에 Task/Workflow 타입과 위상 정렬 + cycle detection + errgroup 병렬 실행 + 취소 전파를 구현하고, 4개 관찰 지점을 각각 재현하는 `-race` 테스트를 작성한다.
  - **Prerequisite**: Unit 6-0 통과 + Unit 5-1-2의 errgroup 패턴이 재사용 가능

- [ ] **Unit 6-2. 호텔 검색 E2E (Search → Filter → Ranking → Summary)**
  - **Purpose**: → PLAN: Phase 6 > Task 6-2 (정확한 순서 trace / 최종 요약 / Workflow 정렬 해소)
  - **Approach**: TaskDecomposer + Manager + WorkerAgent(Runtime 주입)를 조립하고, Filter/Ranking mock tool과 MockLLMClient 시나리오로 4단계 실행 trace와 최종 응답을 테스트로 재현한다.
  - **Prerequisite**: Unit 6-1 통과 (Workflow 경계가 있어야 시나리오를 올릴 바닥이 생김)

- [ ] **Unit 6-D. Phase 6 설계 결정 기록**
  - **Purpose**: → PLAN: Phase 6 Exit Criteria (`docs/decisions/phase6.md` 기록)
  - **Approach**: 의존 방향(6-D1 확정 경위), Manager vs Workflow 역할 분리, Task 간 데이터 전달 방식을 `docs/decisions/phase6.md`에 정리한다.
  - **Prerequisite**: Unit 6-1 ~ 6-2 통과

---

## Phase 7 — Runtime 서비스화

> Decision Point 7-D1 권장안(표준 `net/http` ServeMux), 7-D2 권장안(ask_user 반환 + 재호출), 7-D3 확정(**admin 인증 미적용**)을 전제로 구현됨.

- [ ] **Unit 7-0. Phase 6 회귀 체크**
  - **Purpose**: → PLAN: Phase 7 > Task 7-0 (호텔 E2E trace 관찰 + orchestration race 통과)
  - **Approach**: Phase 6 시나리오와 `go test -race ./internal/orchestration/...`를 재실행해 기준선을 잡는다.

- [ ] **Unit 7-S. `docs/scope.md`에 admin 인증 미적용 명시**
  - **Purpose**: → PLAN: Phase 7 > Decision Point 7-D3 확정 내용 (Task 7-4의 전제)
  - **Approach**: 기존 `docs/scope.md`에 "admin 엔드포인트는 인증/인가를 적용하지 않는다"와 근거 한 줄을 추가한다.
  - **Prerequisite**: Unit 7-0 통과

- [ ] **Unit 7-1. `cmd/agent-api` 기동 + 4개 엔드포인트 응답**
  - **Purpose**: → PLAN: Phase 7 > Task 7-1 (`/v1/agent/run`, `/v1/tasks/{id}`, `/v1/sessions/{id}`, `/health` 라운드트립)
  - **Approach**: `cmd/agent-api` 진입점과 `internal/api` 핸들러를 `net/http` ServeMux로 연결하고, `httptest` 기반 핸들러 테스트로 4개 엔드포인트 응답을 재현한다.
  - **Prerequisite**: Unit 7-0 통과

- [ ] **Unit 7-2. Queue + Worker, 단일/multi-agent 분기, graceful shutdown**
  - **Purpose**: → PLAN: Phase 7 > Task 7-2 (task ID 즉시 반환 / 단일·multi 분기 / 상태 전이 / SIGTERM in-flight 보존 / race 통과)
  - **Approach**: `internal/queue`에 TaskQueue 인터페이스 + 버퍼 채널 InMemory 구현 + Worker loop + 단일/multi 경로 라우팅 + AsyncTask 상태 기계와 InMemory Repository를 구현하고, SIGTERM 주입 시 완료 보장을 `-race` 테스트로 재현한다.
  - **Prerequisite**: Unit 7-1 통과 (핸들러가 Queue에 enqueue할 대상이 있어야 의미가 있음)

- [ ] **Unit 7-3. RedisAsyncTaskRepository 교체 + 재시작 복원**
  - **Purpose**: → PLAN: Phase 7 > Task 7-3 (재시작 후 동일 task ID 조회 시 이전 결과 반환)
  - **Approach**: InMemory AsyncTaskRepository 인터페이스 뒤에 Redis 구현을 붙이고 주입을 교체한 뒤, 프로세스 재시작 후 조회 성공을 integration test로 재현한다.
  - **Prerequisite**: Unit 7-2 통과 (Repository 인터페이스와 상태 기계가 먼저 필요)

- [ ] **Unit 7-4. Admin API 4종 + tool 통계 집계**
  - **Purpose**: → PLAN: Phase 7 > Task 7-4 (최근/실패 task + session dump + tool 통계 JSON + 호출 누적 관찰)
  - **Approach**: `internal/api`에 admin 라우트 4개와 tool 호출 횟수/평균 latency 집계기를 추가하고, 동일 tool 다회 호출 시 통계 증가를 테스트로 확인한다.
  - **Prerequisite**: Unit 7-2 통과 + Unit 7-S 통과 (인증 미적용 방침이 scope 문서에 고정된 상태)

- [ ] **Unit 7-5. HTTP `ask_user` 대기/재개**
  - **Purpose**: → PLAN: Phase 7 > Task 7-5 (waiting_for_user 전이 + 입력 제출 후 succeeded 도달)
  - **Approach**: AsyncTask에 `waiting_for_user` 상태를 추가하고, Runtime이 `ask_user` 시 반환하도록 하며, 입력 제출 엔드포인트에서 새 Run() 호출로 재개하는 경로를 mock LLM 시나리오로 재현한다.
  - **Prerequisite**: Unit 7-2 통과 (AsyncTask 상태 기계 위에 새 상태를 덧붙이는 구조)

- [ ] **Unit 7-D. Phase 7 설계 결정 기록**
  - **Purpose**: → PLAN: Phase 7 Exit Criteria (`docs/decisions/phase7.md` 기록)
  - **Approach**: 라우터 선택(7-D1), ask_user 처리 방식(7-D2), admin 인증 범위(7-D3), `orchestration.Task` vs `api.AsyncTask` 개념 분리 근거를 `docs/decisions/phase7.md`에 정리한다.
  - **Prerequisite**: Unit 7-1 ~ 7-5 전부 통과

---

## Phase 8 — 운영 고도화

> Decision Point 8-D1 권장안(stdout exporter 시작, 교체 가능 구조)을 전제로 구현됨. Phase 9에서 trace 시각화가 필요하면 OTLP로 교체.

- [ ] **Unit 8-0. Phase 7 회귀 체크**
  - **Purpose**: → PLAN: Phase 8 > Task 8-0 (Task 7-2 ~ 7-5 Exit Criteria + queue/api race 통과)
  - **Approach**: `go test -race ./internal/queue/... ./internal/api/...`와 Phase 7 시나리오를 재실행해 기준선을 잡는다.

- [ ] **Unit 8-1. tool별 timeout config + 전체 request deadline**
  - **Purpose**: → PLAN: Phase 8 > Task 8-1 (config 주입 tool timeout 관찰 + 전체 deadline 초과 시 loop 종료)
  - **Approach**: config에 tool 이름 → timeout 맵을 추가하고 ToolRouter에서 이 값을 context deadline으로 적용, Runtime 진입 시점에 전체 request deadline을 설정한다.
  - **Prerequisite**: Unit 8-0 통과

- [ ] **Unit 8-2. Session token tracker + 비용 한도 → loop 중단**
  - **Purpose**: → PLAN: Phase 8 > Task 8-2 (임계값 교차 직후 loop 종료 + llm race 통과)
  - **Approach**: session 단위 누적 token tracker를 동시성 안전하게 구현하고 임계값 초과 시 loop 중단 신호를 Runtime에 연결, `-race` 테스트로 확인한다.
  - **Prerequisite**: Unit 8-0 통과

- [ ] **Unit 8-3. OTel SDK + 구간 span + logger trace_id 연동**
  - **Purpose**: → PLAN: Phase 8 > Task 8-3 (단일 trace span tree + 로그 trace_id 일치)
  - **Approach**: OTel SDK 초기화와 stdout exporter를 붙이고 Runtime/Planner/Tool/Verifier/Memory 경계마다 span을 생성, structured logger의 trace_id 필드를 활성 span의 TraceID로 채운다.
  - **Prerequisite**: Unit 8-0 통과

- [ ] **Unit 8-4. PolicyLayer 파사드 + Runtime 단일 호출**
  - **Purpose**: → PLAN: Phase 8 > Task 8-4 (개별 정책 호출 제거 + `Policy.Check()` 단일 호출 + 3개 정책 유효)
  - **Approach**: tool 사용 제한 / max step / 비용 한도를 묶는 PolicyLayer를 도입하고 Runtime loop의 개별 정책 호출을 `Policy.Check()` 한 곳으로 치환한다.
  - **Prerequisite**: Unit 8-2 통과 (비용 한도가 Policy에 합류할 준비가 되어야 함)

- [ ] **Unit 8-5. 에러 책임 주체(user/system/provider) 분류 레이블**
  - **Purpose**: → PLAN: Phase 8 > Task 8-5 (대표 에러 4종 예상 레이블 태깅)
  - **Approach**: AgentError에 기존 retryable/fatal과 직교하는 책임 주체 필드를 추가하고, 각 에러 생성 지점에서 user/system/provider 중 하나를 지정한다.
  - **Prerequisite**: Unit 8-0 통과

- [ ] **Unit 8-D. Phase 8 설계 결정 기록**
  - **Purpose**: → PLAN: Phase 8 Exit Criteria (`docs/decisions/phase8.md` 기록)
  - **Approach**: OTel exporter 선택(8-D1), PolicyLayer 파사드, TokenTracker 동시성 전략을 `docs/decisions/phase8.md`에 정리한다.
  - **Prerequisite**: Unit 8-1 ~ 8-5 전부 통과

---

## Phase 9 — 문서화 / 포트폴리오

- [ ] **Unit 9-0. Phase 8 회귀 체크**
  - **Purpose**: → PLAN: Phase 9 > Task 9-0 (전체 race 통과 + integration 통과)
  - **Approach**: `go test -race ./...`와 `docker-compose up` 환경에서 `make test-integration`을 실행해 기준선을 잡는다.

- [ ] **Unit 9-1. GitHub Actions CI + README 배지**
  - **Purpose**: → PLAN: Phase 9 > Task 9-1 (push/PR 시 녹색 체크 + 배지 + integration 제외 명시)
  - **Approach**: `.github/workflows`에 build + vet + `make test-unit` + race 실행 워크플로우를 추가하고 README 상단에 CI 배지와 integration 제외 코멘트를 붙인다.
  - **Prerequisite**: Unit 9-0 통과

- [ ] **Unit 9-2. 포트폴리오 문서 묶음**
  - **Purpose**: → PLAN: Phase 9 > Task 9-2 (README / 4개 시나리오 로그 / 컴포넌트 경계 문서 / 전체 테스트 통과)
  - **Approach**: README(아키텍처 다이어그램 + 기동법 + 대표 시나리오), 컴포넌트별 문서, 4개 시나리오(날씨 / 호텔 / 실패 후 retry / multi-agent) 실행 로그를 정리하고 마지막에 `go test ./...`로 전체 통과를 확인한다.
  - **Prerequisite**: Unit 9-1 통과
