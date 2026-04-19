# PLAN.md — 완료 경계 정의

본 문서는 Phase 5 Task 5-1-2 이후 남은 구간에 대한 완료 판정 기준을 정의한다.
Phase 0 ~ Phase 5 Task 5-1-1까지는 이미 완료되어 본 문서의 범위에서 제외된다.
각 Task는 "무엇이 동작해야 완료로 볼 수 있는가"만 기술하며, 구현 방법과 파일 구조는 IMPLEMENT.md에서 다룬다.

---

## Phase 5 — Verifier / Retry / Concurrency

### Task 5-0. 이전 Phase 회귀 체크

- **목적**: Phase 5 재개 시점에 Phase 4 Exit Criteria가 여전히 유효함을 확인한다. Session 복원과 Long-term Memory 주입 경로는 Phase 5 이후 모든 loop 분기가 암묵적으로 전제하는 불변이므로, 재진입 직전에 회귀 여부를 한 번 관찰하지 않으면 문제가 생겼을 때 원인을 이번 Phase 변경분으로 오인하게 된다.
- **입력**: Phase 4 완료 시점의 통합 환경 (Redis / Postgres 연결 포함)
- **Exit Criteria**:
  - 동일 SessionID 재요청 시 이전 RecentContext 복원이 관찰됨
  - Long-term Memory 조회 결과가 LLMPlanner system prompt에 반영되는 것이 관찰됨
  - `go test ./...` 전체 통과

### Step 5-1. Concurrency 기초

- **Task 5-1-2. 독립 tool 2개가 errgroup으로 병렬 실행되고 취소가 전파된다**
  - **목적**: Phase 6 Workflow 병렬 실행과 Phase 7 Worker goroutine 관리가 모두 전제하는 "errgroup + context 취소" 패턴을 얕은 범위에서 먼저 확립한다. 이 패턴이 없는 상태에서 Phase 6으로 진입하면 그래프 실행 디버깅과 goroutine leak 디버깅이 한 번에 몰린다.
  - **입력**: Task 5-1-1에서 확립된 context 전파 패턴이 동작하는 상태
  - **Exit Criteria**:
    - 두 tool이 동시에 시작되는 것이 테스트 타이밍으로 관찰됨
    - 한쪽이 에러를 반환하면 나머지 tool의 context가 Done으로 전이됨
    - `go test -race`로 경고 없이 통과

### Step 5-2. Verifier 경계

- **Task 5-2. Runtime이 Verifier를 거쳐 done/retry/fail 3분기로 종료 판정한다**
  - **목적**: Runtime 내부에 흩어져 있던 종료 판정 로직을 "검증 결과"라는 단일 경계로 모은다. loop가 언제 끝나는지를 Verifier 한 곳에서 설명할 수 있어야 Task 5-3(retry), 5-5(reflection)가 같은 축 위에 붙을 수 있다.
  - **입력**: Phase 1의 Runtime loop와 IsFinished 종료 판정, Phase 2의 에러 분류가 동작하는 상태
  - **Exit Criteria**:
    - FinalAnswer가 비어 있는 결과에서 verifier가 `retry`를 반환하면 loop가 한 step 더 진행됨
    - tool 에러가 섞인 결과에서 `fail`을 반환하면 Status가 `failed`로 전이되며 loop가 종료됨
    - 정상 결과에서 `done`을 반환하면 loop가 정상 종료됨

### Step 5-3. RetryPolicy 단일화

- **Task 5-3. RetryPolicy가 retry 결정의 단일 지점이 되고 LLMPlanner 내부 하드코딩 retry는 사라진다**
  - **목적**: Phase 3의 JSON 파싱 재시도(하드코딩 1회)와 Phase 5에서 도입하는 정책 retry가 이중으로 돌지 않도록 retry 진입점을 하나로 모은다. 무한 재시도 방지가 retry 레이어의 존재 이유이므로 상한 검증이 핵심이다.
  - **입력**: Task 5-2 완료로 verifier가 retry 신호를 낼 수 있는 상태
  - **Exit Criteria**:
    - 연속 실패 시 설정된 최대 횟수에서 loop가 종료됨
    - `llm_parse_error`가 RetryPolicy 외의 경로에서는 재시도되지 않음 (Phase 3 LLMPlanner 회귀 테스트 통과로 확인)

### Step 5-4. Failure 분기 단일화

- **Task 5-4. 에러 유형별 loop 제어 신호가 단일 진입점에서 분기된다**
  - **목적**: 에러 분류(Phase 2)를 "그래서 loop는 어떻게 해야 하는가"로 변환하는 단일 함수를 확보한다. 분기가 Runtime 여러 지점에 흩어지면 새 실패 유형 추가 시 누락이 반드시 발생한다.
  - **입력**: Phase 2의 AgentError 분류, Task 5-3의 RetryPolicy 경계
  - **Exit Criteria**:
    - `tool_not_found` 발생 시 즉시 종료됨
    - `tool_execution_failed` + timeout 조합이 RetryPolicy 경로로 진입함
    - 빈 결과에 대해서는 loop가 속행되어 다음 step에서 Planner가 다른 접근을 선택할 기회를 가짐

### Step 5-5. Reflection

- **Task 5-5. Reflection 결과가 다음 Plan 호출과 loop 속행 판단에 실제로 반영된다**
  - **목적**: LLM 자기검증을 "관찰 가능한 상태 변화"로 고정한다. Reflection을 추가하면서 loop 동작에 아무 영향이 없으면 prompt 한 번 더 태우는 것에 그치므로, "prompt에 들어가는가"와 "loop가 한 번 더 도는가" 두 관찰 지점이 핵심이다.
  - **입력**: Task 5-2 완료 상태(Verifier 경계 부착), Phase 3 prompt_builder
  - **Exit Criteria**: 동일 입력을 mock LLM으로 돌렸을 때
    - 첫 Plan 호출 → 부족 판정 → 두 번째 Plan 호출 prompt에 missing conditions 문자열이 포함됨
    - 두 번째 step에서 FinalAnswer가 채워져 loop가 종료됨

### Phase 5 Exit Criteria

- Task 5-0 (Phase 4 회귀) 통과
- Task 5-1-2 ~ 5-5 각 Task Exit Criteria 통과
- `go test -race ./internal/agent/... ./internal/verifier/...` 통과
- Verifier vs Reflector 역할 분리 / RetryPolicy 단일화 / Failure 분기 기준이 `docs/decisions/phase5.md`에 기록

---

## Phase 6 — Multi-Agent Orchestration

### Decision Point 6-D1. orchestration 패키지 의존 방향

> **결정: 선택지 A 확정 (2026-04-19 사용자 승인).**

- **선택지 A (확정)**: `orchestration → agent` — WorkerAgent가 Runtime을 주입받아 내부에서 호출. Runtime은 multi-agent 존재를 모름
- **선택지 B**: `agent → orchestration` — Runtime이 Manager를 직접 호출. Runtime이 multi-agent를 인식
- **Trade-off**:
  - A: Runtime 재사용 + 단방향 의존 + 역할 경계 유지. Phase 7 Worker가 Runtime만 호출해도 되는 단순함 유지
  - B: Runtime이 multi-agent 인지 책임을 져 결합 증가. 단, multi-agent 실행 경로 추적이 Runtime 한 곳에서 가능
- **확정 근거**: 프로젝트 CLAUDE.md `internal/orchestration` 규칙("`internal/agent`는 `internal/orchestration`을 알지 않는다. 역방향 의존 금지")과 일치

### Task 6-0. 이전 Phase 회귀 체크

- **목적**: Phase 5에서 도입한 Verifier / RetryPolicy / Failure 분기 경계가 Phase 6 진입 시점에 여전히 단일 지점으로 동작하는지 확인한다. Phase 6은 Runtime을 재사용(의존 방향 A 선택 시)하므로 Phase 5 경계가 깨져 있으면 multi-agent 디버깅이 즉시 Phase 5 회귀 디버깅으로 전환되어 원인 분리가 불가능해진다.
- **입력**: Phase 5 Exit Criteria 통과 상태
- **Exit Criteria**:
  - done/retry/fail 3분기 단일 verifier 경로 관찰
  - retry 상한에서 loop 종료 관찰
  - `go test -race ./internal/agent/... ./internal/verifier/...` 통과

### Step 6-1. Workflow 그래프

- **Task 6-1. Task DAG가 위상 정렬되고 독립 Task는 병렬로 실행되며 순환은 에러로 감지된다**
  - **목적**: 실행 엔진 이전에 그래프 구조 자체가 옳은지 격리해서 검증한다. 그래프 오류와 goroutine 오류가 디버깅 단계에서 한 덩어리로 나오면 원인 분리가 불가능해진다.
  - **입력**: Task 5-1-2의 errgroup + context 취소 패턴
  - **Exit Criteria**:
    - 선형 의존 Task들이 의존 순서대로 실행됨
    - 독립 Task 2개가 동시에 시작되는 것이 테스트 타이밍으로 관찰됨
    - 순환 의존 그래프 투입 시 cycle 에러가 반환됨
    - 한 Task 실패 시 나머지에 취소가 전파되고 최종 결과에 실패가 병합됨
    - `go test -race` 통과

### Step 6-2. End-to-End 시나리오

- **Task 6-2. "호텔 찾아줘" 입력 하나가 Search → Filter → Ranking → Summary 4단계로 실제 실행되어 결과가 반환된다**
  - **목적**: TaskDecomposer + Manager + Worker + Workflow 조합이 사용자 입력에서 결과까지 흐르는지를 단일 관찰점에 모은다. 각 단계를 개별 Task로 쪼개면 "시나리오가 실제로 돈다"는 핵심 증거가 사라진다.
  - **입력**: Task 6-1의 Workflow 경계, Decision Point 6-D1 결정, Phase 3의 MockLLMClient
  - **Exit Criteria**:
    - mock LLM으로 고정된 시나리오 입력을 주면 4개 worker가 정확한 순서(Search → Filter → Ranking → Summary)로 호출되는 것이 trace 로그로 관찰됨
    - 최종 응답에 요약 문자열이 채워져 반환됨
    - Filter와 Ranking 의존 관계가 Workflow 정렬로 해소됨

### Phase 6 Exit Criteria

- Task 6-0 (Phase 5 회귀) 통과
- Task 6-1 ~ 6-2 각 Task Exit Criteria 통과
- `go test -race ./internal/orchestration/...` 통과
- 의존 방향(6-D1) 결정, Manager vs Workflow 역할 분리, Task 간 데이터 전달 방식이 `docs/decisions/phase6.md`에 기록

---

## Phase 7 — Runtime 서비스화

> Kafka 등 외부 브로커는 이 Phase의 범위가 아니다. TaskQueue는 buffered channel 기반 InMemory 구현으로 고정. 인터페이스만 경계로 두고 이후 확장 가능.

### Decision Point 7-D1. HTTP 라우터 선택

- **선택지 A**: 표준 `net/http` ServeMux (Go 1.22+ path parameter)
- **선택지 B**: `chi` / `gorilla/mux` 등 외부 라우터
- **Trade-off**: A는 외부 의존 0 + 기능 최소, B는 미들웨어 생태계 확보
- **기본 권장**: A (Phase 0 Task 0-3-3에서 Go 1.22+ 고정됨)

### Decision Point 7-D2. ask_user 비동기 대기 방식

- **선택지 A**: `Runtime.Run()`이 `ask_user` 발생 시 반환하고, 사용자 입력 수신 후 새 `Run()` 호출로 재개 (시그니처 불변)
- **선택지 B**: Runtime loop 내부에서 channel로 대기 (Worker goroutine 차단)
- **Trade-off**: A는 Runtime 시그니처 불변 + Worker 비차단, B는 loop 상태를 그대로 유지하지만 Worker 당 task 병렬성이 줄어듦
- **기본 권장**: A

### Decision Point 7-D3. Admin 엔드포인트 인증 범위

> **결정: 인증 미적용 확정 (2026-04-19 사용자 승인).**

- **확정 내용**: admin 엔드포인트는 인증/인가를 적용하지 않는다. 이 커리큘럼의 목적은 runtime 제어 흐름 학습이며 인증은 명시적 비목표. `docs/scope.md`에 "admin 인증 미적용" 명시가 Task 7-4의 전제

### Task 7-0. 이전 Phase 회귀 체크

- **목적**: Phase 6 multi-agent 경로가 Phase 7에서 HTTP 경계와 연결되기 직전, Workflow 정렬/병렬/cycle/실패 전파 4개 관찰 지점이 여전히 통과하는지 확인한다. HTTP 연결 이후 회귀가 발견되면 원인이 API 경계인지 orchestration 경계인지 구분이 어려워진다.
- **입력**: Phase 6 Exit Criteria 통과 상태
- **Exit Criteria**:
  - 호텔 검색 E2E 시나리오가 trace 로그 + 최종 응답으로 관찰됨
  - `go test -race ./internal/orchestration/...` 통과

### Step 7-1. HTTP 진입 경계

- **Task 7-1. `cmd/agent-api` 프로세스가 기동되고 `/v1/agent/run`, `/v1/tasks/{id}`, `/v1/sessions/{id}`, `/health`가 응답한다**
  - **목적**: API 서버로서 최소한의 기동 경로를 관찰 가능한 상태로 확보한다. 라운드트립이 되는 서버 없이는 이후 Task들이 전부 공중에 뜬다.
  - **입력**: Phase 6 완료, Decision Point 7-D1 결정
  - **Exit Criteria**:
    - 서버 기동 후 `POST /v1/agent/run`에 JSON 전달 시 200 + task ID 반환
    - `GET /v1/tasks/{id}` 및 `/v1/sessions/{id}`가 라운드트립됨
    - `GET /health`가 의존 서비스 상태 JSON을 반환함
    - 핸들러 테스트로 재현됨

### Step 7-2. 비동기 실행 경계

- **Task 7-2. HTTP 요청이 Queue를 거쳐 Worker goroutine에서 실행되고, 단일/multi-agent 경로가 분기되며, graceful shutdown이 in-flight task를 잃지 않는다**
  - **목적**: API 계층과 실행 엔진을 물리적으로 분리하고, Phase 6에서 만든 multi-agent 경로를 HTTP 경계에 연결한다. multi-agent가 CLI에서만 돌면 Phase 6의 산출물이 실사용 경로에서 죽는다. 저장소는 이 시점에선 InMemory로 두고, 프로세스 재시작 영속성은 Task 7-3에서 교체한다.
  - **입력**: Task 7-1의 핸들러 경로, Phase 6 Manager 경계
  - **Exit Criteria**:
    - POST 직후 task ID가 즉시 반환됨 (동기 실행 아님)
    - Worker가 단일 agent 요청과 multi-agent 요청을 각각 맞는 경로로 처리하는 것이 테스트로 관찰됨
    - AsyncTask 상태 전이(queued → running → succeeded/failed)가 관찰되고 잘못된 전이는 거부됨
    - SIGTERM 전송 시 진행 중 task가 완료된 뒤 프로세스가 종료되고 결과는 Repository에 저장됨
    - `go test -race ./internal/queue/...` 통과

### Step 7-3. 결과 영속성

- **Task 7-3. task 결과가 프로세스 재시작 후에도 `GET /v1/tasks/{id}`로 조회된다**
  - **목적**: 서비스화의 최소 운영 요건인 "결과 휘발 방지"를 관찰 지점으로 고정한다. Task 7-2의 InMemory 저장소만으로는 재시작 시 유실되어 Phase 7이 실질적 서비스 상태에 도달하지 못한다.
  - **입력**: Task 7-2 완료, Phase 4의 Redis 연결 인프라
  - **Exit Criteria**:
    - POST → task 완료 → API 프로세스 재시작 후 동일 task ID 조회 시 이전 결과가 그대로 반환되는 것이 integration test로 관찰됨

### Step 7-4. 운영 관측 채널

- **Task 7-4. 운영자가 admin API만으로 최근 task / 실패 task / session dump / tool 통계를 조회할 수 있다**
  - **목적**: 로그 grep 없이 운영 신호를 확보한다. Phase 8 timeout 튜닝과 Phase 9 포트폴리오 시나리오 데모에 쓸 최소 관측 채널이 이 단계에서 필요하다.
  - **입력**: Task 7-2 완료, Decision Point 7-D3 결정
  - **Exit Criteria**:
    - 최근 task / 실패 task / session dump / tool 통계 각 엔드포인트가 예상 형태의 JSON을 반환
    - tool 통계는 동일 tool 여러 호출 후 호출 횟수와 평균 latency가 증가하는 것이 테스트로 관찰됨

### Step 7-5. ask_user HTTP 완성

- **Task 7-5. HTTP 환경에서 `ask_user` 발생 시 task가 대기 상태로 전환되고 사용자 입력 제출 후 재개된다**
  - **목적**: Phase 3에서 CLI 대체 처리로 미뤄둔 `ask_user`를 HTTP 경계에서 완성한다. 이 Task가 없으면 `ask_user`는 영원히 CLI 전용 미완성 ActionType으로 남는다.
  - **입력**: Task 7-2 완료, Decision Point 7-D2 결정
  - **Exit Criteria**:
    - mock LLM으로 `ask_user`를 유도하면 task가 `waiting_for_user` 상태로 전이되는 것이 `GET /v1/tasks/{id}`로 관찰됨
    - 입력 제출 엔드포인트로 값 전달 시 task가 재개되어 최종 `succeeded` 상태에 도달함

### Phase 7 Exit Criteria

- Task 7-0 (Phase 6 회귀) 통과
- Task 7-1 ~ 7-5 각 Task Exit Criteria 통과
- `go test -race ./internal/queue/... ./internal/api/...` 통과
- 라우터 선택(7-D1), `ask_user` 처리 방식(7-D2), admin 인증 범위(7-D3), `orchestration.Task` vs `api.AsyncTask` 개념 분리 근거가 `docs/decisions/phase7.md`에 기록

---

## Phase 8 — 운영 고도화

### Decision Point 8-D1. OTel exporter 선택

- **선택지 A**: stdout exporter (인프라 추가 없음, 시각화 없음)
- **선택지 B**: OTLP → Jaeger / Collector (docker-compose에 컨테이너 추가, trace tree 시각화 가능)
- **Trade-off**: A는 기동 부담 최소 + span 구조 확인은 로그로 수행, B는 trace 시각화가 Phase 9 데모에 직접 쓰임
- **기본 권장**: A로 시작하고 exporter 교체 가능한 구조로 둔다. 단, Phase 9 포트폴리오에 trace 스크린샷이 필요하면 B

### Task 8-0. 이전 Phase 회귀 체크

- **목적**: Phase 7 HTTP + Queue + Worker 구조가 Phase 8 관측성/정책 레이어가 부착되기 전 여전히 안정적인지 확인한다. Task 8-1의 timeout 외부화와 8-3의 trace 연동은 Phase 7 Runtime 진입 경로를 수정하므로, 기준선이 불안정하면 정책 주입 이후 회귀 원인 파악이 어렵다.
- **입력**: Phase 7 Exit Criteria 통과 상태
- **Exit Criteria**:
  - Task 7-2 ~ 7-5 Exit Criteria가 여전히 통과
  - `go test -race ./internal/queue/... ./internal/api/...` 통과

### Step 8-1. Timeout 외부화

- **Task 8-1. tool별 timeout이 config로 주입되고 전체 request deadline도 적용된다**
  - **목적**: Task 5-1-1에서 확립한 context 패턴에 (1) 운영 중 조정 가능한 외부화, (2) 루프 전체 상한을 덧붙인다. 전자는 tool 특성별 튜닝, 후자는 loop 무한 진행 방지가 이유다.
  - **입력**: Task 5-1-1의 context 전파 패턴, Phase 7 완료
  - **Exit Criteria**:
    - config에서 특정 tool의 timeout을 짧게 설정하면 해당 tool 실행이 `tool_execution_failed`(retryable)로 중단됨
    - 전체 deadline 초과 시 loop가 `context.Canceled` 계열로 즉시 종료됨

### Step 8-2. 비용 한도

- **Task 8-2. session 누적 token이 임계값을 넘으면 loop가 중단된다**
  - **목적**: 단일 session이 무제한 비용을 내는 것을 강제로 막는다. Phase 7 이후 Worker가 동시 실행되는 환경에서 tracker는 반드시 동시성 안전해야 한다.
  - **입력**: Phase 3의 TokenUsage 기록 경로, Phase 7의 Worker
  - **Exit Criteria**:
    - 작은 임계값 설정 + 다회 호출 시나리오에서 세션이 임계값을 교차한 직후 loop가 종료됨
    - `go test -race ./internal/llm/...` 통과

### Step 8-3. Trace 연결

- **Task 8-3. 한 요청이 request → planner → tool → verifier → memory 구간에서 단일 trace로 연결되고 로그의 trace_id가 span TraceID와 일치한다**
  - **목적**: latency 병목과 실패 지점을 trace 하나로 파악 가능하게 만든다. span이 붙어 있어도 서로 연결이 끊기면 Phase 9 시나리오 데모에서 "왜 느렸는지"를 설명할 수 없다.
  - **입력**: Decision Point 8-D1 결정, Phase 3 structured logger
  - **Exit Criteria**:
    - 단일 요청에서 exporter 출력으로 부모-자식 관계가 연결된 span tree가 확인됨
    - 동일 요청의 로그 라인들이 모두 동일한 trace_id를 갖는 것이 관찰됨

### Step 8-4. 정책 파사드

- **Task 8-4. tool 사용 제한 + max step + 비용 한도가 Runtime의 단일 `Policy.Check()` 호출로 적용된다**
  - **목적**: 정책 호출 지점이 Runtime 여러 곳에 분산되는 것을 막고 향후 정책 추가 지점을 하나로 고정한다. 파사드 구조가 없으면 Task 8-2의 비용 정책과 기존 max step 처리가 각자 다른 위치에서 호출되어 누락/중복이 생긴다.
  - **입력**: Task 8-2 완료, Phase 1의 max step 처리
  - **Exit Criteria**:
    - Runtime 코드에서 개별 정책 호출이 사라지고 `Policy.Check()` 단일 호출만 남음
    - 3개 정책(tool 사용 제한 / max step / 비용 한도)이 단일 호출 경로에서 모두 유효하게 동작하는 것이 테스트로 확인됨

### Step 8-5. 에러 책임 주체 분류

- **Task 8-5. 에러가 user / system / provider 분류 레이블을 갖는다**
  - **목적**: Phase 2의 retryable/fatal 분류는 "retry 할지 말지"만 결정할 뿐 운영 단계에서 필요한 "누구 잘못인가"를 구분하지 못한다. 사용자 응답 톤(사과할지 재시도 요청할지), 알림 채널(oncall 호출할지 사용자 공지할지), 메트릭 레이블(provider 장애율 측정)을 결정하려면 책임 주체 기준의 별도 축이 필요하다. 기존 분류와 직교하는 태깅을 덧대 "provider 장애라 사용자 잘못 아님" 같은 판단을 코드가 직접 내릴 수 있게 만든다.
  - **입력**: Phase 2 AgentError 분류
  - **Exit Criteria**:
    - 대표 에러 4종(`input_validation_failed`, `tool_execution_failed`, `llm_parse_error`, `tool_not_found`)이 각각 예상 분류 레이블(user / system / provider)로 태깅되는 것이 테스트로 관찰됨

### Phase 8 Exit Criteria

- Task 8-0 (Phase 7 회귀) 통과
- Task 8-1 ~ 8-5 각 Task Exit Criteria 통과
- `go test -race ./...` 전체 통과
- OTel exporter 선택(8-D1), PolicyLayer 파사드, TokenTracker 동시성 전략이 `docs/decisions/phase8.md`에 기록

---

## Phase 9 — 문서화 / 포트폴리오

### Task 9-0. 이전 Phase 회귀 체크

- **목적**: 포트폴리오 제출 직전 시점에 Phase 8까지의 운영 고도화(timeout / 비용 / trace / 정책 / 에러 분류)가 여전히 동작하는지 확인한다. CI 도입과 문서 갱신 과정에서 통합 테스트 실행 환경이 흐트러질 수 있으므로 이 시점의 회귀 체크는 외부 공개 시 "레포가 실제로 돈다"는 보증 근거가 된다.
- **입력**: Phase 8 Exit Criteria 통과 상태
- **Exit Criteria**:
  - `go test -race ./...` 전체 통과
  - 로컬 `docker-compose up` 환경에서 `make test-integration` 통과

### Step 9-1. CI

- **Task 9-1. push / PR마다 build + vet + unit test + race detector가 GitHub Actions에서 자동 실행된다**
  - **목적**: "레포의 코드가 실제로 돌아간다"는 증거를 외부인에게 보여줄 단일 신호를 확보한다. CI 배지 없는 포트폴리오는 신뢰도가 낮다.
  - **입력**: Phase 4 Task 4-0-1의 `make test-unit` 타겟
  - **Exit Criteria**:
    - 레포에 푸시 시 CI가 녹색 체크를 반환함
    - README 상단 CI 배지가 렌더링됨
    - integration 테스트는 CI 범위 외임이 워크플로우 코멘트로 명시됨

### Step 9-2. 포트폴리오 문서

- **Task 9-2. 외부인이 레포 루트 문서만 보고 구조 / 기동법 / 대표 시나리오 / 설계 근거를 파악할 수 있다**
  - **목적**: 포트폴리오로서의 최종 제출물. 코드만으로는 설계 의도가 드러나지 않으므로 "왜 이렇게 나눴는가"를 문서로 연결한다. 개별 문서 작성을 Task로 쪼개는 대신 "외부인이 이해 가능한가"라는 하나의 관찰 기준으로 묶는다.
  - **입력**: Phase 0 ~ Phase 8의 `docs/decisions/*` 누적 기록, Phase 2의 tool spec 문서
  - **Exit Criteria**:
    - README만 읽어도 기동법과 전체 구조가 파악됨
    - 4개 시나리오(날씨 / 호텔 / 실패 후 retry / multi-agent)에 실제 실행 로그가 첨부됨
    - 각 컴포넌트(runtime / planner / memory / tool router / multi-agent)의 경계와 의존 방향이 문서화됨
    - `go test ./...` 전체 통과

### Phase 9 Exit Criteria

- Task 9-0 (Phase 8 회귀) 통과
- CI 녹색 + README 배지 노출
- Task 9-2의 4개 관찰 지점 모두 통과
- `go test ./...` 전체 통과
