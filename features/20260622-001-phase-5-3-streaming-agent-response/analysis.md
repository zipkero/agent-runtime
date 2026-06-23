# phase-5-3-streaming-agent-response 분석

## 근거

읽은 기준 문서:

- `docs/20260622-001-phase-5-3-streaming-agent-response/spec.md` 전체. 범위는 provider-neutral streaming LLM contract,
  streaming event, Runner streaming 실행, Agent streaming 조립, CLI streaming 출력, structured output final
  검증, middleware 관계다. partial structured output 검증, tool call delta 고급 조립, Multi-Agent/RAG/MCP/A2A
  streaming relay는 제외다.
- `ROADMAP.md` Phase 5.3. Phase 5.2에서 optional로 남아 있던 streaming response를 Phase 5.3으로 분리했고,
  Phase 6 RAG 전에 Runner/CLI streaming 표면을 정리하는 위치임을 확인했다.
- `docs/20260618-001-phase-5-2-agent-runtime/spec.md`와 `analysis.md`. 기존 Runner, middleware, structured output은
  비스트림 `ChatResponse` 기준이고, streaming은 명시 제외되어 있었다.

코드베이스에서 확인한 사실:

- `internal/llm.LLMClient`는 `Chat(ctx, req) (ChatResponse, error)` 단일 계약만 가진다. 현재 provider 구현은
  완성된 assistant message를 반환한다.
- `internal/llm.OllamaClient.Chat`은 `/api/chat`에 `stream:false`를 명시해 단일 JSON 응답을 받는다.
- `internal/llm.ClaudeClient.Chat`은 Anthropic SDK `Messages.New`를 사용해 단일 message 응답을 받는다.
- `internal/agent.Agent.Run`은 Graph node가 완성된 `ChatResponse`를 받고, middleware `PostModel` 실행 뒤
  `AgentState.Messages`에 assistant message를 누적한다.
- `internal/agent.Runner.Run`은 `Agent.Run` 결과를 `RunnerResult`로 매핑하고, optional `OutputContract`가 있으면
  final text를 JSON Schema로 검증한다.
- `cmd/agent-runtime.run`은 현재 항상 `Runner.Run`을 호출하고, 성공 시 final text를 stdout에 한 번 출력한다.

추정으로 분리:

- Claude와 Ollama는 provider 차원에서 streaming API를 지원하지만, 이 분석은 각 SDK/wire 세부를 확인하지 않고
  provider-neutral Runtime 계약과 책임 경계를 먼저 확정한다. provider별 wire 구현 세부는 구현 단계에서 해당 SDK와
  HTTP 응답 형식을 다시 확인해야 한다.

## 1. 구조

Phase 5.3은 기존 비스트림 경로를 유지하면서 streaming을 선택 실행 표면으로 추가한다(SPEC §5.9). 핵심 구조는
`internal/llm`의 streaming contract, `internal/agent`의 Runner/Agent streaming 조립, `cmd/agent-runtime`의 CLI
streaming 선택 경로로 나눈다.

`internal/llm`에는 기존 `LLMClient`를 바꾸지 않고 `LLMStreamer` capability interface를 둔다. 이는 별도 provider
client 인스턴스가 아니라, 기존 provider client가 선택적으로 구현하는 streaming 능력이다. 기존 provider와 테스트가
`LLMClient.Chat`에 의존하므로 이를 변경하면 Phase 5.2의 비스트림 계약을 불필요하게 흔든다(SPEC §5.9). Runner는
streaming 실행을 요청받았을 때 주입된 client가 `LLMStreamer`를 구현하는지 확인하고, 구현하지 않으면 streaming 실행
실패로 반환한다(SPEC §5.1, §5.4). 이 실패는 provider 호출 실패가 아니라 실행 표면 미지원 실패로 분류해야 한다.

streaming event는 `internal/llm`의 provider event와 `internal/agent`의 Runner event를 분리한다. provider event는
text delta와 provider 호출 완료/error 같은 낮은 수준의 model stream 결과를 표현한다. Runner event는 호출자가
Agent 실행을 관찰할 수 있는 표면으로, text delta, step 완료, final result, error를 포함한다(SPEC §5.1, §5.2).
이 분리는 provider wire 형식을 Agent Runtime 밖으로 새지 않게 한다.

Agent streaming 조립은 `internal/agent`가 소유한다. 현재 graph node는 완성된 `ChatResponse`만 다루므로, streaming을
Graph 엔진 자체에 넣지 않고 Agent의 LLM 호출 경계에서 stream을 소비해 최종 `ChatResponse`를 조립한다. 조립된
response는 기존 `PostModel` middleware와 state 누적 경로로 전달한다(SPEC §5.2, §5.7, §5.8). 이렇게 하면
`internal/graph`, `internal/message`, `internal/tool`의 기존 contract를 유지할 수 있다.

CLI는 기본 비스트림 실행을 유지하고, streaming mode를 명시적으로 선택할 때만 Runner streaming 경로를 사용한다
(SPEC §5.3, §5.9). CLI streaming mode에서는 text delta event를 stdout에 즉시 쓰고, stream 완료 후 exit code 0을
반환한다. 실패 event 또는 실행 error는 stderr와 non-zero exit code로 매핑한다(SPEC §5.4).

Structured output은 stream 완료 후 조립된 final text에만 적용한다(SPEC §5.5). streaming 중 partial JSON은 대부분
유효한 JSON이 아니므로, partial validation을 시도하면 false negative가 많고 spec 제외 범위를 위반한다. output
contract가 있을 때 streaming stdout에 이미 text delta를 출력한 뒤 final validation이 실패할 수 있으므로, CLI는
검증 실패를 stderr와 non-zero exit code로 보고한다. 이미 출력한 stdout을 되돌리지는 않는다.

## 2. 데이터 흐름

기본 비스트림 흐름은 유지한다.

```text
CLI 또는 caller
→ Runner.Run(ctx, prompt)
→ Agent.Run(ctx, prompt)
→ LLMClient.Chat(ctx, req)
→ PostModel middleware
→ AgentState 누적
→ RunnerResult
```

streaming Runner 흐름은 별도 실행 표면으로 둔다.

```text
caller
→ Runner.Stream(ctx, prompt, sink)
→ Agent streaming loop
→ PreModel middleware로 ChatRequest 변경
→ LLMStreamer.Stream(ctx, req)
→ text delta event를 sink로 전달
→ stream 완료 시 assistant message 조립
→ PostModel middleware 실행
→ AgentState 누적
→ tool call이 없으면 final result
→ tool call이 있으면 tool node 실행 후 다음 LLM streaming step
```

tool call이 없는 일반 text streaming은 text delta를 순차 전달하고, 완료 시 조립된 assistant text를 state에 누적한다
(SPEC §5.1, §5.2). tool call이 있는 경우 provider stream에서 tool call 전체를 조립할 수 있어야 tool node로 이동할
수 있다. 다만 tool call argument delta의 고급 partial UI는 제외 범위다. 따라서 Phase 5.3은 tool call을 user-facing
token stream으로 노출하지 않고, stream 완료 후 조립된 assistant message의 tool call block으로 기존 tool node를
실행한다.

CLI streaming 흐름:

```text
main
→ config/load, client 생성, prompt 읽기, registry 생성
→ streaming mode 확인
→ Runner.Stream(ctx, prompt, stdout sink)
→ text delta는 stdout에 즉시 write
→ final success면 필요한 newline 처리 후 exit 0
→ max steps, provider error, middleware error, structured output error면 stderr + exit 1
```

structured output contract가 있는 streaming 흐름:

```text
stream text delta 전달
→ stream 완료 후 final text 조립
→ PostModel middleware 적용
→ RunnerResult final text 생성
→ OutputContract JSON parse + schema validate
→ success 또는 structured_output_error
```

middleware 실패 흐름은 Phase 5.2와 같은 의미를 유지한다. PreModel 실패는 streaming provider 호출 전 실패로
전환하고, PostModel 실패는 stream 완료 후 state 누적 전 실패로 전환한다(SPEC §5.7, §5.8). provider stream 중
error가 발생하면 조립 중이던 partial text는 final result로 승인하지 않고 error event와 실패 결과로 반환한다
(SPEC §5.4).

## 3. 인터페이스

`internal/llm` streaming contract:

```go
type LLMStreamer interface {
    Stream(ctx context.Context, req ChatRequest) (ChatStream, error)
}

type ChatStream interface {
    Recv() (ChatStreamEvent, error)
    Close() error
}

type ChatStreamEvent struct {
    Type      ChatStreamEventType
    TextDelta string
    Message   message.Message
}
```

`ChatStreamEvent.Type`은 최소 `text_delta`, `message_complete`를 구분한다. error는 `Recv`의 error로 표현한다. 완료된
assistant message가 provider에서 한 번에 구성되면 `Message`로 전달할 수 있고, provider가 text delta만 제공하면
Agent가 text block을 조립한다. tool call 지원을 위해 `Message`는 최종 complete event에서 tool call block을 포함할
수 있어야 한다.

`internal/agent` Runner streaming 표면:

```go
type RunnerStreamEvent struct {
    Type      RunnerStreamEventType
    TextDelta string
    Result    *RunnerResult
    Err       error
}

type RunnerStreamSink interface {
    OnEvent(ctx context.Context, event RunnerStreamEvent) error
}

func (r *Runner) Stream(ctx context.Context, prompt string, sink RunnerStreamSink) RunnerResult
```

`Runner.Stream`은 event를 sink로 전달하면서 최종적으로 `RunnerResult`를 반환한다(SPEC §5.2). sink가 error를 반환하면
streaming 실행을 중단하고 `RunnerStatusAgentError`로 매핑한다. final event에는 기존 `RunnerResult`와 같은 final
text, final message, AgentState, structured output 필드가 들어간다.

CLI는 사용자 visible flag 또는 config로 streaming mode를 선택해야 한다. 기존 `run` 시그니처를 바로 깨기보다
`runOptions` 같은 내부 구조를 둘 수 있다. 외부 CLI 명령 계약은 아직 단일 binary stdin 기반이므로, 구현 단계에서
flag parsing을 추가하더라도 streaming 미지정 기본값은 false다(SPEC §5.3, §5.9).

## 4. 영향 범위

변경되는 범위:

- `internal/llm`: `LLMStreamer`, stream event, stream reader contract를 추가한다. 기존 `LLMClient.Chat`은 유지한다.
- `internal/llm/ollama.go`: `/api/chat` streaming 응답을 provider-neutral event로 변환하는 경로를 추가한다.
  기존 `stream:false` Chat 경로는 유지한다.
- `internal/llm/claude.go`: Anthropic SDK streaming 경로를 provider-neutral event로 변환하는 경로를 추가한다.
  기존 `Messages.New` Chat 경로는 유지한다.
- `internal/agent`: Runner streaming 실행 표면, Agent streaming 조립, final result 매핑, structured output final
  검증 연결을 추가한다.
- `cmd/agent-runtime`: streaming mode 선택 입력과 stdout/stderr/exit code 분기를 추가한다. 기본 비스트림 경로는
  유지한다.
- 테스트: `internal/llm`, `internal/agent`, `cmd/agent-runtime`에 provider stream 변환, Runner event, CLI stdout
  streaming, 실패 경로, 기존 비스트림 회귀 테스트를 추가한다.

변경하지 않는 범위:

- `internal/graph`: streaming event engine으로 확장하지 않는다. Agent LLM node 경계에서 stream을 소비한 뒤 기존
  state update를 만든다.
- `internal/message`: 새 content block 종류를 추가하지 않는다. 최종 조립 결과는 기존 text/tool call block을 사용한다.
- `internal/tool`: tool interface, registry, dispatcher, timeout 정책은 유지한다.
- RAG, Memory, Multi-Agent, MCP, A2A: Phase 5.3에서는 streaming relay를 다루지 않는다.

## 5. Decision Points

### D1. streaming을 기존 `LLMClient.Chat`에 통합할 것인가

- 옵션 A: `ChatRequest`에 streaming flag를 추가하고 `Chat`이 channel 또는 callback을 반환하게 바꾼다.
- 옵션 B: 기존 `LLMClient`는 유지하고 `LLMStreamer` capability interface를 추가한다.
- 트레이드오프: A는 표면이 하나지만 기존 모든 provider와 테스트의 반환 계약을 깨뜨린다. B는 provider가 streaming을
  선택적으로 구현할 수 있고 기존 비스트림 경로를 보존한다. 다만 Runner는 streaming 지원 여부를 런타임에
  확인해야 한다.
- 채택: **B**. SPEC §5.9가 기존 비스트림 contract 유지를 요구하므로 streaming은 같은 provider client가 선택 구현하는
  capability interface로 둔다.

### D2. Runner streaming API는 channel 반환인가 sink callback인가

- 옵션 A: `Stream(ctx, prompt) (<-chan RunnerStreamEvent, RunnerResult)` 형태로 channel을 반환한다.
- 옵션 B: `Stream(ctx, prompt, sink)` 형태로 callback/sink를 받아 event를 밀어 넣고 final `RunnerResult`를 반환한다.
- 트레이드오프: A는 Go 관용적인 소비가 쉽지만 final result와 channel drain, goroutine lifecycle을 호출자가 관리해야
  한다. B는 CLI stdout write와 테스트 캡처가 단순하고 final result 반환이 명확하지만, sink 구현이 필요하다.
- 채택: **B**. 이번 Runtime은 CLI와 동기 실행 함수가 중심이므로 sink 기반이 exit code와 final result 매핑에 단순하다.

### D3. PostModel middleware를 언제 실행할 것인가

- 옵션 A: 각 text delta마다 PostModel을 실행한다.
- 옵션 B: stream 완료 후 조립된 `ChatResponse`에 한 번 실행한다.
- 옵션 C: streaming 전용 middleware hook을 새로 만든다.
- 트레이드오프: A는 기존 PostModel 계약과 맞지 않는 partial response를 계속 전달한다. C는 강력하지만 middleware
  contract를 새로 설계해야 하며 이번 spec의 기존 PostModel 관계 요구를 넘는다. B는 기존 PostModel 의미를 유지하고
  stream 완료 후 state 누적 전 변경·실패 전환을 보존한다.
- 채택: **B**. SPEC §5.8은 기존 PostModel middleware가 stream 완료 후 조립된 response에 적용되는 것을 요구한다.

### D4. structured output을 streaming 중에 검증할 것인가

- 옵션 A: text delta가 올 때마다 partial JSON 검증을 시도한다.
- 옵션 B: stream 완료 후 final text에만 기존 output contract 검증을 적용한다.
- 트레이드오프: A는 사용자에게 빠른 형식 오류를 줄 수 있지만 partial JSON은 대부분 invalid라 오탐이 많고 제외
  범위를 위반한다. B는 검증 시점이 늦지만 기존 structured output 의미와 일치한다.
- 채택: **B**. SPEC §5.5와 제외 범위가 partial structured output 검증을 명시적으로 배제한다.

### D5. streaming stdout과 structured output 실패를 어떻게 함께 다룰 것인가

- 옵션 A: output contract가 있으면 stdout streaming을 중단하고 final 검증 후 한 번에 출력한다.
- 옵션 B: output contract가 있어도 text delta를 stdout에 출력하고, final 검증 실패는 stderr와 exit code로 보고한다.
- 트레이드오프: A는 invalid JSON 노출을 막지만 streaming UX를 잃는다. B는 streaming UX를 유지하지만 검증 실패 시 이미
  stdout에 출력된 내용을 되돌릴 수 없다.
- 채택: **B**. SPEC §5.3은 CLI streaming mode에서 chunk를 stdout에 순차 출력해야 하며, SPEC §5.5는 final 검증을
  요구한다. 검증 실패는 stderr와 non-zero exit code로 관찰하게 한다.

### D6. Graph engine을 streaming-aware로 바꿀 것인가

- 옵션 A: `internal/graph`가 node event stream을 직접 다루도록 확장한다.
- 옵션 B: Agent LLM node 내부에서 stream을 소비하고 기존 `NodeResult`로 최종 state update만 반환한다.
- 트레이드오프: A는 범용 graph streaming으로 확장성이 있지만 Phase 5.3 범위를 넘어 graph contract를 크게 바꾼다.
  B는 Agent streaming 요구를 충족하면서 기존 graph와 tool node를 유지한다.
- 채택: **B**. SPEC §5.9가 기존 graph 기반 tool call 실행 회귀를 요구하고, streaming relay는 Agent 실행 표면의
  책임으로 충분하다.
