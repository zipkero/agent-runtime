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
- `Agent.executeToolCall`은 `Tool.Execute`를 goroutine으로 시작하고 timeout 시 종료를 기다리지 않는다.
  `go test -race ./...`에서는 timeout test가 Agent 반환 후 Tool 상태와 경쟁하는 상황이 재현된다.
- File Read는 정리된 path 문자열만 root 내부인지 확인한 뒤 `os.ReadFile`을 사용하므로 root 내부 symbolic link가
  root 밖을 가리키는 경로를 읽을 수 있다. File Save는 symbolic link 검증 전에 `os.MkdirAll`을 실행하므로 실패 전에
  root 밖에 parent directory를 만들 수 있다.
- `MaxSteps`는 model 호출 수만 제한하고 한 응답의 Tool call 수, File Read·Web Search result 크기, 전체 run 시간은
  제한하지 않는다. Claude와 Ollama adapter는 provider의 raw stop reason을 보존하지만 Agent는 이를 상태 전이에 사용하지
  않는다.
- Code Execution은 `go run`과 `go test`를 허용하고 `os.Environ()` 전체를 자식 process에 전달한다. 현재 CLI는 아직
  Tool을 등록하지 않지만 Task 004의 기존 계획은 모든 Tool을 기본 등록하도록 되어 있다.
- Config는 parse 가능한 `LLM_TIMEOUT`이면 0이나 음수도 허용한다. 현재 CLI의 `context.WithTimeout`과 Runner의
  `ModelTimeout > 0` 조건은 비양수 값을 서로 다르게 해석한다.
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
상위 실행 경계로 두고, Runner 생성 시 model 호출 옵션을 비공개 Agent 생성 경계로 전달하고 선택적인 structured output
validator를 조립한 뒤 기존 Agent를 내부에서 생성한다. 이렇게 하면 Phase 2·3에서 확정된 `Agent.Run` contract를 깨지
않고 `SPEC §5.1`, `SPEC §5.2`, `SPEC §5.8`을 만족할 수 있다.

```text
Caller / CLI
→ Runner
  → Agent loop
    → pre-model hooks
    → per-model timeout context
    → LLMClient
    → post-model hooks
    → Tool registry / Tool execution
  → optional structured output validator
→ RunnerResult
```

Runner는 `RunnerOptions`로 client, model, max step, model timeout, Tool registry, Tool timeout, middleware 목록,
Tool 호출·result 제한, 선택적인 output schema를 받는다. Runner는 model timeout과 middleware 목록, 실행 제한을 Agent
실행 옵션으로 전달하고, Agent loop는 각 model 요청마다 `pre-model`, timeout이 적용된 `LLMClient.Chat`, `post-model`을
명시적인 순서로 호출한다. Tool loop의 두 번째 이후 호출도 같은 loop 본문을 다시 지나므로 middleware가 빠지지 않는다
(`SPEC §5.2`~`SPEC §5.5`).

Middleware의 순회와 typed error 생성은 `internal/agent/middleware.go`의 provider-neutral helper가 소유하고, Agent loop는
요청 상태 분리, 적용 시점과 실패 상태 전이를 소유한다. `internal/llm`은 provider 호출 contract만 계속 소유하며 Provider
구현이나 `llm.ChatRequest`에는 middleware 필드를 추가하지 않는다. 이 경계는 실제 provider client를 middleware 실행
객체처럼 포장하지 않으면서 model 호출 전후 흐름을 코드에 드러낸다.

Structured output validator도 `internal/agent`의 Runner 내부 구성요소로 둔다. Runner 생성 시 schema를 한 번 compile해
재사용하고, final assistant text만 instance로 파싱·검증한다. 외부 library 타입은 공개 Runner API에 노출하지 않고
입력과 결과는 `json.RawMessage`로 유지한다. Schema가 없으면 validator를 만들지 않아 기존 일반 text 실행 경로에
영향을 주지 않는다(`SPEC §5.6`~`SPEC §5.8`).

CLI 조립은 `cmd/agent-runtime`이 소유한다. 현재 작업 디렉터리를 File Read, File Save와 활성화된 Code Execution
Tool의 공통 허용 root로 사용하고, calculator, File Read, Web Search, File Save를 기본 registry에 등록한다. Code
Execution은 `ENABLE_CODE_EXECUTION=true`일 때만 추가한다. Runtime 패키지는 CLI의 Tool 선택이나 작업 디렉터리를 알지
않는다(`SPEC §5.9`, `SPEC §5.16`).

## 2. 데이터 흐름

Runner 생성 시 `RunnerOptions`의 client 존재 여부와 실행 값을 확인한다. Output schema가 있으면 JSON 문서로 파싱한
뒤 in-memory resource로 compiler에 등록하고 compile한다. Schema 문법, 지원하지 않는 dialect, 해석할 수 없는 `$ref`
같은 compile 실패는 model 호출 전 configuration 성격의 structured output 오류로 반환한다. 기본 dialect는 Draft
2020-12로 두고, schema가 지원되는 `$schema`를 명시하면 해당 dialect를 따른다.

Phase 4.2 schema는 self-contained 문서로 제한한다. 같은 문서 안의 `$defs`와 local reference는 허용하지만 file 또는
HTTP(S)에서 외부 schema를 불러오지 않는다. Runner 생성이 호출자 제공 schema 때문에 예기치 않은 filesystem 또는
network I/O를 수행하지 않게 하는 경계이며, `SPEC §5.6`, `SPEC §5.7`의 구조 검증에는 외부 resource가 필요하지 않다.

Run은 기존 Agent처럼 user message를 상태에 넣고 step을 시작한다. Agent는 누적 메시지를 깊게 복제하고, Tool registry가
복제해 반환한 schema를 인수해 Agent 상태와 분리된 `llm.ChatRequest`를 만든 뒤 `pre-model` helper를 호출한다. 각 hook은
앞 hook이 반환한 요청을 받아 등록 순서대로 변경하고, 최종 요청이 provider에 전달된다. Hook이 요청에만 추가한 system
message 같은 값은 model 요청에는 반영되지만 Agent의 대화 상태를 직접 변경하지 않는다(`SPEC §5.3`).

Agent는 pre-model 처리가 끝난 뒤 LLM 요청 trace를 기록하고, Runner에서 주입된 model timeout으로 자식 context를 만들어
`LLMClient.Chat`을 호출한다. 이 timeout은 provider 호출에만 적용하고 Runner 호출자가 전달한 context는 전체 run 취소를
계속 제어한다. Provider 응답이 오면 Agent는 최종 pre-model 요청과 응답을 `post-model` helper에 전달한다. 각 hook은 앞
hook의 응답 변경을 등록 순서대로 이어받고, 마지막 응답이 메시지 누적, Tool 실행 여부, final 여부를 결정한다(`SPEC §5.4`).
Agent는 마지막 assistant 응답마다 `ToolCalls` 편의 상태를 다시 계산하므로 final 응답에서는 이전 step의 호출이 남지 않는다.

Middleware가 오류를 반환하면 helper는 stage, middleware 이름, 원인을 포함한 typed 오류를 반환한다. Agent loop는 pre
오류에서 provider를 호출하지 않고, post 오류에서는 이미 끝난 provider 호출의 응답을 상태에 append하지 않는다. 두
경우 모두 오류 상태와 middleware 전용 trace action을 기록하고 즉시 종료하므로 오류 이후 Tool 또는 model 호출이 없다
(`SPEC §5.5`).

Agent가 Tool call을 반환받으면 Tool 호출 시도 수를 증가시키고 최대 20회를 넘기기 전에 실행을 중단한다. Tool은 timeout
context를 받아 동기 실행되며 `Execute`가 반환된 뒤에만 다음 상태로 전이한다. 이 timeout은 cooperative cancellation
계약이므로 Tool 구현은 context 취소를 관찰하고 반환해야 한다. 제한 안의 Tool 오류는 기존처럼 오류 Tool result로 LLM에
전달하고, 호출 수나 전체 deadline 초과는 실행 제한 typed error와 전용 trace를 남긴 뒤 run을 중단한다
(`SPEC §5.2`, `SPEC §5.12`, `SPEC §5.14`).

Tool result는 64KiB를 넘으면 다음 model 요청에 그대로 누적하지 않고 크기 제한 오류 result로 바꾼다. File Read와 Web
Search는 전체 payload를 먼저 메모리에 적재하지 않도록 읽기 경계에서도 같은 상한을 적용한다. File Read와 File Save는
Go 1.26 표준 `os.Root` 연산으로 실제 filesystem 경로를 root 안에 가두고, symbolic link 검사와 사용 사이의 별도
문자열 검사를 보안 경계로 사용하지 않는다(`SPEC §5.13`, `SPEC §5.14`).

Post-model 처리가 끝난 응답은 provider-neutral finish reason을 기준으로 분기한다. 정상 완료 또는 기존 custom client의
빈 값은 Tool call 유무에 따라 기존 final·Tool 흐름을 따른다. `length_limit`, `blocked`, `unknown`은 assistant 원문을
진단용 메시지로 보존하되 `FinalAnswer`를 비우고 incomplete response typed error와 trace를 남긴다. 불완전 응답의 Tool
call도 실행하지 않는다(`SPEC §5.15`).

Agent가 final 상태로 끝나고 output schema가 없으면 Runner는 상태를 그대로 반환한다. Schema가 있으면 final text의
앞뒤 공백만 제거한 원문을 엄격한 JSON 문서로 파싱한다. Markdown code fence 제거, JSON substring 추출, 자동 수정은
하지 않는다. 검증 성공 시 같은 JSON bytes를 `RunnerResult.StructuredOutput`에 보존하고 final 상태와 `FinalAnswer`를
유지한다(`SPEC §5.6`).

JSON 파싱이나 schema validation이 실패하면 마지막 assistant message는 진단을 위해 `AgentState.Messages`에 남기되,
`FinalAnswer`는 비우고 상태를 error로 바꾼다. `LastError`에는 typed structured output 오류를 넣고 structured output
전용 trace action을 추가한다. 이 결과는 일반 LLM 오류와 `errors.As` 또는 오류 kind helper로 구분되고 일반 text
성공으로 처리되지 않는다(`SPEC §5.7`).

CLI는 기존 prompt 검증과 config/client 생성 뒤 현재 작업 디렉터리를 구한다. 네 기본 Tool을 registry에 등록하고
`ENABLE_CODE_EXECUTION=true`이면 Code Execution을 추가한 뒤 Runner를 만든다. Code Execution 자식 process에는
`PATH`, `TMPDIR`, `GOROOT`, `GOCACHE`, `GOMODCACHE`, `GOPATH`, `GOOS`, `GOARCH`, `CGO_ENABLED` 중 현재 process에
존재하는 값과 강제된 `GOWORK=off`만 전달한다. Final 상태면 `FinalAnswer`를 stdout에 한 번 출력하고, final이 아닌 상태는
stdout을 비워 둔 채 stderr와 0이 아닌 코드로 종료한다(`SPEC §5.9`, `SPEC §5.10`, `SPEC §5.16`).

CLI의 `LLM_TIMEOUT`은 각 model 호출 timeout으로 Runner에 전달한다. 전체 loop는 Runner context cancellation과 CLI의
양수 max step 기본값, 최대 20회의 Tool 호출, result당 64KiB, CLI가 생성한 10분 context deadline으로 제한한다. Config는
0 이하 `LLM_TIMEOUT`을 거부하고 Tool 호출은 기존 Agent 기본 Tool timeout을 사용한다. 이 경계는 model 호출별 예산과
전체 run 예산을 분리한다(`SPEC §5.14`, `SPEC §5.17`).

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
	MaxToolCalls  int
	MaxToolResultBytes int
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

`MaxToolCalls`와 `MaxToolResultBytes`는 0이면 각각 20과 64KiB 기본값을 적용하고 음수이면 Runner 생성 오류로 거부한다.
같은 제한은 기존 `Agent.Options`에도 추가해 Runner를 통하지 않는 Agent 실행이 무제한 경로로 남지 않게 한다. 전체 run
시간은 새 duration 필드 대신 `Run`의 context deadline이 소유하고 CLI는 10분 deadline을 명시적으로 적용한다.

Middleware는 상태를 가진 구현체 계층을 강제하지 않고 함수 hook을 묶은 값으로 표현한다. Runner는 선택적인 model
timeout과 middleware 목록을 비공개 Agent 생성 경계로 전달해 기존 `Agent.Options` contract를 확장하지 않는다.

```go
type PreModelHook func(context.Context, llm.ChatRequest) (llm.ChatRequest, error)
type PostModelHook func(context.Context, llm.ChatRequest, llm.ChatResponse) (llm.ChatResponse, error)

type ModelMiddleware struct {
	Name      string
	PreModel  PreModelHook
	PostModel PostModelHook
}
```

`Name`은 middleware 오류 식별에 사용한다. 비어 있거나 앞뒤 공백이 있는 이름과 중복 이름은 Runner 생성 오류로 거부한다.
Pre 또는 post 중 하나만 필요한 middleware는 사용하지 않는 hook을 nil로 둘 수 있지만 두 hook이 모두 nil인 값은
거부한다. Hook closure로 상태를 보존할 수 있으므로 별도 interface와 no-op method를 요구하지 않는다.

Agent는 model 호출용 요청을 만들 때 message와 그 안의 tool call argument, tool result 같은 중첩 참조값을 복제한다.
Tool schema는 `Registry.Schemas`가 registry 상태와 분리해 반환한 값을 요청이 그대로 인수한다. 각 hook은 현재 작업값을
받아 변경된 요청이나 응답을 반환하고, helper는 반환값을 다시 복제하지 않고 등록 순서대로 다음 hook에 전달한다. 값
구조체 안의 slice나 pointer를 통한 중첩 변경은 작업값에 허용하며, 최초 깊은 복사가 Agent 상태와 registry 원본으로 변경이
번지는 것을 막는다. 마지막 응답의 소유권은 Agent로 이전되고 Agent는 이 응답을 상태와 다음 판단에 사용한다. Hook은
반환해 소유권을 이전한 요청이나 응답의 내부 참조를 보관하거나 이후 변경하지 않는다. Post hook에는 모든 pre hook이 끝난
실제 provider 요청을 읽기 전용으로 전달한다.

`LLMClient.Chat`도 요청을 읽기 전용으로 사용하고 참조를 보관하지 않으며, 반환한 응답의 소유권을 호출자에게 이전한다.
이 계약으로 Agent와 provider 사이에 추가 복사 없이 동일한 model 요청과 최종 응답을 순차 전달한다.

Runner 전용 오류는 `internal/agent`의 typed error로 둔다. 최소 kind는 `middleware`, `structured_output`,
`execution_limit`, `incomplete_response`이며,
middleware 오류는 stage와 middleware 이름을, structured output 오류는 schema compile, JSON parse, validation 중 실패한
operation을 보존한다. 실행 제한 오류는 초과한 제한 이름과 현재 값을, incomplete response 오류는 정규화된 finish reason과
provider raw stop reason을 보존한다. `Unwrap`과 kind helper를 제공해 기존 `llm.Error`와 구분하면서 원인 오류도 검사할 수
있게 한다(`SPEC §5.5`, `SPEC §5.7`, `SPEC §5.14`, `SPEC §5.15`).

`llm.ChatResponse`에는 provider raw `StopReason`을 유지하면서 `FinishReason`을 추가한다. 값은 `complete`, `tool_call`,
`length_limit`, `blocked`, `unknown`으로 제한하고 Claude와 Ollama adapter가 각 provider 값을 정규화한다. 빈 값은 custom
`LLMClient` 호환을 위해 Agent 경계에서 `complete`로 해석한다.

Structured output validator의 공개 입력은 `json.RawMessage` schema뿐이다. 구현은
`github.com/santhosh-tekuri/jsonschema/v6`의 compiler와 compiled schema를 Runner 내부에 보존하되 외부 package 타입을
`RunnerOptions`나 `RunnerResult`에 노출하지 않는다. Final JSON도 library 전용 값으로 다시 직렬화하지 않고 검증한
원문 bytes를 복제해 반환한다.

CLI에는 structured output flag나 middleware flag를 새로 추가하지 않는다. 기존 positional args/stdin 입력 contract는
유지하고, Code Execution만 `ENABLE_CODE_EXECUTION` 환경변수로 opt-in한다. CLI용 Runner는 진입점에서 max step 10,
max Tool call 20, Tool result 64KiB, 전체 run 10분과 현재 config의 `LLM_TIMEOUT`, `TAVILY_API_KEY`, 현재 작업
디렉터리를 조립한다.

## 4. 영향 범위

`internal/agent`가 주 변경 대상이다. 새 Runner, model middleware helper, structured output validator와 테스트가 추가된다.
기존 `agent.go`는 model timeout과 middleware를 선택적으로 주입받아 model 호출 전후 순서와 middleware 오류 상태 전이를
직접 소유하고, Runner가 final 검증 실패 상태를 만들 수 있도록 확장한다. 기존 `Agent`, `Options`, `Run` 사용 방식과
middleware 미지정 동작은 유지한다.
Runner 테스트는 stub LLM client와 함수 hook으로 순서, 요청·응답 변경, 오류 이후 호출 중단, structured output
성공·실패를 외부 provider 없이 확인한다(`SPEC §5.11`).

`go.mod`와 새 `go.sum`에는 구현 단계에서 `github.com/santhosh-tekuri/jsonschema/v6@v6.0.2`가 직접 의존성으로
추가된다. 해당 module의 `go.mod` 기준 runtime 의존성으로 `golang.org/x/text`가 들어올 수 있다. JSON Schema 기능을
직접 불완전하게 구현하지 않고 `SPEC §5.6`, `SPEC §5.7`의 표준 schema 검증을 제공하기 위한 제한된 의존성이다.

`cmd/agent-runtime/main.go`는 client-only 조립에서 Tool registry와 Runner 조립으로 바뀐다. 현재 작업 디렉터리 조회,
네 기본 Tool과 opt-in Code Execution 등록, 10분 run context, Runner 상태별 stdout/stderr와 종료 코드 처리가 추가된다.
`main_test.go`는 단발 request 단언을 Runner의 반복 request, Tool schema, Tool result, 제한과 final 출력 단언으로
갱신한다. Stub client가 Tool call과 final 응답을 순서대로 반환하게 해 외부 provider 없이 CLI loop를 확인한다
(`SPEC §5.11`, `SPEC §5.14`, `SPEC §5.16`).

`internal/config`와 `.env.example`에는 기본 false인 `ENABLE_CODE_EXECUTION`을 추가하고 `LLM_TIMEOUT` 양수 검증을
적용한다. CLI max step, Tool 호출·result 제한, 전체 run timeout은 `cmd`의 명시적인 기본값으로 두며 새 환경변수로
노출하지 않는다.

`internal/llm`은 provider wire format을 바꾸지 않고 `ChatResponse`에 정규화된 finish reason을 추가한다.
`internal/message`의 타입 형태는 유지한다. `LLMClient`에는 읽기 전용 요청과 응답 소유권 이전 계약을 명시하고, Tool
registry에는 schema를 생성하지 않고 등록 수를 확인하는 `Len`을 추가한다. Middleware는 Agent loop가 provider-neutral
helper로 적용하고 structured output은 final 응답 이후 검증하므로 provider native output schema 필드가 필요하지 않다.

외부 저장소나 DB는 추가하지 않는다. Structured output schema와 compiled validator는 Runner 메모리 안에만 있고,
CLI Tool의 파일 root는 실행 시 현재 작업 디렉터리로 정해진다.

## 5. Decision Points

1. Runner와 기존 Agent의 책임 경계
   - 옵션 A: 새 Runner가 model 호출 옵션을 비공개 생성 경계로 주입하고 validator를 조립해 기존 Agent를 loop 엔진으로 사용한다.
   - 옵션 B: middleware, schema, structured output 상태를 모두 기존 `Agent.Options`와 `Agent.Run`에 넣는다.
   - 옵션 C: 기존 Agent를 Runner로 이름 변경하고 호출자를 모두 전환한다.
   - trade-off: A는 공개 Agent contract를 보존하면서 상위 실행 경계를 명확히 하지만 옵션 필드가 일부 중복된다. B는
     타입 수가 적지만 loop 책임과 실행 조립 책임이 섞인다. C는 이름은 수렴하지만 Phase 2·3 API를 불필요하게 깨뜨린다.
   - 채택안: 옵션 A.
   - 근거: `SPEC §5.1`, `SPEC §5.8`은 기존 loop를 보존하는 Runner 경계를 요구하고, Phase 4.3은 같은 Runner 위에
     streaming을 추가해야 한다.

2. Middleware 적용 위치와 표현
   - 옵션 A: Agent loop가 `ModelMiddleware` helper를 호출해 pre-model, `LLMClient.Chat`, post-model 순서를 직접 소유한다.
   - 옵션 B: `ModelMiddleware` 함수 hook 목록을 사용하는 LLMClient decorator를 Runner가 Agent에 주입한다.
   - 옵션 C: `internal/llm` provider client마다 middleware를 구현한다.
   - trade-off: A는 실행 순서, timeout 범위, 응답 누적 전 실패 지점을 Agent 상태 전이와 함께 드러내며, 순회·상태 분리
     세부사항은 helper에 남겨 loop 비대화를 제한한다. B는 기존 Agent client contract를 그대로 재사용하지만 middleware
     실패가 `LLMClient.Chat` 오류처럼 숨고 Agent가 다시 오류 종류를 해석해야 한다. C는 provider 중복과 동작 차이를 만든다.
   - 채택안: 옵션 A. Hook은 현재 요청이나 응답 값을 받아 변경된 값을 반환하며, helper는 반환값을 등록 순서대로
     이어서 전달한다.
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
   - 옵션 A: `cmd/agent-runtime`이 현재 작업 디렉터리를 File Tool과 활성화된 Code Execution Tool의 root로 사용한다.
   - 옵션 B: 새 `TOOL_ROOT` 환경변수를 추가한다.
   - 옵션 C: 실행 파일 위치나 repository root를 자동 탐색한다.
   - trade-off: A는 `.env`를 현재 작업 디렉터리에서 읽는 기존 CLI 동작과 맞고 새 config가 없다. B는 명시적이지만
     spec 밖의 설정 contract가 늘어난다. C는 실행 위치에 따라 예측하기 어렵고 repository가 아닌 실행을 처리해야 한다.
   - 채택안: 옵션 A.
   - 근거: `SPEC §5.9`는 진입점 조립을, spec 제약은 기존 File/Code Tool root 제한 유지를 요구한다.

7. CLI model timeout과 loop 제한
   - 옵션 A: `LLM_TIMEOUT`을 각 model 호출에 적용하고, 전체 loop는 max step과 별도 caller context로 제한한다.
   - 옵션 B: `LLM_TIMEOUT` 하나를 전체 Runner run deadline으로 사용한다.
   - 옵션 C: model timeout과 run timeout 환경변수를 새로 분리한다.
   - trade-off: A는 기존 변수 의미를 보존하고 여러 step이 각각 같은 provider 예산을 갖는다. B는 총 실행 시간은 짧게
     제한하지만 Tool 사용 시간이 다음 model 호출 예산을 소모한다. C는 가장 명확하지만 새 config 범위를 추가한다.
   - 채택안: 옵션 A. CLI max step은 10 model 호출, 전체 caller context deadline은 10분으로 둔다.
   - 근거: `SPEC §5.1`, `SPEC §5.9`, `SPEC §5.10`, `SPEC §5.14`는 model 호출별 제한과 전체 run 제한을 구분해
     관찰할 수 있어야 한다.

8. Tool timeout과 실행 수명
   - 옵션 A: Agent가 timeout context로 `Tool.Execute`를 동기 호출하고 Tool이 cooperative cancellation을 준수한다.
   - 옵션 B: 현재처럼 goroutine에서 실행하고 timeout 시 Agent만 먼저 반환한다.
   - 옵션 C: 모든 Tool을 별도 process나 worker에서 실행해 timeout 시 강제 종료한다.
   - trade-off: A는 Tool 반환 뒤 background 실행이 남지 않고 상태 전이가 단순하지만 context를 무시하는 Tool을 강제
     종료하지 못한다. B는 Agent가 빨리 반환하지만 부작용과 goroutine이 run 이후 남는다. C는 가장 강하지만 현재
     in-process Tool contract보다 큰 격리 계층이 필요하다.
   - 채택안: 옵션 A. Runtime timeout은 cooperative contract로 명시하고 내장 Tool이 context 취소를 준수하게 한다.
   - 근거: `SPEC §5.12`는 timeout 이후 실행 수명과 Agent 상태 전이가 일치하도록 요구한다.

9. File Tool root 격리
   - 옵션 A: Go 1.26 표준 `os.Root`로 File Read와 File Save의 실제 filesystem 연산을 root 안에 가둔다.
   - 옵션 B: `filepath.EvalSymlinks` 후 문자열 root 검사를 반복한다.
   - 옵션 C: 현재 lexical path 검사와 `Lstat` 순회를 유지한다.
   - trade-off: A는 표준 library가 symbolic link와 rename 경계를 처리하지만 File Tool 내부가 `os.Root` 수명과 오류를
     관리해야 한다. B와 C는 검사와 사용 사이에 path가 바뀌는 TOCTOU를 보안 경계에서 제거하기 어렵다.
   - 채택안: 옵션 A. 각 실행에서 `os.OpenRoot`로 root를 열고 닫아 별도 public lifecycle contract는 추가하지 않는다.
   - 근거: `SPEC §5.13`은 symbolic link를 통한 root 밖 접근과 실패 전 부작용을 모두 금지한다.

10. Tool 실행 예산
   - 옵션 A: Agent와 Runner에 Tool 호출 수·result 크기 제한을 두고 전체 시간은 caller context가 소유한다.
   - 옵션 B: max step과 Tool별 timeout만 유지한다.
   - 옵션 C: 모든 제한을 CLI에만 두고 programmatic Agent·Runner에는 두지 않는다.
   - trade-off: A는 모든 실행 경로에 같은 상한을 적용하면서 전체 deadline의 소유자를 호출자로 유지한다. B는 한 model
     응답의 다수 Tool call과 큰 result를 제한하지 못한다. C는 CLI 밖 Runtime 사용이 무제한 경로로 남는다.
   - 채택안: 옵션 A. 기본값은 Tool call 20회와 result 64KiB이며 CLI caller context는 10분으로 둔다.
   - 근거: `SPEC §5.14`는 model step과 독립적인 Tool·메모리·전체 시간 예산을 요구한다.

11. Provider 완료 사유와 Agent 상태
   - 옵션 A: raw stop reason과 별도로 provider-neutral finish reason을 두고 불완전 응답을 typed 오류로 종료한다.
   - 옵션 B: provider raw 문자열을 Agent가 직접 분기한다.
   - 옵션 C: Tool call이 없으면 완료 사유와 관계없이 final로 처리한다.
   - trade-off: A는 provider 차이를 adapter에 가두고 Agent 정책을 일관되게 만들지만 `ChatResponse` contract가 확장된다.
     B는 타입 추가가 없지만 provider별 문자열이 Agent에 누출된다. C는 잘린 응답을 성공으로 숨긴다.
   - 채택안: 옵션 A. `complete`, `tool_call`, `length_limit`, `blocked`, `unknown`을 사용하고 빈 값은 기존 custom client
     호환을 위해 `complete`로 해석한다.
   - 근거: `SPEC §5.15`는 불완전 model 응답을 정상 final과 구분하도록 요구한다.

12. CLI Code Execution 활성화와 환경
   - 옵션 A: 기본 비활성화하고 `ENABLE_CODE_EXECUTION=true`일 때만 등록하며 자식 환경은 allowlist로 구성한다.
   - 옵션 B: 다른 Tool과 함께 항상 등록하고 process 환경 전체를 전달한다.
   - 옵션 C: Phase 4.2 CLI에서 Code Execution Tool을 완전히 제외한다.
   - trade-off: A는 기존 capability를 opt-in으로 제공하면서 기본 프롬프트 실행의 위험과 secret 노출을 줄인다. B는 사용이
     간단하지만 명시적 승인 없이 host code 실행 capability를 노출한다. C는 가장 안전하지만 Phase 4.1 Tool 조립을
     CLI에서 검증하지 못한다.
   - 채택안: 옵션 A. allowlist는 `PATH`, `TMPDIR`, `GOROOT`, `GOCACHE`, `GOMODCACHE`, `GOPATH`, `GOOS`, `GOARCH`,
     `CGO_ENABLED`이고 `GOWORK=off`를 강제한다.
   - 근거: `SPEC §5.16`은 Code Execution의 기본 노출과 process secret 전달을 제한한다.

13. 비양수 timeout과 제한값
   - 옵션 A: config의 `LLM_TIMEOUT`은 양수만 허용하고 Agent·Runner의 음수 timeout과 실행 제한은 생성 시 거부한다.
   - 옵션 B: 0과 음수를 모든 경계에서 timeout 비활성화로 해석한다.
   - 옵션 C: 현재처럼 CLI와 Runner가 비양수 값을 각자 해석한다.
   - trade-off: A는 사용자 config 오류를 조기에 발견하면서 programmatic API의 0 기본값은 유지한다. B는 무제한 실행을
     설정 실수와 구분하기 어렵다. C는 CLI 전환 전후에 같은 값의 의미가 바뀐다.
   - 채택안: 옵션 A. Programmatic `ModelTimeout=0`은 timeout 미지정, `ToolTimeout=0`은 기존 30초 기본값으로 유지한다.
   - 근거: `SPEC §5.17`은 동일한 설정값이 실행 경계에 따라 상반되게 동작하지 않도록 요구한다.
