# Phase 2 Agent Loop 분석

## 근거

확인한 사실:

- `spec.md`는 Phase 2 범위를 새 `internal/agent` 패키지와 기존 `internal/message`, `internal/llm` contract의 사용
  경계로 제한한다.
- `SPEC §5.1`과 `SPEC §5.2`는 사용자 입력을 `AgentState` 메시지에 저장하고, 같은 메시지 상태를 LLM 요청에
  전달하며, assistant 응답을 순서대로 누적해야 한다고 요구한다.
- `SPEC §5.3`과 `SPEC §5.4`는 assistant 응답의 tool call 유무로 final 상태와 추가 행동 필요 상태를 구분하고,
  final answer text와 tool call 정보를 호출자가 확인할 수 있어야 한다고 요구한다.
- `SPEC §5.5`와 `SPEC §5.6`은 max step 종료와 LLM 호출 오류를 별도 경로로 구분 가능하게 요구한다.
- `SPEC §5.7`과 `SPEC §5.8`은 메모리 안 trace 구조와 외부 provider 없는 stub `LLMClient` 테스트를 요구한다.
- `internal/message.Message`는 `Role`, `Text`, `ToolCalls`, `ToolResult`를 포함하고, `message.Assistant`가 tool call
  정보를 보존한다.
- `internal/llm.LLMClient`는 `Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)` contract를 제공하고,
  `ChatRequest`는 `Model`과 `[]message.Message`, `ChatResponse`는 assistant `Message`, provider, model, stop reason,
  usage metadata를 포함한다.
- 현재 `cmd/agent-runtime`은 사용자 prompt를 읽어 `message.User(prompt)` 하나로 단발 `LLMClient.Chat`을 호출한다.
  Phase 2 spec은 기존 CLI 실행 contract 변경을 제외 범위로 둔다.
- `features/20260628-002-phase-1-llm-client/analysis.md`는 메시지 타입을 `internal/message`가 소유하고, provider별
  JSON 변환은 `internal/llm` adapter 경계에 둔다는 구조를 채택했다.
- 프로젝트 안에서 별도 `AGENTS.md`와 `docs/languages.md`는 확인되지 않았다.

추정:

- Phase 2에서 tool 실행이 제외되므로 일반적인 run은 user message 저장 뒤 LLM step 한 번에서 final 또는 tool 대기
  상태로 종료된다.
- max step 종료 경로는 이후 tool 실행 loop가 붙으면 더 자주 쓰이지만, Phase 2에서는 `MaxSteps`가 0이거나 이미
  허용 step을 모두 사용한 상태에서 LLM 호출을 막는 경로로 테스트할 수 있다.

## 1. 구조

Phase 2는 새 `internal/agent` 패키지가 Agent run의 상태, step 진행, 종료 사유, trace를 소유하는 구조로 둔다.
`internal/agent`는 `internal/llm.LLMClient`를 주입받아 호출하고, 메시지 표현은 기존 `internal/message.Message`를
그대로 사용한다. Provider 선택, HTTP 요청 변환, timeout 분류, provider별 오류 변환은 기존 `internal/llm` 경계에
남긴다. 이렇게 해야 Agent loop가 Claude, Ollama 같은 provider 구현을 직접 알지 않으면서 상태 누적과 종료 판단만
담당한다(SPEC §5.1, SPEC §5.2, SPEC §5.6).

실행 객체는 `Agent`라는 이름으로 두고 LLM client, model, max step 설정을 보관하는 작은 타입으로 제한한다.
생성 옵션은 `Options{Client, Model, MaxSteps}` 수준으로 제한하고, provider 설정이나 CLI 입력 처리는 받지 않는다.
실행 API는 사용자 입력 하나를 받아 새 `AgentState`를 만들고 run이 끝난 최종 상태를 반환한다. 오류도 상태 안의
error 경로에 반영하므로, LLM 호출 오류를 Go return error만으로 숨기지 않는다(SPEC §5.1, SPEC §5.5, SPEC §5.6).

`AgentState`는 run의 단일 소유 상태다. 필요한 필드는 `Messages []message.Message`, `Step int`, `Status`, `FinalAnswer`,
`PendingToolCalls []message.ToolCall`, `LastError error`, `Trace []TraceEvent`다. `Messages`는 LLM 입력으로 전달되는
대화 상태이자 호출자가 최종 상태를 확인하는 근거다. `FinalAnswer`와 `PendingToolCalls`는 마지막 assistant message에서
도출되는 편의 상태이며, 원본 정보는 반드시 `Messages` 안에도 남긴다(SPEC §5.2, SPEC §5.3, SPEC §5.4).

`Status`는 최소한 `running`, `final`, `needs_action`, `max_steps`, `error`를 구분한다. Phase 2에서는 tool call이 있으면
tool 실행을 시도하지 않고 `needs_action`으로 종료한다. `max_steps`는 실패 오류가 아니라 허용 step 정책에 따른 종료
상태로 다루며, `error`는 `LLMClient.Chat`이 error를 반환한 경우에만 사용한다(SPEC §5.4, SPEC §5.5, SPEC §5.6).

Trace는 별도 logger나 export format이 아니라 `AgentState.Trace`에 저장되는 메모리 구조다. 각 trace event는 step,
action, result 또는 error, 종료 사유를 담을 수 있어야 한다. Trace action 이름은 구현 세부 로그 문구가 아니라 테스트
가능한 상태 전이를 표현한다. 예를 들어 user message 저장, LLM 요청, LLM 응답 수신, final 종료, tool 대기 종료,
max step 종료, LLM 오류를 구분한다(SPEC §5.7, SPEC §5.8).

## 2. 데이터 흐름

정상 final 흐름은 호출자가 `Agent.Run(ctx, input)` 같은 실행 API를 호출하면서 시작한다. Agent는 빈 `AgentState`를
만들고 `message.User(input)`을 `Messages`에 append한다. 그 뒤 LLM 호출 직전에 `Step`과 `MaxSteps`를 비교해 허용
step을 넘는 호출을 막는다. 호출이 허용되면 현재 `Messages`를 그대로 `llm.ChatRequest{Model, Messages}`에 담아
`LLMClient.Chat`을 호출한다(SPEC §5.1, SPEC §5.5).

LLM이 assistant message를 반환하면 Agent는 응답 message를 `Messages` 끝에 append하고 trace에 응답 수신 사실을
기록한다. 반환된 assistant message에 tool call이 없으면 `Status`를 `final`로 바꾸고 `FinalAnswer`에
`resp.Message.Text`를 저장한다. 이때 최종 text는 편의 필드에만 두지 않고 assistant message의 `Text`에도 그대로
남겨 호출자가 누적 메시지와 final answer를 모두 확인할 수 있게 한다(SPEC §5.2, SPEC §5.3, SPEC §5.7).

Assistant message에 하나 이상의 tool call이 있으면 Agent는 tool registry나 tool executor를 호출하지 않는다.
대신 assistant message를 상태에 append한 뒤 `PendingToolCalls`에 같은 tool call 목록을 복사하고 `Status`를
`needs_action`으로 바꾼다. tool call의 ID, name, arguments는 `message.ToolCall` 그대로 보존한다. 이 흐름은 Phase 2의
안전한 멈춤 지점이며, 이후 Tool Calling Runtime은 이 상태를 이어받아 tool 실행과 tool result message 생성을 붙일 수
있다(SPEC §5.4).

Max step 흐름은 LLM 호출 직전에 처리한다. `Step >= MaxSteps`이면 Agent는 새 LLM 요청을 만들지 않고 `Status`를
`max_steps`로 종료하며 trace에 종료 사유를 남긴다. 호출 전에 검사해야 실제 provider 호출 횟수가 설정된 max step을
넘지 않는다. Phase 2에서는 tool 실행이 없어 일반 성공 경로가 한 step에서 끝나므로, max step 테스트는 `MaxSteps: 0`
또는 이미 허용 step을 사용한 상태를 만들 수 있는 내부 테스트 helper로 확인하는 편이 가장 좁은 범위다(SPEC §5.5,
SPEC §5.8).

LLM 오류 흐름은 `LLMClient.Chat`이 error를 반환할 때 시작한다. Agent는 assistant message를 append하지 않고
`Status`를 `error`로 바꾸며 `LastError`에 원인 error를 저장한다. Trace에는 어떤 step의 LLM 호출에서 실패했는지와
오류 값을 남긴다. Provider 오류 분류 자체는 `internal/llm`의 책임이므로 Agent는 오류 종류를 재분류하지 않는다.
호출자는 `LastError`와 `Status`로 실패 상태와 원인을 확인한다(SPEC §5.6, SPEC §5.7).

## 3. 인터페이스

`internal/agent`의 주 인터페이스는 외부 provider를 숨긴 내부 Runtime API다. 추천 형태는 다음 의미를 갖는다.

```go
type Options struct {
	Client   llm.LLMClient
	Model    string
	MaxSteps int
}

type Agent struct {
	// fields are unexported
}

func New(opts Options) (*Agent, error)
func (a *Agent) Run(ctx context.Context, input string) AgentState
```

`New`는 nil client처럼 실행 자체가 불가능한 조립 오류만 반환한다. `Model`은 기존 `llm.ChatRequest.Model`로 그대로
전달하되 provider별 필수 model 검증은 `internal/llm`에 남긴다. `MaxSteps`는 Agent 정책이므로 Agent가 해석한다.
`Run`은 run 중 LLM 오류를 return error로 분리하지 않고 `AgentState.Status`와 `AgentState.LastError`에 담는다. 이
방식은 호출자가 성공과 실패 모두에서 메시지와 trace를 같은 경로로 확인해야 한다는 완료 조건에 맞다(SPEC §5.1,
SPEC §5.5, SPEC §5.6, SPEC §5.7).

`AgentState`는 테스트와 이후 런타임 조립 코드가 읽을 수 있는 구조체로 둔다. `Messages`는 append 순서를 보존해야
하고, 외부 호출자가 상태를 임의로 고쳐 run 중 불변식을 깨지 않게 하려면 `Run`이 최종 값 복사본을 반환하는
방식이 적합하다. Phase 2는 동시성 실행이나 long-running shared session을 요구하지 않으므로, `AgentState`를 전역
저장소나 공유 mutable session으로 만들지 않는다(SPEC §5.1, SPEC §5.2).

`TraceEvent`는 내부 관찰 contract다. 외부 JSON field 이름이나 로그 문구를 고정하지 않고 Go 타입 수준에서 action,
step, status, error를 확인할 수 있게 한다. `TraceEvent`가 `llm.ChatRequest` 전체나 provider response 전체를
그대로 들고 있으면 이후 비밀값이나 큰 payload 노출 정책을 먼저 고정해야 하므로, Phase 2에서는 step 번호와 action,
결과 상태, 오류 참조 정도로 제한한다(SPEC §5.7).

`cmd/agent-runtime`의 CLI contract는 Phase 2에서 바꾸지 않는다. 따라서 `internal/agent` API는 CLI에서 바로 사용하지
않아도 되며, CLI 출력 방식이나 exit code를 Agent 상태와 동기화하는 작업은 이후 단계에서 다룬다. Phase 2 테스트는
`internal/agent` 패키지 안에서 stub `LLMClient`를 사용해 정상 final, tool 대기, max step, LLM 오류, trace 기록을
확인하는 방식이 적합하다(SPEC §5.8).

## 4. 영향 범위

새로 추가되는 주된 영향 범위는 `internal/agent` 패키지와 그 테스트다. 이 패키지는 Agent 상태 타입, 상태 enum,
trace event 타입, 실행 객체와 실행 API를 포함한다. 기존 `internal/message`와 `internal/llm`의 타입을 재사용하므로
provider adapter나 메시지 생성자의 기존 contract를 바꾸지 않는다(SPEC §5.1, SPEC §5.2, SPEC §5.4).

`internal/message`는 Phase 2의 tool 대기 상태에서 `message.ToolCall` 보존 근거로 사용된다. 현재 타입은 ID, name,
arguments를 이미 가지고 있어 Phase 2 요구사항을 충족한다. `message.ToolResult`와 `message.Tool`은 Phase 2에서
실행하지 않지만, 이후 Tool Calling Runtime이 같은 메시지 목록에 tool result를 append할 수 있는 확장 지점으로
남긴다(SPEC §5.4).

`internal/llm`은 Agent가 호출하는 provider-neutral 경계다. Phase 2는 `LLMClient` interface, `ChatRequest`,
`ChatResponse`를 수정하지 않는 방향이 가장 작다. Agent 테스트는 stub client만 사용하므로 Claude/Ollama 실제 HTTP
adapter나 provider 설정 검증에는 새 테스트 의존을 만들지 않는다(SPEC §5.6, SPEC §5.8).

`cmd/agent-runtime`은 제외 범위에 따라 변경하지 않는다. 단발 CLI가 여전히 직접 `LLMClient.Chat`을 호출해도 Phase 2
Agent API 완료 조건과 충돌하지 않는다. 이후 CLI를 Agent loop 기반으로 전환할 때는 `internal/agent` API를 조립 계층에
연결하면 된다(SPEC §5.1, SPEC §5.8).

문서 영향은 이 feature의 `analysis.md`와 `README.md` 상태 갱신에 제한된다. Phase 2 분석 단계에서는 `README.md`,
`ROADMAP.md`, `.env.example` 같은 사용자 실행 문서를 바꾸지 않는다. CLI contract와 공개 환경변수 contract를 바꾸지
않기 때문이다.

## 5. Decision Points

1. Agent 상태 소유 위치
   - 옵션 A: 새 `internal/agent` 패키지가 `AgentState`, status, trace, run 실행을 소유한다.
   - 옵션 B: 기존 `internal/llm` 또는 `cmd/agent-runtime`에 loop 상태를 붙인다.
   - trade-off: 옵션 A는 provider 호출 contract와 Agent loop 정책을 분리하고, CLI 변경 없이 테스트 가능한 내부 API를
     만들 수 있다. 옵션 B는 파일 수는 줄지만 provider adapter나 CLI 실행 contract에 loop 상태 책임이 섞인다.
   - 채택안: 옵션 A.
   - 근거: spec은 새 `internal/agent` 패키지와 기존 `internal/message`, `internal/llm` contract의 사용 경계를 Phase 2
     범위로 명시한다(SPEC §5.1, SPEC §5.6, SPEC §5.8).

2. Run 오류 반환 방식
   - 옵션 A: `Run`은 최종 `AgentState`를 반환하고, LLM 호출 오류는 `error` 상태와 `LastError`에 담는다.
   - 옵션 B: `Run`이 `(AgentState, error)`를 반환하고 LLM 오류를 return error로도 노출한다.
   - trade-off: 옵션 A는 성공, tool 대기, max step, 오류 모두를 같은 상태 관찰 모델로 테스트할 수 있다. 옵션 B는
     Go 관용 오류 처리에는 익숙하지만, 호출자가 오류 시에도 상태와 trace를 별도로 확인해야 하는 중복 경로가
     생긴다.
   - 채택안: 옵션 A.
   - 근거: 완료 조건은 LLM 오류가 Agent 상태의 error 경로에 반영되고 호출자가 실패 상태와 원인 오류를 확인해야
     한다고 요구한다(SPEC §5.6, SPEC §5.7).

3. Max step 검사 시점
   - 옵션 A: 매 LLM 호출 직전에 `Step >= MaxSteps`를 검사하고, 초과 시 호출하지 않은 채 `max_steps`로 종료한다.
   - 옵션 B: LLM 응답을 받은 뒤 다음 반복 여부를 판단하면서 max step을 검사한다.
   - trade-off: 옵션 A는 실제 LLM 호출 횟수가 설정값을 넘지 않는다는 보장을 직접 제공한다. 옵션 B는 loop 코드가
     조금 단순할 수 있지만, 경계값에서 이미 허용량을 넘긴 provider 호출이 발생할 수 있다.
   - 채택안: 옵션 A.
   - 근거: 완료 조건은 Agent run이 설정된 max step을 넘기지 않고 종료해야 한다고 요구한다(SPEC §5.5).

4. Tool call 처리 수준
   - 옵션 A: assistant message의 tool call을 상태와 메시지에 보존하고 `needs_action`으로 멈춘다.
   - 옵션 B: Phase 2에서 unknown tool 오류나 dummy tool result까지 생성한다.
   - trade-off: 옵션 A는 Tool Runtime 제외 범위를 지키면서 이후 단계가 이어받을 대기 상태를 명확히 만든다. 옵션 B는
     더 실제 loop처럼 보이지만 tool registry, unknown tool 처리, tool result 전달 의미를 Phase 2에서 먼저 고정한다.
   - 채택안: 옵션 A.
   - 근거: spec은 tool call이 있으면 tool을 실행하지 않고 추가 행동이 필요하다는 상태로 멈추며, tool 실행 관련
     흐름은 제외한다고 명시한다(SPEC §5.4).

5. Trace 저장 경계
   - 옵션 A: `AgentState.Trace`에 step, action, status, error 중심의 메모리 event만 저장한다.
   - 옵션 B: JSON export나 stdout 로그 문구까지 함께 정의한다.
   - trade-off: 옵션 A는 테스트 가능한 내부 관찰성을 제공하면서 외부 trace 형식을 고정하지 않는다. 옵션 B는
     사람이 바로 보기 쉽지만 Phase 2 제외 범위인 외부 출력 contract를 만들게 된다.
   - 채택안: 옵션 A.
   - 근거: spec은 trace를 메모리 안에서 확인 가능한 구조로 두고, 파일 저장, 로그 출력, JSON export 같은 외부 형식은
     고정하지 않는다고 명시한다(SPEC §5.7).
