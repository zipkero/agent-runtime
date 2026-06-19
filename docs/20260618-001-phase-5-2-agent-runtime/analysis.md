# phase-5-2-agent-runtime 분석

## 근거

읽은 기준 문서:

- `docs/20260618-001-phase-5-2-agent-runtime/spec.md` 전체. 범위는 Single Agent runner, Graph 기반 실행 보존,
  pre/post model middleware, structured output contract, CLI 연결이다. streaming, provider별 JSON mode,
  schema registry, plugin system, RAG·Memory·Multi-Agent·MCP/A2A는 제외다.
- `ROADMAP.md` Phase 5.2. Phase 5.1 tool 묶음 이후 middleware hook, structured output, Single Agent runner,
  Graph 기반 Single Agent 실행을 완성하는 단계임을 확인했다.
- `docs/20260614-001-phase-5-1-tool-bundle/analysis.md`. Phase 5.1은 tool 묶음과 CLI 등록 경로까지이며,
  middleware, structured output, runner public API는 Phase 5.2로 제외되어 있었다.
- `README.md`. `internal/agent`는 Single Agent 실행 구조를 담당하고, `internal/llm`은 provider-neutral LLM
  호출 계약, `internal/graph`는 범용 graph 실행 엔진을 담당한다는 package 책임을 확인했다.

코드베이스에서 확인한 사실:

- `internal/agent.Agent`는 `llm.LLMClient`, model, max steps, `ReflectionHook`, `tool.Registry`,
  tool timeout을 주입받아 `AgentState`를 반환한다. 내부적으로 Graph를 만들고 `llm_node → tool_node →
  llm_node` 흐름을 실행한다.
- `llm_node`는 `llm.ChatRequest{Model, Messages, Tools}`를 만들고 `client.Chat(ctx, req)`를 직접 호출한다.
  pre/post model hook을 넣어야 하는 지점은 이 node 안이다.
- 현재 `ReflectionHook`은 step 경계 관찰 전용이고, 요청·응답을 변경하거나 에러를 반환할 수 없다. SPEC §5.4,
  §5.5, §5.7을 만족하려면 별도 middleware 계약이 필요하다.
- `cmd/agent-runtime.run`은 Agent를 직접 생성하고 종료 상태별 stdout/stderr/exit code를 처리한다. CLI 조립과
  재사용 가능한 실행 표면이 분리되어 있지 않다.
- `internal/llm.LLMClient`와 `ChatRequest`/`ChatResponse`는 provider-neutral 계약이다. Claude/Ollama provider
  구현도 이 계약 뒤에 있으므로 Phase 5.2가 provider 계약을 바꾸지 않아도 된다.
- `internal/graph.Graph`는 generic 실행 엔진이며 agent domain 타입을 import하지 않는다. middleware나 structured
  output을 graph package에 넣으면 package 책임이 흐려진다.
- `go.mod`에는 `github.com/invopop/jsonschema`가 indirect로 있지만, 현재 코드에서 JSON Schema 검증을 수행하는
  구현은 없다. structured output을 JSON Schema 기준으로 검증하려면 검증 책임을 새로 둬야 한다.

추정으로 분리:

- structured output의 JSON Schema validator 구현체는 아직 코드에 없다. 구현 단계에서 새 직접 의존성을 추가하거나
  제한된 validator를 직접 구현해야 한다. 본 분석은 표준 JSON Schema 검증 동작을 요구하는 구조로 설계한다.

## 1. 구조

Phase 5.2는 Single Agent 실행 표면을 `internal/agent` 안에서 정리한다. `internal/agent`가 이미 `llm`,
`message`, `tool`, `graph`를 조합하는 계층이므로 runner, middleware, structured output 결과를 이 경계에 두는
것이 package 책임과 맞다(SPEC §5.1, §5.2). `internal/llm`은 provider-neutral chat 계약을 유지하고,
`internal/graph`는 domain-agnostic engine으로 유지한다.

새 실행 표면은 `Runner`가 소유한다. Runner는 `LLMClient`, model, `tool.Registry`, max steps, tool timeout,
middleware 목록, optional output contract를 구성으로 받고 `Run(ctx, prompt)`로 실행 결과를 반환한다. 기존
`Agent.Run`은 graph 기반 loop를 계속 담당하되, Runner는 Agent 생성과 결과 해석을 감싸는 상위 실행 표면이 된다
(SPEC §5.1, §5.2). CLI는 직접 `agent.NewAgent`를 호출하지 않고 Runner를 통해 실행한다(SPEC §5.3).

middleware는 Agent의 LLM node 안에서 실행한다. pre-model middleware는 `ChatRequest`가 만들어진 뒤 LLM 호출 전
등록 순서대로 실행하고, 변경된 request를 다음 middleware와 실제 `LLMClient.Chat`에 전달한다(SPEC §5.4,
§5.6). post-model middleware는 `LLMClient.Chat` 호출 후 등록 순서대로 실행하며, response가 있으면 변경된
response를 다음 middleware와 Agent state에 전달한다(SPEC §5.5, §5.6). LLM 호출 error가 있으면 post-model
middleware가 error를 관찰하고 wrapping 또는 replacement error를 반환할 수 있지만, 이번 범위에서는 error를
정상 response로 복구하는 fallback 동작은 정의하지 않는다. provider fallback이 SPEC 제외 범위이기 때문이다.

structured output은 Runner 결과 해석 단계가 소유한다. Agent가 `StatusFinal`로 종료하고 최종 assistant text를
얻은 뒤, Runner가 optional `OutputContract`를 확인한다. contract가 없으면 기존 text-only 결과를 그대로
반환한다(SPEC §5.10). contract가 있으면 최종 assistant text를 JSON으로 파싱하고 JSON Schema로 검증한다.
성공하면 raw text와 structured value를 Runner 결과에 함께 담고, 실패하면 structured output 실패로 구분 가능한
결과를 반환한다(SPEC §5.8, §5.9). 이 처리는 중간 tool call이나 tool result에 적용하지 않는다.

기존 `ReflectionHook`은 step 경계 관찰 역할로 유지한다. middleware가 이 역할을 대체할 수 있지만, 기존 테스트와
사용 경로를 깨지 않기 위해 제거하지 않는다. 다만 새 runner API에서는 `ReflectionHook`을 고급 옵션으로만
노출하고, model 호출 전후 제어는 middleware가 담당하게 한다.

## 2. 데이터 흐름

Runner 기반 기본 실행 흐름:

```text
caller 또는 CLI
→ agent.RunnerConfig 구성(client, model, registry, max steps, timeout, middleware, output contract)
→ Runner.Run(ctx, prompt)
→ Agent 생성
→ Agent.Run(ctx, prompt)
→ graph 실행(llm_node ↔ tool_node)
→ RunnerResult 반환
```

LLM node의 middleware 포함 흐름:

```text
AgentState
→ llm_node가 ChatRequest 생성(Messages, Tools, Model)
→ pre-model middleware[0..n]가 등록 순서대로 request 관찰·변경
→ LLMClient.Chat(ctx, changedReq)
→ post-model middleware[0..n]가 response 또는 error 관찰·변경
→ response.Message를 AgentState.Messages에 누적
→ tool_call이 없으면 StatusFinal, 있으면 tool_node로 이동
```

pre-model middleware가 에러를 반환하면 LLM 호출을 시작하지 않는다. Graph는 node error로 중단되고,
AgentState는 `StatusError`가 된다. 호출자는 RunnerResult의 error stage 또는 typed error로 pre-model 실패임을
확인한다(SPEC §5.7).

LLM 호출이 error를 반환하면 post-model middleware가 error를 관찰할 수 있다. post hook이 error를 반환하면
Graph는 error로 종료되고, 호출자는 post-model 또는 LLM failure context를 확인한다. post hook이 nil error를
반환하더라도 response가 없으면 정상 진행하지 않는다. response 없는 성공은 provider-neutral chat 계약상 유효한
assistant 메시지를 만들 수 없기 때문이다.

tool calling 흐름은 기존과 같다. pre hook이 tool schema를 제거하거나 바꾸면 실제 LLM 요청의 `Tools`가 바뀐다.
LLM이 tool call을 반환하면 post hook이 response를 변경할 수 있고, 최종적으로 state에 들어간 assistant 메시지가
tool_node의 판단 기준이 된다(SPEC §5.2, §5.5). 따라서 post hook이 tool_call을 제거하면 그 응답은 final로
판정될 수 있고, tool_call을 추가하면 tool_node로 진행할 수 있다. 이 변경 가능성은 middleware의 의도된 책임이다.

structured output 흐름:

```text
Agent.Run 종료
→ state.Status 확인
→ StatusFinal이면 FinalMessage 추출
→ assistant text block을 최종 raw text로 결합
→ OutputContract가 없으면 raw text만 반환
→ OutputContract가 있으면 raw text JSON parse
→ JSON Schema validation
→ 성공: StructuredValue 포함
→ 실패: structured output failure로 RunnerResult 반환
```

structured output 실패는 LLM 호출 실패나 tool 실행 실패와 다르다. LLM 호출은 성공했고 final text도 있었지만,
호출자가 요구한 output contract를 만족하지 못한 경우다(SPEC §5.9). 따라서 RunnerResult에는 AgentState와 raw
text를 가능한 한 보존하고, 실패 종류를 structured output으로 분류한다.

CLI 흐름은 Runner를 쓰도록 바뀐다.

```text
main
→ config.Load
→ llm.NewClient
→ readPrompt
→ buildRegistry
→ agent.Runner.Run
→ StatusFinal이면 raw text stdout
→ 실패이면 stderr + non-zero exit code
```

CLI는 기본적으로 output contract와 middleware를 지정하지 않는다. 따라서 기존 stdout/stderr/exit code 계약이
유지된다(SPEC §5.3, §5.10).

## 3. 인터페이스

Runner 생성 표면:

```go
type RunnerConfig struct {
    Client      llm.LLMClient
    Model       string
    MaxSteps    int
    Registry    *tool.Registry
    ToolTimeout time.Duration
    Middleware  []Middleware
    Output      *OutputContract
    Hook        ReflectionHook
}

type Runner struct {
    // config 보관
}

func NewRunner(cfg RunnerConfig) (*Runner, error)
func (r *Runner) Run(ctx context.Context, prompt string) RunnerResult
```

`NewRunner`는 client 부재처럼 실행이 불가능한 구성만 error로 반환한다. `Registry == nil`, middleware 없음,
output contract 없음은 기존 동작 보존을 위해 유효한 구성으로 둔다(SPEC §5.10).

Runner 결과 표면:

```go
type RunnerResult struct {
    State           AgentState
    FinalMessage    message.Message
    FinalText       string
    StructuredValue any
    StructuredRaw   json.RawMessage
    Status          RunnerStatus
    Err             error
}
```

`RunnerStatus`는 최소한 success, agent_error, max_steps, structured_output_error를 구분한다. AgentState의
`Status`만으로는 structured output 실패와 LLM/tool 흐름 실패를 충분히 구분하기 어렵기 때문이다(SPEC §5.7,
§5.9). max steps와 agent error는 기존 CLI 계약을 유지하도록 stderr와 non-zero exit code로 매핑한다.

middleware 인터페이스:

```go
type Middleware interface {
    PreModel(ctx context.Context, in PreModelInput) (llm.ChatRequest, error)
    PostModel(ctx context.Context, in PostModelInput) (llm.ChatResponse, error)
}

type PreModelInput struct {
    Step    int
    State   AgentState
    Request llm.ChatRequest
}

type PostModelInput struct {
    Step     int
    State    AgentState
    Request  llm.ChatRequest
    Response llm.ChatResponse
    Err      error
}
```

`PreModel`은 request를 반환해 변경을 표현한다. `PostModel`은 response를 반환해 변경을 표현한다. `Err != nil`인
post input에서는 response가 zero 값일 수 있으며, hook은 error를 관찰·대체할 수 있다. 구현에서는 이 경우를
위해 `PostModel`이 response와 error를 함께 반환할 수 있는 형태도 고려할 수 있다. 중요한 계약은 SPEC §5.5와
§5.7이 요구하는 "응답 변경"과 "에러 관찰·전파"가 모두 가능해야 한다는 점이다.

에러 구분 표면:

```go
type MiddlewareStage string

const (
    MiddlewareStagePreModel  MiddlewareStage = "pre_model"
    MiddlewareStagePostModel MiddlewareStage = "post_model"
)

type MiddlewareError struct {
    Stage MiddlewareStage
    Index int
    Err   error
}
```

pre/post middleware 실패는 typed error로 감싸 호출자가 `errors.As`로 stage와 index를 확인할 수 있게 한다
(SPEC §5.7). RunnerResult도 같은 정보를 상태로 노출할 수 있다.

structured output 표면:

```go
type OutputContract struct {
    Name        string
    Description string
    Schema      json.RawMessage
}
```

`Name`과 `Description`은 호출자가 결과 의미를 식별하는 metadata다. schema 검증 자체에는 `Schema`만 필요하지만,
계약 이름이 있어야 여러 runner 구성과 오류 메시지를 구분할 수 있다. `Schema`는 JSON Schema다(SPEC §5.8).

JSON Schema 검증은 `internal/agent` 내부 helper 또는 하위 파일에 둔다. 이 helper는 raw text를 JSON으로 파싱하고
contract schema로 검증한 뒤 `json.RawMessage`와 `any` 형태의 decoded value를 반환한다. provider-specific
JSON mode는 사용하지 않는다.

CLI 경계:

`cmd/agent-runtime.run`은 지금처럼 exit code를 반환하되 내부에서 Runner를 사용한다. CLI 기본 경로는 middleware와
output contract를 비워 둔다. CLI에 structured output 입력 flag나 config를 추가하는 것은 본 spec의 완료 조건에
없으므로 이번 분석에서 확정하지 않는다.

## 4. 영향 범위

신규 또는 주요 변경:

- `internal/agent`: Runner, RunnerConfig, RunnerResult, RunnerStatus, middleware 인터페이스,
  OutputContract, structured output parsing/validation helper를 추가한다. 기존 Agent graph loop는 재사용한다.
- `internal/agent/agent.go`: Agent가 middleware를 보유하고 `llm_node`에서 pre/post model hook을 실행하도록 확장한다.
  기존 `NewAgent` 호출 경로는 보존하거나 compatibility wrapper로 유지한다.
- `cmd/agent-runtime/main.go`: `run`이 직접 Agent를 만들지 않고 Runner를 통해 실행하도록 바꾼다. stdout/stderr/exit
  code 관찰 동작은 유지한다.
- `cmd/agent-runtime/main_test.go`: 기존 CLI 동작 회귀가 Runner 기반 경로에서도 유지되는지 확인한다.
- `internal/agent/agent_test.go`: middleware 요청 변경, 응답 변경, 등록 순서, 에러 stage, structured output 성공·실패,
  output contract 미지정 text-only 경로를 검증한다.
- `go.mod`/`go.sum`: 표준 JSON Schema 검증을 위해 새 직접 의존성을 추가할 수 있다. 구현 전에 선택한 validator의
  이유와 영향을 다시 보고해야 한다.

변경하지 않는 범위:

- `internal/llm`: `LLMClient`, `ChatRequest`, `ChatResponse`, Claude/Ollama provider 변환은 변경하지 않는다.
  middleware는 provider가 아니라 Agent 실행 경계 책임이다.
- `internal/graph`: generic graph engine은 변경하지 않는다. pre/post model hook은 graph node 내부의 agent domain
  동작이다.
- `internal/tool`: tool interface, registry, dispatcher, Phase 5.1 tool 묶음의 schema와 실행 정책은 변경하지 않는다.
- `internal/message`: message role, content block, tool call/result 타입은 변경하지 않는다.
- `internal/config`: 이번 spec은 CLI에 middleware나 output contract 설정을 추가하지 않는다. 설정 표면 확장은 후속
  요구사항이 있을 때 별도 spec으로 다룬다.

## 5. Decision Points

### D1. Runner를 어느 package에 둘 것인가

- 옵션 A: `internal/agent`에 둔다.
- 옵션 B: 새 `internal/runner` package를 만든다.
- 옵션 C: `cmd/agent-runtime`에만 둔다.
- 트레이드오프: B는 이름이 깔끔하지만 `agent`, `llm`, `tool`, `message`를 다시 조합하는 얇은 package가 생긴다.
  C는 CLI 요구는 빠르게 만족하지만 SPEC §5.1의 재사용 가능한 실행 표면을 만족하기 어렵다. A는 기존 Agent 실행
  책임과 자연스럽게 이어지고 package 수를 늘리지 않는다.
- 채택: **A**. Single Agent 실행 구조는 README 기준으로 `internal/agent` 책임이며, Runner는 Agent graph loop를
  재사용하는 상위 실행 표면이다.

### D2. middleware를 `llm` 계층에 둘 것인가, `agent` 계층에 둘 것인가

- 옵션 A: `llm.LLMClient` wrapper로 구현한다.
- 옵션 B: `agent`의 `llm_node` 안에서 실행한다.
- 트레이드오프: A는 provider 호출 wrapper로 단순하지만 `AgentState`, step, tool schema, Graph 흐름을 알기 어렵다.
  B는 Agent state와 request/response를 함께 보여 줄 수 있고 SPEC §5.4, §5.5가 요구하는 상태 기반 관찰에 맞다.
- 채택: **B**. middleware는 provider가 아니라 Single Agent 실행의 횡단 관심사다. `internal/llm` 계약은 유지한다.

### D3. post-model middleware 실행 순서

- 옵션 A: pre와 post 모두 등록 순서대로 실행한다.
- 옵션 B: pre는 등록 순서, post는 역순으로 실행한다.
- 트레이드오프: B는 HTTP middleware stack 패턴과 유사하지만, SPEC은 "등록 순서"와 변경 전파의 예측 가능성을
  요구한다. A는 모든 hook에서 앞 hook의 변경 결과가 뒤 hook에 전달된다는 규칙이 단순하다.
- 채택: **A**. pre/post 모두 등록 순서대로 실행해 SPEC §5.6을 직접 만족한다.

### D4. post-model middleware가 LLM error를 정상 response로 복구할 수 있게 할 것인가

- 옵션 A: error 관찰과 wrapping/replacement만 허용하고, error를 정상 response로 복구하지 않는다.
- 옵션 B: post hook이 error를 response로 복구할 수 있게 한다.
- 트레이드오프: B는 fallback이나 synthetic response를 만들 수 있어 강력하지만 provider fallback과 retry 정책에
  가까워진다. A는 범위가 작고 실패 처리가 명확하다.
- 채택: **A**. SPEC 제외 범위에 provider fallback과 retry가 있으므로, 이번 Phase에서는 error를 관찰하고 실행 실패로
  전환하는 책임까지만 둔다.

### D5. structured output을 어디에서 처리할 것인가

- 옵션 A: provider 호출 전 request에 JSON schema를 실어 provider가 강제하게 한다.
- 옵션 B: Agent final 이후 Runner가 raw text를 JSON parse + schema validate 한다.
- 옵션 C: caller가 Runner 밖에서 직접 파싱한다.
- 트레이드오프: A는 모델 출력 품질을 높일 수 있지만 provider별 JSON mode나 constrained decoding으로 빠진다.
  C는 runtime 책임을 회피해 SPEC §5.8, §5.9의 관찰 가능한 결과 표면을 만족하기 어렵다. B는 provider-neutral
  계약을 유지하면서 Runtime이 structured output 성공/실패를 구분할 수 있다.
- 채택: **B**. structured output contract는 Runner 입력이고, 검증은 final text 이후 Runtime에서 수행한다.

### D6. JSON Schema 검증을 직접 구현할 것인가, 검증 의존성을 추가할 것인가

- 옵션 A: `type`, `properties`, `required`, `additionalProperties` 같은 최소 subset만 직접 구현한다.
- 옵션 B: JSON Schema validator를 직접 의존성으로 추가한다.
- 트레이드오프: A는 의존성을 늘리지 않지만 "JSON Schema"라는 contract를 부분적으로만 만족해 호출자가 오해할 수
  있다. B는 dependency 관리가 생기지만 SPEC §5.8의 schema 기준 검증을 더 정확히 만족한다.
- 채택: **B**. 이 프로젝트는 LangChain/LangGraph 금지가 핵심이고, 표준 검증 라이브러리 사용은 Runtime 본체 위임이
  아니라 schema validation 구현 선택이다. 구현 단계에서 새 의존성 추가 이유와 영향을 보고한 뒤 적용한다.

### D7. 기존 `NewAgent` 생성자를 바꿀 것인가

- 옵션 A: 기존 `NewAgent` 시그니처를 middleware까지 포함하도록 변경한다.
- 옵션 B: `AgentOptions` 또는 runner 전용 생성 경로를 추가하고, 기존 `NewAgent`는 compatibility wrapper로 유지한다.
- 트레이드오프: A는 표면이 하나라 단순하지만 기존 테스트와 호출부를 넓게 고친다. B는 새 기능을 옵션 구조로
  수용하면서 기존 호출부를 안정적으로 유지한다.
- 채택: **B**. Phase 5.2는 실행 표면을 정리하지만 기존 Agent loop 계약을 불필요하게 깨지 않는다는 SPEC 제약을
  따른다.
