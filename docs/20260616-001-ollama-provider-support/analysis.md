# ollama-provider-support — ANALYSIS

## 승인 전 확인
- Ollama 연동을 SDK가 아니라 직접 HTTP `/api/chat`로 구현하고, Claude는 기존 Anthropic SDK 경로를
  유지한다(provider별 구현 방식 비대칭). LLMClient 계약 뒤에 가려지므로 수용 가능한가? 관련 본문: §5
- 범용 `LLM_HOST`를 Ollama 데몬 host와 Claude base URL override 양쪽 의미로 쓴다. 단일 키로 두는
  설계가 맞는가? 관련 본문: §5

## 근거

읽은 spec.md 범위: §1 범위, §2 목표, §3 제약, §4 제외 범위, §5 완료 조건 1–5 전체. 본 분석은
spec.md §1이 정한 범위(실행 시점 provider 선택, 범용 조건부 검증, Ollama HTTP chat + tool calling,
Claude 경로 유지·연결) 안에서만 설계한다. 직전 "Ollama 전용" 설계를 범용 provider 선택으로 다시
확장한 판본이다.

코드베이스에서 확인한 사실(현재 상태, task-001 적용 후):
- `internal/llm/llm.go`: `LLMClient`는 `Chat(ctx, ChatRequest) (ChatResponse, error)` 단일 메서드.
  `ChatRequest`(`Model`, `Messages []message.Message`, `Tools []message.ToolSpec`)·`ChatResponse`
  (`Message`)는 provider-neutral이며 provider 식별자를 포함하지 않는다. 이 계약은 불변이다.
- `internal/agent/agent.go`: agent loop는 `llm.LLMClient`와 `message` 타입에만 의존한다. `llmNode`가
  `req.Tools`로 tool spec을 싣고 `toolNode`가 tool_call 블록을 dispatch해 `RoleTool` 메시지에
  tool_result를 담아 되돌린다. tool 왕복의 internal 모델 형태는 고정이며 provider client가 wire와
  양방향 변환만 하면 된다.
- `internal/llm/claude.go`: `ClaudeClient`(Anthropic SDK)는 이미 chat + tool calling을 구현한다.
  현재 `cfg.AnthropicAPIKey`(claude.go:44)·`config.EnvAnthropicAPIKey`(claude.go:46)·`cfg.Model`을
  읽고, baseURL·httpClient 주입용 내부 생성자 `newClaudeClient(cfg, opts)`를 가진다. 범용 config
  필드 도입 시 이 참조들이 갱신 대상이다(빌드 정합성).
- `internal/config/config.go`(task-001 적용 후): 현재 `EnvAnthropicAPIKey`·`EnvOllamaHost`·
  `EnvOllamaModel`·`DefaultOllamaHost` 상수와 `Config{AnthropicAPIKey, OllamaHost, Model, Timeout,
  TavilyAPIKey}`를 가진다. `Load()`는 Ollama host(기본값)·model(필수)을 검증한다. 범용 설계로
  전환하면 이 env 상수·Config 필드·Load() 검증이 provider 선택 + 범용 필드로 재구성된다.
- `internal/config/config_test.go`(task-001 적용 후): 현재 Ollama env 기반으로 갱신돼 있다. 범용
  전환으로 env 키·검증 규칙이 다시 바뀌므로 함께 갱신 대상이다.
- `cmd/agent-runtime/main.go`: `config.Load()` → `llm.NewClaudeClient(cfg)` 하드코딩(main.go:36)
  → `run(ctx, client, cfg.Model, ...)`. client 생성 지점이 provider 선택으로 바뀐다. `buildRegistry`·
  `run`·`readPrompt`는 불변.
- `cmd/agent-runtime/main_test.go`: `run()`에 stub을 주입하므로 client 생성 방식 변경과 무관하게
  통과한다.
- `internal/message/message.go`: `ToolCall{ID, Name, Input}`, `ToolResult{ToolCallID, Content,
  IsError}`, `ToolSpec{Name, Description, InputSchema}`. tool_call↔tool_result 매칭은 `ID`↔
  `ToolCallID`.
- `go.mod`: 직접 의존은 `anthropic-sdk-go`, `godotenv`. 직접 HTTP 방식이므로 Ollama 의존성 추가 없음.

Ollama `/api/chat` HTTP 사실(Ollama API 문서·Go 타입 기준):
- `POST {host}/api/chat`, JSON body `{model, messages, tools, stream}`. `stream:false`면 단일 JSON
  응답.
- 메시지 role: `system`/`user`/`assistant`/`tool`. assistant가 `tool_calls`를 가질 수 있고, tool
  결과 메시지는 `{role:"tool", content, tool_name}`(버전에 따라 `tool_call_id`)로 싣는다.
- `tools`: `[{type:"function", function:{name, description, parameters}}]`, parameters는 JSON Schema.
- 응답 `message.tool_calls`: `[{function:{name, arguments}}]`이며 호출 식별 `id`가 없을 수 있다(§5 D4).

## 1. 구조

새 레이어를 도입하지 않고 세 경계 안에서 끝낸다.

- LLM provider 경계(`internal/llm`): `LLMClient`·`ChatRequest`·`ChatResponse`는 불변. 새 구현체
  `OllamaClient`(net/http로 `/api/chat` 직접 호출)를 `ClaudeClient`(Anthropic SDK, 기존)와 같은
  패키지에 둔다. provider 식별값으로 둘 중 하나를 생성하는 factory(`NewClient(cfg)`)를 이 경계가
  소유한다. 기존 `ClaudeClient`는 범용 config 필드를 읽도록 참조만 갱신한다.
- 설정 경계(`internal/config`): provider 식별 타입(claude/ollama 문자열 상수)과 범용 접속·자격
  설정(model, host, api key)을 표현하도록 `Config`를 재구성하고, `Load()`가 provider를 파싱(미지정
  → ollama, 미인식 → error)한 뒤 선택된 provider에 필요한 값만 검증한다(SPEC §5.4·§5.5).
- 조립 경계(`cmd/agent-runtime`): `main`의 client 생성 한 줄을 factory 호출로 바꾼다. agent loop·
  `buildRegistry`·`run`은 불변.

## 2. 데이터 흐름

### 부팅·provider 선택 흐름
1. `config.Load()`가 환경변수를 읽어 provider를 확정한다(미지정 → ollama, SPEC §3). 인식 불가 값이면
   error를 반환하고 main이 stderr 출력 + 비정상 종료한다(SPEC §5.5).
2. `Load()`가 선택된 provider에 필요한 범용 필드만 검증한다(SPEC §5.4): ollama면 model 필수·host
   기본값(`http://localhost:11434`), claude면 api key·model 필수(host는 선택적 base URL override).
   미선택 provider의 값 부재는 통과한다.
3. `main`이 `llm.NewClient(cfg)` factory로 provider에 맞는 구현체를 받는다. 이후 `buildRegistry`·
   `run`은 provider와 무관하게 흐른다.

### chat·tool calling 왕복 흐름(provider=ollama)
`llmNode`가 `ChatRequest`(messages + tool specs)를 `OllamaClient.Chat`에 넘기면:
- 요청 변환(internal → `/api/chat` body):
  - `RoleSystem` text → `{role:"system", content}`.
  - `RoleUser`/`RoleAssistant` text → `{role, content}`. assistant의 tool_call 블록 → 같은 메시지
    `tool_calls[]`(`{id, function:{name, arguments}}`), internal `Input`을 `arguments`로 싣는다.
  - `RoleTool`의 각 tool_result 블록 → 개별 `{role:"tool", content, tool_name, tool_call_id}`로
    1:N 변환.
  - `req.Tools` → `tools[]`(`type:"function"`, function.name/description, parameters는 InputSchema
    의 type/properties/required 사상).
  - body에 `stream:false`, model(req.Model 우선, 없으면 client 기본)을 싣는다.
- HTTP 호출: `POST {host}/api/chat`을 ctx와 함께. ctx 취소·연결 실패·비정상 status는 ctx.Err()를
  먼저 존중해 error로 표면화. 응답 JSON 디코드.
- 응답 변환: `message.content` → text 블록, `message.tool_calls[]` → tool_call 블록.
이후 흐름은 기존과 동일하다(tool_call 있으면 toolNode, 없으면 StatusFinal → stdout, SPEC §5.1·§5.3).

### chat 흐름(provider=claude)
`ClaudeClient.Chat`이 기존대로 Anthropic Messages API로 변환·호출해 응답을 internal 메시지로
되돌린다(SPEC §5.2). 범용 config 전환에서 바뀌는 것은 자격·base URL을 읽는 필드명뿐이고 변환 로직은
불변이다.

### tool_call ID 매칭 흐름(Ollama, 비자명 지점)
internal 모델은 `ToolCall.ID`↔`ToolResult.ToolCallID`로 매칭한다. Ollama 응답 tool_call `id`가 비면
OllamaClient가 응답 내 등장 순번으로 결정적 ID를 채워 internal `ToolCall.ID`에 싣고, 요청 변환 때
그 ID를 tool_call `id`/tool 메시지 `tool_call_id`로 되싣어 왕복 동안 유지한다(§5 D4).

## 3. 인터페이스

- `llm.LLMClient` (불변): `Chat(ctx, ChatRequest) (ChatResponse, error)`. `ChatRequest`·
  `ChatResponse` 형태 불변. OllamaClient·ClaudeClient 모두 구현한다.
- `llm.NewClient(cfg config.Config) (LLMClient, error)`: provider 식별값으로 구현체를 고르는
  factory. main이 client를 얻는 진입점이다. provider는 config가 이미 검증했으므로 factory는 검증된
  provider만 받는다.
- `llm.NewOllamaClient(cfg) (*OllamaClient, error)`: 공개 생성자. host·model 부재를 error로 반환.
  테스트 주입용 내부 생성자(base host·http.Client 주입)는 내부 helper다.
- `llm.NewClaudeClient(cfg) (*ClaudeClient, error)`(기존): 범용 config 필드(api key·model, 선택적
  base URL)를 읽도록 참조만 갱신한다. 시그니처·역할 불변.
- `config.Config`(재구성): main·client와의 계약. provider 식별값과 범용 접속·자격 필드(model, host,
  api key)를 포함한다. `Timeout`·`TavilyAPIKey`는 불변.

내부 변환 helper(internal↔`/api/chat` wire, JSON schema→tool)는 `internal/llm` 안에서만 쓰인다.

## 4. 영향 범위

- `internal/llm/`(신규 `OllamaClient` + factory): net/http 기반 OllamaClient와 변환 helper·테스트,
  `NewClient` factory 추가. `claude.go`는 범용 config 필드 참조로 갱신(변환 로직 불변).
  `llm.go`·`stub.go`는 불변.
- `internal/config/config.go`: provider 식별 타입·상수, 범용 env 상수, `Config` 필드 재구성,
  `Load()`를 provider 파싱 + 조건부 검증으로 변경. 직전 `OLLAMA_*`/`AnthropicAPIKey` 구성을 대체.
- `internal/config/config_test.go`: 범용 env·provider 선택·조건부 검증 규칙에 맞게 갱신.
- `cmd/agent-runtime/main.go`: client 생성 한 줄을 `NewClient(cfg)` factory 호출로 교체.
  `buildRegistry`·`readPrompt`·`run` 불변. `main_test.go`는 stub 주입이라 영향 없음.
- `.env.example`: `LLM_PROVIDER`·범용 `LLM_MODEL`·`LLM_HOST`·`LLM_API_KEY` 안내로 갱신.
- `go.mod`/`go.sum`: 변경 없음(직접 HTTP, 신규 의존성 없음).

하위 호환·동작 변경:
- env 키가 provider 접두사(`ANTHROPIC_*`/`OLLAMA_*`)에서 범용(`LLM_*`)으로 바뀌므로, 기존 `.env`는
  새 키로 갱신해야 한다. 저장 데이터·외부 contract 변경은 없다.
- agent·tool·graph 패키지: 해당 없음.

## 5. Decision Points

### D1. provider 선택 메커니즘과 기본값
- 옵션: env `LLM_PROVIDER` / CLI flag / 설정 파일. 기본값 ollama vs claude.
- 채택: env `LLM_PROVIDER`, 미지정 시 ollama(SPEC §3), 인식 불가 값은 error(SPEC §5.5). 근거:
  기존 설정이 모두 env 기반이고 `.env`/godotenv 경로에 자연히 얹힌다.

### D2. 범용 config 필드 vs provider 접두사 필드
- 옵션: (a) provider 접두사 env(`ANTHROPIC_*`/`OLLAMA_*`)와 분리 필드. (b) 범용 env(`LLM_MODEL`/
  `LLM_HOST`/`LLM_API_KEY`)와 단일 필드 집합 + provider별 조건부 검증.
- 트레이드오프: (a)는 provider별 의미가 키에 드러나지만 provider 추가·전환마다 키·필드가 늘고,
  단일 활성 provider인데 여러 provider 키가 공존한다. (b)는 키가 단순하고 전환이 env 값 하나로
  끝나지만, 같은 키가 provider마다 다른 의미를 가질 수 있다(host).
- 채택: (b) 범용 필드 + 조건부 검증. host/api key를 XOR로 강제하지 않고 각 provider가 필요한
  필드만 검증한다. 근거: 사용자 결정이며 "env 값 하나로 provider 전환" 목표에 맞다. host의 provider별
  의미 차이는 §승인 전 확인으로 노출한다.

### D3. Ollama 연동 방식 — 직접 HTTP `/api/chat`
- 옵션: (a) 공식 SDK `ollama/api`, (b) 직접 HTTP, (c) OpenAI 호환 endpoint.
- 트레이드오프: SDK는 본체 monorepo 패키지라 go.mod가 그 모듈에 묶인다. 직접 HTTP는 wire·tool
  스키마를 직접 관리하지만 의존성이 없고 API가 단순하다.
- 채택: (b) 직접 HTTP. 근거: 사용자 결정. Claude는 기존 SDK 경로 유지(비대칭은 LLMClient 뒤에 가림).

### D4. tool_call ID 매칭 보장(Ollama id 부재 대비)
- 사실: Ollama `/api/chat` 응답 tool_calls에 `id`가 없을 수 있다. internal 모델은 ID 매칭이다.
- 옵션: (a) Ollama id를 쓰되 비면 응답 내 순번 기반 결정적 ID 생성. (b) 항상 client 재발급.
  (c) 순서 의존.
- 채택: (a). 비면 등장 순번으로 결정적 ID(`call_<n>`)를 채워 internal ID에 싣고, 요청 변환 때
  되돌려 왕복 동안 유지한다. 근거: 같은 client가 발급·소비를 통제해 매칭이 보장되고, Ollama가 id를
  주면 그 값을 존중한다.

### D5. provider→client 사상 위치 — llm factory
- 옵션: (a) `internal/llm`에 `NewClient(cfg)` factory, main은 호출만. (b) main 내부 switch.
- 채택: (a). 근거: provider 식별→구현체 선택은 llm 경계 책임이고, main은 "config→client→run" 조립만
  담당한다. provider 추가 시 한 곳만 손댄다. 인식 불가 provider 검증은 D1에서 config가 끝낸다.

### D6. 비스트림 응답 취합 / 테스트 가로채기
- 채택: `/api/chat` 요청 body에 `stream:false`로 단일 JSON 응답을 받는다(streaming은 SPEC §4 제외).
  OllamaClient는 공개 생성자 + 테스트 주입용 내부 생성자(base host·http.Client)로 나눠 httptest로
  `/api/chat`을 가로채 요청·응답 변환을 검증한다(claude_test.go의 결과 동일).
