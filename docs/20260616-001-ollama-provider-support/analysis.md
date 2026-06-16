# ollama-provider-support — ANALYSIS

## 승인 전 확인
- Ollama 연동을 SDK가 아니라 직접 HTTP `/api/chat` 호출로 구현한다. wire 타입·tool calling 스키마를
  직접 관리하는 대신 외부 의존성을 추가하지 않는 방향이 맞는가? 관련 본문: §5
- 이 feature는 활성 LLM 경로를 Ollama로 바꾸고 `ClaudeClient`는 코드만 유지(비활성)한다. config는
  더 이상 `ANTHROPIC_*`를 필수로 요구하지 않으므로, 비활성 ClaudeClient가 호출되지 않는 전제가
  유지되는가? 관련 본문: §4·§5

## 근거

읽은 spec.md 범위: §1 범위, §2 목표, §3 제약, §4 제외 범위, §5 완료 조건 1–3 전체. 본 분석은
spec.md §1이 정한 범위(활성 경로를 Ollama로 전환, Ollama chat + tool calling, host·model 설정·검증,
Claude 코드는 비활성 유지) 안에서만 설계하며 요구사항을 추가하지 않는다. 이 문서는 직전 2-provider
선택 설계를 Ollama 전용 + 직접 HTTP로 재작성한 것이다.

코드베이스에서 확인한 사실:
- `internal/llm/llm.go`: `LLMClient`는 `Chat(ctx, ChatRequest) (ChatResponse, error)` 단일
  메서드 인터페이스다. `ChatRequest`는 `Model string`, `Messages []message.Message`,
  `Tools []message.ToolSpec`이고 `ChatResponse`는 `Message message.Message` 하나다. provider-neutral
  이며 provider 식별자를 포함하지 않는다 — Ollama 전용 전환에서도 이 계약은 그대로다.
- `internal/agent/agent.go`: agent loop(`llmNode`/`toolNode`)는 `llm.LLMClient`와 `message`
  타입에만 의존한다. `llmNode`가 `req.Tools = a.registry.Specs()`로 tool spec을 싣고, `toolNode`가
  마지막 assistant 메시지의 `BlockTypeToolCall` 블록마다 `dispatcher.Dispatch`로 실행해 `RoleTool`
  메시지에 `tool_result` 블록들을 담아 다시 loop로 보낸다. tool_call→tool_result 왕복의 internal
  모델 형태는 이미 고정돼 있고, provider client는 이를 wire와 양방향 변환만 하면 된다.
- `internal/llm/claude.go`: `ClaudeClient`는 이번 범위에서 변경하지 않고 비활성으로 남는다.
  단 OllamaClient 설계의 참조 패턴으로 쓴다 — (a) 공개 생성자 + 테스트 주입용 내부 생성자(baseURL·
  httpClient 주입으로 httptest 가로채기), (b) 생성 시 필수값 부재를 error로 반환, (c) `Chat`이
  ctx.Err()를 먼저 존중하고 호출 실패를 ctx 취소·연결 오류로 분기, (d) internal↔wire 변환에서
  role·tool 블록 사상, (e) tool JSON schema에서 `type`/`properties`/`required` 분리·나머지 보존,
  (f) 응답을 text·tool_call 블록으로 환원.
- `internal/llm/claude_test.go`: httptest로 SDK baseURL을 가로채 변환을 검증하는 결. OllamaClient도
  같은 결로 `/api/chat`을 httptest로 가로채 검증한다.
- `internal/config/config.go`: `Load()`가 `ANTHROPIC_API_KEY`·`ANTHROPIC_MODEL`을 무조건 요구한다
  (config.go:36-44). `Config`는 `AnthropicAPIKey`, `Model`, `Timeout`, `TavilyAPIKey` 필드를 가진다.
  Ollama 전용 전환에서 이 무조건 검증 대상이 Ollama 설정으로 바뀐다.
- `internal/config/config_test.go`: `ANTHROPIC_API_KEY` 필수(`TestLoadRequiresAPIKey`)와
  `cfg.Model`을 `ANTHROPIC_MODEL`에서 읽는다는 가정을 검증한다. 검증 대상 env가 Ollama로 바뀌면
  이 테스트들이 영향을 받아 함께 갱신 대상이다.
- `cmd/agent-runtime/main.go`: `config.Load()` → `llm.NewClaudeClient(cfg)` 하드코딩 생성
  (main.go:36) → `run(ctx, client, cfg.Model, ...)`. `buildRegistry`는 provider와 무관.
- `cmd/agent-runtime/main_test.go`: `run()`에 stub(`llm.LLMClient`)을 주입해 검증한다. `run()`
  시그니처가 그대로면 provider 교체와 무관하게 통과하므로 영향 없음.
- `internal/message/message.go`: `ToolCall{ID, Name, Input json.RawMessage}`,
  `ToolResult{ToolCallID, Content string, IsError bool}`, `ToolSpec{Name, Description,
  InputSchema json.RawMessage}`. tool_call↔tool_result 매칭은 `ID`↔`ToolCallID`로 한다.
- `go.mod`: 직접 의존은 `anthropic-sdk-go`, `godotenv`뿐. 직접 HTTP 방식이므로 Ollama 관련 의존성
  추가는 없다(net/http·encoding/json stdlib만 사용).

Ollama `/api/chat` HTTP 사실(Ollama API 문서·Go 타입 기준):
- `POST {host}/api/chat`, JSON body: `{model, messages, tools, stream}`. `stream:false`면 단일
  JSON 응답이 온다(스트리밍은 SPEC §4 제외 범위).
- 메시지 role: `system`/`user`/`assistant`/`tool`. assistant 메시지가 `tool_calls`를 가질 수 있고,
  tool 결과 메시지는 `{role:"tool", content, tool_name}`(버전에 따라 `tool_call_id`)로 싣는다 —
  Anthropic이 tool_result를 user content block에 싣는 것과 다른 지점이다.
- `tools`: `[{type:"function", function:{name, description, parameters}}]`. `parameters`는 JSON
  Schema(`type`/`properties`/`required`).
- 응답 `message.tool_calls`: `[{function:{name, arguments}}]` 형태이며, 호출 식별 `id`가 응답에
  포함되지 않을 수 있다. 매칭 보장은 §5 D3에서 다룬다.

## 1. 구조

새 레이어를 도입하지 않고 다음 두 경계 안에서 끝난다. 직전 설계의 "provider 선택" 경계는 제거된다
(SPEC §4: 실행 시점 provider 선택은 범위 밖).

- LLM provider 경계(`internal/llm`): `LLMClient`·`ChatRequest`·`ChatResponse`는 그대로 둔다. 새
  구현체 `OllamaClient`를 `ClaudeClient`와 같은 패키지에 추가하되, SDK가 아니라 `net/http`로
  `/api/chat`을 직접 호출한다. 공개 생성자 + 테스트 주입용 내부 생성자(base host·http.Client 주입)로
  나누고, internal↔Ollama wire 요청 변환 helper, 응답 변환 helper, JSON schema→Ollama tool 변환
  helper를 둔다. `ClaudeClient`(`claude.go`)는 변경 없이 비활성으로 남는다.
- 설정 경계(`internal/config`): `Load()`의 무조건 `ANTHROPIC_*` 검증을 Ollama 설정(host·model)
  검증으로 바꾼다(SPEC §5.3). Ollama host·model env 상수와 `Config` 필드를 추가한다. 비활성
  ClaudeClient가 읽는 `AnthropicAPIKey` 필드는 컴파일·후속 재연결을 위해 남기되 더 이상 필수가
  아니다.
- 조립 경계(`cmd/agent-runtime`): `main`의 client 생성 한 줄(main.go:36)을 `NewClaudeClient`에서
  `NewOllamaClient`로 바꾼다. provider 분기·팩토리는 두지 않는다(단일 활성 provider). `buildRegistry`·
  `run`·`readPrompt`는 불변.

## 2. 데이터 흐름

### 부팅 흐름
1. `config.Load()`가 `.env`/환경변수를 읽어 `Config`를 만들고, Ollama host·model을 검증한다.
   host는 미지정 시 기본값(`http://localhost:11434`)으로 채우고, model이 없으면 error를 반환한다
   (SPEC §5.3). `ANTHROPIC_*` 부재는 더 이상 실패가 아니다.
2. `main`이 `NewOllamaClient(cfg)`로 client를 생성한다. host·model이 비면 그 기준 error를 stderr에
   쓰고 비정상 종료(SPEC §5.3). 이후 `buildRegistry`·`run`은 기존과 동일하게 흐른다.

### chat·tool calling 왕복 흐름
agent loop 구조는 provider에 무관하다. `llmNode`가 `ChatRequest`(messages + tool specs)를
`OllamaClient.Chat`에 넘기면 OllamaClient가 다음을 수행한다:

- 요청 변환(internal → `/api/chat` body):
  - `RoleSystem` 메시지의 text → `{role:"system", content:...}` (Anthropic의 별도 System 필드와
    달리 일반 메시지 한 건).
  - `RoleUser`/`RoleAssistant`의 text 블록 → `{role, content}`. assistant 메시지의 `tool_call`
    블록 → 같은 메시지의 `tool_calls[]`(`{id, function:{name, arguments}}`). internal `Input`
    (json.RawMessage)을 `arguments`로 싣는다.
  - `RoleTool` 메시지의 각 `tool_result` 블록 → 개별 `{role:"tool", content:<result>,
    tool_name:..., tool_call_id:<internal ToolCallID>}`. 한 internal `RoleTool` 메시지가 여러
    tool_result를 담을 수 있으므로 1:N로 풀어 여러 tool 메시지로 변환한다.
  - `req.Tools`(ToolSpec[]) → `tools[]`: `type:"function"`, `function.name`/`description`,
    `function.parameters`는 ToolSpec.InputSchema에서 `type`/`properties`/`required` 사상.
  - body에 `stream:false`, `model`(req.Model 우선, 없으면 client 기본)을 싣는다.
- HTTP 호출: `POST {host}/api/chat`을 ctx와 함께 보낸다. ctx 취소·연결 실패·비정상 status code는
  ClaudeClient와 같은 결로 error 표면화(ctx.Err() 우선). 응답 body(JSON)를 디코드한다.
- 응답 변환(`/api/chat` → internal): `message.content`(text) → text 블록, `message.tool_calls[]`
  → `tool_call` 블록(`ToolCall{ID, Name, Input}`). 둘 다 없거나 알 수 없는 형태면 error.

이후 흐름은 기존과 동일하다: tool_call이 있으면 router가 `toolNode`로, dispatcher가 실행한
`tool_result`를 `RoleTool` 메시지로 누적해 다시 `llmNode`로. tool_call 없는 assistant 응답이면
`StatusFinal`로 stdout 출력(SPEC §5.1·§5.2).

### tool_call ID 매칭 흐름(비자명 지점)
internal 모델은 `ToolCall.ID`↔`ToolResult.ToolCallID`로 호출-결과를 매칭한다. 응답 변환에서
Ollama가 준 tool_call `id`를 그대로 internal `ToolCall.ID`로 쓰되, 비어 있으면 OllamaClient가
결정적 ID를 생성해 채운다(매칭 불능 방지). 생성 규칙은 §5 D3. 요청 변환 때는 internal `ID`를
tool_call `id`/tool 메시지 `tool_call_id`로 되싣어 왕복 동안 동일 ID를 유지한다.

## 3. 인터페이스

경계를 가로지르는 계약만 기술한다.

- `llm.LLMClient` (불변): `Chat(ctx, ChatRequest) (ChatResponse, error)`. `ChatRequest`
  (`Model`, `Messages`, `Tools`)·`ChatResponse`(`Message`)도 불변. agent loop는 이 계약만 알고
  provider를 모른다(SPEC §3). OllamaClient가 이 인터페이스를 구현한다.
- `llm.NewOllamaClient(cfg config.Config) (*OllamaClient, error)`: ClaudeClient 생성자와 대칭.
  Ollama host·model 부재를 error로 반환한다(SPEC §5.3). 테스트 주입용 내부 생성자(base host·
  http.Client 주입)는 내부 helper라 외부 계약이 아니다.
- `config.Config`(확장): main과의 계약. Ollama host·model을 표현하는 필드를 추가한다. 활성 model은
  공용 `Model` 필드로 normalize한다(§5 D2). `AnthropicAPIKey`는 비활성 ClaudeClient용으로 남기되
  더 이상 필수 검증 대상이 아니다. `Timeout`·`TavilyAPIKey`는 불변.

내부 변환 helper(internal↔`/api/chat` wire, JSON schema→tool)는 `internal/llm` 안에서만 쓰이므로
외부 인터페이스가 아니다.

## 4. 영향 범위

이 변경이 실제로 건드리는 대상:
- `internal/llm/`(신규): `OllamaClient`(net/http 기반)와 변환 helper·테스트. `LLMClient`/
  `ChatRequest`/`ChatResponse`(`llm.go`)·`ClaudeClient`(`claude.go`)·`StubClient`(`stub.go`)는
  내용 변경 없음(claude.go는 비활성으로 잔존).
- `internal/config/config.go`: Ollama host·model env 상수와 `Config` 필드 추가, `Load()`의 무조건
  `ANTHROPIC_*` 검증을 Ollama 설정 검증으로 변경(config.go:36-44). `Model`을 Ollama model에서 읽도록
  변경. `AnthropicAPIKey` 필드 자체는 유지.
- `internal/config/config_test.go`: `ANTHROPIC_*` 필수·`Model=ANTHROPIC_MODEL` 가정을 검증하는
  케이스(`TestLoadRequiresAPIKey` 등)가 새 검증 규칙과 어긋나므로 함께 갱신 대상.
- `cmd/agent-runtime/main.go`: client 생성 한 줄을 `NewClaudeClient`→`NewOllamaClient`로 교체
  (main.go:36). `run`에 넘기는 `cfg.Model`은 Ollama model이 된다. `buildRegistry`·`readPrompt`·
  `run` 본문 로직은 불변. `main_test.go`는 `run()`에 stub을 주입하므로 영향 없음.
- `.env.example`: Ollama host(예 `http://localhost:11434`)·model 안내 추가, `ANTHROPIC_*`가 현재
  비활성(후속 Claude 재연결용)임을 명시.

하위 호환·동작 변경:
- 활성 경로가 Ollama로 바뀌고 config가 `ANTHROPIC_*` 대신 Ollama 설정을 요구하므로, 기존
  `ANTHROPIC_*`만 채운 `.env`로는 더 이상 실행되지 않는다(Ollama host·model 필요). 마이그레이션
  필요 항목은 `.env`에 Ollama 설정 추가뿐이며, 저장 데이터·외부 contract 변경은 없다.
- `go.mod`/`go.sum`: 변경 없음(직접 HTTP, 신규 의존성 없음).
- agent·tool·graph 패키지: 해당 없음(provider 무관, 변경 불필요).

## 5. Decision Points

### D1. Ollama 연동 방식 — 직접 HTTP `/api/chat`
- 옵션: (a) 공식 SDK `github.com/ollama/ollama/api`, (b) 직접 HTTP `/api/chat`(`net/http`),
  (c) OpenAI 호환 endpoint.
- 트레이드오프: SDK는 타입을 그대로 쓰지만 본체 monorepo 패키지라 go.mod가 그 큰 모듈에 묶인다.
  직접 HTTP는 wire 타입·tool 스키마를 직접 관리해야 하지만 외부 의존성이 없고, Ollama API가 단순·
  안정적이라 관리 부담이 작다. Claude를 지금 붙이지 않으므로 "Claude=SDK, Ollama=직접" 비대칭
  부담도 없다.
- 채택: (b) 직접 HTTP. 근거: 사용자 결정이며, 의존성을 늘리지 않고 native chat API의 tools/
  tool_calls를 internal 모델에 직접 사상한다.

### D2. Ollama model·host 설정 표현
- 옵션: (a) 활성 model을 공용 `Model` 필드 하나로 normalize(env는 Ollama용으로 읽음). (b) provider
  별 model 필드를 분리 보관.
- 트레이드오프: (a)는 `run(ctx, client, cfg.Model, ...)`·agent의 단일 model 전달 경로(SPEC §3:
  호출자 불변)와 그대로 맞고 cmd 변경이 최소다. (b)는 단일 활성 provider(SPEC §3)에서 동시 보유의
  이득이 없다.
- 채택: (a) 공용 단일 `Model`(Ollama model에서 읽음). Ollama host는 별도 필드(예 `OllamaHost`,
  기본 `http://localhost:11434`)로 둔다(API key 불필요). 근거: 호출자 model 전달 경로를 건드리지
  않는 가장 작은 변경이다.

### D3. tool_call ID 매칭 보장(Ollama id 부재 대비)
- 사실: Ollama `/api/chat` 응답 `tool_calls`에 호출 식별 `id`가 포함되지 않을 수 있다. internal
  모델은 `ToolCall.ID`↔`ToolResult.ToolCallID`로 호출-결과를 매칭한다.
- 옵션: (a) Ollama id를 그대로 쓰되 비면 OllamaClient가 결정적 ID 생성(응답 내 tool_call 순번 기반
  `call_<n>`). (b) 항상 client가 자체 ID 재발급. (c) 매칭을 순서에 의존.
- 트레이드오프: (c)는 internal 모델·dispatcher가 ID 기반이라 맞지 않는다. (b)는 Ollama가 id를 주면
  그 값을 버려 추적성이 준다.
- 채택: (a). 응답 변환에서 id가 비면 그 응답 안 tool_call 등장 순번으로 결정적 ID를 채워 internal
  `ToolCall.ID`에 싣고, 요청 변환 때 그 ID를 tool_call `id`/tool 메시지 `tool_call_id`로 되돌려
  왕복 동안 유지한다. 근거: 같은 client가 발급·소비를 통제하므로 매칭이 보장된다.

### D4. 비스트림 응답 취합
- 사실: `/api/chat`은 기본 스트리밍(JSON 라인 다건)이다. streaming 처리는 SPEC §4 제외 범위다.
- 채택: 요청 body에 `stream:false`를 실어 단일 JSON 응답으로 받고, 그 `message`를 단일
  `ChatResponse`로 변환한다. 근거: 제외 범위를 지키면서 단발 응답 계약을 만족한다.

### D5. 테스트 전략 — httptest 가로채기
- 사실: OllamaClient가 base host·http.Client를 주입받으면 httptest 서버로 `/api/chat`을 가로챌 수
  있다(ClaudeClient의 baseURL·httpClient 주입과 동일 구조).
- 채택: 공개 생성자 + 테스트 주입용 내부 생성자로 나누고, 내부 생성자에 base host·http.Client를
  받아 httptest로 `/api/chat`을 가로채 요청 변환(system 메시지·tool 메시지·tools·stream:false 사상)
  과 응답 변환(text·tool_call·id 매칭)을 관찰 검증한다. 근거: 기존 claude_test.go의 결을 따른다.
