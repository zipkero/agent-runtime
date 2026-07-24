# agent-runtime

Go 기반 AI Agent Runtime 구현 프로젝트입니다.

LLM이 도구를 호출하고 여러 단계를 거쳐 판단하는 구조를, 필요한 구성 요소부터 하나씩 직접 구현하며 키워갑니다.

## 이 프로젝트가 향하는 곳

이 프로젝트의 목적은 LLM 기반 Agent를 구성하는 핵심 개념을 Go 기반 Runtime 구조로 흡수하는 것입니다.
단발성 LLM 호출에서 시작해, Agent loop와 Tool Calling, RAG, Memory를 거쳐, 여러 Agent가 협력하고 A2A로
서로를 호출하는 **Multi-Agent Runtime**까지 하나의 코드베이스로 성장시킵니다.

도착점은 다음 한 문장으로 요약됩니다 — **여러 Agent가 역할을 나눠 협력하고, 그 Agent들을 local 실행과
A2A 기반 remote 실행으로 동일하게 다루는 Orchestrator 기반 Multi-Agent System.**

예제 코드를 단계별로 분리해 보관하지 않고, 하나의 Go 코드베이스를 점진적으로 발전시키는 방식으로 진행합니다.

엔진 본체는 특정 도메인에 묶이지 않습니다. system prompt, tool 목록, worker 구성 같은 도메인 성격은
런타임을 수정하지 않고 진입점에서 주입하며, 본체는 어떤 구성이 주입되든 바뀌지 않습니다.

## 목표

이 프로젝트의 목표는 다음과 같습니다.

* LLM 기반 Agent의 의사결정 구조 이해
* Agent Loop (tool-use 반복 판단) 구현
* Tool Calling Runtime 구현
* inline/process-backed Tool Execution Backend 구현
* Single Agent 구현 (Web Search / File / Code Tool 포함)
* RAG Runtime 구현
* Memory Runtime (단기 / 장기) 구현
* Multi-Agent Runtime 구현
* Routing / Orchestrator-Workers 등 Multi-Agent 협력 패턴 구현
* MCP 기반 외부 Tool 연동 구조 구현
* A2A 기반 Agent 간 상호운용 구조 구현
* 최종적으로 local/remote Worker를 함께 다루는 Orchestrator 기반 Multi-Agent Runtime 구현
* 엔진 본체를 특정 도메인에 묶지 않고, prompt·tool·worker 구성을 진입점 주입으로만 표현

## 무엇을 직접 만들고, 무엇을 연결하는가

이 프로젝트는 직접 구현 대상과 외부 연결 대상을 명확히 구분합니다.

### 직접 구현 (Runtime 본체)

다음은 Runtime의 본체로, 라이브러리에 기대지 않고 직접 구현합니다.

* Agent Loop (LLM 호출 → tool 실행 → 반복 → 종료 판단)
* Tool Calling Runtime (registry / schema / 검증 / 실행 / timeout / process 수명 관리)
* Memory Runtime (단기 / 장기 / trimming / summary)
* Multi-Agent 협력 (Routing / Orchestrator-Workers)
* Orchestrator

### 외부 연결 (SDK / HTTP 사용)

아래 외부 세계와의 연결은 공식 SDK 또는 HTTP로 처리합니다.

* LLM 호출: 실제 Claude API + 로컬 모델(Ollama). GPT 등 다른 provider는 같은 interface 뒤에 추가한다 (provider별
  client는 interface 뒤에 둠)
* 임베딩 생성: 임베딩 API 또는 로컬 모델(Ollama 등)
* 웹 검색: 검색 API (Tavily)
* 벡터 저장 / 검색: Postgres + pgvector
* MCP: 공식 Go SDK + Runtime adapter 자작
* A2A: 공식 Go SDK + Runtime adapter 자작

핵심은 **두뇌·뼈대(Agent 로직·Tool·Memory·Multi-Agent·Orchestration)는 직접 만들고, 바깥 세계로
나가는 배선은 SDK/HTTP로 연결한다**는 것입니다.

## 진행 방식

코드는 하나의 Runtime으로 계속 발전시킵니다. 단계별 폴더를 따로 만들지 않고, 각 단계에서 만든 개념을 기존
Runtime 구조에 반영합니다.

```text
LLM 호출 계층
→ internal/llm, internal/message

Agent 실행 계층
→ internal/agent, internal/tool

RAG
→ internal/rag

Memory
→ internal/memory

Multi-Agent
→ internal/multiagent, internal/orchestrator

Protocol 확장
→ internal/protocol/mcp, internal/protocol/a2a

Final 통합
→ cmd/agent-runtime, internal/orchestrator
```

진행 단계와 완료 기준은 `ROADMAP.md`가 소유합니다.

## 프로젝트 구조

아래는 목표 구조입니다. Phase 3 기준으로 `cmd/agent-runtime`, `internal/config`, `internal/llm`,
`internal/message`, `internal/agent`, `internal/tool`, `.env.example`, `.gitignore`, `README.md`, `ROADMAP.md`가
존재하며, 나머지 Runtime 패키지는 이후 Phase에서 차례로 만들어집니다.

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
│   ├── tool/
│   ├── rag/                 (예정)
│   ├── memory/              (예정)
│   ├── multiagent/          (예정)
│   ├── orchestrator/        (예정)
│   └── protocol/            (예정)
│       ├── mcp/
│       └── a2a/
├── .env.example
├── .gitignore
├── go.mod
├── ROADMAP.md
└── README.md
```

## 디렉터리 역할

### `cmd/agent-runtime`

Runtime 실행 진입점입니다.

Phase 1에서는 단발 CLI 실행 경로를 제공합니다. positional argument가 있으면 이를 prompt로 사용하고, 인자가
없으면 stdin 전체를 prompt로 읽어 설정된 LLM provider를 한 번 호출합니다.

성공 시 provider 응답 text만 stdout에 출력하고, 입력·설정·provider 오류는 stderr와 non-zero exit로 처리합니다.
HTTP API 또는 Agent Server 형태로의 확장은 본 로드맵 범위 밖의 확장 과제로 다룹니다.

실행 예시는 다음과 같습니다.

```bash
go run ./cmd/agent-runtime "요약해줘"
echo "요약해줘" | go run ./cmd/agent-runtime
```

### `internal/config`

환경변수와 실행 설정 로딩 계층입니다.

주요 책임:

* `.env` 및 환경변수 로딩
* LLM provider / model / API key 설정
* 실행 timeout 등 제한 기본값
* 외부 연동(Tavily, Postgres 등) 설정 값 제공
* logger 설정 값(로그 레벨 등) 제공

필요한 환경변수와 최초 등장 Phase는 다음과 같다. 값 자체는 `.env.example`이 소유하고, 여기서는 목록만 둔다.

| 환경변수                            | 용도                          | 최초 필요 Phase |
| ---------------------------------- | ----------------------------- | -------------- |
| `LLM_PROVIDER` / `LLM_MODEL` / `LLM_HOST` | LLM provider·모델 선택, 호출  | Phase 1        |
| `LLM_API_KEY`                      | claude provider 호출 키        | Phase 1        |
| `LLM_TIMEOUT`                      | LLM 호출 timeout               | Phase 1        |
| `TAVILY_API_KEY`                   | 웹 검색 Tool                   | Phase 4.1      |
| `LOG_LEVEL`                        | CLI 기본 로그 레벨              | Phase 0        |
| Postgres DSN (Phase 5.1에서 추가)    | pgvector / 장기 메모리          | Phase 5.1      |

### `internal/llm`

LLM Provider 추상화 계층입니다.

실제 Claude API와 로컬 모델(Ollama)을 호출하며, Provider를 교체 가능하게 만드는 것이 목표입니다. GPT 등 다른
provider도 같은 interface 뒤에서 다룰 수 있습니다.

주요 책임:

* Chat request / response 정의
* Tool call response 처리
* Claude provider client 구현
* Ollama provider client 구현
* provider 선택과 필수 설정 검증
* provider 오류와 timeout 오류 구분
* HTTP 기반 실제 provider 호출
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

* 사용자 입력 기반 Agent run 실행
* 메시지 상태 누적
* Final answer 감지
* Tool call 대기 상태 처리
* max step 기반 종료
* LLM 오류 상태 보존
* 메모리 trace 기록

등록된 Tool registry가 있으면 assistant 응답의 tool call을 실행하고, tool result를 메시지에 누적한 뒤 다음 LLM
판단을 이어간다. registry가 없으면 기존처럼 `needs_action` 상태로 멈춘다. Phase 4.2에서는 model 호출 전후
middleware, structured output 검증, provider-neutral Single Agent Runner와 CLI 실행 경로까지 확장했다. Streaming
Runner는 Phase 4.3에서 확장한다.

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
* 기본 Tool (calculator / file read)
* 이후 Phase의 확장 Tool (file save / web search / code execution)

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
* 메모리 영속화 (MemoryStore, Postgres backend)

### `internal/multiagent`

여러 Agent가 협력하는 구조를 담당합니다.

이 패키지는 협력의 *패턴과 기본 구성요소*(Worker interface, routing, orchestrator-workers 패턴)를 제공합니다.
이를 최종 시스템으로 조립하는 상위 계층은 `internal/orchestrator`입니다.

주요 책임:

* Worker agent (transport-agnostic interface)
* Routing (요청을 분류해 적절한 worker로 디스패치)
* Orchestrator-workers (작업 분해 → worker 호출 → 결과 합성)
* Worker-as-tool adapter (Worker agent를 Tool로 감싸 호출)
* Planner-worker flow

### `internal/orchestrator`

최종 Multi-Agent Runtime의 오케스트레이션 계층입니다.

`internal/multiagent`가 제공하는 패턴을 사용해 최종 시스템을 조립합니다. 협력 패턴 자체를 새로 정의하지 않고,
intent 분석 → plan 생성 → worker 호출 → 응답 합성의 실제 실행 흐름을 책임집니다.

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

기능 우선순위는 구현 의존성을 따릅니다. 예를 들어 Tool Calling Runtime은 Single Agent보다 먼저 구현합니다
(Agent loop가 Tool 실행을 전제하기 때문).

MCP, A2A, Orchestrator 같은 후반부 기능을 먼저 구현하지 않습니다.

### 직접 구현과 외부 연결을 구분한다

Runtime 본체(Agent loop·Tool·Memory·Multi-Agent·Orchestration)는 라이브러리에 기대지 않고 직접 구현합니다.
바깥 세계로 나가는 연결(LLM·임베딩·웹 검색·벡터 저장·MCP·A2A)은 공식 SDK 또는 HTTP로 처리합니다. 메시지
역할(user / assistant / tool)이나 tool calling처럼 LLM API가 정해 주는 형식은 그 계약을 그대로 따릅니다.

### Runtime과 Provider를 분리한다

LLM Provider가 바뀌어도 Agent Runtime은 바뀌지 않아야 합니다.

```text
Runtime
→ LLMClient interface
→ Provider implementation (실제 API 호출)
```

### 엔진은 도메인에 묶이지 않는다

`internal/*` 엔진 코드에는 특정 도메인의 어휘나 가정이 들어가지 않습니다. 도메인 성격은 프롬프트·코퍼스·Tool·
Worker 구성으로만 표현합니다.

### 도메인 구성은 주입받는다

system prompt, tool 목록, worker 구성은 코드에 박지 않고 주입받습니다. 조립은 진입점(`cmd`)에서 합니다.

### Tool은 schema-first로 설계한다

Tool Calling은 LLM의 자연어 응답에 의존하지 않고 명확한 schema 기반으로 처리합니다.

### 테스트는 stub으로 격리한다

Runtime 본체 로직은 stub `LLMClient`와 stub 외부 클라이언트로 단위 테스트하고, 외부 인프라(실제 LLM
API·Tavily·Postgres) 의존 경로는 통합 테스트로 분리한다. 각 Phase 완료 기준의 "CLI 1회 실행"은 통합
확인이며, 본체 판단 로직은 stub 기반 단위 테스트로 검증한다.

### 제어 흐름은 일반 Go 코드로 표현한다

Agent의 반복 판단과 분기는 평범한 Go 제어 흐름으로 표현합니다.

```text
반복 판단
→ Go for 루프 (agent loop)

분기
→ Go if / switch + 라우팅 함수

상태
→ Go struct (AgentState)

worker 선택
→ Go 라우팅 함수
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

In-process Tool은 context 기반 cooperative cancellation을 따른다. Phase 4.4의 process-backed Tool은 timeout 시 cancel,
grace period, 필요할 때 kill, wait 순서로 실제 실행 종료를 확인한 뒤 다음 상태로 전이한다. Remote Tool은 로컬 요청
취소와 늦게 도착한 결과 폐기를 보장하지만 원격 서버의 실행 종료까지 강제하지 않는다.

### Trace를 남긴다

Agent는 실행 과정이 중요합니다.

최종 답변만으로는 디버깅이 어렵기 때문에 다음 정보를 기록합니다.

* step
* agent
* action
* tool call
* tool result
* error
* latency
* model
* token usage

이 필드들은 한 번에 갖춰지지 않고, 각 Phase가 자신이 도입한 정보를 더하며 자랍니다. step과 action은 Phase 2,
tool call / tool result는 Phase 3, agent는 Phase 7에서 더해집니다. error는 Phase 2의 error state에서 시작해
Phase 3의 tool error handling으로 확장되고, latency / model / token usage는 LLM·Runner 계층에서 채워지며,
trace 구조 자체는 Phase 2에서 한 번 세우고 이후엔 같은 구조에 필드만 더해, Phase 11은 필드 이름 통일·
중복 제거 같은 마무리만 합니다. 각 Phase의 실제 진행 상태는 `ROADMAP.md`가 소유합니다.

## 최종 산출물

이 프로젝트를 완료하면 다음 구조를 가진 Go 기반 Agent Runtime을 얻게 됩니다.

* LLM Client Abstraction
* Message Model
* Tool Calling Runtime
* Tool Execution Backend
* Agent Loop
* Single Agent Runtime
* RAG Runtime
* Memory Runtime
* Multi-Agent Runtime
* MCP Adapter
* A2A Adapter
* Orchestrator
* CLI Entry Point

## 제외 범위

이 프로젝트는 Agent Runtime을 직접 만드는 데 집중합니다.

아래 항목은 현재 범위에서 빼되, Runtime 완성 후 확장 과제로 다룰 수 있습니다. 이 목록의 단일 출처는 이 절이며,
`ROADMAP.md`의 「확장 과제」가 이를 참조합니다.

이 Runtime을 최종적으로 무엇으로 만들지(임베드용 라이브러리 / 단일 사용자 CLI 도구 / 다중 사용자 API 서비스
등)는 아직 정하지 않았다. 그 결정이 아래 「외부 서비스로 노출할 때만 필요」 항목의 실제 필요 범위를 정한다. 용도가
정해지면 이 절을 갱신한다.

### production이면 용도와 무관하게 필요

Runtime을 실제로 운영(외부 API 호출·비용 발생)하는 순간 필요해지는 항목이다. 어떤 용도든 production 전환 시
공통으로 검토한다.

* 비용·토큰 예산 제어 (상한 / 차단 / 쿼터)
* 외부 호출 전반의 재시도·백오프·서킷브레이커 (LLM / 임베딩 / 검색 / DB)
* 시크릿 관리 (`.env` 너머 Vault·KMS, 키 로테이션)
* 보안 sandbox·위협모델
  (Code Execution Tool의 filesystem·network·syscall·CPU·memory 격리, 프롬프트 인젝션 대응)
* DB 스키마 마이그레이션·백업/복구
* CI/CD 파이프라인

### 외부 서비스로 노출할 때만 필요

Runtime을 독립 서비스로 띄우거나 다중 사용자에게 제공할 때만 필요하다. 임베드용 라이브러리로 쓰면 호출하는
쪽이 책임진다.

* HTTP API / Agent Server 진입점
* 인증 / 인가 (복잡한 권한 시스템 포함)
* 요청 단위 rate limit·동시성 제어
* 관찰가능성 표준 연동 (OpenTelemetry / Datadog), 메트릭·알람·SLO
* SaaS 멀티 테넌시 (사용자별 메모리/RAG 데이터 격리)
* Kubernetes 배포·오토스케일, 운영형 Agent Gateway
* Web UI
* Agent Harness
