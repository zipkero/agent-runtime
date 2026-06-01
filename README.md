# agent-runtime

Go 기반 AI Agent Runtime 구현 프로젝트입니다.

이 프로젝트는 LangChain, LangGraph를 사용하지 않고 LLM 기반 Agent Runtime을 직접 구현합니다.

예제 코드를 단계별로 분리해 보관하지 않고, 하나의 Go 코드베이스를 점진적으로 발전시키는 방식으로 진행합니다.

즉 이 프로젝트의 목적은 LLM 기반 Agent를 구성하는 핵심 개념을 Go 기반 Runtime 구조로 흡수해, 하나의
Agent Runtime으로 성장시키는 것입니다.

## 목표

이 프로젝트의 목표는 다음과 같습니다.

* LLM 기반 Agent의 의사결정 구조 이해
* ReAct 기반 Agent Loop 구현
* Tool Calling Runtime 구현
* State / Node / Edge / Conditional Edge 구조를 라이브러리 없이 직접 구현
* Single Agent 구현
* Web Search / File / Code / RAG Tool 구현
* Multi-Agent Runtime 구현
* Supervisor / Network / Hierarchical Agent Pattern 구현
* Short-term Memory / Long-term Memory 구현
* MCP 기반 외부 Tool 연동 구조 구현
* A2A 기반 Agent 간 상호운용 구조 구현
* 최종적으로 Web Search, RAG, File Management, Orchestrator를 포함한 Multi-Agent Runtime 구현

## 무엇을 직접 만들고, 무엇을 연결하는가

이 프로젝트는 직접 구현 대상과 외부 연결 대상을 명확히 구분합니다.

### 직접 구현 (Runtime 본체)

LangChain / LangGraph가 제공하던 기능은 사용하지 않고 직접 구현합니다. 이 부분이 곧 Runtime의 본체입니다.

* Graph 엔진 (State / Node / Edge / Conditional Edge / Reducer / Runner)
* ReAct Agent Loop / Reflection
* Tool Calling Runtime (registry / schema / 검증 / 실행 / timeout)
* Memory Runtime (단기 / 장기 / trimming / summary)
* Multi-Agent 패턴 (Supervisor / Network / Hierarchical / Handoff)
* Orchestrator

### 외부 연결 (SDK / HTTP 사용)

금지 대상은 LangChain / LangGraph 계열뿐입니다. 아래 외부 세계와의 연결은 공식 SDK 또는 HTTP로 처리합니다.

* LLM 호출: 실제 Claude / GPT API (provider별 client는 interface 뒤에 둠, 초기에는 Claude 우선)
* 임베딩 생성: 임베딩 API 또는 로컬 모델(Ollama 등)
* 웹 검색: 검색 API
* 벡터 저장 / 검색: Postgres + pgvector
* MCP: 공식 Go SDK + Runtime adapter 자작
* A2A: 공식 Go SDK + Runtime adapter 자작

핵심은 **두뇌·뼈대(Agent 로직·Graph·Tool·Memory·Orchestration)는 직접 만들고, 바깥 세계로 나가는 배선은
SDK/HTTP로 연결한다**는 것입니다.

## 진행 방식

코드는 하나의 Runtime으로 계속 발전시킵니다. 단계별 폴더를 따로 만들지 않고, 각 단계에서 만든 개념을 기존
Runtime 구조에 반영합니다.

```text
LLM 호출 계층
→ internal/llm, internal/message

Graph 실행 계층
→ internal/graph

Agent 실행 계층
→ internal/agent, internal/tool

RAG
→ internal/rag

Multi-Agent
→ internal/multiagent, internal/orchestrator

Memory
→ internal/memory

Protocol 확장
→ internal/protocol/mcp, internal/protocol/a2a

Final 통합
→ cmd/agent-runtime, internal/orchestrator
```

진행 단계와 완료 기준은 `ROADMAP.md`가 소유합니다.

## 프로젝트 구조

```text
agent-runtime/
├── cmd/
│   └── agent-runtime/
│       └── main.go
├── internal/
│   ├── config/
│   ├── llm/
│   ├── message/
│   ├── agent/
│   ├── graph/
│   ├── tool/
│   ├── rag/
│   ├── memory/
│   ├── multiagent/
│   ├── orchestrator/
│   └── protocol/
│       ├── mcp/
│       └── a2a/
├── go.mod
├── ROADMAP.md
└── README.md
```

## 디렉터리 역할

### `cmd/agent-runtime`

Runtime 실행 진입점입니다.

초기에는 CLI 중심으로 시작하고, 이후 HTTP API 또는 Agent Server 형태로 확장합니다.

### `internal/llm`

LLM Provider 추상화 계층입니다.

실제 Claude / GPT API를 호출하되, Provider를 교체 가능하게 만드는 것이 목표입니다. 초기에는 한 Provider로
시작하고 interface 뒤에서 다른 Provider를 추가합니다.

주요 책임:

* Chat request / response 정의
* Message 구조 정의
* Tool call response 처리
* Structured output 처리
* Provider별 client 구현 (실제 API 호출)
* 테스트용 stub client (실행 경로는 실제 API, 테스트만 stub 교체)

### `internal/message`

Agent Runtime 전체에서 사용하는 메시지 타입을 정의합니다.

주요 책임:

* User message
* Assistant message
* Tool message
* System message
* Tool call
* Tool result

### `internal/agent`

Single Agent 실행 구조를 담당합니다.

주요 책임:

* ReAct loop
* Reflection step
* Final answer detection
* Agent state transition
* Tool call decision 처리

### `internal/graph`

State / Node / Edge / Conditional Edge 개념을 Go로 직접 구현합니다.

주요 책임:

* State
* Node
* Edge
* Router
* Conditional Edge
* Reducer
* Graph Runner

### `internal/tool`

Tool Calling Runtime을 담당합니다.

주요 책임:

* Tool interface
* Tool registry
* Tool schema
* Tool input validation
* Tool execution
* Tool timeout
* Tool result normalization

### `internal/rag`

RAG에 필요한 문서 검색 구조를 담당합니다.

벡터 저장과 검색은 Postgres + pgvector로 처리하고, 임베딩 생성은 외부(임베딩 API 또는 로컬 모델)에 위임합니다.

주요 책임:

* Document loader
* Chunker
* Embedding client (외부 호출, interface로 추상화)
* Vector store (Postgres + pgvector)
* Retriever
* Retrieval tool

### `internal/memory`

Agent Memory Runtime을 담당합니다.

주요 책임:

* Short-term memory
* Long-term memory
* Message trimming
* Summary memory
* User memory
* Category-based memory search
* State 영속화 (Postgres backend)

### `internal/multiagent`

여러 Agent가 협력하는 구조를 담당합니다.

주요 책임:

* Worker agent (transport-agnostic interface)
* Supervisor pattern (handoff / agent-as-tool)
* Network pattern
* Hierarchical pattern
* Handoff command
* Agent-as-tool adapter (Worker agent를 Tool로 호출)
* Planner-worker flow

### `internal/orchestrator`

최종 Multi-Agent Runtime의 오케스트레이션 계층입니다.

주요 책임:

* Intent 분석
* Plan 생성
* Agent routing
* Remote agent 호출
* 결과 통합
* 최종 응답 생성

### `internal/protocol/mcp`

MCP 기반 외부 Tool 연동을 담당합니다.

프로토콜 자체는 공식 Go SDK(`modelcontextprotocol/go-sdk`)를 사용하고, Runtime의 Tool interface에 맞물리는
adapter만 직접 구현합니다.

주요 책임:

* MCP client / server (SDK 기반)
* multi-server client (여러 MCP 서버 동시 연결, Tool 목록 통합)
* MCP tool adapter (자작)
* Tool discovery
* Remote tool call
* local tool과 remote tool 통합

### `internal/protocol/a2a`

A2A 기반 Agent 상호운용을 담당합니다.

프로토콜 자체는 공식 Go SDK(`a2aproject/a2a-go`)를 사용하고, Runtime의 Worker interface에 맞물리는 adapter만
직접 구현합니다.

주요 책임:

* Agent card
* Agent executor
* A2A client / server (SDK 기반)
* Remote worker agent adapter (자작)

## 개발 원칙

### 하나의 Runtime으로 발전시킨다

단계별 예제 코드를 복사해서 보관하지 않습니다.

각 단계에서 학습한 개념은 기존 Runtime 구조에 반영합니다.

### 의존성 순서로 단계적으로 진행한다

기능 우선순위는 구현 의존성을 따릅니다. 예를 들어 Tool Calling Runtime은 Graph보다 먼저 구현합니다
(ReAct loop와 Graph의 tool node가 Tool을 필요로 하기 때문).

MCP, A2A, Orchestrator 같은 후반부 기능을 먼저 구현하지 않습니다.

### LangChain / LangGraph만 금지한다

금지 대상은 LangChain / LangGraph 계열 라이브러리뿐입니다. LLM SDK, MCP / A2A SDK, Postgres / pgvector
드라이버 등은 사용합니다. 즉 Runtime 본체는 직접 만들고, 외부 연결은 SDK/HTTP로 처리합니다.

### Runtime과 Provider를 분리한다

LLM Provider가 바뀌어도 Agent Runtime은 바뀌지 않아야 합니다.

```text
Runtime
→ LLMClient interface
→ Provider implementation (실제 API 호출)
```

### Tool은 schema-first로 설계한다

Tool Calling은 LLM의 자연어 응답에 의존하지 않고 명확한 schema 기반으로 처리합니다.

### Graph 개념을 직접 구현한다

State / Node / Edge / Conditional Edge 개념을 라이브러리 없이 Go로 구현합니다.

```text
State
→ Go AgentState

Node
→ Go Node interface

Edge
→ Go Edge (정적 연결)

Conditional Edge
→ Go Router (분기 함수)

Reducer
→ Go State Reducer
```

### Worker는 transport-agnostic하게 설계한다

Multi-Agent의 Worker interface는 local 실행과 remote(A2A) 실행을 동일하게 다룰 수 있어야 합니다. 처음부터
직렬화 가능한 입력/출력과 context 기반 취소·timeout만 시그니처에 두어, local → remote 전환이 구현체 교체로
끝나도록 설계합니다.

### 모든 실행은 제한한다

Agent Runtime에는 반드시 제한이 필요합니다.

* max steps
* max tool calls
* max duration
* context cancellation
* timeout
* output size limit
* allowed tools

### Trace를 남긴다

Agent는 실행 과정이 중요합니다.

최종 답변만으로는 디버깅이 어렵기 때문에 다음 정보를 기록합니다.

* step
* node
* action
* tool call
* tool result
* error
* latency
* model
* token usage

## 최종 산출물

이 프로젝트를 완료하면 다음 구조를 가진 Go 기반 Agent Runtime을 얻게 됩니다.

* LLM Client Abstraction
* Message Model
* Tool Calling Runtime
* ReAct Agent
* Graph Runtime
* RAG Runtime
* Memory Runtime
* Multi-Agent Runtime
* MCP Adapter
* A2A Adapter
* Orchestrator
* CLI 또는 Server Entry Point

## 제외 범위

초기 진행 중에는 다음을 핵심 목표로 삼지 않습니다.

* Web UI
* SaaS 멀티 테넌시
* Kubernetes 배포
* OpenTelemetry / Datadog 연동
* 복잡한 권한 시스템
* Agent Marketplace
* 완전한 Framework 제품화

이 항목들은 Runtime 완성 후 확장 과제로 다룹니다.
