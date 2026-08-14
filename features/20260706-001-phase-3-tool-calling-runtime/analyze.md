# Phase 3 Tool Calling Runtime 분석

## 근거

확인한 사실:

- `spec.md`는 Phase 3 범위를 새 `internal/tool` 패키지, 기존 `internal/agent`, 기존 `internal/message`, 그리고
  tool schema를 LLM 요청 경계에서 다루는 provider-neutral contract로 정의한다.
- `SPEC §5.1`과 `SPEC §5.2`는 `Tool` contract, `ToolRegistry`, 이름 기반 등록/조회, 중복 등록과 unknown tool
  구분을 요구한다.
- `SPEC §5.3`은 Agent가 LLM을 호출할 때 등록된 Tool schema 목록이 LLM 요청에 포함되어야 한다고 요구한다.
- `SPEC §5.4`, `SPEC §5.5`, `SPEC §5.6`은 assistant tool call 실행, `message.ToolResult` 기반 tool message 누적,
  다음 LLM 요청 전달, 오류 result 전달을 요구한다.
- `SPEC §5.7`은 Tool 실행이 포함된 반복에서도 `MaxSteps`를 넘지 않아야 한다고 요구한다.
- `SPEC §5.8`은 Phase 2의 메모리 trace 구조에 tool call, tool result, tool 오류 또는 timeout 관찰 지점을 더해야
  한다고 요구한다.
- `SPEC §5.9`와 `SPEC §5.10`은 기본 calculator Tool과 file read Tool을 요구한다.
- `SPEC §5.11`은 실제 외부 provider 호출 없이 stub `LLMClient`와 테스트 Tool로 registry, schema 전달, 정상 실행,
  오류 result, max step, trace 기록을 확인해야 한다고 요구한다.
- 현재 `internal/message`는 `ToolCall`, `ToolResult`, `message.Tool`을 이미 제공하며, tool result는 `ToolCallID`,
  `Name`, `Content`, `IsError`를 보존한다.
- 현재 `internal/llm.ChatRequest`는 `Model`과 `Messages`만 포함하고, 등록된 Tool schema를 전달하는 필드는 없다.
- Claude와 Ollama provider 구현은 기존 assistant `ToolCalls`와 tool result message 변환은 처리하지만, 요청에
  사용 가능한 Tool schema 목록을 싣는 wire format은 아직 갖고 있지 않다.
- 현재 `internal/agent.Agent`는 assistant 응답에 tool call이 있으면 tool을 실행하지 않고 `needs_action`으로 멈춘다.
- 현재 `AgentState.Trace`는 step, action, status, error만 가진 메모리 trace event이며 외부 저장 contract는 없다.
- 저장소 안에 `docs/languages.md` 또는 언어별 문서는 확인되지 않았다.

추정:

- Phase 3에서도 기존 CLI는 Phase 1 단발 LLM 호출 경로를 유지한다. `spec.md`가 CLI 전환을 제외 범위로 두기 때문이다.
- `MaxSteps`는 Phase 2 구현과 호환되도록 LLM 요청 step 수로 해석하는 편이 가장 작은 변경이다.

## 1. 구조

Phase 3는 `internal/tool`을 새 Runtime 하위 계층으로 추가한다. `internal/tool`은 Tool contract, registry,
기본 Tool 구현, 입력 검증, timeout을 포함한 실행 helper를 소유한다. Agent는 `internal/tool`의 registry를 주입받아
assistant tool call을 실행하지만, Claude나 Ollama 같은 provider 구현을 알지 않는다. 이 구조는 Tool 실행 책임을
provider와 분리하면서 등록/lookup 요구사항을 한곳에 모은다(SPEC §5.1, SPEC §5.2, SPEC §5.4).

Tool schema는 provider wire format이 아니라 Runtime의 provider-neutral contract여야 한다. 기존 `internal/llm`이
`internal/message`에만 의존하고 있고, Phase 1 문서가 LLM client가 Agent/Tool 계층에 의존하지 않는 방향을 잡았으므로,
schema 타입은 `internal/tool`이 아니라 공유 하위 contract에 두는 편이 안전하다. 채택안은 `internal/message`에
`ToolSchema` 성격의 provider-neutral 타입을 추가하고, `llm.ChatRequest`가 `Tools []message.ToolSchema`를 받는
구조다. 그러면 `internal/tool`은 자신이 가진 Tool을 `message.ToolSchema`로 노출하고, `internal/llm`은 이 schema를
provider별 요청 형식으로 변환한다(SPEC §5.3).

Agent는 Phase 2의 `needs_action` 종료를 기본 동작으로 유지하되, registry가 설정된 경우에만 tool-use loop로 확장한다.
Tool registry가 없거나 비어 있으면 기존처럼 tool call 대기 상태로 멈출 수 있어야 기존 Phase 2 테스트와 contract가
불필요하게 흔들리지 않는다. registry가 있으면 Agent는 assistant message를 누적한 뒤 tool call 목록을 순서대로
처리하고, 각 실행 결과를 `message.Tool`로 append한다. 모든 tool result가 append되면 다음 LLM 요청으로 이어진다
(SPEC §5.4, SPEC §5.5).

Tool 오류는 Agent의 `StatusError`나 `LastError`로 직접 올리지 않는다. `LastError`는 LLM 호출 실패처럼 Agent run
자체가 더 진행될 수 없는 오류에 남기고, unknown tool, 입력 검증 실패, 실행 오류, timeout은 `ToolResult.IsError`가
true인 tool message로 표현한다. 이렇게 해야 오류도 다음 LLM 판단에 전달된다는 요구사항과 맞는다(SPEC §5.6).

Trace는 외부 저장소를 만들지 않고 `AgentState.Trace`를 확장한다. 기존 `TraceEvent`에 tool 이름, tool call ID,
result error 여부 같은 필드를 추가하거나 action만 늘리는 방법이 있다. Phase 3에서는 테스트가 tool call/result를
구분해야 하므로 action은 최소 `tool_call`, `tool_result`, `tool_error` 또는 `tool_timeout`을 구분하고, event에는
tool name과 tool call ID를 담을 수 있어야 한다(SPEC §5.8).

기본 calculator Tool은 외부 의존 없이 deterministic한 계산 결과를 반환하는 테스트 기준 Tool로 둔다. 기본 file read
Tool은 read-only이며, root directory를 명시적으로 주입받고 그 root 밖으로 벗어나는 경로는 오류 result로 처리한다.
절대경로, `..`로 root 밖을 가리키는 경로, 디렉터리 읽기, 읽기 실패는 모두 오류 result다. 이 정책은 Phase 3의 로컬
파일 읽기 요구를 만족하면서 파일 저장, 삭제, 명령 실행 제외 범위를 침범하지 않는다(SPEC §5.10).

## 2. 데이터 흐름

Agent 생성 시점에는 기존 `Options{Client, Model, MaxSteps}`에 Tool registry와 Tool timeout 정책을 추가한다.
registry는 Tool 목록을 schema 배열로 변환해 LLM 요청마다 `ChatRequest.Tools`에 포함할 수 있어야 한다. schema 목록은
Agent가 tool-use 판단을 할 수 있는 유일한 입력이므로 LLM 요청 복사본에도 보존되어야 한다(SPEC §5.3, SPEC §5.11).

Run 시작 흐름은 Phase 2와 같다. Agent는 user message를 `AgentState.Messages`에 append하고 trace에 user message를
남긴다. LLM 호출 직전에는 `Step >= MaxSteps`를 검사하고, 호출이 허용되면 `Step++` 뒤 현재 messages와 tool schema를
담아 `LLMClient.Chat`을 호출한다. LLM 호출 오류는 기존처럼 `StatusError`와 `LastError`로 종료한다(SPEC §5.7).

LLM이 tool call 없는 assistant message를 반환하면 기존 final 경로를 유지한다. assistant message를 append하고
`StatusFinal`, `FinalAnswer`를 설정하며 final trace를 남긴다. 이 경로는 Tool registry가 있어도 동일하게 동작해야
한다(SPEC §5.5).

LLM이 tool call 있는 assistant message를 반환하면 Agent는 assistant message를 append하고 각 tool call을 순서대로
실행한다. 각 실행은 registry lookup, arguments validation, timeout이 적용된 context 생성, Tool 실행, result
normalization 순서로 처리한다. 정상 result는 `message.ToolResult{ToolCallID, Name, Content, IsError:false}`로,
오류 result는 같은 구조에 `IsError:true`로 append한다(SPEC §5.4, SPEC §5.6).

여러 tool call이 한 assistant message에 들어 있으면 같은 assistant message 뒤에 tool result message를 같은 순서로
append한다. Phase 3는 별도 병렬 실행 요구가 없고 trace 순서 검증이 중요하므로 순차 실행을 기본으로 한다.
순차 실행은 각 tool result와 trace 순서를 테스트하기 쉽고, timeout과 오류 result를 call 단위로 분리하기 쉽다
(SPEC §5.8, SPEC §5.11).

tool result append 후에는 다음 LLM 요청으로 반복한다. 반복 직전에는 다시 `Step >= MaxSteps`를 검사한다. `MaxSteps`는
LLM 요청 횟수 제한으로 유지하고, tool call은 해당 LLM step의 결과 처리로 본다. 따라서 `MaxSteps`가 1이고 첫 LLM이
tool call을 반환하면 tool result는 append될 수 있지만, 두 번째 LLM 요청은 `max_steps` 상태로 막힌다(SPEC §5.7).

unknown tool, validation failure, tool execution error, timeout은 모두 tool result 오류 message로 누적된다. 이 경우
Agent run은 즉시 `StatusError`로 끝나지 않고 다음 LLM 판단을 시도한다. 단, 오류 result를 append한 뒤 다음 LLM 요청
전에 max step 제한에 걸리면 `StatusMaxSteps`로 종료한다(SPEC §5.6, SPEC §5.7).

Tool timeout은 Agent run context에서 파생된 child context로 처리한다. Agent의 상위 context가 먼저 취소되면 Tool도
취소되어 오류 result가 되고, Tool별 timeout이 먼저 끝나도 오류 result가 된다. LLM provider timeout 분류는 기존
`internal/llm` 책임으로 남긴다(SPEC §5.6).

## 3. 인터페이스

`internal/tool`의 핵심 contract는 이름, 설명, schema, validation, execution을 분리해서 읽을 수 있어야 한다. 추천
형태는 다음 의미를 갖는다.

```go
type Tool interface {
	Name() string
	Description() string
	Schema() message.ToolSchema
	Validate(args json.RawMessage) error
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}
```

`Result`는 Tool 실행의 정상 output을 Runtime 내부 문자열 content로 정규화한다. Tool이 반환한 Go error는 Agent가
`ToolResult.IsError=true`인 메시지로 바꾼다. Tool 구현이 직접 `message.ToolResult`를 만들게 하지 않으면 tool call
ID와 result normalization 책임이 Agent/tool runner 경계에 남아 일관된다(SPEC §5.4, SPEC §5.6).

`ToolRegistry`는 등록과 조회를 명시적으로 제공한다. `Register(tool Tool) error`, `Lookup(name string) (Tool, bool)`,
`Schemas() []message.ToolSchema` 정도가 Phase 3 완료 조건에 맞다. `Register`는 빈 이름, nil Tool, 중복 이름을
오류로 거부해야 하고, `Schemas`는 LLM 요청에 넣을 안정적인 목록을 반환해야 한다(SPEC §5.1, SPEC §5.2, SPEC §5.3).

`message.ToolSchema`는 provider-neutral schema contract다. 최소 필드는 name, description, input schema JSON이다.
input schema는 provider별 wire format으로 바로 변환할 수 있도록 `json.RawMessage` 또는 구조화된 object 형태여야
한다. Phase 3에서는 full JSON Schema engine을 새 의존성으로 추가하지 않고, schema는 LLM 안내와 요청 변환에 쓰며
실제 검증은 각 Tool의 `Validate`가 맡는다(SPEC §5.3, SPEC §5.6).

`llm.ChatRequest`에는 `Tools []message.ToolSchema`가 추가된다. Claude provider는 이 목록을 Messages API의 tools
field로 옮기고, Ollama provider는 Chat API의 tools field로 옮긴다. 이 변환은 provider별 wire format 책임이므로
`internal/llm` 안에 둔다. stub `LLMClient`는 request에 들어온 Tools를 복사해 테스트에서 schema 전달을 검증한다
(SPEC §5.3, SPEC §5.11).

`agent.Options`에는 registry와 Tool timeout이 추가된다. `Tools *tool.Registry` 또는 `Registry tool.Registry`
형태로 주입하되, nil이면 Phase 2와 같은 tool 대기 상태로 종료하는 호환 경로를 유지한다. Tool timeout은 `time.Duration`
값으로 두고, 0이면 기본 timeout을 사용하거나 timeout 없음이 아니라 명시 오류로 볼지 하나로 정해야 한다.
채택안은 Agent 생성 시 0이면 package default timeout을 적용하는 방식이다. 이렇게 해야 모든 Tool 실행 제한 요구를
기본값으로 만족한다(SPEC §5.6).

Trace action은 기존 action enum을 확장한다. 최소 action은 `tool_call`, `tool_result`, `tool_error`, `tool_timeout`
이다. `TraceEvent`에는 tool name과 tool call ID 필드를 추가한다. Error 필드는 validation/execute/timeout의 원인
error를 참조할 수 있지만, trace가 외부 저장 contract가 아니므로 직렬화 형식은 정의하지 않는다(SPEC §5.8).

기본 calculator Tool의 입력 schema는 계산식 문자열 또는 명확한 숫자/연산자 구조 중 하나여야 한다. 채택안은
`{"left": number, "operator": string, "right": number}` 형태다. 이 구조는 `eval`이 필요 없고, 허용 연산자를
명시적으로 제한할 수 있어 테스트가 단순하다(SPEC §5.9).

기본 file read Tool의 입력 schema는 `{"path": string}`이다. Tool 생성자는 root directory를 받는다. 실행 시 path를
clean/abs 처리한 뒤 root 내부인지 확인하고, 일반 파일만 읽어 content로 반환한다. root 밖 경로, 빈 path, 디렉터리,
읽기 실패는 오류 result로 이어진다(SPEC §5.10).

## 4. 영향 범위

새 영향 범위는 `internal/tool` 패키지다. 이 패키지는 Tool contract, registry, runner 성격의 실행 helper, calculator
Tool, file read Tool과 단위 테스트를 포함한다. 외부 SDK나 DB 의존은 추가하지 않는다(SPEC §5.1, SPEC §5.9,
SPEC §5.10).

`internal/message`는 provider-neutral tool schema 타입을 추가하는 영향이 있다. 기존 `ToolCall`, `ToolResult`,
`Message`의 의미는 유지한다. schema 타입 추가는 기존 provider response나 message constructor contract를 깨지 않는
방향으로 제한한다(SPEC §5.3).

`internal/llm`은 `ChatRequest.Tools`를 provider별 요청 body로 변환해야 한다. Claude와 Ollama 모두 기존에는 assistant
tool call과 tool result message만 변환했으므로, Phase 3에서는 사용 가능한 tool schema 목록 변환 테스트를 추가해야
한다. provider 실제 호출 contract는 기존 `LLMClient.Chat` 메서드 하나로 유지한다(SPEC §5.3).

`internal/agent`는 `Options`, `Agent`, `AgentState`, `TraceEvent`, `Run` 흐름이 변경된다. tool registry가 있으면
`needs_action`으로 멈추지 않고 tool result를 append한 뒤 다음 LLM 요청을 수행한다. registry가 없으면 기존 Phase 2
대기 동작을 유지해 기존 테스트가 의미를 잃지 않게 한다(SPEC §5.4, SPEC §5.5).

`internal/config`, `.env.example`, `cmd/agent-runtime`은 Phase 3의 필수 변경 대상이 아니다. CLI의 Agent loop 전환은
spec 제외 범위이며, Tool timeout 기본값도 Agent 옵션 기본값으로 처리할 수 있다. 나중에 CLI가 Agent loop를 사용하게
될 때 config/env로 노출할 수 있다.

Feature 문서는 이후 `implement-init`에서 Task와 검증 조건을 만든다. `analyze.md`는 구현 순서를 고정하지 않고,
구조와 경계만 결정한다.

## 5. Decision Points

1. Tool schema 타입 위치
   - 옵션 A: `internal/message`에 provider-neutral `ToolSchema`를 둔다.
   - 옵션 B: `internal/tool`에 schema 타입을 두고 `internal/llm`이 `internal/tool`을 import한다.
   - 옵션 C: 새 공유 패키지에 schema 타입만 둔다.
   - trade-off: 옵션 A는 기존 provider-neutral 메시지 contract 옆에 tool call/result/schema를 모아 import 방향을
     단순하게 유지한다. 옵션 B는 이름상 자연스럽지만 `internal/llm`이 Tool Runtime에 의존해 Phase 1의 provider 분리
     원칙을 약화한다. 옵션 C는 가장 엄격하지만 schema 타입 하나를 위해 패키지를 늘린다.
   - 채택안: 옵션 A.
   - 근거: `internal/llm`은 이미 `internal/message`에 의존하고 있고, Phase 3는 LLM 요청 경계에서 schema를 관찰해야
     한다(SPEC §5.3).

2. Tool validation 방식
   - 옵션 A: 각 Tool이 `Validate(json.RawMessage) error`를 제공하고, common runner가 실행 전에 호출한다.
   - 옵션 B: `Execute` 안에서 입력 검증과 실행을 모두 처리한다.
   - 옵션 C: JSON Schema 검증 라이브러리를 추가해 schema 기반 검증을 공통화한다.
   - trade-off: 옵션 A는 실행 전 검증 실패를 명확히 분리하면서 새 의존성을 추가하지 않는다. 옵션 B는 interface가
     작지만 검증 실패와 실행 실패를 공통 runner가 구분하기 어렵다. 옵션 C는 schema와 검증을 강하게 일치시킬 수
     있지만 Phase 3에 새 의존성과 schema dialect 결정을 추가한다.
   - 채택안: 옵션 A.
   - 근거: spec은 입력 검증 실패가 Tool 실행 전에 오류 result로 관찰 가능해야 한다고 요구한다(SPEC §5.6).

3. Tool 오류 처리 위치
   - 옵션 A: unknown tool, validation failure, execute error, timeout을 모두 `ToolResult.IsError=true`로 메시지에
     append하고 다음 LLM 판단에 전달한다.
   - 옵션 B: unknown tool이나 validation failure는 Agent `StatusError`로 종료한다.
   - trade-off: 옵션 A는 LLM이 오류를 보고 복구하거나 final answer를 만들 수 있게 하며 spec의 다음 LLM 판단 전달
     요구와 맞다. 옵션 B는 실패가 명확하지만 Tool Calling Runtime의 일반 오류를 provider 오류처럼 취급한다.
   - 채택안: 옵션 A.
   - 근거: spec은 Tool 관련 오류를 provider 오류가 아니라 tool result 오류로 표현해 다음 LLM 판단에 전달하는 것을
     기본으로 한다(SPEC §5.6).

4. `MaxSteps` 의미
   - 옵션 A: `MaxSteps`를 LLM 요청 횟수로 유지하고, tool call 실행은 해당 LLM step의 결과 처리로 본다.
   - 옵션 B: LLM 요청과 Tool 실행을 모두 step으로 센다.
   - trade-off: 옵션 A는 Phase 2의 기존 `Step` 의미와 호환되고 provider 호출 제한을 안정적으로 유지한다. 옵션 B는
     더 넓은 실행 제한처럼 보이지만 기존 테스트와 상태 의미를 바꾸고, 여러 tool call이 있는 응답의 step 의미가
     복잡해진다.
   - 채택안: 옵션 A.
   - 근거: Phase 2 구현은 LLM 호출 직전에만 `Step++`을 수행하며, Phase 3 완료 조건은 LLM 또는 Tool을 추가로 실행하지
     않는 max step 종료를 기존 Agent run 제한의 확장으로 요구한다(SPEC §5.7).

5. 여러 tool call 실행 방식
   - 옵션 A: assistant message에 들어온 tool call을 응답 순서대로 순차 실행한다.
   - 옵션 B: 여러 tool call을 병렬 실행한다.
   - trade-off: 옵션 A는 trace와 message 순서가 deterministic하고 timeout/error result를 call 단위로 검증하기 쉽다.
     옵션 B는 느린 Tool 묶음에서 유리할 수 있지만 ordering, cancellation, partial result 정책을 새로 정해야 한다.
   - 채택안: 옵션 A.
   - 근거: Phase 3는 병렬 실행을 요구하지 않고, 테스트에서 trace 순서와 result 누적을 확인해야 한다(SPEC §5.8,
     SPEC §5.11).

6. File read Tool 접근 정책
   - 옵션 A: Tool 생성 시 root directory를 받고, clean/abs 후 root 내부 일반 파일만 읽는다.
   - 옵션 B: process working directory 기준으로 상대 경로를 자유롭게 읽는다.
   - 옵션 C: 절대경로까지 허용한다.
   - trade-off: 옵션 A는 테스트가 쉽고 허용 범위가 명확하며, 저장/삭제/명령 실행 제외 범위를 침범하지 않는다.
     옵션 B와 C는 구현이 단순하지만 Runtime이 의도하지 않은 로컬 파일을 읽을 가능성이 커진다.
   - 채택안: 옵션 A.
   - 근거: spec은 허용된 로컬 파일만 읽고 허용되지 않은 접근은 오류 result로 반환해야 한다고 요구한다
     (SPEC §5.10).

7. Tool timeout 설정
   - 옵션 A: Agent Options에 `ToolTimeout`을 두고 0이면 package default timeout을 적용한다.
   - 옵션 B: 각 Tool이 timeout을 직접 책임진다.
   - 옵션 C: Phase 3에서 timeout 설정을 config/env로 노출한다.
   - trade-off: 옵션 A는 모든 Tool 실행에 같은 제한을 적용하면서 CLI/config 제외 범위를 지킨다. 옵션 B는 Tool별
     유연성은 높지만 Runtime 차원의 timeout 보장이 약해진다. 옵션 C는 운영 설정에는 좋지만 Phase 3 제외 범위인 CLI
     전환과 환경변수 contract 확장으로 번진다.
   - 채택안: 옵션 A.
   - 근거: spec은 Tool 실행 timeout을 요구하지만 CLI/config 변경은 요구하지 않는다(SPEC §5.6).
