# Phase 4.3 Streaming Agent Response 분석

## 근거

확인한 사실:

- `spec.md`는 Claude와 Ollama의 provider별 stream을 Runtime 공통 계약으로 정규화하고, Runner가 모든 model 호출의
  text delta와 정확히 한 번의 final 또는 error 결과를 제공하도록 요구한다.
- CLI는 별도 flag 없이 streaming Runner를 사용한다. Interactive terminal은 임시 text를 표시한 뒤 정상 종료 시
  final answer만 남기고, redirect된 stdout은 중간 text와 terminal 제어 문자 없이 final answer만 한 번 출력한다.
- Phase 4.3은 Tool lifecycle event를 공개하지 않는다. Tool 호출·결과·오류·timeout event와 execution backend는
  Phase 4.4 범위다.
- `internal/llm.LLMClient`는 현재 `Chat(ctx, ChatRequest) (ChatResponse, error)`만 제공한다. 기존 custom client도 이
  interface를 구현하므로 메서드를 직접 추가하면 기존 구현이 깨진다.
- Claude adapter는 `/v1/messages`의 단일 JSON 응답을 처리한다. Ollama adapter는 `/api/chat` 요청에
  `stream: false`를 보내고 단일 JSON 응답을 처리한다.
- 현재 `ChatResponse`는 assistant text, 완성된 Tool call, provider-neutral `FinishReason`, raw `StopReason`,
  token usage를 한 값으로 보존한다.
- `internal/agent.Agent.Run`은 pre-model middleware, model timeout, LLM 호출, post-model middleware, 완료 사유
  판정, Tool 실행과 실행 제한을 하나의 loop에서 소유한다.
- `Runner.Run`은 Agent loop가 끝난 뒤 final answer에 structured output validator를 적용한다. 검증 실패는
  `RunnerErrorKindStructuredOutput`과 `AgentState`의 error 상태로 보존한다.
- CLI `run` 함수는 현재 `io.Writer`에 final answer를 한 번 출력하며, 테스트는 `bytes.Buffer`로 stdout과 stderr
  계약을 확인한다. Interactive terminal 여부를 전달하거나 판별하는 경계는 없다.
- Claude 공식 streaming 문서는 SSE의 `message_start`, content block start·delta·stop, `message_delta`,
  `message_stop` 순서와 `text_delta`, Tool input의 `input_json_delta`, stream 내부 error를 정의한다.
  알 수 없는 event type이 추가될 수 있으므로 이를 정상적으로 무시할 것을 요구한다.
  ([Claude streaming 문서](https://platform.claude.com/docs/en/build-with-claude/streaming))
- Claude Tool input delta는 부분 JSON 문자열이며 content block이 끝난 뒤 완성된 JSON으로 파싱해야 한다.
  ([Claude fine-grained Tool streaming 문서](https://platform.claude.com/docs/en/agents-and-tools/tool-use/fine-grained-tool-streaming))
- Ollama 공식 문서는 streaming 응답을 NDJSON으로 전송하며, text와 Tool call이 여러 chunk에 걸쳐 올 수 있으므로
  `content`와 `tool_calls`를 모두 모아 다음 Agent turn에 사용하도록 설명한다.
  ([Ollama streaming 문서](https://docs.ollama.com/api/streaming),
  [Ollama Tool calling 문서](https://docs.ollama.com/capabilities/tool-calling))
- 프로젝트는 Go 1.26을 사용한다. 표준 `iter.Seq`와 `iter.Seq2`는 consumer의 `yield`가 false를 반환하면 producer가
  동기적으로 중단하고 정리할 수 있는 push iterator 계약을 제공한다.
  ([Go iter 문서](https://pkg.go.dev/iter@go1.26.4))
- 저장소에는 프로젝트가 지시한 `docs/languages.md`와 Go 세부 문서가 없다.
- 분석 시작 시 `go test ./...`는 모든 패키지에서 통과했다.

추정:

- Interactive terminal 판별은 stdout이 character device인지 확인하는 현재 process의 출력 환경을 기준으로 한다.
  별도 CLI mode flag가 없고 redirect 출력은 terminal 제어 문자를 금지하기 때문이다.
- Phase 4.3은 Claude와 Ollama가 현재 지원하는 text와 client Tool call만 Runtime 메시지로 조립한다. Thinking·reasoning
  조각과 provider server Tool block은 SPEC 제외 범위이므로 공개 event나 Agent 메시지에 추가하지 않는다.

## 1. 구조

기존 Agent loop를 non-streaming과 streaming으로 복제하지 않는다. Agent가 가진 상태 전이와 제한 정책은 하나의 내부
실행 함수가 계속 소유하고, model 호출 방식만 완성 응답 호출과 streaming 호출로 주입한다. 이 구조는 두 경로가 같은
Agent 상태, Tool loop, middleware 순서와 완료 사유 판정을 사용하게 해 `SPEC §5.2`, `SPEC §5.4`, `SPEC §5.5`,
`SPEC §5.6`, `SPEC §5.10`, `SPEC §5.14`를 함께 보장한다.

```text
Programmatic caller / CLI
→ Runner.RunStream
  → Agent 공통 loop
    → pre-model middleware
    → StreamingLLMClient.StreamChat
      → provider adapter
        → Claude SSE 또는 Ollama NDJSON
        → provider-neutral text delta
        → 완성된 ChatResponse
    → post-model middleware
    → 완료 사유 판정
    → 선택적 Tool 실행 후 다음 model stream
  → 선택적 structured output final 검증
  → Runner final 또는 error event
→ CLI terminal renderer 또는 programmatic consumer
```

### LLM 경계

기존 `LLMClient`는 변경하지 않고, 이를 포함하는 별도 `StreamingLLMClient`를 추가한다. Claude와 Ollama client의 구체
타입은 두 interface를 모두 구현한다. Provider registry와 `ClientFactory`는 계속 `LLMClient`를 반환하고, Runner가
streaming 시작 시 선택적으로 `StreamingLLMClient`를 확인한다. 따라서 기존 `Chat` custom client와 `Runner.Run`은
변경 없이 유지된다(`SPEC §5.3`, `SPEC §5.14`).

Provider adapter는 raw transport event를 외부로 넘기지 않는다. Adapter 내부 accumulator가 provider event를 text,
Tool call, finish reason과 usage로 조립하고, text가 도착할 때만 provider-neutral delta를 내보낸다. 정상 stream은
완성된 `ChatResponse`를 마지막 LLM event로 내보내며, transport·decode·sequence 오류는 기존 `llm.Error` 분류로
반환한다(`SPEC §5.1`, `SPEC §5.3`, `SPEC §5.4`, `SPEC §5.9`).

### Agent와 Runner 경계

Agent 공통 loop는 model 호출 전후 상태 전이를 계속 소유한다. Streaming model caller는 adapter iterator를
동기적으로 소비하면서 text delta에 현재 `Step`을 붙여 상위로 전달하고, 완성된 `ChatResponse`를 기존 post-model
이후 흐름에 넘긴다. Tool call은 이 완성 응답에만 존재하므로 부분 arguments가 Tool 실행 경계로 넘어가지 않는다
(`SPEC §5.4`, `SPEC §5.5`).

Runner는 Agent에서 올라온 text delta를 `RunnerStreamEvent`로 전달하고, Agent 종료 후 structured output을 기존과
같이 검증한다. 검증까지 성공한 `StatusFinal`은 final event가 되고, structured output 실패와 non-final Agent 상태는
error event가 된다. Tool lifecycle event는 만들지 않아 Phase 4.4 경계를 보존한다(`SPEC §5.1`, `SPEC §5.7`,
`SPEC §5.8`, `SPEC §5.9`).

### CLI 출력 경계

CLI는 항상 `Runner.RunStream`을 소비하지만 stdout의 성격에 따라 renderer만 선택한다.

- Interactive renderer는 첫 text delta 전에 cursor 위치를 저장하고 delta를 즉시 쓴다. Terminal event에서 저장한
  위치로 돌아가 이후 영역을 지운 뒤, final이면 `RunnerResult.State.FinalAnswer`만 다시 출력한다.
- Redirect renderer는 text delta를 화면에 쓰지 않고 terminal event까지 소비한다. Final이면 final answer만 한 번
  출력하고, error이면 stdout을 비운 채 기존 stderr 경로를 사용한다.

Renderer는 full-screen 상태, 입력 처리나 별도 화면 model을 소유하지 않는다. Cursor 저장·복원·영역 지우기만 담당하는
최소 terminal renderer로 제한한다(`SPEC §5.12`, `SPEC §5.13`, `SPEC §5.16`).

## 2. 데이터 흐름

### 정상 text 응답

1. `Runner.RunStream` iterator를 소비하면 Agent 상태를 `running`으로 만들고 사용자 메시지를 기록한다.
2. Agent는 현재 메시지와 Tool schema의 복사본으로 요청을 만들고 pre-model middleware를 순서대로 적용한다.
3. Model 호출별 timeout context로 `StreamingLLMClient.StreamChat` iterator를 소비한다.
4. Adapter가 text delta를 내보낼 때 Agent는 현재 step을 붙여 Runner text event로 전달한다.
5. Adapter가 stream 종료 metadata까지 확인한 뒤 완성된 `ChatResponse`를 반환한다.
6. Agent는 post-model middleware를 적용하고 응답 메시지, finish reason과 trace를 상태에 반영한다.
7. Tool call이 없고 finish reason이 complete이면 `StatusFinal`과 `FinalAnswer`를 만든다.
8. Runner는 선택적인 structured output을 검증한다.
9. 검증까지 성공하면 `RunnerResult`를 가진 final event를 정확히 한 번 내보내고 iterator를 끝낸다.

이 흐름에서 text delta는 post-model과 structured output 검증 전의 임시 값이다. Programmatic consumer는 final event의
결과를 권위 있는 값으로 사용하고, CLI interactive renderer도 임시 text를 지운 뒤 final answer를 다시 출력한다
(`SPEC §5.1`, `SPEC §5.2`, `SPEC §5.5`, `SPEC §5.7`, `SPEC §5.12`).

### Tool loop

Claude adapter는 `tool_use` content block의 index, ID, 이름과 부분 JSON을 모은다. `content_block_stop`에서 JSON
문서 하나로 파싱하고, `message_stop`과 `stop_reason`을 받은 뒤 content block index 순서대로 `ToolCall`을 만든다.
Ollama adapter는 각 NDJSON chunk의 `message.content`를 이어 붙이고, chunk의 완성된 `tool_calls`를 index 또는 도착
순서대로 모은다. 최종 `done: true` chunk에서 `done_reason`과 usage를 확정한다.

Agent는 adapter가 최종 response를 반환하기 전에는 Tool을 실행하지 않는다. Post-model middleware가 Tool call을
변경했다면 변경된 완성값을 사용해 기존 registry lookup, validation, timeout과 result 제한을 적용한다. Tool result를
메시지에 추가한 뒤 다음 loop step에서 새 streaming model 호출을 시작한다. 각 step의 text delta는 programmatic
consumer에게 `Step`과 함께 전달되지만 CLI에서는 전체 run이 끝날 때까지 임시 화면으로 취급한다
(`SPEC §5.4`, `SPEC §5.5`, `SPEC §5.10`).

### Structured output

Output schema는 기존처럼 Runner 생성 시 한 번 compile한다. Streaming run은 model text delta를 그대로 전달하지만
부분 JSON을 파싱하거나 검증하지 않는다. Agent가 `StatusFinal`을 만든 뒤 post-model 결과인 `FinalAnswer` 전체를
기존 validator로 검증한다.

검증 성공 시 `StructuredOutput`을 포함한 final event를 내보낸다. JSON parse 또는 validation 실패 시 Agent 상태를
structured output error로 바꾼 뒤 error event를 내보낸다. CLI interactive renderer는 임시 JSON text를 지우고
오류를 stderr에 표시하며, redirect renderer는 stdout에 JSON 일부를 쓰지 않는다(`SPEC §5.7`, `SPEC §5.8`).

### 오류와 중단

HTTP status 오류는 body streaming 전에 기존 provider 오류로 처리한다. 정상 status 이후 SSE error event,
NDJSON decode 실패, EOF 이전 미완성 content block, final marker 누락과 잘못된 Tool JSON은 provider stream 오류로
종료한다. Model timeout은 기존 `llm.ErrorKindTimeout`을 유지하고, caller 전체 deadline은 Agent의
`execution_limit` 상태를 유지한다.

Adapter가 완성 응답을 반환해도 finish reason이 length limit, blocked 또는 unknown이면 Agent가
`incomplete_response` 상태를 만들고 Runner가 error event를 내보낸다. 이미 전달한 delta는 programmatic caller에게
남지만 성공 final로 승격되지 않는다(`SPEC §5.9`, `SPEC §5.10`).

Runner와 provider iterator는 별도 goroutine을 만들지 않고 같은 호출 stack에서 동작한다. Consumer가 range를
중단하면 `yield`가 false를 반환하고 Agent, provider parser 순서로 즉시 unwind한다. Provider adapter의 deferred
response body close가 실행되므로 별도 cancel channel이나 producer goroutine이 남지 않는다. Context 취소도
HTTP request와 Tool 실행에 그대로 전달된다(`SPEC §5.11`).

CLI 오류 시 interactive renderer는 저장한 cursor 위치를 복원하고 임시 영역을 지운 뒤 stderr에 오류를 쓴다.
Redirect renderer는 delta를 쓰지 않았으므로 stdout을 비운 상태로 유지한다. `StatusMaxSteps`,
`StatusNeedsAction`, `StatusError`는 모두 Runner error terminal event지만 기존 상태값은 `RunnerResult`에 보존한다
(`SPEC §5.13`, `SPEC §5.16`).

## 3. 인터페이스

### Provider-neutral LLM stream

기존 interface는 그대로 둔다.

```go
type LLMClient interface {
	Chat(context.Context, ChatRequest) (ChatResponse, error)
}
```

새 streaming interface는 기존 contract를 포함한다.

```go
type ChatStreamEventKind string

const (
	ChatStreamEventTextDelta ChatStreamEventKind = "text_delta"
	ChatStreamEventResponse  ChatStreamEventKind = "response"
)

type ChatStreamEvent struct {
	Kind      ChatStreamEventKind
	TextDelta string
	Response  *ChatResponse
}

type StreamingLLMClient interface {
	LLMClient
	StreamChat(context.Context, ChatRequest) iter.Seq2[ChatStreamEvent, error]
}
```

`StreamChat` iterator는 zero or more text delta 뒤에 완성된 response를 정확히 한 번 내보낸다. 오류가 발생하면
`error` pair를 한 번 내보내고 종료하며 response event를 내보내지 않는다. Consumer가 range를 중단하면 추가 event를
만들지 않고 HTTP body를 닫는다. `Response`가 아닌 event는 response pointer를 갖지 않고, response event는 text
delta를 갖지 않는다.

Request와 response의 소유권은 기존 `LLMClient`와 동일하다. Adapter는 request 참조를 보관하지 않고, 내보낸 event와
response를 이후 수정하지 않는다. Provider raw SSE name, NDJSON chunk와 부분 Tool JSON은 이 interface 밖으로
노출하지 않는다(`SPEC §5.1`, `SPEC §5.3`, `SPEC §5.11`, `SPEC §5.14`).

### Runner stream

Runner event는 Phase 4.3 공개 범위만 표현한다.

```go
type RunnerStreamEventKind string

const (
	RunnerStreamEventTextDelta RunnerStreamEventKind = "text_delta"
	RunnerStreamEventFinal     RunnerStreamEventKind = "final"
	RunnerStreamEventError     RunnerStreamEventKind = "error"
)

type RunnerStreamEvent struct {
	Kind      RunnerStreamEventKind
	Step      int
	TextDelta string
	Result    *RunnerResult
}

func (r *Runner) RunStream(context.Context, string) iter.Seq[RunnerStreamEvent]
```

Text event는 `Step`과 `TextDelta`만 사용한다. Final과 error event는 `Result`를 사용하며 text delta를 갖지 않는다.
Final result는 `StatusFinal`이고 선택적 structured output 검증까지 성공한 값이다. Error result는
`StatusError`, `StatusMaxSteps` 또는 `StatusNeedsAction`과 기존 trace·`LastError`를 보존한다.

Streaming을 지원하지 않는 custom `LLMClient`로 `RunStream`을 호출하면 provider 호출이나 Tool 실행 없이
`StatusError` result를 가진 error event 한 번으로 종료한다. `Runner.Run`은 같은 client에서 계속 기존처럼 동작한다.
Non-streaming fallback을 text delta 하나처럼 가장하지 않는다(`SPEC §5.1`, `SPEC §5.9`, `SPEC §5.14`).

Consumer가 iterator를 끝까지 소비하면 정확히 한 번의 terminal event를 받는다. Consumer가 의도적으로 중단하면
그 시점부터 producer가 종료되므로 terminal event를 추가로 받지 않는다. 이는 소비 중단 뒤 실행이 남지 않아야 한다는
`SPEC §5.11`의 의미이며, consumer가 중단한 뒤에도 terminal event를 강제로 보내기 위한 background producer는 만들지
않는다.

### Agent 내부 model caller

Agent의 공개 `Run` contract는 바꾸지 않는다. 내부 loop는 다음 의미의 model caller와 text sink를 받도록 분리한다.

```go
type modelCaller func(
	context.Context,
	llm.ChatRequest,
	func(llm.ChatStreamEvent) bool,
) (llm.ChatResponse, bool, error)
```

두 번째 반환값은 완성 response를 받았는지를 뜻한다. Consumer 중단이면 false와 nil error로 공통 loop를 즉시 끝내고,
provider 오류면 false와 error를 반환한다. Non-streaming caller는 delta를 내보내지 않고 `Chat` 결과와 true를
반환한다. 이 타입은 구현 내부 전용이며 새 공개 Agent abstraction으로 만들지 않는다.

Model timeout context는 caller 전체를 감싸므로 첫 byte 대기뿐 아니라 마지막 response event까지 같은 호출별 timeout을
적용한다. Pre-model과 post-model은 caller 밖의 공통 loop에 남겨 두 경로의 순서를 일치시킨다
(`SPEC §5.2`, `SPEC §5.5`, `SPEC §5.6`, `SPEC §5.10`).

### CLI renderer

CLI는 stdout writer와 `interactive` 판정을 renderer에 주입한다. `main`은 실제 `os.Stdout`의 file mode가 character
device인지 확인하고, 테스트는 이 판정을 명시적으로 주입해 terminal과 redirect 경로를 재현한다.

Renderer 계약은 다음 책임만 가진다.

```go
type streamRenderer interface {
	WriteDelta(string) error
	Finish(string) error
	Reset() error
}
```

Interactive 구현은 첫 delta에서 cursor를 저장하고 모든 delta를 즉시 쓴다. `Finish`는 cursor를 복원하고 아래 영역을
지운 뒤 final answer와 개행을 쓴다. `Reset`은 오류나 non-final 상태에서 cursor와 terminal 영역만 복구한다.
Redirect 구현은 `WriteDelta`를 무시하고 `Finish`에서 final answer만 쓴다. ANSI cursor 제어는 interactive 구현
안에만 존재한다(`SPEC §5.12`, `SPEC §5.13`, `SPEC §5.16`).

## 4. 영향 범위

`internal/llm/client.go`는 streaming interface와 공통 event 타입을 추가한다. 기존 `LLMClient`, `ChatRequest`,
`ChatResponse` 필드와 의미는 유지한다. Provider registry의 factory signature도 바꾸지 않는다.

`internal/llm/claude.go`는 request의 streaming 선택과 SSE parser·accumulator를 추가한다. Non-streaming `Chat`은
현재 JSON request·response 경로를 유지하고 request 조립, response decode와 finish reason helper만 가능한 범위에서
공유한다. Claude 테스트는 local HTTP server로 SSE event 순서, multi-line data, ping·unknown event 무시, text와
Tool JSON 조립, usage·stop reason, SSE error, EOF와 timeout을 확인해야 한다.

`internal/llm/ollama.go`는 streaming request에서 `stream: true`를 보내고 연속 NDJSON object를 `json.Decoder`로
소비한다. Content는 도착 순서대로 합치고 Tool call은 provider index 또는 도착 순서로 모으며 `done: true` 전에는
완성 response를 만들지 않는다. Ollama 테스트는 text·Tool call chunk, final usage·done reason, decode 오류, EOF와
timeout을 확인해야 한다.

`internal/agent/agent.go`는 현재 `Run` loop를 공통 내부 함수로 추출하고 non-streaming·streaming model caller를
연결한다. 기존 상태 전이, trace, Tool 제한과 `executeToolCall` 계약은 유지한다. Streaming test는 여러 model step의
delta 순서, 완성 Tool call 이후 실행, middleware 실패 이후 중단, deadline과 consumer 조기 중단을 확인해야 한다.

`internal/agent/runner.go`는 `RunStream`, Runner event, terminal result 조립을 추가한다. `Run`과 `RunStream`이 같은
structured output finalization helper를 사용하게 해 validator 의미를 공유한다. Runner test는 final/error event
단일성, streaming 미지원 client, structured output 성공·실패와 non-streaming 회귀를 확인해야 한다.

`internal/agent/middleware.go`와 `structured_output.go`의 공개 계약은 바꾸지 않는다. 다만 공통 loop와 terminal
finalization에서 기존 helper를 재사용하므로 관련 테스트가 streaming 경로에서도 같은 순서와 오류 분류를 검증한다.

`cmd/agent-runtime/main.go`는 `Runner.Run` 대신 `RunStream`을 소비하고 stdout 성격에 따라 renderer를 선택한다.
Renderer는 별도 파일로 분리해 terminal 제어와 run 조립 책임을 나눈다. CLI 테스트는 주입된 interactive 판정과
기록 writer로 delta 즉시 표시, final-only 화면 sequence, redirect final-only bytes, 오류 reset과 stderr·종료 코드를
확인해야 한다.

`internal/message`, `internal/tool`, `internal/config`의 contract와 환경변수는 바뀌지 않는다. Streaming flag나
새 설정도 추가하지 않는다. 표준 `iter`, `bufio`, `encoding/json`, `net/http`와 terminal 제어 문자열로 구현할 수 있어
새 module 의존성은 추가하지 않는다.

`README.md`와 `ROADMAP.md`는 구현 완료 시 CLI의 기본 streaming 동작과 Phase 4.3 상태를 실제 코드에 맞춰 갱신해야
한다. Phase 4.4의 Tool lifecycle event 범위는 현재 문구를 유지한다.

## 5. Decision Points

1. 기존 LLM contract 확장 방식
   - 옵션 A: `LLMClient`에 `StreamChat`을 직접 추가한다.
   - 옵션 B: `LLMClient`를 포함하는 별도 `StreamingLLMClient`를 추가한다.
   - 옵션 C: 모든 기존 client를 Runner에서 한 chunk짜리 stream으로 감싼다.
   - Trade-off: A는 타입 수가 적지만 기존 custom client를 모두 깨뜨린다. B는 capability 확인이 필요하지만 기존
     non-streaming contract를 보존한다. C는 호환되어 보이지만 실제 streaming 부재를 숨긴다.
   - 채택안: 옵션 B. Streaming 미지원 client의 `RunStream`은 명확한 error event로 종료한다.
   - 근거: `SPEC §5.1`, `SPEC §5.3`, `SPEC §5.14`는 실제 provider-neutral stream과 기존 API 호환을 동시에 요구한다.

2. Event 전달 방식
   - 옵션 A: producer goroutine과 channel을 사용한다.
   - 옵션 B: callback을 직접 공개한다.
   - 옵션 C: Go 표준 `iter.Seq`·`iter.Seq2`를 사용한다.
   - Trade-off: A는 익숙하지만 consumer가 읽기를 멈췄을 때 별도 cancel과 channel drain 없이는 goroutine이 남는다.
     B는 동기 정리가 쉽지만 중첩 callback API가 provider와 Runner에 반복된다. C는 callback의 동기 정리 성질을
     표준 range 문법으로 제공하고 `yield=false`가 조기 중단을 전달한다.
   - 채택안: 옵션 C. Provider는 `iter.Seq2[event, error]`, Runner는 `iter.Seq[event]`를 사용한다.
   - 근거: `SPEC §5.1`, `SPEC §5.11`은 순차 event와 소비 중단 뒤 실행 수명 종료를 함께 요구한다.

3. Agent loop 구성
   - 옵션 A: 기존 `Run`과 새 `RunStream`에 loop를 각각 구현한다.
   - 옵션 B: 상태 머신은 하나로 유지하고 model caller만 교체한다.
   - 옵션 C: 기존 `Run`을 streaming event를 전부 모으는 wrapper로 바꾼다.
   - Trade-off: A는 초기 구현이 직관적이지만 middleware, Tool 제한과 오류 상태가 곧 갈라진다. B는 내부 caller
     abstraction이 필요하지만 두 경로가 같은 상태 전이를 쓴다. C는 provider transport와 기존 테스트의 의미까지
     바꾸고 streaming 미지원 custom client를 처리하기 어렵다.
   - 채택안: 옵션 B. `Run`은 non-streaming caller, `RunStream`은 streaming caller를 공통 loop에 전달한다.
   - 근거: `SPEC §5.2`, `SPEC §5.4`~`SPEC §5.6`, `SPEC §5.10`, `SPEC §5.14`의 동작 일치를 가장 직접적으로 보장한다.

4. Raw stream 조립 책임
   - 옵션 A: Runner가 Claude SSE와 Ollama NDJSON을 직접 해석한다.
   - 옵션 B: Provider adapter가 raw stream을 완성 `ChatResponse`로 조립한다.
   - 옵션 C: Agent가 provider별 부분 Tool call을 보관한다.
   - Trade-off: A와 C는 provider 전송 형식을 Runtime 본체에 누출한다. B는 adapter 코드가 늘지만 기존
     provider-neutral 경계를 유지하고 non-streaming decode helper도 재사용할 수 있다.
   - 채택안: 옵션 B. Adapter만 raw event와 부분 JSON을 알고 Agent는 완성 응답만 받는다.
   - 근거: `SPEC §5.3`, `SPEC §5.4`, `SPEC §5.9`는 caller와 Tool 실행에서 provider raw 형식을 숨기도록 요구한다.

5. Middleware와 delta 순서
   - 옵션 A: post-model이 끝날 때까지 delta를 buffer한다.
   - 옵션 B: raw text delta는 즉시 전달하고, post-model은 완성 응답에 적용한다.
   - 옵션 C: streaming 전용 per-delta middleware를 추가한다.
   - Trade-off: A는 final과 delta 일치를 보장하지만 streaming 지연을 없애지 못한다. B는 delta가 임시 값이라는
     구분이 필요하지만 현재 middleware contract를 보존한다. C는 새 요구사항과 hook 소유권을 만든다.
   - 채택안: 옵션 B. Pre-model은 stream 전, post-model은 완성 response 뒤에 실행하고 terminal result만 권위
     있는 결과로 본다.
   - 근거: `SPEC §5.5`, `SPEC §5.6`이 이 순서와 provisional delta 의미를 명시한다.

6. Tool call streaming
   - 옵션 A: 부분 Tool arguments가 올 때마다 Runner event로 공개하고 실행을 준비한다.
   - 옵션 B: Adapter가 완성한 Tool call만 post-model과 기존 Tool loop에 전달한다.
   - 옵션 C: Phase 4.3에서 Tool lifecycle event까지 함께 공개한다.
   - Trade-off: A는 invalid JSON과 중단 stream을 실행 경계로 누출한다. B는 첫 Tool 실행까지 기다리지만 현재
     validation contract를 지킨다. C는 Phase 4.4 event와 execution backend 범위를 선점한다.
   - 채택안: 옵션 B. Text만 공개하고 Tool call은 response terminal에서 완성한다.
   - 근거: `SPEC §5.4`, `SPEC §5.9`와 Phase 4.4 제외 범위를 지킨다.

7. Structured output 검증 시점
   - 옵션 A: JSON delta마다 부분 schema 검증을 수행한다.
   - 옵션 B: Delta는 그대로 전달하고 final answer 전체만 기존 validator로 검증한다.
   - 옵션 C: Schema가 있으면 모든 delta를 숨긴다.
   - Trade-off: A는 부분 JSON이 문서가 아니므로 기존 validator contract와 맞지 않는다. B는 invalid provisional
     text가 보일 수 있지만 final 성공 의미를 유지한다. C는 안전하지만 schema run의 streaming 가치를 없앤다.
   - 채택안: 옵션 B. Structured output 성공은 validator 이후 final event에서만 확정한다.
   - 근거: `SPEC §5.7`, `SPEC §5.8`의 명시적 사용자 결정이다.

8. Runner terminal event 표현
   - 옵션 A: Final과 error를 Go return value로만 반환한다.
   - 옵션 B: Text, final, error를 모두 `RunnerStreamEvent`로 표현하고 terminal event에 `RunnerResult`를 넣는다.
   - 옵션 C: Channel close를 성공으로, 별도 error channel을 실패로 사용한다.
   - Trade-off: A는 iterator consumer가 마지막 상태를 별도 API에서 받아야 한다. B는 하나의 순서 안에서 terminal
     단일성을 검증할 수 있고 기존 Agent 상태를 보존한다. C는 두 stream의 ordering 문제가 생긴다.
   - 채택안: 옵션 B. 완전 소비 시 final 또는 error event가 정확히 한 번 발생한다.
   - 근거: `SPEC §5.1`, `SPEC §5.2`, `SPEC §5.9`의 관찰 계약과 맞는다.

9. CLI final-only 화면
   - 옵션 A: 모든 delta를 stdout에 append하고 final도 이어서 출력한다.
   - 옵션 B: Interactive terminal은 cursor를 저장해 임시 delta를 표시한 뒤 지우고 final을 다시 출력하며,
     redirect는 delta를 무시한다.
   - 옵션 C: Full-screen TUI library를 도입한다.
   - Trade-off: A는 Tool step 중간 text가 최종 출력에 남고 redirect 출력도 오염된다. B는 최소 ANSI renderer와
     terminal 판별이 필요하지만 final-only UX와 pipe 계약을 모두 만족한다. C는 가장 풍부하지만 명시적 제외 범위다.
   - 채택안: 옵션 B. Renderer 동작은 stdout이 character device인지 여부를 명시적으로 주입해 테스트한다.
   - 근거: `SPEC §5.12`, `SPEC §5.13`, `SPEC §5.16`의 사용자 확정 UX다.

10. Terminal 판별과 의존성
    - 옵션 A: `os.File.Stat`의 character device 여부를 사용한다.
    - 옵션 B: `golang.org/x/term` 같은 새 module로 판별한다.
    - 옵션 C: 항상 interactive renderer를 사용한다.
    - Trade-off: A는 표준 library만 사용하고 pipe·일반 파일을 구분할 수 있다. B는 terminal 판별 helper를
      제공하지만 현재 완료 조건에 필요하지 않은 의존성을 추가한다. C는 redirect에 제어 문자를 흘린다.
    - 채택안: 옵션 A. 실제 판별 결과를 `run` 경계에 주입해 renderer 로직과 분리한다.
    - 근거: `SPEC §5.13`을 새 의존성 없이 만족하고 기존 CLI 테스트 방식을 유지한다.

11. Claude SSE 처리
    - 옵션 A: 고정 크기 `bufio.Scanner` token으로 data line을 읽는다.
    - 옵션 B: `bufio.Reader`로 SSE field와 blank-line event 경계를 읽고 JSON data를 해석한다.
    - 옵션 C: SSE 외부 library를 추가한다.
    - Trade-off: A는 긴 Tool JSON에서 scanner 상한에 걸릴 수 있다. B는 multi-line data와 event 이름을 직접
      처리해야 하지만 표준 library만으로 protocol 경계를 보존한다. C는 구현량을 줄일 수 있으나 새 의존성이 필요하다.
    - 채택안: 옵션 B. Ping과 unknown event는 무시하고 error event와 잘못된 sequence는 provider 오류로 변환한다.
    - 근거: Claude 공식 event 계약과 `SPEC §5.3`, `SPEC §5.9`, `SPEC §5.15`에 맞는다.

12. Ollama NDJSON 처리
    - 옵션 A: line scanner로 각 chunk를 읽는다.
    - 옵션 B: `json.Decoder`로 연속 JSON object를 decode하고 `done: true`를 terminal로 사용한다.
    - 옵션 C: 응답 body 전체를 읽은 뒤 줄로 나눈다.
    - Trade-off: A는 scanner 크기 설정이 필요하다. B는 JSON whitespace와 chunk 크기를 decoder에 맡기면서
      도착 순서대로 처리한다. C는 streaming latency와 메모리 이점을 없앤다.
    - 채택안: 옵션 B. Content와 Tool calls를 누적하고 final chunk의 usage·done reason을 사용한다.
    - 근거: Ollama NDJSON contract와 `SPEC §5.3`, `SPEC §5.4`, `SPEC §5.9`를 만족한다.

13. Non-streaming provider 경로
    - 옵션 A: 기존 `Chat`도 내부적으로 streaming request를 전부 소비하도록 바꾼다.
    - 옵션 B: 기존 non-streaming HTTP 경로를 유지하고 request·decode helper만 공유한다.
    - 옵션 C: Non-streaming provider 호출을 제거한다.
    - Trade-off: A는 조립 코드를 공유하지만 transport 의미와 기존 custom test를 바꾼다. B는 일부 parser가
      분리되지만 회귀 범위를 제한한다. C는 명세와 호환되지 않는다.
    - 채택안: 옵션 B. Phase 4.3은 streaming을 추가하며 기존 호출의 transport까지 바꾸지 않는다.
    - 근거: `SPEC §5.14`와 현재 provider adapter 테스트 contract를 보존한다.

미해결 Decision Point는 없다.
