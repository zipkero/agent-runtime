# ROADMAP

이 문서는 `agent-runtime`을 Go 기반으로 단계적으로 발전시키기 위한 로드맵입니다.

이 프로젝트는 단계별 예제 폴더를 만들지 않습니다.

대신 하나의 Go Agent Runtime 코드베이스를 계속 개선하며, 각 단계의 개념을 Runtime 기능으로 흡수합니다.

도착점은 **여러 Agent가 역할을 나눠 협력하고, 그 Agent들을 local 실행과 A2A 기반 remote 실행으로 동일하게
다루는 Orchestrator 기반 Multi-Agent System**입니다. 앞 단계(LLM·ReAct·Tool·Graph·RAG·Memory)는 이
도착점을 떠받치는 구성 요소입니다.

## 진행 원칙

```text
구현 의존성 순서로 단계적으로 발전시킨다.
LangChain / LangGraph는 사용하지 않는다. (그 외 SDK는 사용한다)
단계별 예제 폴더는 만들지 않는다.
하나의 Runtime을 계속 발전시킨다.
진행 상태는 docs, commit, tag로 추적한다.
```

## 진행 현황

```text
[x] Phase 0  Project Foundation
[x] Phase 1  LLM Client          (Claude / GPT + Ollama 로컬 provider)
[x] Phase 2  Agent State / ReAct
[x] Phase 3  Tool Calling Runtime
[x] Phase 4  Graph Runtime
[x] Phase 5  Single Agent Runtime (5.1 Tool 묶음 / 5.2 실행 구조 / 5.3 Streaming)
[ ] Phase 6  RAG Runtime
[ ] Phase 7  Memory Runtime
[ ] Phase 8  Multi-Agent Runtime
[ ] Phase 9  MCP Adapter
[ ] Phase 10 A2A Adapter
[ ] Phase 11 Orchestrator Runtime
[ ] Phase 12 Runtime Refinement
```

---

## Phase 0. Project Foundation — 완료

### 목표

Go 기반 Agent Runtime 프로젝트의 기본 구조를 만든다.

### 구현 범위

* Go module 생성
* 기본 디렉터리 구조 구성
* config loader
* logger
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
* 실제 Claude / GPT API를 호출하는 client
* 로컬 모델 provider (Ollama, tool calling 포함)
* 테스트용 stub client
* model / api key config
* context timeout 처리

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
* LLM Provider 교체 가능성을 interface로 표현한다. (Claude / GPT / Ollama)
* request timeout이 동작한다.

---

## Phase 2. Agent State와 ReAct 기초 — 완료

### 목표

Agent가 단발성 LLM 호출이 아니라, 상태를 유지하며 반복적으로 판단하는 구조임을 코드로 표현한다.

### 구현 범위

* `AgentState`
* `Agent`
* ReAct loop
* step counter
* final answer detection
* reflection hook
* max steps
* error state

### 주요 패키지

```text
internal/agent
internal/message
```

### 학습 포인트

* Agent는 LLM 호출 한 번으로 끝나지 않는다.
* LLM 응답을 보고 다음 행동을 Runtime이 해석해야 한다.
* 무한 루프 방지를 위한 max step이 필요하다.

### 완료 기준

* 사용자 입력이 `AgentState`에 저장된다.
* LLM 응답이 state에 누적된다.
* step 기반 실행이 가능하다.
* max step 초과 시 안전하게 종료된다.

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

### 주요 패키지

```text
internal/tool
internal/agent
```

### 학습 포인트

* LLM은 Tool을 직접 실행하지 않는다.
* LLM은 Tool Call을 요청하고, Runtime이 실행한다.
* Tool schema가 명확해야 안정적인 Tool Calling이 가능하다.

### 완료 기준

* Tool 등록 가능
* 이름 기반 Tool lookup 가능
* Agent가 Tool Call을 실행 가능
* Tool 결과를 다시 LLM 입력에 포함 가능

---

## Phase 4. Go Graph Runtime — 완료

### 목표

State / Node / Edge / Conditional Edge 개념을 Go로 직접 구현한다.

### 구현 범위

* `GraphState`
* `Node` interface
* `Edge`
* `Router`
* `ConditionalRouter`
* `Reducer`
* `Graph`
* graph execution loop
* graph max steps
* node error handling

### 주요 패키지

```text
internal/graph
internal/agent
```

### 학습 포인트

* Graph의 핵심은 State, Node, Edge, Conditional Edge다.
* Agent 실행 흐름은 Graph로 표현할 수 있다.
* Tool Calling Agent는 `llm_node → tool_node → llm_node → end` 구조로 표현 가능하다.
* 정적 Edge만 연결하면 단계를 미리 지정한 workflow가 되고, Conditional Router + ReAct loop를 쓰면 동적
  자율 에이전트가 된다. 같은 Graph 엔진으로 두 구조를 모두 표현한다.

### 완료 기준

* State가 Node를 거치며 변경된다.
* 조건에 따라 다음 Node가 달라진다.
* Tool Call이 있으면 Tool Node로 이동한다.
* Tool Call이 없으면 종료된다.

---

## Phase 5. Single Agent Runtime — 완료

### 목표

Tool Calling이 가능한 Single Agent를 구현한다.

### 구현 범위 (하위 분할)

이 Phase는 이질적인 Tool과 Agent 실행 구조가 섞여 있어 feature-dir와 implement 사이클을 세 갈래로 나눴다.
소수점 번호는 5.1 → 5.2 → 5.3 순서의 선행 의존을 뜻한다.

#### Phase 5.1 — Tool 묶음

* Web Search Tool (Tavily 검색 API 연동)
* File Save Tool
* Code Execution Tool

#### Phase 5.2 — Agent 실행 구조 (5.1 이후)

* Middleware hook (pre / post model)
* Structured Output
* Single Agent runner
* Graph 기반 Single Agent 실행

#### Phase 5.3 — Streaming Agent Response (5.2 이후)

* Provider-neutral streaming LLM contract
* Runner streaming event
* CLI streaming 출력
* streaming 완료 후 final response 조립
* Structured Output final 검증과 streaming 관계 정리

### 주요 패키지

```text
internal/agent
internal/tool
internal/graph
```

### 학습 포인트

* Single Agent는 하나의 Agent가 직접 판단하고 Tool을 호출하는 구조다.
* Tool이 많아질수록 Tool schema와 routing 품질이 중요해진다.
* Code Execution Tool은 강력하지만 보안 위험이 크므로 제한이 필요하다.
* model 호출 전후를 가로채는 middleware 훅으로 횡단 관심사를 분리할 수 있다.

### 완료 기준

* Agent가 Web Search Tool을 호출할 수 있다.
* Agent가 File Tool을 호출할 수 있다.
* Agent가 제한된 Code Execution Tool을 호출할 수 있다.
* Structured Output을 파싱할 수 있다.
* Streaming mode에서 model text chunk를 순차적으로 확인할 수 있다.

---

## Phase 6. RAG Runtime — 예정

### 목표

내부 문서를 검색하고 답변에 활용하는 RAG 구조를 구현한다.

### 구현 범위 (하위 분할)

이 Phase는 인덱싱 파이프라인과 검색·활용이 선형 의존이라 feature-dir와 implement 사이클을 두 갈래로 나눈다.
소수점 번호는 6.1 → 6.2 순서로 진행하는 선행 의존을 뜻한다.

#### Phase 6.1 — 인덱싱 파이프라인

* document loader
* chunker
* embedding client (외부 임베딩 API 또는 로컬 모델 호출, interface로 추상화)
* vector store (Postgres + pgvector)

#### Phase 6.2 — 검색·활용 (6.1 이후)

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

## Phase 7. Memory Runtime — 예정

### 목표

Agent가 단기/장기 메모리를 사용할 수 있도록 구현한다.

### 구현 범위

* `MemoryStore` interface
* short-term memory
* session memory
* message trimming
* summary memory
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

## Phase 8. Multi-Agent Runtime — 예정

### 목표

여러 Agent가 역할을 나누어 협력하는 구조를 구현한다. 이 프로젝트의 핵심 Phase다.

### 구현 범위 (하위 분할)

이 Phase는 분량이 크고 패턴이 서로 독립적이라 feature-dir와 implement 사이클을 세 갈래로 나눈다.
소수점 번호는 정수 Phase 같은 직렬 의존이 아니라, 8.1을 토대로 8.2·8.3이 올라가는 동급 분할을 뜻한다.

#### Phase 8.1 — Worker / Supervisor 기반

* `WorkerAgent` interface (transport-agnostic)
* `Supervisor`
* `HandoffCommand`
* `WorkerAgent` → `Tool` adapter (agent-as-tool 슈퍼바이저용)
* Supervisor Pattern (handoff형 / agent-as-tool형)

#### Phase 8.2 — 협력 패턴 (8.1 위 동급)

* Network Pattern
* Hierarchical Pattern
* Planner-Worker Pattern

#### Phase 8.3 — 구체 Worker & 합성 (8.1 위 동급)

* Research Agent
* Writer Agent
* Result aggregation

### 주요 패키지

```text
internal/multiagent
internal/orchestrator
internal/agent
```

### 학습 포인트

* Multi-Agent는 Agent를 많이 만드는 것이 아니라 책임을 나누는 것이다.
* Supervisor는 task decomposition과 routing을 담당한다.
* Handoff는 다음 Agent에게 작업을 넘기는 명시적 구조다. LLM/노드가 다음 목적지(goto)를 런타임에 정하므로,
  Phase 4 Conditional Router의 특수형으로 볼 수 있다.
* Supervisor는 두 방식으로 구현된다 — handoff형(다음 Agent로 제어권을 넘김)과 agent-as-tool형(WorkerAgent를
  Tool로 감싸 호출하고 제어권을 유지). 후자는 `WorkerAgent` → `Tool` adapter로 표현한다.
* `WorkerAgent`는 처음부터 직렬화 가능한 입력/출력 + context 기반 시그니처로 설계해, 이후 remote(A2A) 워커로
  교체해도 Orchestrator가 바뀌지 않도록 한다.

### 완료 기준

* Supervisor가 사용자 요청을 task로 분해한다.
* 적절한 WorkerAgent를 선택한다.
* WorkerAgent 결과를 수집한다.
* 최종 응답을 합성한다.
* Worker interface가 local/remote 어느 쪽으로도 구현될 수 있는 형태다.

---

## Phase 9. MCP Adapter — 예정

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

## Phase 10. A2A Adapter — 예정

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
* Remote Agent도 Local WorkerAgent처럼 다룰 수 있어야 한다. (Phase 8의 Worker interface 재사용)
* A2A adapter는 Worker interface 뒤에 들어가므로, 호출하는 쪽은 local/remote 여부를 몰라도 된다.

### 완료 기준

* Agent가 자신의 capability를 Agent Card로 표현한다.
* A2A Server가 외부 요청을 받아 Agent를 실행한다.
* A2A Client가 Remote Agent를 호출한다.
* Remote Agent를 Phase 8의 `WorkerAgent`로 다룰 수 있다.

---

## Phase 11. Orchestrator Runtime — 예정

### 목표

Web Search, RAG, File Management를 포함한 범용 Multi-Agent를 하나의 `agent-runtime`으로 통합한다.
local Worker와 A2A 기반 remote Worker를 동일한 Orchestrator 아래에서 다룬다.

### 구현 범위 (하위 분할)

이 Phase는 Worker 구성과 오케스트레이션이 선형 의존이라 feature-dir와 implement 사이클을 두 갈래로 나눈다.
소수점 번호는 11.1로 Worker들을 갖춘 뒤 11.2가 이를 묶는 선행 의존을 뜻한다. 대부분 앞 Phase 재사용·조립이다.

#### Phase 11.1 — Worker Agent 구성

* Web Search Agent (Tavily 검색 API)
* Internal RAG Agent (Postgres + pgvector, Phase 6 재사용)
* File Management Agent (로컬 파일시스템 기반; Google Drive 등 외부 스토리지는 이후 어댑터로 확장)
* Writer Agent
* optional Reviewer Agent

#### Phase 11.2 — 오케스트레이션 (11.1 이후)

* Orchestrator Agent
* intent analysis
* plan generation
* A2A 기반 remote worker 호출
* local worker fallback
* result aggregation
* final response generation

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

## Phase 12. Runtime Refinement — 예정

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

* Runtime 구조가 자연스럽게 정리된다.
* 각 package의 책임이 명확하다.
* README만 보고 프로젝트 목적을 이해할 수 있다.
* ROADMAP과 현재 코드 상태가 일치한다.

---

# 진행 순서 요약

```text
00. Project Foundation                         [x]
01. LLM Client (Claude / GPT + Ollama)         [x]
02. Agent State / ReAct                         [x]
03. Tool Calling Runtime                        [x]
04. Graph Runtime                               [x]
05. Single Agent Runtime                        [x]
    05.1 Tool 묶음
    05.2 Agent 실행 구조
    05.3 Streaming Agent Response
06. RAG Runtime                                 [ ]
    06.1 인덱싱 파이프라인
    06.2 검색·활용
07. Memory Runtime                              [ ]
08. Multi-Agent Runtime                         [ ]
    08.1 Worker / Supervisor 기반
    08.2 협력 패턴
    08.3 구체 Worker & 합성
09. MCP Adapter                                 [ ]
10. A2A Adapter                                 [ ]
11. Orchestrator Runtime                        [ ]
    11.1 Worker Agent 구성
    11.2 오케스트레이션
12. Runtime Refinement                          [ ]
```

# Phase별 구현 위치 요약

| Phase    | 구현 위치                                        |
| -------- | -------------------------------------------- |
| Phase 1  | `internal/llm`, `internal/message`           |
| Phase 2  | `internal/agent`                             |
| Phase 3  | `internal/tool`                              |
| Phase 4  | `internal/graph`                             |
| Phase 5  | `internal/agent`, `internal/tool`            |
| Phase 6  | `internal/rag`                               |
| Phase 7  | `internal/memory`                            |
| Phase 8  | `internal/multiagent`                        |
| Phase 9  | `internal/protocol/mcp`                      |
| Phase 10 | `internal/protocol/a2a`                      |
| Phase 11 | `internal/orchestrator`, `cmd/agent-runtime` |

# 중단 기준

각 Phase는 다음 조건을 만족하기 전까지 다음 Phase로 넘어가지 않는다.

* 실행 가능한 Runtime 상태가 있고, CLI에서 실제로 한 번 돌려 trace로 실행 흐름을 확인했다.
* 해당 단계 개념이 코드에 반영되어 있다.
* README 또는 ROADMAP에 현재 상태가 반영되어 있다.
* 최소 하나 이상의 실패 케이스가 정리되어 있다.

# 확장 과제

다음 항목은 Runtime 완성 후 진행한다.

```text
Agent Harness
OpenTelemetry
Datadog 연동
Kubernetes 배포
Web UI
권한 시스템
멀티 테넌시
운영형 Agent Gateway
```

# 최종 완료 기준

이 로드맵이 끝났을 때 다음을 구현하고 설명할 수 있어야 한다.

```text
LLM 기반 Agent 의사결정 구조
ReAct Agent Loop
Tool Calling Runtime
Graph State / Node / Edge / Conditional Edge
Single Agent Runtime
RAG Agent
Multi-Agent Runtime
Memory Runtime
MCP Tool Adapter
A2A Agent Adapter
Orchestrator 기반 Multi-Agent System
```
