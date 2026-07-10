# Phase 4.2 Agent Execution 분석

## 근거

확인한 사실:

- `spec.md`는 Phase 4.2 범위를 모든 model 호출에 적용되는 순차 `pre-model`/`post-model` middleware, JSON Schema
  기반 structured output 검증, provider-neutral Single Agent Runner, CLI의 Agent loop 전환으로 정의한다.
- 사용자는 middleware가 요청과 응답을 변경할 수 있고 오류 시 run을 중단하는 방향, Runner가 final JSON을 schema로
  검증하되 provider native 강제 출력과 자동 재시도를 제외하는 방향을 확인했다.
- 현재 `internal/agent.Agent`는 `llm.LLMClient`, model, max step, Tool registry, Tool timeout을 소유하고, 각 step에서
  `LLMClient.Chat`을 직접 호출한다. Tool result는 메시지에 누적되며 final, needs action, max step, error 상태는
  `AgentState`에 보존된다.
- 현재 `AgentState`는 일반 text `FinalAnswer`, `LastError`, 메모리 trace를 제공하지만 structured output 결과와
  middleware·structured output 전용 오류 분류는 제공하지 않는다.
- 현재 `llm.ChatRequest`와 `llm.ChatResponse`는 provider-neutral 값이며 Claude와 Ollama 구현이 같은 `LLMClient`
  interface를 따른다. 두 provider 모두 전달된 context로 HTTP 요청을 취소한다.
- 현재 `cmd/agent-runtime`은 입력과 config를 읽고 `LLMClient.Chat`을 한 번만 호출한다. `LLM_TIMEOUT` context는 이
  단발 호출을 감싸며, CLI는 Tool registry나 `internal/agent.Agent`를 조립하지 않는다.
- `internal/tool`에는 calculator, file read, web search, file save, code execution Tool이 있다. File Tool과 Code
  Execution Tool 생성자는 허용 root를 요구하고, Web Search Tool은 `TAVILY_API_KEY`가 없어도 생성되지만 호출 시
  configuration 오류 result를 반환한다.
- 현재 `go.mod`에는 외부 module 의존성이 없다. Go 표준 `encoding/json`은 JSON 파싱은 제공하지만 JSON Schema
  keyword 검증은 제공하지 않는다.
- `go list -m -json`으로 확인한 `github.com/santhosh-tekuri/jsonschema/v6` 최신 안정 버전은 `v6.0.2`이고 Go 1.21을
  요구한다. 이 프로젝트는 Go 1.26을 사용한다. 공식 문서는 Draft 2020-12, 2019-09, 7, 6, 4 지원과 schema compile,
  instance validation, 구조화된 validation error를 설명한다.
- 프로젝트가 지시한 `docs/languages.md`와 Go 세부 기준 문서는 현재 저장소에 없다.

추정:

- Phase 4.2 CLI는 structured output schema를 받는 새 flag를 추가하지 않는다. `SPEC §5.6`, `SPEC §5.7`의 schema 입력은
  Runner API가 제공하고, CLI 완료 조건인 `SPEC §5.9`, `SPEC §5.10`은 Tool loop의 final text 출력과 실패 종료를
  요구하기 때문이다.
- Phase 4.2는 새 domain system prompt를 고정하지 않는다. `spec.md`가 system prompt template을 제외하고 Runtime
  의존성과 Tool 구성을 진입점 주입으로 제한하기 때문이다.

## 1. 구조

기존 `Agent`는 메시지 상태, step 전이, Tool 실행을 소유하는 loop 엔진으로 유지한다. 새 `Runner`는 호출자가 사용하는
상위 실행 경계로 두고, Runner 생성 시 model 호출 decorator와 선택적인 structured output validator를 조립한 뒤 기존
Agent를 내부에서 생성한다. 이렇게 하면 Phase 2·3에서 확정된 `Agent.Run` contract를 깨지 않고 `SPEC §5.1`,
`SPEC §5.2`, `SPEC §5.8`을 만족할 수 있다.

```text
Caller / CLI
→ Runner
  → middleware client
    → pre-model hooks
    → per-model timeout
    → LLMClient
    → post-model hooks
  → Agent loop
    → Tool registry / Tool execution
  → optional structured output validator
→ RunnerResult
```

Runner는 `RunnerOptions`로 client, model, max step, model timeout, Tool registry, Tool timeout, middleware 목록,
선택적인 output schema를 받는다. Runner 내부의 Agent에는 middleware가 감싼 client를 주입한다. Agent가 model을 호출할
때마다 같은 decorator를 통과하므로 Tool loop의 두 번째 이후 호출에도 middleware가 빠지지 않는다
(`SPEC §5.2`~`SPEC §5.5`).

Middleware 경계는 `internal/agent` 안의 provider-neutral client decorator로 둔다. `internal/llm`은 provider 호출
contract만 계속 소유하고, middleware 등록·순서·오류 분류는 Single Agent 실행 책임이므로 `internal/agent`가
소유한다. Provider 구현이나 `llm.ChatRequest`에 middleware 필드를 추가하지 않는다.

Structured output validator도 `internal/agent`의 Runner 내부 구성요소로 둔다. Runner 생성 시 schema를 한 번 compile해
재사용하고, final assistant text만 instance로 파싱·검증한다. 외부 library 타입은 공개 Runner API에 노출하지 않고
입력과 결과는 `json.RawMessage`로 유지한다. Schema가 없으면 validator를 만들지 않아 기존 일반 text 실행 경로에
영향을 주지 않는다(`SPEC §5.6`~`SPEC §5.8`).

CLI 조립은 `cmd/agent-runtime`이 소유한다. 현재 작업 디렉터리를 File Read, File Save, Code Execution Tool의 공통
허용 root로 사용하고, calculator와 Phase 4.1 Tool을 하나의 registry에 등록한다. Runtime 패키지는 특정 Tool 묶음이나
작업 디렉터리를 알지 않는다(`SPEC §5.9`).

## 2. 데이터 흐름

Runner 생성 시 `RunnerOptions`의 client 존재 여부와 실행 값을 확인한다. Output schema가 있으면 JSON 문서로 파싱한
뒤 in-memory resource로 compiler에 등록하고 compile한다. Schema 문법, 지원하지 않는 dialect, 해석할 수 없는 `$ref`
같은 compile 실패는 model 호출 전 configuration 성격의 structured output 오류로 반환한다. 기본 dialect는 Draft
2020-12로 두고, schema가 지원되는 `$schema`를 명시하면 해당 dialect를 따른다.

Phase 4.2 schema는 self-contained 문서로 제한한다. 같은 문서 안의 `$defs`와 local reference는 허용하지만 file 또는
HTTP(S)에서 외부 schema를 불러오지 않는다. Runner 생성이 호출자 제공 schema 때문에 예기치 않은 filesystem 또는
network I/O를 수행하지 않게 하는 경계이며, `SPEC §5.6`, `SPEC §5.7`의 구조 검증에는 외부 resource가 필요하지 않다.

Run은 기존 Agent처럼 user message를 상태에 넣고 step을 시작한다. Agent가 만든 `llm.ChatRequest`는 registry schema와
누적 메시지를 가진 채 middleware client로 전달된다. 각 `pre-model` hook은 앞 hook이 반환한 요청을 받아 등록 순서대로
변경하고, 최종 요청이 provider에 전달된다. Hook이 요청에만 추가한 system message 같은 값은 model 요청에는 반영되지만
Agent의 대화 상태를 직접 변경하지 않는다(`SPEC §5.3`).

Provider 호출에는 Runner의 model timeout으로 만든 자식 context를 적용한다. 이 timeout은 model HTTP 호출에만 적용하고
Runner 호출자가 전달한 context는 전체 run 취소를 계속 제어한다. Provider 응답이 오면 각 `post-model` hook이 최종
pre-model 요청과 앞 hook이 반환한 응답을 받아 같은 등록 순서로 처리한다. 마지막 응답이 Agent에 돌아가 메시지 누적,
Tool 실행 여부, final 여부를 결정한다(`SPEC §5.4`).

Middleware가 오류를 반환하면 decorator는 stage, middleware 이름, 원인을 포함한 typed 오류를 반환한다. Pre 오류에서는
provider를 호출하지 않고, post 오류에서는 이미 끝난 provider 호출의 응답을 Agent 상태에 append하지 않는다. Agent는
오류 상태와 middleware 전용 trace action을 기록하고 loop를 종료하므로 오류 이후 Tool 또는 model 호출이 없다
(`SPEC §5.5`).

Agent가 Tool call을 반환받으면 기존 registry lookup, validation, timeout, result message 누적 흐름을 그대로 사용한다.
누적된 Tool result를 포함한 다음 model 요청도 전체 middleware chain과 model timeout을 다시 통과한다. Tool 오류는
기존처럼 오류 Tool result로 LLM에 전달되고 Runner process 오류로 승격하지 않는다(`SPEC §5.2`).

Agent가 final 상태로 끝나고 output schema가 없으면 Runner는 상태를 그대로 반환한다. Schema가 있으면 final text의
앞뒤 공백만 제거한 원문을 엄격한 JSON 문서로 파싱한다. Markdown code fence 제거, JSON substring 추출, 자동 수정은
하지 않는다. 검증 성공 시 같은 JSON bytes를 `RunnerResult.StructuredOutput`에 보존하고 final 상태와 `FinalAnswer`를
유지한다(`SPEC §5.6`).

JSON 파싱이나 schema validation이 실패하면 마지막 assistant message는 진단을 위해 `AgentState.Messages`에 남기되,
`FinalAnswer`는 비우고 상태를 error로 바꾼다. `LastError`에는 typed structured output 오류를 넣고 structured output
전용 trace action을 추가한다. 이 결과는 일반 LLM 오류와 `errors.As` 또는 오류 kind helper로 구분되고 일반 text
성공으로 처리되지 않는다(`SPEC §5.7`).

CLI는 기존 prompt 검증과 config/client 생성 뒤 현재 작업 디렉터리를 구한다. 다섯 Tool을 registry에 등록하고 Runner를
만든 뒤 사용자 입력 하나를 실행한다. Final 상태면 `FinalAnswer`를 stdout에 한 번 출력한다. Error, max steps,
needs action처럼 final이 아닌 상태는 stdout을 비워 두고 상태와 원인을 stderr에 기록한 뒤 0이 아닌 코드로 종료한다
(`SPEC §5.9`, `SPEC §5.10`).

CLI의 `LLM_TIMEOUT`은 각 model 호출 timeout으로 Runner에 전달한다. 전체 loop는 Runner context cancellation과 CLI의
양수 max step 기본값으로 제한하고, Tool 호출은 기존 Agent 기본 Tool timeout을 사용한다. 이 경계는 여러 model 호출
전체에 하나의 `LLM_TIMEOUT` 예산을 나누는 대신 기존 환경변수의 model 호출 의미를 보존한다.

## 3. 인터페이스

Runner 공개 경계는 기존 `Agent.Options`와 `AgentState`를 재사용하되 middleware와 structured output을 포함하는 별도
타입으로 둔다. 구체적인 필드 기준은 다음과 같다.

```go
type RunnerOptions struct {
	Client        llm.LLMClient
	Model         string
	MaxSteps      int
	ModelTimeout  time.Duration
	Tools         *tool.Registry
	ToolTimeout   time.Duration
	Middleware    []ModelMiddleware
	OutputSchema  json.RawMessage
}

type RunnerResult struct {
	State            AgentState
	StructuredOutput json.RawMessage
}

func NewRunner(opts RunnerOptions) (*Runner, error)
func (r *Runner) Run(ctx context.Context, input string) RunnerResult
```

`Runner.Run`은 기존 Agent의 상태 중심 오류 contract를 따른다. 실행 중 오류는 `RunnerResult.State.Status`, `LastError`,
trace에 남고 별도 `error` 반환값을 중복해서 두지 않는다. Runner 생성에 필요한 client나 schema 자체가 잘못된 경우만
`NewRunner`가 오류를 반환한다(`SPEC §5.1`, `SPEC §5.5`, `SPEC §5.7`).

Middleware는 상태를 가진 구현체 계층을 강제하지 않고 함수 hook을 묶은 값으로 표현한다.

```go
type PreModelHook func(context.Context, llm.ChatRequest) (llm.ChatRequest, error)
type PostModelHook func(context.Context, llm.ChatRequest, llm.ChatResponse) (llm.ChatResponse, error)

type ModelMiddleware struct {
	Name      string
	PreModel  PreModelHook
	PostModel PostModelHook
}
```

`Name`은 middleware 오류 식별에 사용하고 비어 있으면 Runner 생성 오류로 거부한다. Pre 또는 post 중 하나만 필요한
middleware는 사용하지 않는 hook을 nil로 둘 수 있지만 두 hook이 모두 nil인 값은 거부한다. Hook closure로 상태를
보존할 수 있으므로 별도 interface와 no-op method를 요구하지 않는다.

Decorator는 hook에 넘기기 전에 요청의 message, tool schema, tool call argument 같은 slice와 `json.RawMessage`를
복제한다. Hook이 반환한 값은 다음 hook과 provider에 전달하지만 Agent가 이미 소유한 메시지나 registry schema를 alias로
변경하지 않는다. Post hook에는 모든 pre hook이 끝난 실제 provider 요청을 함께 전달한다.

Runner 전용 오류는 `internal/agent`의 typed error로 둔다. 최소 kind는 `middleware`와 `structured_output`이며,
middleware 오류는 stage와 middleware 이름을, structured output 오류는 schema compile, JSON parse, validation 중 실패한
operation을 보존한다. `Unwrap`과 kind helper를 제공해 기존 `llm.Error`와 구분하면서 원인 오류도 검사할 수 있게 한다
(`SPEC §5.5`, `SPEC §5.7`).

Structured output validator의 공개 입력은 `json.RawMessage` schema뿐이다. 구현은
`github.com/santhosh-tekuri/jsonschema/v6`의 compiler와 compiled schema를 Runner 내부에 보존하되 외부 package 타입을
`RunnerOptions`나 `RunnerResult`에 노출하지 않는다. Final JSON도 library 전용 값으로 다시 직렬화하지 않고 검증한
원문 bytes를 복제해 반환한다.

CLI에는 structured output flag, middleware flag, Tool 선택 flag를 새로 추가하지 않는다. 기존 positional args/stdin
입력 contract는 유지하고, 실행 엔진만 Runner로 교체한다. CLI용 Runner는 진입점에서 positive max step 기본값과 현재
config의 `LLM_TIMEOUT`, `TAVILY_API_KEY`, 현재 작업 디렉터리를 조립한다.

## 4. 영향 범위

`internal/agent`가 주 변경 대상이다. 새 Runner, model middleware decorator, structured output validator와 테스트가
추가된다. 기존 `agent.go`는 middleware와 structured output 오류를 정확한 trace action으로 기록하고 Runner가 final
검증 실패 상태를 만들 수 있도록 좁게 확장한다. 기존 `Agent`, `Options`, `Run` public contract는 유지한다.
Runner 테스트는 stub LLM client와 함수 hook으로 순서, 요청·응답 변경, 오류 이후 호출 중단, structured output
성공·실패를 외부 provider 없이 확인한다(`SPEC §5.11`).

`go.mod`와 새 `go.sum`에는 구현 단계에서 `github.com/santhosh-tekuri/jsonschema/v6@v6.0.2`가 직접 의존성으로
추가된다. 해당 module의 `go.mod` 기준 runtime 의존성으로 `golang.org/x/text`가 들어올 수 있다. JSON Schema 기능을
직접 불완전하게 구현하지 않고 `SPEC §5.6`, `SPEC §5.7`의 표준 schema 검증을 제공하기 위한 제한된 의존성이다.

`cmd/agent-runtime/main.go`는 client-only 조립에서 Tool registry와 Runner 조립으로 바뀐다. 현재 작업 디렉터리 조회,
다섯 Tool 생성·등록, Runner 상태별 stdout/stderr와 종료 코드 처리가 추가된다. `main_test.go`는 단발 request 단언을
Runner의 반복 request, Tool schema, Tool result, final 출력 단언으로 갱신한다. Stub client가 Tool call과 final 응답을
순서대로 반환하게 해 실제 외부 provider 없이 CLI loop를 확인한다(`SPEC §5.11`).

`internal/config`와 `.env.example`은 변경하지 않는다. CLI max step은 `cmd`의 명시적인 양수 기본값으로 두고,
`LLM_TIMEOUT`은 Runner의 model timeout에 매핑한다. 새 환경변수를 추가하는 것은 Phase 4.2 명세의 관찰 조건에 필요하지
않다.

`internal/llm`, `internal/message`, `internal/tool`의 public contract와 Claude·Ollama provider wire format은 변경하지
않는다. Middleware는 LLMClient decorator로 구현하고 structured output은 final 응답 이후 검증하므로 provider native
output schema 필드가 필요하지 않다.

외부 저장소나 DB는 추가하지 않는다. Structured output schema와 compiled validator는 Runner 메모리 안에만 있고,
CLI Tool의 파일 root는 실행 시 현재 작업 디렉터리로 정해진다.

## 5. Decision Points

1. Runner와 기존 Agent의 책임 경계
   - 옵션 A: 새 Runner가 middleware client와 validator를 조립하고 기존 Agent를 내부 loop 엔진으로 사용한다.
   - 옵션 B: middleware, schema, structured output 상태를 모두 기존 `Agent.Options`와 `Agent.Run`에 넣는다.
   - 옵션 C: 기존 Agent를 Runner로 이름 변경하고 호출자를 모두 전환한다.
   - trade-off: A는 공개 Agent contract를 보존하면서 상위 실행 경계를 명확히 하지만 옵션 필드가 일부 중복된다. B는
     타입 수가 적지만 loop 책임과 실행 조립 책임이 섞인다. C는 이름은 수렴하지만 Phase 2·3 API를 불필요하게 깨뜨린다.
   - 채택안: 옵션 A.
   - 근거: `SPEC §5.1`, `SPEC §5.8`은 기존 loop를 보존하는 Runner 경계를 요구하고, Phase 4.3은 같은 Runner 위에
     streaming을 추가해야 한다.

2. Middleware 적용 위치와 표현
   - 옵션 A: `ModelMiddleware` 함수 hook 목록을 사용하는 LLMClient decorator를 Runner가 Agent에 주입한다.
   - 옵션 B: Agent loop 안에 pre/post 분기를 직접 넣는다.
   - 옵션 C: `internal/llm` provider client마다 middleware를 구현한다.
   - trade-off: A는 모든 provider와 모든 loop step에 한 경계를 재사용하고 hook closure도 허용한다. B는 trace 접근은
     쉽지만 Agent loop가 횡단 관심사를 직접 소유한다. C는 provider 중복과 동작 차이를 만든다.
   - 채택안: 옵션 A.
   - 근거: `SPEC §5.3`~`SPEC §5.5`는 provider-neutral 순서와 오류 중단을 요구한다.

3. Structured output 실패 상태
   - 옵션 A: Runner가 final text를 검증하고 실패하면 AgentState를 error로 전환하며 raw assistant message만 보존한다.
   - 옵션 B: AgentState는 final로 두고 `RunnerResult`의 별도 error만 반환한다.
   - 옵션 C: 실패한 JSON도 일반 text final answer로 반환한다.
   - trade-off: A는 기존 상태 중심 contract에서 성공과 실패가 하나로 수렴하고 진단 원문도 남는다. B는 같은 실행이
     final과 error를 동시에 가져 호출자가 두 상태를 해석해야 한다. C는 schema 불일치를 성공으로 숨긴다.
   - 채택안: 옵션 A.
   - 근거: `SPEC §5.7`, `SPEC §5.10`은 structured output 실패와 CLI 비정상 종료를 명확히 구분해야 한다.

4. JSON Schema validator
   - 옵션 A: `github.com/santhosh-tekuri/jsonschema/v6@v6.0.2`를 Runner 내부 구현 의존성으로 사용한다.
   - 옵션 B: Phase 4.2에서 필요한 `type`, `required`, `properties` 같은 일부 keyword만 직접 구현한다.
   - 옵션 C: validator interface를 Runner 호출자에게 주입하고 Runtime은 schema 검증을 제공하지 않는다.
   - trade-off: A는 새 module과 `go.sum`이 생기지만 여러 draft와 표준 test suite 기반 검증을 제공한다. B는 의존성이
     없지만 지원 범위가 암묵적인 자체 schema dialect가 된다. C는 core가 단순하지만 명세 책임을 호출자에게 넘긴다.
   - 채택안: 옵션 A. 기본 Draft 2020-12, 지원되는 `$schema` dialect 선택, self-contained schema만 허용한다.
   - 근거: `SPEC §5.6`, `SPEC §5.7`은 단순 JSON 파싱이 아니라 JSON Schema 검증을 Runtime이 제공하도록 요구한다.

5. Structured output 검증 시점
   - 옵션 A: Runner 생성 시 schema를 compile하고 final 응답마다 compiled schema로 instance만 검증한다.
   - 옵션 B: 매 run 또는 final 응답마다 schema를 다시 compile한다.
   - 옵션 C: Runner 생성은 허용하고 첫 structured run에서 지연 compile한다.
   - trade-off: A는 잘못된 schema를 model 호출 전에 발견하고 validator를 재사용한다. B는 구현은 직선적이지만 반복
     비용과 늦은 설정 실패가 있다. C는 schema를 사용하지 않는 run의 비용을 미루지만 Runner 설정 오류도 늦어진다.
   - 채택안: 옵션 A.
   - 근거: schema는 Runner 구성이고 model output은 run 결과이므로 configuration 실패와 validation 실패 경계를
     분리하는 것이 `SPEC §5.6`, `SPEC §5.7`을 가장 명확하게 만족한다.

6. CLI Tool root와 조립 위치
   - 옵션 A: `cmd/agent-runtime`이 현재 작업 디렉터리를 root로 사용해 Phase 3·4.1 Tool 전체를 등록한다.
   - 옵션 B: 새 `TOOL_ROOT` 환경변수를 추가한다.
   - 옵션 C: 실행 파일 위치나 repository root를 자동 탐색한다.
   - trade-off: A는 `.env`를 현재 작업 디렉터리에서 읽는 기존 CLI 동작과 맞고 새 config가 없다. B는 명시적이지만
     spec 밖의 설정 contract가 늘어난다. C는 실행 위치에 따라 예측하기 어렵고 repository가 아닌 실행을 처리해야 한다.
   - 채택안: 옵션 A.
   - 근거: `SPEC §5.9`는 진입점 조립을, spec 제약은 기존 File/Code Tool root 제한 유지를 요구한다.

7. CLI model timeout과 loop 제한
   - 옵션 A: `LLM_TIMEOUT`을 각 model 호출에 적용하고, 전체 loop는 positive max step 기본값과 caller context로 제한한다.
   - 옵션 B: `LLM_TIMEOUT` 하나를 전체 Runner run deadline으로 사용한다.
   - 옵션 C: model timeout과 run timeout 환경변수를 새로 분리한다.
   - trade-off: A는 기존 변수 의미를 보존하고 여러 step이 각각 같은 provider 예산을 갖는다. B는 총 실행 시간은 짧게
     제한하지만 Tool 사용 시간이 다음 model 호출 예산을 소모한다. C는 가장 명확하지만 새 config 범위를 추가한다.
   - 채택안: 옵션 A. CLI max step 기본값은 10 model 호출로 둔다.
   - 근거: `SPEC §5.1`, `SPEC §5.9`, `SPEC §5.10`은 실행 제한과 기존 config 사용을 요구하지만 새 timeout 설정은
     요구하지 않는다.
