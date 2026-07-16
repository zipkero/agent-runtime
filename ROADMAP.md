# ROADMAP

이 문서는 `agent-runtime`을 Go 기반으로 단계적으로 발전시키기 위한 로드맵입니다.

이 프로젝트는 단계별 예제 폴더를 만들지 않습니다.

대신 하나의 Go Agent Runtime 코드베이스를 계속 개선하며, 각 단계의 개념을 Runtime 기능으로 흡수합니다.

도착점은 `README.md` 「이 프로젝트가 향하는 곳」이 정의하는 **Orchestrator 기반 Multi-Agent System**입니다.
앞 단계(LLM·Agent loop·Tool·RAG·Memory)는 이 도착점을 떠받치는 구성 요소입니다.

## 진행 원칙

```text
구현 의존성 순서로 단계적으로 발전시킨다.
제어 흐름·상태·협력은 일반 Go 코드(for / switch / 함수 / struct)로 직접 구현한다.
Runtime 본체는 직접 만들고, 외부 연결(LLM·임베딩·웹 검색·벡터 저장·MCP·A2A)은 공식 SDK 또는 HTTP로 처리한다.
단계별 예제 폴더는 만들지 않는다.
하나의 Runtime을 계속 발전시킨다.
진행 상태는 docs, commit, tag로 추적한다.
```

## 진행 현황

```text
[x] Phase 0  Project Foundation
[x] Phase 1  LLM Client          (Claude + Ollama 로컬 provider)
[x] Phase 2  Agent Loop
[x] Phase 3  Tool Calling Runtime
[ ] Phase 4  Single Agent Runtime (4.1 Tool 묶음 / 4.2 실행 구조 / 4.3 Streaming / 4.4 Tool 실행 backend)
[ ] Phase 5  RAG Runtime          (5.1 인덱싱 / 5.2 검색·활용)
[ ] Phase 6  Memory Runtime       (6.1 단기 메모리 / 6.2 장기 메모리 & Tool)
[ ] Phase 7  Multi-Agent Runtime  (7.1 Worker·Routing / 7.2 Orchestrator-Workers / 7.3 구체 Worker)
[ ] Phase 8  MCP Adapter
[ ] Phase 9  A2A Adapter
[ ] Phase 10 Orchestrator Runtime (10.1 Worker 구성 / 10.2 오케스트레이션 / 10.3 견고성·응답 통합)
[ ] Phase 11 Runtime Refinement
```

---

## Phase 0. Project Foundation — 완료

### 목표

Go 기반 Agent Runtime 프로젝트의 기본 구조를 만든다.

### 구현 범위

* Go module 생성
* 기본 디렉터리 구조 구성
* config loader
* logger (별도 패키지 없이 진입점에서 config 설정으로 초기화)
* CLI entry point
* `.env` 로딩
* 기본 README / ROADMAP 문서 작성

### 주요 패키지

```text
cmd/agent-runtime
internal/config
```

### 완료 기준

* `go run ./cmd/agent-runtime` 실행 가능
* 환경변수 로딩 가능
* 기본 로그 출력 가능
* 프로젝트 목적과 진행 방식이 README에 정리됨

---

## Phase 1. LLM 기반 의사결정 구조 — 완료

### 목표

LLM을 Agent Runtime의 판단 주체로 사용하기 위한 기본 추상화를 만든다.

### 구현 범위

* `LLMClient` interface
* `ChatRequest`
* `ChatResponse`
* `Message`
* `ToolCall`
* `ToolResult`
* 실제 Claude API를 호출하는 client
* 로컬 모델 provider (Ollama, tool calling 포함)
* GPT 등 다른 provider는 같은 `LLMClient` interface 뒤에 추가 가능
* 테스트용 stub client
* model / api key config
* context timeout 처리
* 단발 CLI prompt 입력과 provider 응답 stdout 출력

### 주요 패키지

```text
internal/llm
internal/message
internal/config
```

### 학습 포인트

* LLM은 Runtime의 일부이지 Runtime 자체가 아니다.
* Provider와 Runtime은 분리되어야 한다.
* 일반 응답과 Tool Call 응답은 분리해서 다뤄야 한다.
* 실행 경로는 실제 API를 호출하되, interface 뒤에서 테스트는 stub으로 교체할 수 있어야 한다.

### 완료 기준

* CLI에서 사용자 입력을 받아 실제 LLM 응답을 받을 수 있다.
* LLM Provider 교체 가능성을 interface로 표현한다. (Claude / Ollama 구현, GPT 등은 같은 interface로 추가)
* request timeout이 동작한다.

---

## Phase 2. Agent Loop — 완료

### 목표

Agent가 단발성 LLM 호출 결과만 반환하는 것이 아니라, 메시지 상태와 종료 상태를 보존하며 판단하는 구조임을 코드로
표현한다. Phase 2에서는 Tool 실행까지 반복하지 않고, assistant 응답에 tool call이 있으면 추가 행동 필요 상태로
멈춘다.

### 구현 범위

* `AgentState` (평범한 struct: 메시지 / step / status / final answer / tool calls / last error / trace)
* `Agent`
* Agent run 실행 API
* LLM 호출 → 응답 해석 → final 또는 tool 대기 종료
* step counter
* final answer detection
* max steps
* error state
* trace 구조 도입 (step/action/status/error를 담는 단일 trace struct; 이후 Phase는 같은 구조에 필드만 더한다)

### 주요 패키지

```text
internal/agent
internal/message
```

### 학습 포인트

* Agent는 LLM 호출 한 번으로 끝나지 않는다.
* LLM 응답을 보고 다음 행동을 Runtime이 해석해야 한다.
* 무한 루프 방지를 위한 max step이 필요하다.
* Tool 실행이 붙기 전에도 final, tool 대기, max step, error 같은 종료 상태를 먼저 분리해야 한다.

### 완료 기준

* 사용자 입력이 `AgentState`에 저장된다.
* LLM 응답이 state에 누적된다.
* tool call 응답은 Tool 실행 없이 `needs_action` 상태로 멈춘다.
* max step 초과 시 안전하게 종료된다.
* LLM 호출 오류는 `error` 상태와 `LastError`로 확인할 수 있다.
* 각 run의 주요 action과 종료 이유를 `AgentState.Trace`에서 확인할 수 있다.

---

## Phase 3. Tool Calling Runtime — 완료

### 목표

LLM이 선택한 Tool을 Runtime이 안전하게 실행하는 구조를 만든다.

### 구현 범위

* `Tool` interface
* `ToolSchema`
* `ToolRegistry`
* tool input validation
* tool execution timeout
* tool result normalization
* unknown tool handling
* tool error handling
* basic calculator tool
* basic file read tool
* Phase 3 문서 상태 갱신
* trace에 tool call / tool result 기록

### 주요 패키지

```text
internal/tool
internal/agent
```

### 학습 포인트

* LLM은 Tool을 직접 실행하지 않는다.
* LLM은 Tool Call을 요청하고, Runtime이 실행한다.
* Tool schema가 명확해야 안정적인 Tool Calling이 가능하다.
* Tool 실행 결과를 state에 누적해 다음 루프 입력으로 넘기면, Phase 2의 agent loop가 비로소 여러 step을 돈다.

### 완료 기준

* Tool 등록 가능
* 이름 기반 Tool lookup 가능
* Agent가 Tool Call을 실행 가능
* Tool 결과를 다시 LLM 입력에 포함 가능
* 기본 calculator Tool과 file read Tool 사용 가능

---

## Phase 4. Single Agent Runtime — 예정

### 목표

Tool Calling이 가능한 Single Agent를 구현한다.

### 구현 범위 (하위 분할)

이 Phase는 이질적인 Tool, Agent 실행 구조, streaming, Tool 실행 격리를 분리하기 위해 feature-dir와 implement 사이클을
네 갈래로 나눴다. 소수점 번호는 4.1 → 4.2 → 4.3 → 4.4 순서의 진행 단위를 뜻한다. 4.4는 4.2의 Tool 실행 수명
계약 위에 세우되, 기존 번호와 문서 참조를 유지하기 위해 4.3 이후에 진행한다.

#### Phase 4.1 — Tool 묶음

* Web Search Tool (Tavily 검색 API 연동)
* File Save Tool
* Code Execution Tool

#### Phase 4.2 — Agent 실행 구조 (4.1 이후)

* Middleware hook (pre / post model)
* Structured Output
* Single Agent runner
* agent loop 기반 Single Agent 실행

#### Phase 4.3 — Streaming Agent Response (4.2 이후)

* Provider-neutral streaming LLM contract
* Runner streaming event
* CLI streaming 출력
* streaming 완료 후 final response 조립
* Structured Output final 검증과 streaming 관계 정리

#### Phase 4.4 — Tool Execution Backend (4.2 기반, 4.3 이후 진행)

* Agent의 Tool 호출 판단과 실제 실행 방식을 분리하는 Tool execution backend
* 기존 in-process Tool의 context 기반 cooperative cancellation을 유지하는 inline executor
* 강제 종료가 필요한 Runtime 소유 Tool을 위한 process-backed executor
* 실행 요청 ID, Tool 이름, arguments, deadline, result를 전달하는 직렬화 가능한 실행 envelope
* timeout 시 cancel 요청, grace period, process kill, process 종료 회수
* Tool 오류, timeout, Tool process crash의 구분과 trace 기록
* caller deadline과 Tool result 크기의 execution backend 경계 재검증

Phase 4.4의 process-backed executor는 Runtime이 소유한 Tool process의 실행 수명과 강제 종료를 다루며 보안 sandbox를
보장하지 않는다. Filesystem·network·syscall·CPU·memory 격리와 프롬프트 인젝션 방어는 production 확장 과제로
유지한다. 초기 범위는 일회성 process를 기준으로 하며 worker pool, process 재사용, 분산 실행은 포함하지 않는다.
Remote Tool은 로컬 요청 취소와 늦게 도착한 결과 폐기까지만 보장하고 원격 서버의 실제 실행 종료는 강제하지 않는다.

### 주요 패키지

```text
internal/agent
internal/tool
internal/toolexec
```

### 학습 포인트

* Single Agent는 하나의 Agent가 직접 판단하고 Tool을 호출하는 구조다.
* Tool이 많아질수록 Tool schema와 routing 품질이 중요해진다.
* Code Execution Tool은 강력하지만 보안 위험이 크므로 제한이 필요하다.
* model 호출 전후를 가로채는 middleware 훅으로 횡단 관심사를 분리할 수 있다.
* context는 in-process Tool에 취소를 요청하지만 실행을 강제 종료하지는 않는다.
* Tool process와 Multi-Agent의 `WorkerAgent`는 각각 실행 격리와 역할 분담을 책임하는 서로 다른 경계다.

### 완료 기준

* Agent가 Web Search Tool을 호출할 수 있다.
* Agent가 File Tool을 호출할 수 있다.
* Agent가 제한된 Code Execution Tool을 호출할 수 있다.
* Structured Output을 파싱할 수 있다.
* Streaming mode에서 model text chunk를 순차적으로 확인할 수 있다.
* 호출자는 Tool별로 inline 또는 process-backed 실행 방식을 선택할 수 있다.
* 기존 inline Tool은 cooperative cancellation과 결과 전달 동작을 유지한다.
* process-backed Tool이 context를 무시해도 timeout 뒤에는 process가 종료·회수된다.
* Tool timeout과 Tool process crash는 일반 Tool 오류와 구분되어 result와 trace에 기록된다.
* Tool process가 종료되기 전에는 Agent가 다음 model 호출이나 종료 상태로 전이하지 않는다.

---

## Phase 5. RAG Runtime — 예정

### 목표

내부 문서를 검색하고 답변에 활용하는 RAG 구조를 구현한다.

### 구현 범위 (하위 분할)

이 Phase는 인덱싱 파이프라인과 검색·활용이 선형 의존이라 feature-dir와 implement 사이클을 두 갈래로 나눈다.
소수점 번호는 5.1 → 5.2 순서로 진행하는 선행 의존을 뜻한다.

#### Phase 5.1 — 인덱싱 파이프라인

* document loader
* chunker
* embedding client (외부 임베딩 API 또는 로컬 모델 호출, interface로 추상화)
* vector store (Postgres + pgvector)

#### Phase 5.2 — 검색·활용 (5.1 이후)

* retriever
* retrieval tool
* source metadata
* answer generation with retrieved context

### 주요 패키지

```text
internal/rag
internal/tool
internal/agent
```

### 학습 포인트

* RAG는 Agent Runtime과 별도의 검색 시스템이다.
* 벡터 저장/검색은 Postgres + pgvector로 처리하고, 임베딩 생성은 외부에 위임한다.
* Retrieval 결과는 Tool Result로 Agent에 전달할 수 있다.
* 답변에는 source metadata가 유지되어야 한다.

### 완료 기준

* 문서 ingest 가능
* chunk 생성 가능
* embedding 생성 가능
* pgvector 기반 유사 문서 검색 가능
* Agent가 Retrieval Tool을 통해 내부 문서 기반 답변 생성 가능

---

## Phase 6. Memory Runtime — 예정

### 목표

Agent가 단기/장기 메모리를 사용할 수 있도록 구현한다.

### 구현 범위 (하위 분할)

이 Phase는 인메모리 영역과 영속 영역이 선형 의존이라 feature-dir와 implement 사이클을 두 갈래로 나눈다.
소수점 번호는 6.1 → 6.2 순서로 진행하는 선행 의존을 뜻한다. 6.2는 Phase 5에서 갖춘 Postgres 인프라를 재사용한다.

#### Phase 6.1 — 단기 메모리 (Postgres 불필요)

* `MemoryStore` interface
* short-term memory
* session memory
* message trimming
* summary memory

#### Phase 6.2 — 장기 메모리 & Tool (6.1 이후, Postgres 재사용)

* long-term memory (Postgres backend)
* user memory read tool
* user memory write tool
* category memory search tool

### 주요 패키지

```text
internal/memory
internal/tool
internal/agent
```

### 학습 포인트

* Memory는 모든 메시지를 계속 붙이는 것이 아니다.
* Short-term memory와 Long-term memory는 목적이 다르다.
* 긴 대화에서는 trimming과 summary가 필요하다.

### 완료 기준

* session id 기준 대화 저장 가능
* 최근 메시지 로드 가능
* 오래된 메시지 요약 가능
* 사용자 정보를 저장하고 다시 조회 가능
* Agent가 Memory Tool을 통해 개인화된 응답 생성 가능

---

## Phase 7. Multi-Agent Runtime — 예정

### 목표

여러 Agent가 역할을 나누어 협력하는 구조를 구현한다. 이 프로젝트의 핵심 Phase다.

이 Phase는 Phase 6(Memory)에 의존하지 않는다. Phase 4(Single Agent) 위에서 성립하며, Memory 뒤에 둔 것은
직렬 의존이 아니라 난이도·독립성에 따른 권장 순서다. Memory보다 먼저 진행해도 무방하다.

### 구현 범위 (하위 분할)

이 Phase는 분량이 크고 패턴이 서로 독립적이라 feature-dir와 implement 사이클을 세 갈래로 나눈다.
소수점 번호는 정수 Phase 같은 직렬 의존이 아니라, 7.1을 토대로 7.2·7.3이 올라가는 동급 분할을 뜻한다.

#### Phase 7.1 — Worker 인터페이스 & Routing

* `WorkerAgent` interface (transport-agnostic)
* Routing (요청을 분류해 적절한 worker로 디스패치)
* `WorkerAgent` → `Tool` adapter (worker를 Tool로 감싸 호출)
* trace에 agent 식별 기록 (멀티 에이전트 실행 추적)

#### Phase 7.2 — Orchestrator-Workers (7.1 위 동급)

* 작업 분해 (task decomposition)
* worker 호출·제어
* 결과 합성 (result aggregation)
* Planner-Worker flow

#### Phase 7.3 — 구체 Worker & 합성 (7.1 위 동급)

여기 Worker는 7.1·7.2 패턴을 실증하기 위한 최소 구현이다. 7.1에서 정의한 `WorkerAgent` 시그니처를 그대로
따르고, Phase 10.1의 production Worker(Web Search / RAG / File Management / Writer)는 같은 인터페이스 뒤에서
prompt·tool 구성만 교체·확장한다. 즉 7.3 → 10.1은 인터페이스를 유지한 채 구현 내용을 채워 넣는 관계이며,
같은 Agent를 처음부터 두 번 만드는 것이 아니다.

* Research Agent
* Writer Agent
* Result aggregation

### 주요 패키지

```text
internal/multiagent
internal/agent
```

### 학습 포인트

* Multi-Agent는 Agent를 많이 만드는 것이 아니라 책임을 나누는 것이다.
* Routing은 요청을 분류해 알맞은 worker에게 보내는 구조다. 다음 목적지를 런타임에 정한다는 점에서, Phase 2
  agent loop의 분기 판단을 worker 선택으로 확장한 것이다.
* Orchestrator-workers는 한 에이전트가 작업을 분해해 여러 worker를 호출하고 결과를 합성하는 구조다.
* worker를 Tool로 감싸면(worker-as-tool) orchestrator가 제어권을 유지한 채 worker를 호출할 수 있다.
* `WorkerAgent`는 처음부터 직렬화 가능한 입력/출력 + context 기반 시그니처로 설계해, 이후 remote(A2A) 워커로
  교체해도 Orchestrator가 바뀌지 않도록 한다.

### 완료 기준

* orchestrator-workers 패턴으로 사용자 요청을 task로 분해할 수 있다.
* routing이 task를 적절한 WorkerAgent로 디스패치한다.
* WorkerAgent 결과를 수집해 합성할 수 있다.
* Worker interface가 local/remote 어느 쪽으로도 구현될 수 있는 형태다.

---

## Phase 8. MCP Adapter — 예정

### 목표

MCP를 이용해 외부 Tool을 Agent Runtime에 연결한다.

프로토콜 자체는 공식 Go SDK(`modelcontextprotocol/go-sdk`)를 사용하고, Runtime의 Tool interface에 맞물리는
adapter만 직접 구현한다.

### 구현 범위

* MCP client / server (SDK 기반)
* multi-server MCP client (여러 MCP 서버 동시 연결, Tool 목록 통합, 이름 충돌 시 네임스페이스 처리)
* MCP tool discovery
* MCP tool call
* MCP tool adapter (자작)
* local tool과 remote tool 통합
* file explore/save MCP server

### 주요 패키지

```text
internal/protocol/mcp
internal/tool
```

### 학습 포인트

* MCP는 Agent Runtime의 core가 아니라 외부 Tool 연동 protocol이다.
* 프로토콜은 SDK로 처리하고, Runtime에 의미 있는 것은 Tool interface adapter다.
* 내부 Tool interface가 안정되어야 MCP adapter가 의미 있다.
* Agent는 local tool과 MCP tool을 동일한 방식으로 호출할 수 있어야 한다.
* 여러 MCP 서버를 동시에 쓸 때는 Tool 이름 네임스페이스와 서버별 장애 격리를 클라이언트가 책임진다.

### 완료 기준

* MCP Server가 Tool 목록을 제공한다.
* MCP Client가 Tool 목록을 조회한다.
* MCP Tool을 Runtime Tool로 등록할 수 있다.
* Agent가 MCP Tool을 호출할 수 있다.
* 한 클라이언트가 여러 MCP Server에 동시에 접속해 Tool 목록을 하나로 통합한다.

---

## Phase 9. A2A Adapter — 예정

### 목표

A2A를 이용해 Agent 간 상호운용 구조를 구현한다.

프로토콜 자체는 공식 Go SDK(`a2aproject/a2a-go`)를 사용하고, Runtime의 Worker interface에 맞물리는 adapter만
직접 구현한다.

### 구현 범위

* Agent Card
* Agent Executor
* A2A Client / Server (SDK 기반)
* remote agent descriptor
* remote worker agent adapter (자작)
* MCP Agent와 A2A Agent 조합

### 주요 패키지

```text
internal/protocol/a2a
internal/multiagent
```

### 학습 포인트

* MCP는 Tool 호출이고, A2A는 Agent 호출이다.
* Remote Agent도 Local WorkerAgent처럼 다룰 수 있어야 한다. (Phase 7의 Worker interface 재사용)
* A2A adapter는 Worker interface 뒤에 들어가므로, 호출하는 쪽은 local/remote 여부를 몰라도 된다.

### 완료 기준

* Agent가 자신의 capability를 Agent Card로 표현한다.
* A2A Server가 외부 요청을 받아 Agent를 실행한다.
* A2A Client가 Remote Agent를 호출한다.
* Remote Agent를 Phase 7의 `WorkerAgent`로 다룰 수 있다.

---

## Phase 10. Orchestrator Runtime — 예정

### 목표

Web Search, RAG, File Management를 포함한 범용 Multi-Agent를 하나의 `agent-runtime`으로 통합한다.
local Worker와 A2A 기반 remote Worker를 동일한 Orchestrator 아래에서 다룬다.

### 구현 범위 (하위 분할)

이 Phase는 Worker 구성·오케스트레이션·분산 호출 견고성이 선형 의존이라 feature-dir와 implement 사이클을 세
갈래로 나눈다. 소수점 번호는 10.1로 Worker들을 갖춘 뒤 10.2가 이를 묶고, 10.3에서 remote 호출 견고성과 응답
통합을 더하는 선행 의존을 뜻한다. 10.1·10.2는 대부분 앞 Phase 재사용·조립이고, 10.3은 분산 호출 실패 처리와
streaming 통합이라는 신규 관심사다.

#### Phase 10.1 — Worker Agent 구성

* Web Search Agent (Tavily 검색 API)
* Internal RAG Agent (Postgres + pgvector, Phase 5 재사용)
* File Management Agent (로컬 파일시스템 기반; Google Drive 등 외부 스토리지는 이후 어댑터로 확장)
* Writer Agent
* optional Reviewer Agent

#### Phase 10.2 — 오케스트레이션 (10.1 이후)

* Orchestrator Agent
* intent analysis
* plan generation
* A2A 기반 remote worker 호출
* local worker fallback
* result aggregation
* final response generation

#### Phase 10.3 — 견고성 & 응답 통합 (10.2 이후)

* remote worker 호출 실패 처리 (timeout / 재시도 / 부분 실패 집계)
* 다중 worker 응답의 streaming 통합 여부 결정 (Phase 4.3 streaming 계약 재사용 또는 final-only 집계)

### 주요 패키지

```text
cmd/agent-runtime
internal/orchestrator
internal/multiagent
internal/protocol/a2a
internal/tool
internal/rag
internal/memory
```

### 최종 흐름

```text
User Request
→ Orchestrator
→ Intent Analysis
→ Plan Generation
→ Worker Agent Calls
    → Web Search
    → Internal RAG
    → File Management
    → Writer
→ Result Aggregation
→ Final Response
```

### 학습 포인트

* 최종 시스템은 여러 Agent 기능을 조합하는 Orchestrator 구조다.
* Web Search, RAG, File Management는 각각 독립된 capability다.
* A2A를 통해 Agent를 remote worker처럼 연결할 수 있다.

### 완료 기준

* 사용자 요청을 intent로 분류한다.
* 필요한 작업을 plan으로 만든다.
* 적절한 worker agent를 호출한다.
* 결과를 통합해 최종 응답을 만든다.
* local 실행과 remote A2A 실행을 구분해서 사용할 수 있다.

---

## Phase 11. Runtime Refinement — 예정

### 목표

단계적으로 구현한 코드를 재사용 가능한 Runtime 구조로 정리한다. 새 기능이 아니라 수렴·정리 단계다.

### 구현 범위

* package boundary 정리
* interface 정리
* 불필요한 임시 코드 제거
* config 정리
* error handling 정리
* trace 구조 정리
* README 업데이트

### 주요 패키지

```text
전체
```

### 완료 기준

* 각 package가 단일 책임을 갖고, package 간 import 방향에 순환이 없다.
* trace 필드(step / action / agent / tool call / tool result / error / latency / model / token usage)가
  Phase 2부터 써 온 단일 trace 구조 안에서 일관된 형식으로 기록되고, 이름·중복이 최종 정리되어 있다.
* 미사용 임시 코드와 중복 interface가 제거되어 있다.
* README만 보고 프로젝트 목적을 이해할 수 있다.
* ROADMAP의 모든 Phase 상태가 실제 코드·commit과 일치한다.

---

# 진행 순서 요약

```text
00. Project Foundation                         [x]
01. LLM Client (Claude + Ollama)               [x]
02. Agent Loop                                  [x]
03. Tool Calling Runtime                        [x]
04. Single Agent Runtime                        [ ]
    04.1 Tool 묶음
    04.2 Agent 실행 구조
    04.3 Streaming Agent Response
    04.4 Tool Execution Backend
05. RAG Runtime                                 [ ]
    05.1 인덱싱 파이프라인
    05.2 검색·활용
06. Memory Runtime                              [ ]
    06.1 단기 메모리
    06.2 장기 메모리 & Tool
07. Multi-Agent Runtime                         [ ]
    07.1 Worker 인터페이스 & Routing
    07.2 Orchestrator-Workers
    07.3 구체 Worker & 합성
08. MCP Adapter                                 [ ]
09. A2A Adapter                                 [ ]
10. Orchestrator Runtime                        [ ]
    10.1 Worker Agent 구성
    10.2 오케스트레이션
    10.3 견고성 & 응답 통합
11. Runtime Refinement                          [ ]
```

# Phase별 구현 위치 요약

아래 표는 각 Phase의 대표 위치만 적는다. 실제로 손대는 패키지는 각 Phase 본문의 「주요 패키지」를 따른다.

| Phase    | 구현 위치 (대표)                                 |
| -------- | -------------------------------------------- |
| Phase 1  | `internal/llm`, `internal/message`           |
| Phase 2  | `internal/agent`                             |
| Phase 3  | `internal/tool`                              |
| Phase 4  | `internal/agent`, `internal/tool`, `internal/toolexec` |
| Phase 5  | `internal/rag`                               |
| Phase 6  | `internal/memory`                            |
| Phase 7  | `internal/multiagent`                        |
| Phase 8  | `internal/protocol/mcp`                      |
| Phase 9  | `internal/protocol/a2a`                      |
| Phase 10 | `internal/orchestrator`, `cmd/agent-runtime` |

# 중단 기준

각 Phase는 다음 조건을 만족하기 전까지 다음 Phase로 넘어가지 않는다.

* 실행 가능한 Runtime 상태가 있고, CLI에서 실제로 한 번 돌려 그 Phase까지 도입된 trace 범위로 실행 흐름을 확인했다.
* 해당 단계 개념이 코드에 반영되어 있다.
* README 또는 ROADMAP에 현재 상태가 반영되어 있다.
* 최소 하나 이상의 실패 케이스가 정리되어 있다.

외부 의존이 필요한 Phase는 위 "CLI 1회 실행" 확인에 해당 인프라·키 준비가 전제된다. LLM API key는 Phase 1,
Tavily API key는 Phase 4.1, Postgres + pgvector는 Phase 5.1에서 처음 필요하며, Phase 6.2는 Phase 5의
Postgres 인프라를 재사용한다.

# 확장 과제

다음 항목은 Runtime 완성 후 진행한다. 목록의 단일 출처는 `README.md`의 「제외 범위」이며, 여기서는 반복하지 않는다.

# 최종 완료 기준

이 로드맵이 끝났을 때 다음을 구현하고 설명할 수 있어야 한다.

```text
LLM 기반 Agent 의사결정 구조
Agent Loop (tool-use 반복)
Tool Calling Runtime
Single Agent Runtime
RAG Runtime
Multi-Agent Runtime (Routing / Orchestrator-Workers)
Memory Runtime
MCP Tool Adapter
A2A Agent Adapter
Orchestrator 기반 Multi-Agent System
```
