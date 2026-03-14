# AI Agent Runtime

LangChain, LangGraph 같은 고수준 프레임워크 없이 AI Runtime의 핵심 구조를 직접 구현하며 익히기 위한 커리큘럼.

---

## 프로젝트 소개

이 프로젝트는 "프레임워크 사용자"가 아닌 "runtime 설계자" 관점으로 agent system을 이해하는 데 목적이 있다.

planner, executor, tool router, state, memory, verifier, scheduler 같은 runtime 구성요소를 직접 나누고 연결하면서, agent system이 실제로 어떻게 굴러가는지 손으로 체득하는 커리큘럼이다.

---

## 왜 프레임워크를 쓰지 않는가

LangChain이나 LangGraph 같은 프레임워크는 데모를 빠르게 만드는 데는 편하다. 하지만 아래 질문에 답하려면 오히려 추상화가 방해가 된다.

- agent loop는 어떤 조건에서 반복되고 종료되는가
- planner는 어떤 형식의 결정을 내려야 하는가
- tool router는 planner 결과를 어떻게 실제 실행으로 연결하는가
- session state와 memory는 어떻게 구분해야 하는가
- 실패, retry, timeout, verification은 어디서 처리해야 하는가
- runtime을 서비스 구조로 확장하려면 어떤 경계가 먼저 필요한가

추상화가 두꺼우면 loop가 어떻게 도는지, state가 어떻게 흘러가는지 보이지 않는다. 직접 만들어야 설계 감각이 생긴다.

---

## 학습 목표

이 커리큘럼을 따라가면 최종적으로 아래를 설명하고 구현할 수 있어야 한다.

- 최소 agent loop를 직접 구현할 수 있다
- planner, executor, tool router의 책임을 분리할 수 있다
- request state, session state, working memory, long-term memory를 구분할 수 있다
- tool registry와 tool interface를 직접 설계할 수 있다
- verifier, retry, timeout 같은 제어 흐름을 runtime에 녹일 수 있다
- 단일 agent 구조를 multi-agent orchestration으로 확장할 수 있다
- CLI 수준의 실행기를 API + worker 구조로 발전시키는 방향을 설명할 수 있다

---

## 비목표

이 커리큘럼은 아래를 우선 목표로 두지 않는다.

- production-grade agent framework를 처음부터 완성하는 것
- 브라우저 자동화 중심 에이전트
- 자율 코딩 에이전트
- 특정 프레임워크 wrapper
- 화려한 데모를 빠르게 만드는 것

핵심은 "runtime 제어 흐름과 상태 모델을 직접 구현하며 이해하는 것"이다.

---

## 최종 구성요소 목록

이 커리큘럼이 다루는 runtime 핵심 구성요소:

| 구성요소 | 다루는 Phase |
|---|---|
| Runtime | Phase 1 |
| Agent | Phase 1, 6 |
| Planner | Phase 1, 3 |
| Executor | Phase 1 |
| Tool / Tool Registry / Tool Router | Phase 2 |
| Session | Phase 4 |
| Memory (Working / Long-term) | Phase 4 |
| Verifier | Phase 5 |
| Task / Step | Phase 6 |
| Workflow | Phase 6 (Task 의존성 그래프) |
| Scheduler | Phase 7 (async task 큐 + worker 스케줄링) |

---

## 기술 스택

**언어:** Go

**1차 스택 (Phase 0~6)**

- OpenAI API (ChatGPT)
- Redis
- Postgres
- Docker Compose

**후반 확장 스택 (Phase 7~)**

- Kafka
- pgvector / Qdrant
- OpenTelemetry / Prometheus / Grafana
- Kubernetes

---

## 구현 원칙

1. **처음엔 단일 프로세스** — 처음부터 MSA로 쪼개지 않는다. 핵심 loop와 상태 모델을 이해하는 것이 먼저다.
2. **프레임워크보다 인터페이스** — 라이브러리 사용보다 planner / executor / tool / memory 경계를 직접 정의하는 것을 우선한다.
3. **상태는 반드시 분리** — request state / session state / working memory / long-term memory를 하나로 뭉개지 않는다.
4. **LLM은 부품이다** — 핵심은 LLM 호출 자체가 아니라 orchestration이다.
5. **단계마다 동작하는 결과물** — 문서만 쌓지 않는다. 각 Phase마다 CLI 또는 API로 실제 동작해야 다음 단계로 간다.

---

## 전체 로드맵 (한눈에)

```
Phase 0  준비 (환경설정 + 용어 + 설계)
Phase 1  최소 Agent Loop
Phase 2  Tool Registry + Tool Router
Phase 3  Planner 고도화 / LLM 연결
Phase 4  Session / State / Memory 분리
Phase 5  Verifier / Retry / Concurrency
Phase 6  Multi-Agent Orchestration
Phase 7  Runtime 서비스화
Phase 8  운영 고도화
Phase 9  문서화 / 포트폴리오
```

> Phase 3(Planner/LLM)이 Phase 4(Session/Memory)보다 먼저인 이유:
> LLM이 없는 상태에서 session을 붙이면 실제 동작 검증이 불가능하다.
> LLM planner가 먼저 동작해야 session 연결 후 대화 맥락이 제대로 흘러가는지 확인할 수 있다.

---

## Phase 0 — 준비

### 목적

코드보다 개념을 먼저 고정하고, 개발 환경을 세팅한다. 문서만 쓰다 멈추지 않도록 프로젝트 초기화도 같이 진행한다.

### Step 0-1. LLM Provider 확정

**OpenAI API (ChatGPT)** 로 확정한다.

Planner 인터페이스를 설계하기 전에 provider를 고정해야 tool calling 스펙을 반영한 추상화를 제대로 만들 수 있다. 나중에 provider를 바꾸더라도 내부만 교체되도록 LLM 추상화 레이어를 둔다.

```go
type LLMClient interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
```

### Step 0-2. 환경설정 (docker-compose + .env)

`docker-compose.yml` 작성 — Redis와 Postgres를 로컬에서 띄운다.

```yaml
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: agent
      POSTGRES_PASSWORD: agent
      POSTGRES_DB: agentflow
    ports:
      - "5432:5432"
```

환경변수는 `.env` 파일로 관리한다. `.env.example`을 레포에 포함하고, `.env`는 `.gitignore`에 추가한다.

```
# .env.example
OPENAI_API_KEY=sk-...
REDIS_URL=redis://localhost:6379
POSTGRES_URL=postgres://agent:agent@localhost:5432/agentflow?sslmode=disable
```

코드에서는 `os.Getenv` 또는 별도 config 패키지로 읽는다. 환경변수가 없으면 시작 시점에 명확히 에러를 낸다.

### Step 0-3. 프로젝트 초기화

```
cmd/agent-cli/
internal/agent/
internal/planner/
internal/executor/
internal/state/
internal/tools/
docs/
```

각 디렉터리에 빈 인터페이스 stub 파일을 생성하고 `go build ./...`가 통과하는지 확인한다.

### Step 0-4. 용어 정리

`docs/glossary.md` 작성.

Agent / Runtime / Planner / Executor / Tool / Tool Router / Session / Memory / Verifier / Task / Step 각각을 명확히 분리해서 정의한다.

### Step 0-5. 전체 흐름도

`docs/architecture-overview.md` 작성.

```
User Request → Runtime → Planner → Tool Router → Executor → Memory Update → Verifier → Response
```

### Step 0-6. 범위 고정

이번 프로젝트에서 **하지 않을 것**:
- 브라우저 자동조작
- 코드 수정형 에이전트
- 자율 배포 에이전트

**할 것**: QA / Search / Planning형으로 제한

### 테스트

- `docker-compose up` 후 Redis에 `redis-cli ping` 응답 확인
- `docker-compose up` 후 Postgres에 `psql` 접속 확인

### 완료 기준

- `go build ./...` 통과
- `docker-compose up` 후 Redis/Postgres 접속 확인
- `.env.example` 파일 존재, OPENAI_API_KEY 항목 포함
- `docs/glossary.md`, `docs/architecture-overview.md` 문서 존재
- LLM Provider 확정, 범위 문서 존재

---

## Phase 1 — 최소 Agent Loop

### 목적

가장 먼저 만들 것은 loop의 뼈대다.

```
Input → Plan → Execute → Observe → Decide Finish → Response
```

### Step 1-1. CLI 입력기

```bash
go run ./cmd/agent-cli
> 오늘 서울 날씨 알려줘
```

- stdin 입력
- request ID 생성
- session ID 임시 고정
- 입력 받으면 `runtime.Run()` 호출

### Step 1-2. AgentState 구조

```go
type AgentState struct {
    RequestID    string
    SessionID    string
    UserInput    string
    CurrentPlan  PlanResult
    LastToolCall string
    ToolResults  []ToolResult
    FinalAnswer  string
    StepCount    int
    Status       AgentStatus
}
```

`LastToolCall`: 가장 최근에 호출된 tool 이름을 기록한다. retry 판단이나 planner context 구성 시 활용된다.

### Step 1-3. Planner 인터페이스

```go
type Planner interface {
    Plan(ctx context.Context, state AgentState) (PlanResult, error)
}
```

PlanResult 필드: action type / selected tool / extracted params / reason

action type은 이 단계에서 3개로 제한한다:

```go
const (
    ActionToolCall        ActionType = "tool_call"
    ActionRespondDirectly ActionType = "respond_directly"
    ActionFinish          ActionType = "finish"
)
```

> Phase 3(Planner 고도화)에서 이 3개를 유지한 채 추가 타입을 확장한다.

Mock planner로도 loop가 돌아야 한다. planner와 loop는 반드시 분리되어야 한다.

### Step 1-4. Executor 인터페이스

```go
type Executor interface {
    Execute(ctx context.Context, plan PlanResult) (ToolResult, error)
}
```

### Step 1-5. Finish 조건

- planner가 `finish` 반환
- max step 초과
- fatal error 발생
- `respond_directly` 완료

종료 시 반드시 종료 사유를 state에 기록한다.

### 테스트

- mock planner로 loop unit test
  - `tool_call` → 실행 → state 반영 흐름
  - `finish` 시 루프 종료
  - max step 초과 시 강제 종료
- planner를 교체해도 loop가 동작하는지 확인

### 완료 기준

- 질문 1개에 대해 루프가 정상 종료된다
- mock planner로도 동작한다
- 실행 로그에 step / action / tool / 종료 사유가 남는다

---

## Phase 2 — Tool Registry + Tool Router

### 목적

하드코딩 호출 → 등록된 목록에서 선택 후 실행.

### Step 2-1. Tool 인터페이스

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() Schema
    Execute(ctx context.Context, input map[string]any) (ToolResult, error)
}
```

### Step 2-2. Tool Registry

초기 tool 목록: `calculator`, `weather_mock`, `search_mock`

- name으로 조회 가능
- 미등록 tool 조회 시 명확한 에러

```go
type ToolRegistry interface {
    Register(tool Tool)
    Get(name string) (Tool, error)
    List() []Tool
}
```

### Step 2-3. Tool Router

planner 결과 → 실제 tool 연결.

처리해야 할 케이스:
- 미등록 tool name
- input validation 실패
- tool execute 에러

### Step 2-4. Tool Spec 문서화

`docs/tools.md` 작성.

각 tool의 name, description, 입력 스키마(필드명/타입/필수 여부), 출력 형식, 에러 케이스를 정리한다.

```markdown
## calculator

- 입력: expression (string, 필수)
- 출력: result (float64)
- 에러: 잘못된 수식 입력 시 parse error 반환
```

### Step 2-5. Tool 실행 로그

각 tool call마다 기록: request id / session id / tool name / input / output summary / duration / error 여부

### 테스트

- registry: 등록 → 조회 → 미등록 에러 unit test
- router: 유효 / 잘못된 tool name 라우팅 unit test
- 각 tool: 정상 입력 / 잘못된 입력 unit test

### 완료 기준

- 새 tool 추가 시 registry에 등록만 하면 된다
- planner와 tool이 느슨하게 결합된다
- `docs/tools.md`에 각 tool의 입출력 스펙이 정리되어 있다

---

## Phase 3 — Planner 고도화 / LLM 연결

### 목적

mock planner를 실제 LLM 기반 planner로 교체한다. 단일 액션 판단에 집중한다. (Task Decomposition은 Phase 6에서 다룬다)

### Step 3-1. ActionType 확장

Phase 1의 3개(`tool_call`, `respond_directly`, `finish`)를 유지하면서 아래를 추가한다:

```go
const (
    // Phase 1에서 정의한 기본 3개
    ActionToolCall        ActionType = "tool_call"
    ActionRespondDirectly ActionType = "respond_directly"
    ActionFinish          ActionType = "finish"

    // Phase 3에서 추가
    ActionAskUser       ActionType = "ask_user"
    ActionSummarize     ActionType = "summarize"
    ActionSearchMemory  ActionType = "search_memory"
    ActionRetry         ActionType = "retry"
)
```

### Step 3-2. PlanResult 스키마 고정

structured output (JSON schema 강제)으로 구현한다. LLM이 아래 스키마를 반드시 지켜서 출력하도록 system prompt에 JSON schema를 명시한다.

```go
type PlanResult struct {
    Action           ActionType     `json:"action"`
    ToolName         string         `json:"tool_name,omitempty"`
    ToolInput        map[string]any `json:"tool_input,omitempty"`
    ReasoningSummary string         `json:"reasoning_summary"`
    Confidence       float64        `json:"confidence"`
    NextGoal         string         `json:"next_goal,omitempty"`
}
```

system prompt 예시:

```
당신은 agent planner입니다.
반드시 아래 JSON schema에 맞게만 응답하세요.
다른 텍스트는 절대 포함하지 마세요.

{
  "action": "<tool_call|respond_directly|finish|ask_user|summarize|search_memory|retry>",
  "tool_name": "<tool name, action이 tool_call일 때만>",
  "tool_input": { ... },
  "reasoning_summary": "<판단 근거>",
  "confidence": <0.0~1.0>,
  "next_goal": "<다음 목표>"
}
```

### Step 3-3. LLM Planner 연결

구현 포인트:
- system prompt 설계 (action schema 강제 포함)
- invalid JSON 대응: 파싱 실패 시 재생성 1회 시도 후 에러 처리
- hallucination 최소화: 존재하지 않는 tool name 반환 시 에러 처리

```go
type LLMPlanner struct {
    client    LLMClient
    toolSpecs []ToolSpec
}

func (p *LLMPlanner) Plan(ctx context.Context, state AgentState) (PlanResult, error) {
    messages := p.buildMessages(state) // system + user messages
    resp, err := p.client.Complete(ctx, CompletionRequest{
        Model:    "gpt-4o",
        Messages: messages,
    })
    if err != nil {
        return PlanResult{}, err
    }
    return p.parseResult(resp.Content)
}
```

### Step 3-4. Token Usage 로깅

LLM을 연결하는 이 시점부터 token 사용량을 기록한다. 이후 쓴 token을 소급 추적할 방법이 없으므로 지금 시작한다.

기록 항목:
- request id / prompt tokens / completion tokens / total tokens
- planner 호출 횟수 / session 누적 token

최소 구현은 구조화된 로그 출력으로 충분하다. 정책(비용 한도)은 Phase 8에서 추가한다.

### Step 3-5. Reflection

결과가 질문에 충분한지 self-check하는 단계. LLM 별도 호출로 구현한다 (같은 LLMClient를 사용하되 reflection 전용 prompt를 쓴다).

```go
type Reflector interface {
    Reflect(ctx context.Context, state AgentState) (ReflectResult, error)
}

type ReflectResult struct {
    Sufficient bool   `json:"sufficient"`
    MissingConditions []string `json:"missing_conditions"`
    Suggestion string `json:"suggestion"`
}
```

reflection prompt 핵심:

```
현재 tool 실행 결과가 사용자 질문에 충분히 답하는가?
누락된 조건이 있다면 무엇인가?
반드시 아래 JSON schema로 응답하라.
```

### 테스트

- invalid JSON 반환 시 에러 처리 unit test
- PlanResult 스키마: 각 action type별 필수 필드 검증
- token logger: 호출마다 기록 여부 확인
- reflection: sufficient=false 케이스에서 MissingConditions 포함 여부 검증

### 완료 기준

- planner가 실제 LLM으로 tool 선택과 finish 판단을 한다
- LLM 호출마다 token 사용량이 로그에 남는다
- invalid JSON 응답 시 재시도 후 에러를 반환한다
- reflection 결과가 state에 반영된다

---

## Phase 4 — Session / State / Memory 분리

### 목적

LLM planner가 동작하는 상태에서, 대화 맥락과 메모리를 명확히 분리한다. 대화 히스토리를 메모리로 뭉개는 것이 흔한 실수다. 4가지를 코드 레벨에서 분리한다.

| 종류 | 의미 | 생명주기 | 저장소 |
|---|---|---|---|
| Request State | 이번 실행 중 필요한 상태 | 요청 종료 시 폐기 | 메모리 |
| Session State | 연속 대화 맥락 | 세션 유지 | Redis |
| Working Memory | 문제 해결 중간 산출물 | 작업 종료 시 폐기 | 메모리 |
| Long-term Memory | 사용자/도메인 장기 정보 | 영구 | Postgres |

### Step 4-1. Request State

한 번의 실행에만 필요한 상태를 정의한다.

```go
type RequestState struct {
    RequestID      string
    UserInput      string
    ToolResults    []ToolResult
    ReasoningSteps []string
    StartedAt      time.Time
}
```

### Step 4-2. Session State

같은 사용자의 연속 대화에서 이어지는 상태를 정의한다.

```go
type SessionState struct {
    SessionID     string
    RecentContext []Message
    ActiveGoal    string
    LastUpdated   time.Time
}
```

저장소: 1차 in-memory map → 2차 Redis

### Step 4-3. Working Memory

문제 해결 과정의 중간 산출물을 관리한다.

```go
type WorkingMemory struct {
    SearchResults   []any
    FilteredResults []any
    Summaries       []string
}
```

### Step 4-4. Long-term Memory

오래 보관할 사용자 선호나 도메인 정보를 다룬다.

```go
type Memory struct {
    ID        string
    UserID    string
    Content   string
    Tags      []string
    CreatedAt time.Time
}
```

저장소: Postgres

### Step 4-5. Memory Manager

runtime이 저장소를 직접 몰라도 된다.

```go
type MemoryManager interface {
    LoadSession(ctx context.Context, sessionID string) (SessionState, error)
    SaveSession(ctx context.Context, sessionID string, state SessionState) error
    SaveMemory(ctx context.Context, userID string, content string, tags []string) error
    LoadRelevantMemory(ctx context.Context, query string) ([]Memory, error)
}
```

### 테스트

- Session State: 동일 session ID로 맥락이 이어지는지 unit test
- Memory Manager: in-memory 구현으로 저장 → 조회 unit test
- Redis session: 저장 후 process 재시작해도 복원되는지 확인

### 완료 기준

- session ID 기준으로 대화 맥락이 이어진다
- state와 memory가 코드 레벨에서 분리된다
- Redis 연결 후 session이 process 재시작에도 복원된다

---

## Phase 5 — Verifier / Retry / Concurrency

### 목적

실행 결과를 검증하고 필요하면 재시도한다. 여기서부터 Agent다운 느낌이 생긴다.

Phase 6의 Multi-Agent에서 Worker를 병렬 실행하게 된다. 그 전에 goroutine과 context cancellation 기초를 이 단계에서 익혀둔다.

아래 패턴을 코드 기반으로 확인하고 넘어간다:

```go
// context cancellation
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// goroutine + channel
results := make(chan ToolResult, len(tasks))
for _, task := range tasks {
    go func(t Task) {
        results <- execute(ctx, t)
    }(task)
}
```

- `context.WithTimeout` / `context.WithCancel` 사용법
- goroutine leak 방지 (done channel, context done)
- WaitGroup 기본 패턴

### Step 5-1. Verifier 인터페이스

```go
type Verifier interface {
    Verify(ctx context.Context, state AgentState) (VerifyResult, error)
}

type VerifyResult struct {
    Status  VerifyStatus // done | retry | fail
    Reason  string
    Suggestion string
}
```

### Step 5-2. Retry 정책

retry policy는 코드로 분리되어야 한다. loop 안에 인라인으로 쓰지 않는다.

```go
type RetryPolicy interface {
    ShouldRetry(err error, attempt int) bool
    Delay(attempt int) time.Duration
}
```

- tool timeout → 1회 재시도
- planner invalid output → 재생성
- 빈 결과 → 다른 파라미터로 재시도

### Step 5-3. Failure 분류

| 유형 | 처리 |
|---|---|
| tool not found | 에러 상태로 종료 |
| malformed params | planner 재호출 |
| timeout | retry policy 적용 |
| llm parse error | 재생성 시도 |
| empty result | 파라미터 변경 후 재시도 |

### 테스트

- Verifier: done / retry / fail 각 케이스 unit test
- Retry policy: 실패 유형별 재시도 횟수 제한 검증 (max 3회 초과 시 fail 반환)

### 완료 기준

- 실패 유형별 응답 정책이 코드로 분리되어 있다
- 무한 재시도가 발생하지 않는다
- context cancellation이 정상 동작한다 (timeout 시 goroutine이 정리된다)

---

## Phase 6 — Multi-Agent Orchestration

### 목적

단일 agent에서 Manager + Worker 구조로 확장한다. Phase 3에서 다루지 않은 Task Decomposition을 여기서 구현한다.

### 시나리오 (호텔 도메인)

```
Manager Agent
  → Search Agent   (호텔 목록 검색)
  → Filter Agent   (조건 필터링)
  → Ranking Agent  (평점 정렬)
  → Summary Agent  (결과 요약)
```

### Step 6-1. Task Decomposition

Manager가 사용자 요청을 하위 Task로 분해한다.

```
"서울 저렴한 호텔 찾고 평점 높은 순으로 보여줘"
→ hotel_search → filter_by_price → sort_by_rating → summarize
```

```go
type TaskDecomposer interface {
    Decompose(ctx context.Context, userInput string) ([]Task, error)
}
```

### Step 6-2. Agent 인터페이스

```go
type Agent interface {
    Name() string
    CanHandle(task Task) bool
    Execute(ctx context.Context, task Task) (TaskResult, error)
}
```

### Step 6-3. Task Contract

agent 간 주고받는 데이터 형식 통일:

```go
type Task struct {
    ID           string
    Type         string
    InputPayload map[string]any
    Dependencies []string
}

type TaskResult struct {
    TaskID  string
    Output  map[string]any
    Error   error
    Latency time.Duration
}
```

### Step 6-4. Manager Agent

- 요청 분석 → Task 분해 → 필요한 worker 판단 → 실행 순서 결정 → 결과 병합
- 독립 task는 goroutine으로 병렬 실행한다 (Phase 5에서 익힌 패턴 활용)

```go
type ManagerAgent struct {
    decomposer TaskDecomposer
    workers    []Agent
}
```

### Step 6-5. Multi-Agent 실행 로그

기록: 호출된 agent / 호출 순서 / 각 latency / 실패 지점

### 테스트

- Manager: worker 선택 로직 unit test
- Task Decomposition: 입력별 task 목록 검증
- Task Contract: 결과 병합 검증
- 병렬 실행: 독립 task가 실제로 동시에 실행되는지 확인

### 완료 기준

- manager → worker 흐름이 동작한다
- multi-agent trace를 로그로 볼 수 있다
- 독립 task는 병렬 실행된다

---

## Phase 7 — Runtime 서비스화

### 목적

CLI 장난감 → API 서버 + 백그라운드 워커.

### Step 7-1. HTTP API

```
POST /v1/agent/run
GET  /v1/tasks/{id}
GET  /v1/sessions/{id}
```

### Step 7-2. Async Task 상태

```
queued → running → succeeded / failed
```

### Step 7-3. Queue 구조

- 1차: in-memory channel
- 2차: Redis Stream 또는 Kafka
- API 서버와 runtime worker를 논리적으로 분리

### Step 7-4. Admin / Debug API

최근 task 목록 / 실패 task 조회 / session dump / tool stats

### 테스트

- 각 엔드포인트 integration test
- 상태 전이 검증: `queued → running → succeeded` 순서 확인

### 완료 기준

- CLI 없이 API로 실행 가능
- task 상태를 조회할 수 있다
- API 서버와 worker가 논리적으로 분리되어 있다

---

## Phase 8 — 운영 고도화

### 목적

runtime을 실제 운영 관점에서 바라보는 감각을 붙인다.

### Step 8-1. Timeout / Cancellation

- tool별 timeout 설정
- 전체 request deadline 설정
- context 계층 구조 설계

### Step 8-2. 비용 제어 (Phase 3 token 로깅 확장)

- session별 token 누적량 추적
- 비용 한도 초과 시 중단 정책

### Step 8-3. Observability

- structured logging + trace ID
- tool / planner latency metrics
- OpenTelemetry: request → planner → tool → verifier trace 연결

### Step 8-4. 에러 분류

user error / system error / provider error / tool error / retryable error / fatal error

### Step 8-5. Policy Layer

- tool 사용 제한
- 사용자별 max step 제한
- 비용 한도 초과 시 중단

### 완료 기준

- tool별 timeout이 설정되어 있고 실제로 동작한다
- session별 token 누적량을 조회할 수 있다
- OTel trace가 planner ~ verifier까지 연결된다

---

## Phase 9 — 문서화 / 포트폴리오

### 목적

이 커리큘럼 결과물을 설계 역량을 보여주는 자료로 정리한다.

### Step 9-1. README 고도화

- 프로젝트 소개
- 왜 프레임워크를 쓰지 않았는가
- 핵심 구조 다이어그램
- 실행 방법 + 예시 시나리오

### Step 9-2. 아키텍처 문서

```
docs/01-runtime-overview.md
docs/02-planner.md
docs/03-memory.md
docs/04-tool-router.md
docs/05-multi-agent.md
```

### Step 9-3. 실행 시나리오 문서

- 날씨 질의 흐름
- 호텔 검색 흐름
- 실패 후 retry 흐름
- multi-agent 흐름

### 완료 기준

- README만 읽어도 전체 구조를 이해할 수 있다
- 시나리오 문서에 실제 실행 로그 예시가 포함된다

---

## 단계별 산출물 체크리스트

매 Phase마다 아래 4개를 반드시 남긴다.

- [ ] 동작하는 코드
- [ ] 테스트 (unit 또는 integration)
- [ ] docs 또는 README 갱신
- [ ] 다음 Phase TODO

이 4개 없이 다음 단계로 넘어가지 않는다.

---

## 추천 진행 순서 (Sprint)

Phase 순서 변경(Planner/LLM 먼저, Session/Memory 나중)에 맞게 조정된 Sprint 계획이다.

### Sprint 1 — 기반 세팅

- docker-compose.yml 작성 (Redis, Postgres)
- `.env.example` 작성, OPENAI_API_KEY 항목 포함
- `docker-compose up` 후 Redis/Postgres 접속 확인
- glossary.md 작성
- architecture-overview.md 작성
- 프로젝트 디렉터리 구조 초기화, `go build ./...` 통과

### Sprint 2 — 최소 Loop

- CLI 입력기
- AgentState 정의 (LastToolCall 포함)
- mock planner 구현
- mock executor 구현
- loop 종료 조건 (finish / max step / fatal error)

### Sprint 3 — Tool 시스템

- Tool 인터페이스 정의
- calculator tool
- weather_mock tool
- search_mock tool
- Tool Registry 구현
- Tool Router 구현
- `docs/tools.md` 작성
- Tool 실행 로그

### Sprint 4 — LLM Planner 연결

- OpenAI API LLMClient 구현
- structured output 기반 PlanResult 스키마 고정
- LLM Planner 구현 (system prompt + JSON schema 강제)
- invalid JSON 대응 (재시도 1회)
- ActionType 확장 (ask_user, summarize, search_memory, retry 추가)
- Token Usage 로깅
- Reflection 구현 (별도 LLM 호출)

### Sprint 5 — Session / Memory

- Request State 분리
- Session State 정의 및 in-memory 구현
- Working Memory 구현
- Long-term Memory 구조 정의
- Redis 연결, Session State 이전
- Postgres 연결, Memory 저장
- Memory Manager 인터페이스 구현
- session restore 검증 (process 재시작 후 맥락 복원)

### Sprint 6 — Verifier / Retry

- goroutine / context cancellation 기초 확인
- Verifier 인터페이스 구현
- Retry Policy 분리 구현
- Failure 분류 및 처리
- context timeout 동작 검증

### Sprint 7 — Multi-Agent

- Agent 인터페이스 분리
- Task / TaskResult Contract 정의
- TaskDecomposer 구현
- Worker Agent 구현 (Search, Filter, Ranking, Summary)
- Manager Agent 구현 (병렬 실행 포함)
- multi-agent trace 로그

### Sprint 8 — 서비스화

- HTTP gateway 구현
- async task 상태 모델 (queued → running → succeeded/failed)
- in-memory queue → worker 연결
- Admin / Debug API 추가

### Sprint 9 — 운영 + 문서화

- timeout / cancellation 정책 적용
- token 비용 추적 및 한도 정책
- OTel trace 연결
- 에러 분류 체계 정비
- README 고도화
- 아키텍처 문서 작성
- 시나리오 문서화

---

## AI 도구 활용 원칙

Claude Code 같은 도구는 역할을 나눠 쓰는 것이 좋다.

### Architect 역할

구조와 경계를 설계할 때 활용한다.

- 디렉터리 구조 설계
- 인터페이스 설계
- 책임 분리
- 경계 정의

예시 요청:
- "planner, executor, state 경계를 어떻게 나눌지 설계해줘"
- "tool registry 인터페이스 설계해줘"
- "session state와 request state를 어떻게 분리할지 설명해줘"

### Implementer 역할

코드를 직접 작성할 때 활용한다.

- Go 코드 작성
- 테스트 코드 작성
- handler 구현
- repository 구현

예시 요청:
- "Go로 Tool 인터페이스와 registry 구현해줘"
- "Redis 기반 session repository 작성해줘"
- "structured output 기반 LLM planner 구현해줘"

### 원칙

- Architect 단계를 거치지 않고 Implementer부터 쓰면 인터페이스 경계가 흐려진다.
- 구현을 먼저 받아도, 설계 의도를 직접 이해하지 않으면 다음 단계에서 막힌다.
- 도구는 보조다. 각 Phase의 완료 기준은 직접 판단한다.

---

## 하지 말아야 할 것

처음부터 하면 안 되는 것:

- 여러 LLM provider 동시 지원 (provider 고정 먼저)
- 처음부터 MSA 구조
- 처음부터 Kubernetes
- 브라우저 agent
- task decomposition을 Phase 1~2에서 시도 (loop가 안정된 다음에)

먼저 loop → state → planner → tool router → verifier → session memory 순으로 엔진을 만든다.
엔진이 된 다음에 서비스화로 간다.
