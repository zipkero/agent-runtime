# IMPLEMENT.md — 구현 전략 및 진행 추적

**범위**: PLAN.md Task 5-1-2부터 Phase 9까지 (19개 구현 단위 + 5개 Decision Point)
**기준**: PLAN.md의 Exit Criteria가 완료 판정의 유일한 기준이다. 이 문서는 그 상태에 도달하기 위한 수단.
**체크박스 규칙**: 구현 완료 → 이 문서 체크. Exit Criteria 검증 완료 → PLAN.md 체크. 둘은 다른 사건이다.

---

## 1. 아키텍처 (현재 → 목표)

**이미 있는 경계** (Phase 0~4 + Task 5-1-1 완료):
```
CLI(cmd/agent-cli) → Runtime(internal/agent) → Planner(LLMPlanner, prompt_builder)
                                              → Executor(ToolExecutor) → ToolRouter → Tool
                                              → MemoryManager(Session + Long-term)
internal/types (PlanResult, ToolResult, Memory, AgentError)
internal/state (RequestState + SessionState aggregator)
internal/observability (slog 기반 structured logger)
internal/config, internal/llm (OpenAI)
```

**추가될 경계** (본 범위):
```
internal/verifier          ← Task 5-2, 5-5 (Verifier + Reflector)
internal/agent              ← Task 5-3, 5-4 (RetryPolicy, FailureHandler, PolicyLayer)
internal/orchestration      ← Task 6-1, 6-2 (Workflow, Manager/Worker, TaskDecomposer)
internal/api                ← Task 7-1, 7-3, 7-4, 7-5 (Handler, AsyncTask, AdminHandler)
internal/queue              ← Task 7-2 (TaskQueue, Worker)
internal/observability (확장) ← Task 8-3 (OTel tracer)
internal/llm/token_tracker  ← Task 8-2
.github/workflows           ← Task 9-1
docs/scenarios, docs/0N-*.md ← Task 9-2
```

**의존 방향 (신규만)**:
- `orchestration → agent` (6-D1 권장): Worker가 Runtime 재사용
- `queue → agent`, `queue → api`: Worker가 Runtime과 AsyncTaskRepository를 주입받음
- `api → queue`는 금지. Handler는 TaskQueue 인터페이스만 주입받는다 (CLAUDE.md 규칙)
- `verifier`는 `internal/agent`에 주입되며 역방향 의존 금지

---

## 2. 실행 흐름 (본 범위 반영)

### 2.1 단일 agent loop (Phase 5 완성 후)

```
Run(ctx, AgentState)
  ├─ LoadRelevantMemory (Phase 4)
  ├─ for step < maxStep:
  │    ├─ PolicyLayer.Check(state)          [Task 8-4]
  │    ├─ plan = Planner.Plan(ctx, state)
  │    ├─ if IsFinished(plan): break         (Phase 1, ActionType 기반)
  │    ├─ result = Executor.Execute(plan)    [context에 tool timeout 적용, Task 5-1-1/8-1]
  │    ├─ state.Apply(plan, result)
  │    ├─ signal = FailureHandler.Classify(result.Err) [Task 5-4]
  │    │    ├─ fatal   → break with failed
  │    │    ├─ retry   → RetryPolicy.ShouldRetry      [Task 5-3]
  │    │    └─ continue → 다음 step
  │    ├─ verdict = Verifier.Verify(ctx, state)         [Task 5-2]
  │    │    ├─ done  → reflect 체크로 이동
  │    │    ├─ retry → 다음 step
  │    │    └─ fail  → break with failed
  │    └─ reflect = Reflector.Reflect(ctx, state)       [Task 5-5]
  │         ├─ Sufficient=true  → break with finished
  │         └─ Sufficient=false → state.ReflectionState 갱신, 다음 step
  ├─ SaveMemory (Phase 4)
  └─ return state
```

### 2.2 Multi-agent (Phase 6)

```
ManagerAgent.Run(userInput)
  ├─ tasks = TaskDecomposer.Decompose(userInput)
  ├─ workflow = BuildWorkflow(tasks)
  ├─ result = workflow.Execute(ctx, agents)
  │    ├─ TopologicalSort → cycle? → err
  │    └─ 의존 없는 Task를 errgroup으로 병렬 실행
  │         └─ 각 WorkerAgent.Execute(task):
  │              ├─ adapter: Task → AgentState
  │              ├─ runtime.Run(ctx, agentState)
  │              └─ adapter: AgentState → TaskResult
  └─ return merged result
```

### 2.3 HTTP + Worker (Phase 7)

```
POST /v1/agent/run
  └─ Handler → Queue.Enqueue(asyncTask) → 200 {task_id}

Worker goroutine
  ├─ for { task = Queue.Dequeue(); go process(task) }
  └─ process(task):
       ├─ state = adapter: AsyncTask.Payload → AgentState
       ├─ if task.Mode == multi: ManagerAgent.Run else Runtime.Run
       ├─ on ask_user: state = waiting_for_user, save, return (재개 대기)
       └─ on complete: AsyncTaskRepository.Save(task)
SIGTERM → ctx.Cancel → WaitGroup.Wait → graceful exit
```

---

## 3. 상태 모델 (변경분)

| 상태 유형 | 저장 위치 | 변화 규칙 | 도입 시점 |
|-----------|-----------|-----------|-----------|
| `AgentState.ReflectionState` | in-process | Reflector 호출 시 갱신. prompt_builder가 다음 Plan 호출에 반영 | Task 5-5 |
| `AsyncTask.Status` | InMemory Repository → Redis | `queued → running → succeeded | failed | waiting_for_user`. 역방향/건너뛰기 전이 거부 | Task 7-2 → 7-3 |
| `TokenTracker[sessionID]` | in-process map + Mutex | LLM 호출 후 누적. session 종료와 무관하게 in-memory 유지 (Phase 8 범위) | Task 8-2 |
| `ToolStats` | in-process map + Mutex | ToolRouter 실행 후 누적. 프로세스 재시작 시 초기화 허용 | Task 7-4 |
| `Span` (OTel) | TracerProvider | context 전파. defer End() 필수 | Task 8-3 |

---

## 4. Decision Points (구현 차단 요소)

| ID | 내용 | 차단되는 구현 단위 | 기본 권장 | 상태 |
|----|------|---------------------|-----------|------|
| 6-D1 | orchestration 의존 방향 (`orchestration → agent` vs 역방향) | Task 6-1, 6-2 | A (`orchestration → agent`) | ⬜ 승인 대기 |
| 7-D1 | HTTP 라우터 (표준 net/http vs chi) | Task 7-1 | A (표준) | ⬜ 승인 대기 |
| 7-D2 | ask_user 비동기 처리 방식 (Run 반환 재호출 vs loop 내 channel) | Task 7-5 | A (Run 반환) | ⬜ 승인 대기 |
| 7-D3 | Admin 엔드포인트 인증 비목표 명시 | Task 7-4 | 비목표 확정 | ⬜ 승인 대기 |
| 8-D1 | OTel exporter (stdout vs OTLP/Jaeger) | Task 8-3 | A (stdout 시작, 교체 가능 구조) | ⬜ 승인 대기 |

각 Decision Point는 해당 Task 진입 직전까지 확정하면 된다.

---

## 5. 의존 순서 및 진입 조건

각 단위의 선행 조건을 1줄로 명시. 시간순이 아닌 의존 기반.

1. **Task 5-1-2** — 선행: Task 5-1-1 (context deadline 전파)
2. **Task 5-2** — 선행: Runtime loop가 step 단위로 state를 반영하는 동작 (Phase 1 ~ Phase 4)
3. **Task 5-3** — 선행: Task 5-2 (verifier가 retry 신호를 낼 수 있는 상태) + LLMPlanner 하드코딩 retry 제거 여지
4. **Task 5-4** — 선행: Task 5-3 (RetryPolicy 경계)
5. **Task 5-5** — 선행: Task 5-2 (verifier가 loop에 붙어 있는 상태) + prompt_builder 확장 가능 지점
6. **[6-D1 승인]**
7. **Task 6-1** — 선행: Task 5-1-2 (errgroup 패턴)
8. **Task 6-2** — 선행: Task 6-1, 6-D1 결정
9. **[7-D1, 7-D3 승인]**
10. **Task 7-1** — 선행: Task 6-2 완료 상태 (Phase 6 회귀), 7-D1 결정
11. **Task 7-2** — 선행: Task 7-1 (handler 경로), Phase 6 ManagerAgent
12. **Task 7-3** — 선행: Task 7-2 (Worker가 결과를 Repository에 기록하는 경로)
13. **Task 7-4** — 선행: Task 7-2, 7-D3 결정
14. **[7-D2 승인]**
15. **Task 7-5** — 선행: Task 7-2, 7-D2 결정
16. **[8-D1 승인]**
17. **Task 8-1** — 선행: Task 7-2 (Runtime이 Worker goroutine에서 실행되는 상태)
18. **Task 8-2** — 선행: Phase 3 TokenUsage 기록, Task 7-2
19. **Task 8-3** — 선행: 8-D1 결정
20. **Task 8-4** — 선행: Task 8-2 (비용 정책이 존재해야 파사드 대상이 생김)
21. **Task 8-5** — 선행: Phase 2 AgentError
22. **Task 9-1** — 선행: Task 4-0-1의 `make test-unit` 타겟
23. **Task 9-2** — 선행: Phase 8까지의 `docs/decisions/*` 누적

---

## 6. 구현 단위

### [ ] Unit 5-1-2. errgroup 병렬 실행 + 취소 전파 실습

- **목적**: → PLAN: Phase 5 > Task 5-1-2. errgroup + context 취소 패턴을 얕은 범위에서 먼저 돌려보고, Phase 6 Workflow와 Phase 7 Worker가 이 패턴을 전제하도록 보장.
- **책임**:
  - 입력: 독립 Tool 2개 (기존 calculator + weather_mock 재사용), 공유 parent context
  - 출력: 두 Tool 결과의 집합 또는 첫 에러
  - 경계: `internal/agent/parallel_example_test.go` (테스트 전용, 프로덕션 코드 아님) + `golang.org/x/sync/errgroup` 의존성 추가
- **설계**:
  - 구조: 테스트 헬퍼가 errgroup.WithContext로 goroutine 2개 시작 → 각각 ToolRouter.Route 호출 → 한쪽에 의도적 에러/sleep 주입 → 나머지 context가 Done으로 전이되는지 검사
  - 흐름: parent ctx → errgroup → 2개 g.Go() → 한쪽 에러 → errgroup이 ctx cancel → 나머지 goroutine이 ctx.Done() 감지 후 조기 종료 → g.Wait() 반환
- **선택 이유**: errgroup은 WaitGroup + error channel 조합보다 에러 전파가 간결하며, Phase 6 Workflow 실행 엔진의 거의 동일한 패턴. 여기서 손에 익히지 않으면 Phase 6에서 그래프 디버깅과 goroutine leak 디버깅이 겹친다.
- **실패/예외**: goroutine leak은 `go test -race`로 감지. context 취소 미전파 시 테스트가 timeout으로 실패 — 명시적으로 짧은 deadline 설정해서 hang 대신 실패로 드러낸다. `golang.org/x/sync`는 외부 의존이므로 `go mod tidy` 완료 확인이 첫 스텝.

---

### [ ] Unit 5-2. Verifier를 Runtime loop에 통합

- **목적**: → PLAN: Phase 5 > Task 5-2. 종료 판정 로직을 "검증 결과"라는 단일 경계로 모은다.
- **책임**:
  - 입력: `AgentState` (step 실행 후)
  - 출력: `VerifyResult` (done / retry / fail)
  - 경계: `internal/verifier` 패키지. Runtime이 Verifier 인터페이스만 주입받음. nil 허용(기존 Phase 1~4 테스트 보존)
- **설계**:
  - 구조:
    - `verifier.Verifier` 인터페이스 `Verify(ctx, AgentState) (VerifyResult, error)`
    - `verifier.SimpleVerifier` — FinalAnswer 비어 있으면 retry, 마지막 ToolResult에 에러 있으면 fail, 외 done
    - Runtime.Run() loop: `IsFinished(plan) == false` → Executor.Execute → state.Apply → **Verifier.Verify** → 분기
  - 흐름: verdict == done → loop 종료 + Status=finished / retry → 다음 step / fail → Status=failed + loop 종료
  - 상태 변화: verifier는 state를 읽기만 함. Status 전이는 Runtime이 수행
- **선택 이유**: `IsFinished`(Phase 1)는 Plan의 ActionType 기반이고, Verifier는 결과의 충분성 기반. 두 계층을 공존시켜야 Task 5-5 Reflection이 들어갈 자리가 생긴다. 합치면 verifier/reflector/policy 역할이 뒤섞여 Phase 5 Exit Criteria의 "done/retry/fail 3분기 관찰"이 test-able 하지 않게 된다.
- **실패/예외**: verifier 자체 panic → Runtime에서 error로 잡고 Status=failed. Verifier가 nil이면 verifier 단계 skip (Phase 1 호환). 무한 retry 방지는 Unit 5-3 RetryPolicy 책임이지, verifier는 한계를 알 필요 없다.

---

### [ ] Unit 5-3. RetryPolicy 단일화 + LLMPlanner 하드코딩 제거

- **목적**: → PLAN: Phase 5 > Task 5-3. 이중 재시도 방지, retry 결정 지점을 Runtime 한 곳으로 고정.
- **책임**:
  - 입력: 직전 실패 `error` + 누적 `attempt` 카운트
  - 출력: `ShouldRetry bool`, `Delay time.Duration`
  - 경계: `internal/agent/retry_policy.go`. Runtime만 RetryPolicy를 호출한다. 다른 곳(LLMPlanner 등)에서 직접 호출 금지
- **설계**:
  - 구조:
    - `RetryPolicy` 인터페이스 + `LinearRetryPolicy{Max, Interval}`
    - Runtime의 retry 결정 분기 (Verifier retry 또는 FailureHandler retry 신호) → RetryPolicy.ShouldRetry → true면 Delay 후 loop 속행, false면 loop 종료
    - **LLMPlanner 내부 하드코딩 retry 제거**: 기존 `parseAndValidate` 실패 시 1회 재호출 로직을 삭제 → 상위에서 RetryPolicy로 위임 → LLMPlanner는 단일 시도 후 error 반환만 책임
  - 흐름: `llm_parse_error` 발생 → LLMPlanner.Plan이 error 반환 → Runtime이 FailureHandler(5-4)에서 retry 신호 획득 → RetryPolicy.ShouldRetry → 재시도 또는 종료
  - 상태 변화: Runtime이 attempt 카운트를 step별로 추적 (reset 규칙: 성공 시 0)
- **선택 이유**: LLMPlanner 내부 재시도와 Runtime 재시도를 공존시키면 `llm_parse_error` 시 LLMPlanner 1회 + Runtime N회로 이중 재시도가 되어 비용이 배로 뛴다. 재시도는 "loop 제어" 층의 책임이지 "LLM 파싱" 층의 책임이 아니다.
- **실패/예외**: 
  - LLMPlanner 테스트(Task 3-4-7)는 "invalid JSON → 2회 호출"을 기대한다. 하드코딩 제거 시 이 테스트가 깨진다 → **회귀 보호 작업이 본 Unit 범위에 포함**. 해결: 테스트의 "2회 호출" 기대치를 1회로 조정하되, Runtime 레벨 retry test를 별도로 추가하여 end-to-end 커버리지는 유지.
  - Delay가 너무 길면 전체 deadline(Task 8-1) 초과 → 기본값은 짧게 (예: 0 또는 100ms).

---

### [ ] Unit 5-4. FailureHandler (에러 유형별 단일 분기)

- **목적**: → PLAN: Phase 5 > Task 5-4. 에러 → loop 제어 신호 매핑을 단일 함수로 집중.
- **책임**:
  - 입력: `AgentError` (Phase 2 정의) 또는 nil
  - 출력: `ControlSignal { Fatal, Retry, Continue }`
  - 경계: `internal/agent/failure_handler.go`. Runtime만 호출.
- **설계**:
  - 구조: `Classify(err error) ControlSignal` — 에러 Kind별 map
    - `tool_not_found` → Fatal
    - `input_validation_failed` (Runtime 직접 생성) → Fatal
    - `tool_execution_failed` (timeout 포함) → Retry
    - `llm_parse_error` → Retry
    - nil 또는 빈 결과 → Continue
  - 흐름: Runtime이 Executor/Planner 에러를 Classify에 투입 → 신호에 따라 loop 분기. Retry 신호는 RetryPolicy(5-3)로 위임.
- **선택 이유**: 분기가 Runtime 여러 지점에 흩어지면 새 에러 타입 추가 시 반드시 누락이 생긴다. 단일 함수로 모으면 exhaustive test (에러 → 기대 시그널 map)가 가능.
- **실패/예외**: 분류되지 않은 알 수 없는 에러 → 기본값 Fatal (안전한 쪽). 새 에러 타입 추가 시 테스트가 기본값으로 빠지지 않도록 enum-exhaustive 체크 테스트 도입.

---

### [ ] Unit 5-5. Reflector가 다음 Plan 호출과 loop 속행에 실제로 영향

- **목적**: → PLAN: Phase 5 > Task 5-5. LLM 자기검증을 관찰 가능한 상태 변화로 고정.
- **책임**:
  - 입력: `AgentState` (verifier `done` 직후)
  - 출력: `ReflectResult { Sufficient, MissingConditions, Suggestion }`
  - 경계: `internal/verifier/reflector.go` + `internal/state/reflection_state.go` + `internal/planner/prompt_builder.go` 확장 + `internal/agent/runtime.go` 분기
- **설계**:
  - 구조:
    - `verifier.Reflector` 인터페이스 + `verifier.LLMReflector` 구현 (reflection 전용 prompt)
    - `state.ReflectionState` — `internal/state`에 별도 타입 (verifier → state 순환 회피를 위해 ReflectResult와 다른 타입으로 유지)
    - `AgentState.ReflectionState` 필드 추가
    - Runtime loop: verifier == done 시 Reflector.Reflect 호출 → Sufficient=true면 진짜 종료, false면 ReflectionState 갱신 후 loop 속행
    - `prompt_builder`가 AgentState.ReflectionState.MissingConditions를 system prompt 컨텍스트에 삽입
  - 흐름: step 1 → verifier done → reflector Sufficient=false → state 갱신 → step 2 Plan 호출 prompt에 missing conditions 포함 → Plan 수정 → step 2 verifier done → reflector Sufficient=true → 종료
  - 상태 변화: ReflectionState는 매 reflection 호출 시 덮어쓰기 (누적 아님)
- **선택 이유**: Reflector를 Verifier와 합치면 "결과 형식 검증 (verifier)"과 "결과 내용 충분성 검증 (reflector)" 두 역할이 섞여 prompt 설계가 충돌한다. 분리해야 각각 교체 가능하고, mock LLM으로 결정적 시나리오(1차 부족 → 2차 충분)가 가능.
- **실패/예외**: 
  - Reflector가 비용이 크므로 주입이 nil이면 skip (기존 동작 보존)
  - LLM이 Sufficient=true를 hallucinate → Verifier 단계가 1차 필터이므로 완전한 오동작은 아님. 정책적으로 허용.
  - 순환 참조 위험 — `ReflectionState` 타입을 `internal/state`에, `ReflectResult`를 `internal/verifier`에 각각 두고 Runtime이 변환. 이 분리 빠뜨리면 `verifier → state → verifier` 순환 발생.

---

### [ ] Decision Point 6-D1 승인

`orchestration → agent` (권장) 또는 `agent → orchestration` 중 확정. 본 문서의 이후 Unit은 권장안을 가정하고 작성됨.

---

### [ ] Unit 6-1. Workflow 경계 (DAG 정렬 + cycle + 병렬 실행 + 실패 전파)

- **목적**: → PLAN: Phase 6 > Task 6-1. 그래프 로직과 실행 로직을 단일 타입에 모으되 격리 테스트 가능하게.
- **책임**:
  - 입력: `Task` 목록 + `Dependencies map[TaskID][]TaskID`
  - 출력: `map[TaskID]TaskResult` 또는 에러 (cycle 또는 Task 실패)
  - 경계: `internal/orchestration/workflow.go`. 외부 패키지(manager, worker) 의존 금지 — Agent 인터페이스만 받아 실행
- **설계**:
  - 구조: `Workflow` 타입에 `TopologicalSort() ([]TaskID, error)` + `Execute(ctx, agents map[string]Agent) (results, error)` 두 메서드
  - 흐름: Execute → TopologicalSort → 결과에 따라 "readyQueue" 관리 → 같은 레벨 Task들을 errgroup으로 병렬 실행 → 완료 시 후속 Task를 readyQueue에 추가 → 한 Task 실패 시 errgroup ctx cancel → 나머지 취소 + 에러 aggregate
- **선택 이유**: 그래프 정렬만 분리하고 실행을 별도 타입으로 떼면 오히려 중간 데이터 구조(readyQueue)를 노출해야 해서 복잡해진다. 같은 타입 두 메서드로 두되 `workflow_test.go`에서 각각 격리 테스트.
- **실패/예외**: cycle → `ErrCycleDetected`. Task 실패 시 부분 결과 + wrapped error. deadline 초과는 ctx 통해 전파.

---

### [ ] Unit 6-2. Multi-agent E2E 시나리오 (Search → Filter → Ranking → Summary)

- **목적**: → PLAN: Phase 6 > Task 6-2. 분해 + Workflow + Worker + adapter 조합이 실제로 흐른다는 단일 관찰.
- **책임**:
  - 입력: 사용자 입력 문자열 (예: "호텔 찾아줘")
  - 출력: 요약 문자열 + 실행 trace 로그
  - 경계: `internal/orchestration` 전체. 최상위 진입점은 `ManagerAgent.Run`
- **설계**:
  - 구조:
    - `TaskDecomposer` 인터페이스 + `LLMTaskDecomposer` + `MockTaskDecomposer` (테스트 결정성)
    - `Agent` 인터페이스 + `SearchAgent` / `FilterAgent` / `RankingAgent` / `SummaryAgent` 4개 — 각자 내부에서 `runtime.Run` 호출
    - `task_adapter.go`: `Task → AgentState` 변환, 이전 `TaskResult.Output`을 다음 `AgentState.Request.ToolResults`에 주입
    - Filter/Ranking에 필요한 `filter_mock`, `ranking_mock` Tool 추가 (`internal/tools/`)
    - `ManagerAgent.Run`: Decompose → Workflow 구성 → Workflow.Execute(ctx, agents) → 결과 병합
    - `orchestration/trace.go` — 실행 단계별 구조화 로그
  - 흐름: 사용자 입력 → TaskDecomposer(mock) → 고정된 4 Task 반환 → Workflow는 Search→Filter→Ranking→Summary 선형 순서 → 각 Agent가 Task 단위로 Runtime 실행 → 최종 Summary Task 결과 반환
- **선택 이유**: 시나리오 E2E는 단일 Unit으로 합쳐야 "시나리오가 돈다"는 핵심 증거가 하나의 테스트로 관찰된다. 개별 agent/adapter/decomposer를 Unit으로 쪼개면 E2E 관찰 지점이 사라진다.
- **실패/예외**: 
  - 중간 Worker 실패 → Workflow 실패 전파 → ManagerAgent가 부분 결과 + 에러. trace 로그에 실패 지점 기록
  - LLMTaskDecomposer hallucination → E2E는 MockTaskDecomposer 기반 검증 (결정성). LLM decomposer는 별도 integration test로 커버
  - 각 WorkerAgent가 독립 AgentState를 가지므로 세션 간 데이터 공유는 adapter로만 가능 — adapter 누락 시 worker가 앞 단계 결과를 못 봄

---

### [ ] Decision Point 7-D1 승인 (HTTP 라우터)
### [ ] Decision Point 7-D3 승인 (Admin 인증 비목표 명시)

---

### [ ] Unit 7-1. HTTP 진입점 + 4개 엔드포인트

- **목적**: → PLAN: Phase 7 > Task 7-1. API 서버 기동 경로 확보.
- **책임**:
  - 입력: HTTP 요청
  - 출력: JSON 응답 (표준 코드)
  - 경계: `cmd/agent-api/main.go` + `internal/api/handler.go` + `internal/api/types.go`
- **설계**:
  - 구조:
    - `cmd/agent-api/main.go` — 의존성 조립(logger, config, memory manager, queue, worker, handler, http.Server), SIGTERM 시 graceful shutdown (ctx cancel → worker wait → server Shutdown)
    - `handler.go`: POST /v1/agent/run / GET /v1/tasks/{id} / GET /v1/sessions/{id} / GET /health
    - `types.go`: RunRequest/RunResponse/TaskStatusResponse
    - 라우터: 표준 net/http ServeMux (Go 1.22 path param)
  - 흐름: POST → JSON decode → queue.Enqueue → 202 {task_id}. GET tasks/{id} → repo.Load → JSON. /health → 각 의존 서비스 ping → 503 if any down.
- **선택 이유**: Decision 7-D1 권장 A. 외부 라우터 라이브러리 없이 path param 충족. 미들웨어 필요 시 함수 래핑으로 충분.
- **실패/예외**: JSON 오류 → 400. 미등록 경로 → 404. queue Enqueue 실패 → 503. 의존 서비스 down → /health 503 + 각 필드에 상세.

---

### [ ] Unit 7-2. Worker + Queue + AsyncTask 상태 기계 (InMemory)

- **목적**: → PLAN: Phase 7 > Task 7-2. API 계층과 실행 엔진 분리, multi-agent HTTP 연결, graceful shutdown.
- **책임**:
  - 입력: AsyncTask (Payload 포함)
  - 출력: 업데이트된 AsyncTask (완료 상태 + 결과) 저장소 반영
  - 경계: `internal/queue/task_queue.go`, `internal/queue/worker.go`, `internal/api/async_task.go`, `internal/api/async_task_repository.go` (InMemory 구현)
- **설계**:
  - 구조:
    - `TaskQueue` 인터페이스 + `InMemoryTaskQueue` (buffered channel)
    - `AsyncTask` 타입 + 상태 전이 검증 (`queued → running → succeeded|failed`)
    - `AsyncTaskRepository` 인터페이스 + InMemory 구현
    - `Worker` — goroutine 루프: Dequeue → 상태 running → adapter(Payload → AgentState) → mode 분기(단일/multi) → Runtime.Run 또는 ManagerAgent.Run → 결과를 상태로 기록 → Repository.Save
    - `payload_adapter.go`: RunRequest ↔ AsyncTask.Payload ↔ AgentState 변환
    - Graceful shutdown: main에서 ctx cancel + sync.WaitGroup으로 in-flight task 대기
  - 흐름: POST → Enqueue → Worker Dequeue → process → Save → 상태 전이 로그
  - 상태 모델: AsyncTask.Status 전이는 메서드로만 허용 (`task.Start()`, `task.Succeed(result)`, `task.Fail(err)`). 직접 필드 할당 금지.
- **선택 이유**: InMemory로 먼저 E2E 검증. Redis는 Unit 7-3에서 교체. ManagerAgent 주입은 인터페이스로 두고 nil이면 단일 경로만 (Phase 6 미완료 상황 안전).
- **실패/예외**: 
  - Worker panic → defer recover → task Fail + 다음 task로 진행
  - Queue full → Enqueue 503 (buffered channel 한계)
  - SIGTERM 중 in-flight → WaitGroup 대기, 그 사이 들어온 새 요청은 Enqueue 거부
  - multi-agent 분기 조건: Payload.Mode 명시 필드 + ManagerAgent 주입 여부 둘 다 만족 시만

---

### [ ] Unit 7-3. AsyncTaskRepository를 Redis로 교체

- **목적**: → PLAN: Phase 7 > Task 7-3. 재시작 후 결과 조회 가능.
- **책임**:
  - 입력: AsyncTask
  - 출력: Redis 영속
  - 경계: `internal/api/redis_async_task_repository.go`
- **설계**:
  - 구조: Phase 4 `RedisSessionRepository` 패턴 복사 — JSON 직렬화, key prefix `async_task:{id}`, TTL 설정 (예: 7일)
  - 흐름: Save → SET with TTL. Load → GET → JSON unmarshal. ListRecent → SCAN 기반 (admin 엔드포인트에서 사용)
  - 주입 교체: `cmd/agent-api/main.go`에서 InMemory → Redis. 테스트는 InMemory 유지
- **선택 이유**: Redis는 Phase 4 이미 활성. Postgres 대비 단순 key-value에 적합. integration test (`//go:build integration`)로 재시작 시나리오 검증.
- **실패/예외**: Redis 연결 단절 → Repository 에러 → handler 503 + /health 503. JSON 직렬화 실패 → fatal (AsyncTask 구조가 단순해 발생 가능성 낮음). TTL 만료된 task 조회 → 404.

---

### [ ] Unit 7-4. Admin 엔드포인트 4종 + ToolStats 집계기

- **목적**: → PLAN: Phase 7 > Task 7-4. 운영 신호 최소 확보.
- **책임**:
  - 입력: HTTP 요청 (admin 경로)
  - 출력: 집계 JSON
  - 경계: `internal/api/admin_handler.go`, `internal/api/tool_stats.go`
- **설계**:
  - 구조:
    - `tool_stats.go`: `ToolStats` 구조체 (호출 횟수, 총 latency, 에러 횟수) + sync.Mutex. `Record(toolName, duration, err)` 메서드
    - ToolRouter가 실행 후 ToolStats.Record 호출 (Hook)
    - 4개 엔드포인트: `/v1/admin/tasks` (Repository.ListRecent), `/v1/admin/tasks/failed` (필터), `/v1/admin/sessions/{id}` (SessionRepository 직접), `/v1/admin/stats/tools` (ToolStats snapshot)
  - 흐름: 요청 → 각 저장소 조회 → 집계 → JSON
- **선택 이유**: 통계는 프로세스 재시작 시 초기화 허용 (Decision 7-D3 비목표). Phase 8 OTel 메트릭으로 자연 이관.
- **실패/예외**: 미존재 session dump → 404. ToolStats 동시 접근 → Mutex, `go test -race`로 검증.

---

### [ ] Decision Point 7-D2 승인 (ask_user 비동기 방식)

---

### [ ] Unit 7-5. ask_user 비동기 대기 + 재개

- **목적**: → PLAN: Phase 7 > Task 7-5. HTTP 환경에서 ask_user 완성.
- **책임**:
  - 입력: ask_user ActionType → 사용자 입력 제출
  - 출력: 재개된 task 완료
  - 경계: `internal/api/async_task.go` 확장 + `internal/api/handler.go` 확장 + `internal/agent/runtime.go` 분기
- **설계**:
  - 구조: (Decision 7-D2 권장 A 가정)
    - AsyncTask에 `waiting_for_user` 상태 추가 + 전이: `running → waiting_for_user → running → succeeded`
    - Runtime.Run이 ask_user ActionType을 만나면 → FinalAnswer에 질문 문자열 → 반환 (시그니처 불변)
    - Worker는 Runtime 반환 후 state.ActionType이 ask_user면 Repository에 `waiting_for_user`로 저장 + Queue 재삽입 없이 대기
    - `POST /v1/tasks/{id}/input` — body에 입력 문자열 → Repository에서 AsyncTask 로드 → Payload에 사용자 입력 병합 → Queue에 재삽입 → Worker가 새 Runtime.Run 호출 시 이전 state 복원
  - 흐름: POST /run → ask_user → 대기 → 사용자 입력 POST → 재개 → 완료
  - 상태 모델: 재개 시 AgentState 복원 (RecentContext + ToolResults + 사용자 입력 append)
- **선택 이유**: Runtime 시그니처 불변 + Worker 비차단. loop 내 channel 대기는 Worker 1개당 1 task로 동시성이 깎임.
- **실패/예외**: 사용자 입력 없이 방치 → TTL 만료 시 task failed (Phase 8 전체 deadline 또는 Redis TTL). 재개 중 프로세스 재시작 → Redis 영속화로 복구 가능. 같은 task에 입력 중복 제출 → 첫 번째만 수용, 이후 409 Conflict.

---

### [ ] Decision Point 8-D1 승인 (OTel exporter)

---

### [ ] Unit 8-1. Tool별 timeout 외부화 + 전체 request deadline

- **목적**: → PLAN: Phase 8 > Task 8-1. Task 5-1-1 패턴에 외부화와 전체 상한을 덧댄다.
- **책임**:
  - 입력: config의 tool timeout 맵 + 전체 request deadline
  - 출력: context 전파
  - 경계: `internal/config/config.go` 확장 + `internal/tools/router.go` 수정 + `internal/agent/runtime.go` 진입부 수정
- **설계**:
  - 구조: `config.ToolTimeouts map[string]time.Duration` + `config.RequestDeadline time.Duration`. ToolRouter가 tool 이름으로 timeout 조회(누락 시 fallback 기본값) 후 `context.WithTimeout`. Runtime.Run 진입 시 `ctx, cancel = context.WithTimeout(ctx, RequestDeadline)` + defer cancel
  - 흐름: HTTP 요청 → Worker → Runtime.Run(ctx with deadline) → loop step → ToolRouter.Route(ctx with tool timeout) → Tool
- **선택 이유**: Task 5-1-1에서 context 전달 경로가 이미 확립됨. 여기서는 설정값 외부화만 추가. config 변경은 재기동 필요 (Phase 8 범위에서는 hot reload 불요).
- **실패/예외**: Tool 이름 누락 → 기본값 (예: 30s). 전체 deadline 초과 시 `context.Canceled` → FailureHandler(Unit 5-4)가 별도 분류 없이 fatal 처리 (재시도 무의미). 기본값 설계 결정은 `docs/decisions/phase8.md`에 기록.

---

### [ ] Unit 8-2. Session별 TokenTracker + 비용 한도 중단

- **목적**: → PLAN: Phase 8 > Task 8-2. 단일 session 무제한 비용 차단.
- **책임**:
  - 입력: LLM 호출의 TokenUsage 이벤트 + sessionID
  - 출력: session별 누적 상태 + 한도 초과 시 loop 중단 신호
  - 경계: `internal/llm/token_tracker.go` + `internal/agent/cost_policy.go`
- **설계**:
  - 구조:
    - `TokenTracker` 구조체: `map[sessionID]TokenUsage` + `sync.Mutex`. 메서드 `Update(sessionID, usage)`, `Get(sessionID) TokenUsage`
    - LLMClient 또는 LLMPlanner가 응답 수신 후 TokenTracker.Update 호출
    - `CostPolicy`: threshold 초과 시 error 반환. Runtime loop 각 step 시작 시 호출 (또는 PolicyLayer 경유, Unit 8-4)
  - 흐름: LLM 응답 → TokenTracker 누적 → 다음 step 시작 시 CostPolicy 체크 → 초과 시 loop 종료 + Status=failed + reason=cost_limit
  - 상태 모델: TokenTracker는 프로세스 in-memory. Worker 병렬 환경에서 반드시 Mutex로 보호.
- **선택 이유**: TokenTracker가 SessionRepository를 건드리면 `llm → state` 역방향 의존 발생. 자체 map 저장소가 유일한 경계 안전 해결책.
- **실패/예외**: `go test -race ./internal/llm/...` 필수. map 누수 — session 종료 시 정리 없음 → Phase 8에서는 in-memory 유지, Phase 9 이후 TTL 검토. LLM 호출이 sessionID 없이 발생하면 TokenTracker skip (기본 동작 보존).

---

### [ ] Unit 8-3. OTel span 연결 + logger trace_id 교체

- **목적**: → PLAN: Phase 8 > Task 8-3. 요청 단일 trace로 관통.
- **책임**:
  - 입력: 요청
  - 출력: 부모-자식 연결된 span tree + 로그 trace_id = span TraceID
  - 경계: `internal/observability/tracer.go` (신규) + 각 컴포넌트 span 삽입 + `internal/observability/logger.go` 수정
- **설계**:
  - 구조:
    - `observability.InitTracer(exporter)` — OTel TracerProvider 초기화 (exporter는 Decision 8-D1 결정 기반)
    - span 시작 지점: `Runtime.Run` (root), 각 loop step, `Planner.Plan`, `ToolRouter.Route`, 개별 `Tool.Execute`, `Verifier.Verify`, `Reflector.Reflect`, `MemoryManager.LoadRelevantMemory/SaveMemory`. 각각 `defer span.End()`
    - logger는 기존 인터페이스 유지하되 내부에서 `context.Context`로부터 span 꺼내 `trace_id` 필드 자동 주입 (Phase 3 request_id 기반 trace_id는 OTel TraceID로 교체)
    - docker-compose에 Jaeger 컨테이너 추가 여부는 Decision 8-D1 결과에 따라
  - 흐름: 요청 진입 → tracer.Start → child span들 → 종료 → exporter로 전송. 로그 라인은 모두 같은 trace_id
- **선택 이유**: logger 인터페이스 변경 시 전 컴포넌트 수정 필요 → 내부 구현만 교체하는 것이 파급 최소. exporter는 교체 가능 구조 유지.
- **실패/예외**: exporter 연결 실패 → warning 로그 + fail-open (span 기록 생략, 요청은 계속). span leak → `defer span.End()` 누락 시 메모리 누수 → lint rule 또는 패턴 통일로 예방. context 전파 누락 → span 연결 끊김 → 통합 테스트로 검증 (한 요청의 span이 모두 같은 TraceID여야 함).

---

### [ ] Unit 8-4. PolicyLayer 파사드로 정책 호출 단일화

- **목적**: → PLAN: Phase 8 > Task 8-4. Runtime의 정책 호출 지점 하나로 모음.
- **책임**:
  - 입력: AgentState
  - 출력: 허용 또는 거부 (에러 + reason)
  - 경계: `internal/agent/policy.go`. Runtime 외부에서 개별 정책 호출 금지
- **설계**:
  - 구조:
    - `Policy` 인터페이스 `Check(ctx, AgentState) error`
    - `DefaultPolicy` — 내부에 max step / CostPolicy(Unit 8-2) / tool 사용 제한 순차 호출. 거부 발생 시 즉시 반환
    - Runtime loop: 각 step 시작 시 `Policy.Check` 단일 호출. 기존 개별 max step / cost 검사 제거
  - 흐름: step 시작 → Policy.Check → 거부 시 loop 종료 + 결과 필드에 reason 기록
- **선택 이유**: 파사드가 기존 구현체를 감싸기만 하므로 교체 비용 최소. 새 정책 추가 시 Runtime 수정 없이 DefaultPolicy 내부만 확장.
- **실패/예외**: 정책 간 순서에 따라 어느 제약이 먼저 걸리는지 결정됨 → 순서를 `docs/decisions/phase8.md`에 기록. 새 정책이 기존 동작을 덮지 않도록 cumulative 체크 필수.

---

### [ ] Unit 8-5. 책임 주체 기준 에러 분류 확장

- **목적**: → PLAN: Phase 8 > Task 8-5. user/system/provider 태깅으로 운영 레이블 확보.
- **책임**:
  - 입력: `AgentError`
  - 출력: `ResponsibilityClass { User, System, Provider }`
  - 경계: `internal/types/errors.go` 확장
- **설계**:
  - 구조: AgentError에 `Responsibility ResponsibilityClass` 필드 추가 또는 `Responsibility(err error) ResponsibilityClass` 함수. 매핑:
    - `input_validation_failed` → User
    - `tool_execution_failed` → System
    - `llm_parse_error` → Provider
    - `tool_not_found` → System
  - 흐름: 에러 생성 시점에 분류 자동 태깅. 소비 지점(로그, 메트릭, 사용자 응답)에서 조회
- **선택 이유**: 기존 retryable/fatal 축과 직교. 한 함수로 분류하면 테스트가 간단(에러 → 기대 레이블 map).
- **실패/예외**: 분류 누락된 에러 → 기본값 System (안전). 새 에러 추가 시 exhaustive test가 기본값으로 빠지지 않도록 강제.

---

### [ ] Unit 9-1. GitHub Actions CI 워크플로우

- **목적**: → PLAN: Phase 9 > Task 9-1. CI 녹색 신호 + 배지.
- **책임**:
  - 입력: git push / PR
  - 출력: CI 체크 + README 배지
  - 경계: `.github/workflows/ci.yml` + README 배지 태그
- **설계**:
  - 구조: 단일 job — Go 1.22+ setup, actions/cache로 module cache, `go build ./...`, `go vet ./...`, `make test-unit`, `go test -race ./...` (unit 범위). integration 테스트 제외 명시 코멘트.
  - 흐름: push → CI 실행 → 녹색/빨강 → README 배지 상태 반영
- **선택 이유**: 포트폴리오 신뢰 신호. Phase 4 `make test-unit` 타겟 재사용.
- **실패/예외**: race 경고 발생 → CI 실패 → 근본 원인 수정 (race 숨기지 않음). integration 테스트가 흘러들어가면 인프라 미준비로 실패 → `-tags integration` 제외 옵션 엄수.

---

### [ ] Unit 9-2. 레포 이해도 문서 패키지 (README + 컴포넌트 + 시나리오 로그)

- **목적**: → PLAN: Phase 9 > Task 9-2. 외부인이 레포만 보고 구조·기동법·시나리오·근거 파악.
- **책임**:
  - 입력: Phase 0 ~ Phase 8 누적 코드 + `docs/decisions/*` 기록
  - 출력: README + 컴포넌트별 문서 + 시나리오 실행 로그
  - 경계: 저장소 루트 `README.md` + `docs/0N-*.md` + `docs/scenarios/*.md`
- **설계**:
  - 구조:
    - README: 아키텍처 텍스트 다이어그램, 기동법 (docker-compose + make build + make run), 4개 대표 시나리오 링크, CI 배지
    - `docs/01-runtime-overview.md` ~ `docs/05-multi-agent.md` — 각 컴포넌트 설계 의도와 경계
    - `docs/architecture-overview.md` — 개요 유지, 세부는 링크
    - `docs/scenarios/weather.md`, `hotel.md`, `retry.md`, `multi-agent.md` — 실제 실행 로그 첨부
  - 흐름: 각 문서는 이미 존재하는 `docs/decisions/phaseN.md`를 근거로 작성. drift 방지를 위해 코드 최신화 후 작성.
- **선택 이유**: 개별 문서를 Unit으로 쪼개면 "외부인 이해 가능한가" 단일 관찰 기준이 사라진다. 하나의 Unit으로 묶고 Exit Criteria 4개 관찰 지점으로 검증.
- **실패/예외**: 코드와 문서 drift → Unit 진입 시 `docs/decisions/*` 재확인 후 시작. 실행 로그 첨부 시 API 키나 비밀정보 노출 위험 → 시나리오 로그는 mock tool 기반으로만 작성.

---

## 7. PLAN ↔ IMPLEMENT 매핑

| PLAN Task | IMPLEMENT Unit | 상태 |
|-----------|----------------|------|
| Task 5-1-2 | Unit 5-1-2 | ⬜ |
| Task 5-2   | Unit 5-2    | ⬜ |
| Task 5-3   | Unit 5-3    | ⬜ |
| Task 5-4   | Unit 5-4    | ⬜ |
| Task 5-5   | Unit 5-5    | ⬜ |
| Task 6-1   | Unit 6-1    | ⬜ (6-D1 차단) |
| Task 6-2   | Unit 6-2    | ⬜ (6-D1 차단) |
| Task 7-1   | Unit 7-1    | ⬜ (7-D1 차단) |
| Task 7-2   | Unit 7-2    | ⬜ |
| Task 7-3   | Unit 7-3    | ⬜ |
| Task 7-4   | Unit 7-4    | ⬜ (7-D3 차단) |
| Task 7-5   | Unit 7-5    | ⬜ (7-D2 차단) |
| Task 8-1   | Unit 8-1    | ⬜ |
| Task 8-2   | Unit 8-2    | ⬜ |
| Task 8-3   | Unit 8-3    | ⬜ (8-D1 차단) |
| Task 8-4   | Unit 8-4    | ⬜ |
| Task 8-5   | Unit 8-5    | ⬜ |
| Task 9-1   | Unit 9-1    | ⬜ |
| Task 9-2   | Unit 9-2    | ⬜ |

**매핑 누락 PLAN 항목**: 없음 (Phase 5 Exit Criteria의 `docs/decisions/phase5.md` 기록 등 문서 작업은 각 Phase Exit Criteria에 흡수, 별도 Unit 없음).

---

## 8. 진행 추적 규칙

- **IMPLEMENT 체크 (⬜ → ✅)**: 구현 단위의 코드/테스트 작성 완료 + 로컬 빌드/테스트 통과 시
- **PLAN 체크 (`[ ]` → `[x]`)**: 해당 Task의 Exit Criteria가 관찰 가능한 상태로 검증 완료 시
- 두 체크는 분리. IMPLEMENT 체크 후 PLAN 체크 전에 "구현은 끝났지만 검증이 안 됨" 구간이 존재할 수 있다. 이 구간에서 reviewer가 이의 제기 가능.
- **진행 상태의 단일 진입점**: 이 IMPLEMENT.md. 현재 어디까지 구현됐고 다음에 무엇인지는 본 문서만 보고 파악 가능해야 한다.
- **Decision Point 승인**: 승인 시 해당 Decision Point 체크박스 ✅ 처리. 승인 내용과 선택된 옵션을 바로 옆에 1줄 기록.

---

## 9. 다음 작업

**즉시 진입 가능**: Unit 5-1-2 (errgroup 병렬 실행 실습). 선행 조건 `golang.org/x/sync` 의존성 추가부터 시작. Decision Point 미필요.

**Unit 5-1-2 완료 후 진입 가능**: Unit 5-2 (Verifier 통합). 이후 5-3 → 5-4 → 5-5로 의존 순서 따라 진행. Phase 5 완료까지 Decision Point 없음.

**Phase 6 진입 전 확인 필요**: Decision Point 6-D1 승인.
