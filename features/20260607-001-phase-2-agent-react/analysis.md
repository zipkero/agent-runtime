# phase-2-agent-react — 분석과 설계

## 근거

### 읽은 spec 범위

`spec.md` 전체를 읽었다. 범위는 §1·§4가 정의한다. Phase 2는 `internal/agent` 패키지에
`AgentState`·`Agent`·ReAct loop·step counter·max step·final answer detection·error
state·reflection hook을 세우고, CLI 진입점을 단발 Chat에서 Agent loop로 바꾸는 것까지다
(SPEC §1, §5.7). tool의 실제 실행·검증·결과 재투입은 명시적으로 Phase 3로 제외됐다
(SPEC §4). 완료 조건 §5.1–§5.8을 설계 동작의 출처로 삼았다.

`ROADMAP.md` Phase 2(line 93–129)도 읽었다. spec과 동일한 구현 범위를 가리키며,
"LLM 응답을 보고 다음 행동을 Runtime이 해석한다", "무한 루프 방지를 위한 max step"이
핵심 학습 포인트다. 구현 위치는 `internal/agent`로 못박혀 있다(line 113, 641).

### 코드베이스에서 확인한 사실

- `internal/message/message.go`: `Message{Role, Content}`, `ContentBlock`(Text/ToolCall/
  ToolResult), `Role`(user·assistant·tool·system), `ToolCall`, `ToolResult`,
  `ToolSpec`가 정의돼 있다. 특히 `Message.HasToolCalls()`(line 34–41)가 이미 있어
  final answer 판정 술어로 그대로 재사용 가능하다 — tool_call 블록이 하나도 없으면 최종
  답으로 본다. 이 패키지는 다른 internal 패키지를 import하지 않는 최하위 의존이다.
- `internal/llm/llm.go`: `LLMClient` interface는 `Chat(ctx, ChatRequest) (ChatResponse,
  error)` 단일 메서드다. `ChatRequest{Model, Messages, Tools}`, `ChatResponse{Message}`.
  Agent는 이 interface에만 의존하고 provider 구현체(`ClaudeClient`)에는 의존하지 않는다.
- `internal/llm/stub.go`: `StubClient`는 `Response` 하나 또는 `Err` 하나를 고정 반환한다
  (line 8–33). ctx 취소를 먼저 존중한다(line 25). **단일 응답만 반환하므로, step마다
  다른 응답이 필요한 ReAct loop의 다단계 검증에는 그대로는 부족하다.** §5 Decision Point에서
  다룬다.
- `internal/llm/stub_test.go`: 결정적 테스트가 stub에 고정 응답/고정 에러/만료된 deadline
  ctx를 주입해 실제 API 없이 경로를 검증하는 패턴을 확인했다. Phase 2 테스트도 이 결정적
  주입 패턴을 따른다.
- `cmd/agent-runtime/main.go`: `run(ctx, client, model, prompt) int`(line 58–77)가
  `ChatRequest`를 만들어 `client.Chat`을 **한 번** 호출하고 `printResponse`로 출력한다.
  이 단발 호출이 Phase 2에서 Agent loop로 대체될 지점이다(SPEC §5.7).
- `cmd/agent-runtime/main_test.go`: `run`을 직접 호출해 stdout/stderr/종료코드를 검증한다
  (line 61, 91, 111, 135). 즉 `run`의 시그니처·동작 변경은 이 테스트 파일에 직접 전파된다 —
  영향 범위에 포함된다(확인됨, grep `run(` 결과 일치).
- `internal/config/config.go`: `Config{AnthropicAPIKey, Model, Timeout}`. model·timeout
  주입 방식은 `main`이 `cfg`에서 꺼내 `run`에 넘기는 형태(`cfg.Model`, `cfg.Timeout`)다.
  max step 같은 Agent 실행 한도는 config에 아직 없다 — §5에서 주입 경로를 다룬다.

### 추정

- main_test.go가 `run`을 직접 호출하므로, `run`이 Agent loop를 호출하는 형태로 바뀌면
  기존 4개 테스트는 의미가 바뀌거나 갱신이 필요하다(이건 implement.md 소관). 분석 단계에서는
  영향 범위로만 기록한다.

## 1. 구조

Phase 2는 새 패키지 `internal/agent` 하나를 추가하고, 진입점 `cmd/agent-runtime/main.go`를
loop 기반으로 바꾼다. 의존 방향은 `agent → llm.LLMClient`(interface) + `agent → message`이며,
provider 구현체로의 역방향 의존은 없다(SPEC §3).

패키지는 세 가지 책임을 담는다.

- **상태(AgentState)**: 한 번의 실행 동안 누적되는 대화 메시지, 진행한 step 수, 그리고 실행이
  어떤 종료 상태에 있는지를 담는 값. loop의 입력이자 출력이며, 호출자가 결과를 관찰하는 표면이다
  (SPEC §5.1, §5.2).
- **실행기(Agent)**: 주입된 `LLMClient`를 들고 `AgentState` 위에서 ReAct loop를 도는 단위.
  매 step마다 LLM을 호출하고 응답을 state에 누적하며, 종료 조건을 판정한다(SPEC §5.2, §5.3).
  Runtime이 "계속할지/멈출지"를 결정하는 주체라는 점을 이 단위가 코드로 드러낸다.
- **확장 지점(reflection hook)**: step 경계에서 현재 step·누적 state를 관찰할 수 있는 훅.
  Phase 2에서는 관찰만 하며, 기본은 아무 일도 하지 않는다(SPEC §5.6).

CLI 쪽은 `run`이 단발 `client.Chat` 대신 Agent를 만들어 loop를 한 번 돌리고, 종료 상태에 따라
최종 답을 stdout에, 실패를 stderr에 출력하는 형태로 바뀐다(SPEC §5.7).

## 2. 데이터 흐름

### 종료 상태 집합

`AgentState`는 항상 다음 네 종료 상태 중 하나에 있다. 이 집합이 loop 판정과 호출자 관찰의
기준이다.

- **진행중(running)**: 아직 최종 답에 도달하지 않았고 step 여유가 남아 loop가 계속 도는 상태.
- **최종답 도달(final)**: tool_call이 없는 assistant 응답을 받아 최종 답으로 판정해 종료한
  상태. 호출자는 이 상태에서 최종 답 메시지를 얻는다(SPEC §5.3).
- **max step 초과(max steps)**: step counter가 상한에 도달했는데도 최종 답에 이르지 못해
  안전 종료한 상태. 호출자는 이것이 final이 아님을 구분할 수 있어야 한다(SPEC §5.4).
- **에러(error)**: LLM 호출이 에러를 반환해 안전 종료한 상태. 호출자는 그 사실과 원인(에러)을
  확인할 수 있다(SPEC §5.5).

`final`·`max steps`·`error`는 종료 상태이며, loop는 이 중 하나에 도달하면 더 이상 LLM을
호출하지 않는다.

### loop 한 회전

1. **초기화**: 사용자 입력을 `RoleUser` 메시지로 만들어 state의 대화에 넣고, step 0, 상태
   running으로 시작한다. 시작 시점 state에서 그 입력을 확인할 수 있다(SPEC §5.1).
2. **step 경계 — 진입**: 매 회전 시작에서 reflection hook을 호출해 현재 step과 지금까지
   누적된 state를 관찰하게 한다(SPEC §5.6). 기본 훅은 no-op.
3. **max step 검사**: step counter가 상한에 도달했으면 LLM을 더 호출하지 않고 max steps
   상태로 종료한다(SPEC §5.4). 이 검사가 호출 **앞**에 있어야 어떤 응답 패턴에서도 loop가
   무한히 돌지 않는다(SPEC §3, §5.4).
4. **LLM 호출**: state의 누적 메시지로 `ChatRequest`를 만들어 `LLMClient.Chat(ctx, ...)`을
   호출한다. ctx는 호출자가 준 것을 그대로 전파해 취소·timeout이 LLM 호출까지 닿는다(SPEC §3).
   호출이 에러를 반환하면 그 에러를 state에 담고 error 상태로 종료한다(SPEC §5.5).
5. **누적**: 받은 assistant 응답을 state의 대화에 append하고 step counter를 1 증가시킨다.
   step이 진행될수록 state에 쌓인 메시지가 늘어난다(SPEC §5.2).
6. **종료 판정**: 응답에 tool_call이 없으면(`HasToolCalls()`가 false) 최종 답으로 보고
   final 상태로 종료한다(SPEC §5.3). tool_call이 있으면 — Phase 2에서는 **실행하지 않고**
   — 단지 "최종 답이 아니다"로 해석해 running을 유지하고 1로 돌아간다.

### tool_call 응답의 처리(Phase 2 경계)

Phase 2의 가장 중요한 경계다. tool_call이 온 응답은 "아직 최종 답이 아님"을 뜻하는 **신호로만**
쓰인다. tool을 실행하지도, tool_result를 만들어 state에 넣지도 않는다(SPEC §4). 따라서 stub이
계속 tool_call 응답을 돌려주면 loop는 결코 final에 닿지 못하고 step을 소진해 max steps에서
안전 종료한다. 이것이 tool 실행 없이 step counter·max step·종료 상태를 결정적으로 입증하는
자연스러운 실패 케이스이며(SPEC §5.8, §3, ROADMAP 중단 기준), 정상 경로(stub이 text 응답을
주면 즉시 final)와 짝을 이룬다.

### reflection hook 호출 시점

훅은 step 경계 — 즉 매 회전의 시작(LLM 호출 전, step 진행 상황이 확정된 지점) — 에서 호출된다.
훅은 현재 step 번호와 누적 state를 관찰 대상으로 받는다. Phase 2의 책임은 "관찰 가능한 확장
지점이 존재한다"까지이며, 개입(메시지 주입·중단 등)은 spec 범위가 아니다(SPEC §5.6).

## 3. 인터페이스

경계를 가로지르는 표면만 기술한다. 내부 helper·loop 본문·필드 세부는 implement.md 소관이다.

- **Agent 생성**: `LLMClient`·model·max step·reflection hook을 주입받아 `Agent`를 만드는
  생성 표면. provider 구현체가 아니라 interface를 받는다(SPEC §3). max step과 hook은 외부에서
  결정해 주입하는 값이다(§5 참조).
- **Agent 실행**: `ctx context.Context`와 사용자 입력(프롬프트)을 받아, loop를 끝까지 돌린 뒤
  종료된 `AgentState`를 반환하는 단일 실행 표면. ctx로 취소·timeout이 LLM 호출까지 전파된다
  (SPEC §3). 반환된 state로 호출자가 모든 결과를 관찰한다.
- **AgentState 관찰 표면**: 호출자가 종료 후 다음을 구분·취득할 수 있어야 한다 — (a) 종료 상태가
  final/max steps/error 중 무엇인지, (b) final이면 최종 답 메시지, (c) error면 원인 에러,
  (d) 누적된 대화 메시지와 진행한 step 수(SPEC §5.1–§5.5). 종료 상태 식별과 최종 답 취득이
  핵심이다.
- **reflection hook 형태**: step 경계에서 현재 step과 누적 state를 받아 관찰하는 호출 가능한
  확장점. 기본값은 아무 동작도 하지 않는 형태여야 한다(주입하지 않아도 loop가 돈다)(SPEC §5.6).
  인터페이스/함수콜백/no-op 기본 중 어느 형태로 둘지는 §5 Decision Point다.
- **CLI와의 계약**: `cmd/agent-runtime`의 `run`은 Agent를 만들어 실행하고, 반환된 state의
  종료 상태로 출력을 가른다 — final이면 최종 답을 stdout에, error/취소면 원인을 stderr에 쓰고
  비정상 종료코드를 낸다(SPEC §5.7). max steps 종료를 CLI가 어떻게 표현할지(stdout vs stderr,
  종료코드)는 §5 Decision Point다.

## 4. 영향 범위

- **신규 `internal/agent` 패키지(주 구현)**: `AgentState`·`Agent`·loop·step counter·max
  step·final answer detection·error state·reflection hook이 여기에 생긴다. `llm`·`message`만
  import한다(ROADMAP line 113).
- **`cmd/agent-runtime/main.go`**: `run`이 단발 `client.Chat`에서 Agent loop 호출로 바뀐다
  (SPEC §5.7). `printResponse`는 최종 답 출력에 재사용되거나 종료 상태별 출력 분기로 조정될 수
  있다.
- **`cmd/agent-runtime/main_test.go`**: `run`을 직접 호출하는 4개 테스트가 `run`의 새 동작에
  맞춰 갱신 대상이 된다(확인됨 — line 61·91·111·135). 단발 Chat 가정(특히 tool_call 응답을
  그대로 stdout에 찍는 `TestRun_ToolCall_...`)은 loop 의미와 충돌하므로 재정의가 필요하다.
- **테스트용 stub 주입**: Phase 2의 결정적 테스트는 step마다 다른 응답을 줄 수 있는 stub이
  필요하다. 현 `StubClient`는 단일 응답 고정이라 그대로는 부족하다(확인됨, stub.go line 8–33).
  이를 어떻게 메울지는 §5 Decision Point다.
- **`internal/config`**: max step을 config로 노출할지 여부가 열려 있다(현재 max step 항목
  없음). §5 Decision Point에서 다룬다. 노출하기로 하면 `config.go`와 `main`의 주입 경로가
  영향 범위에 들어온다.
- 영향 **없음**: `internal/llm`의 interface·`ClaudeClient`·`internal/message` 타입 정의는
  Phase 2가 재사용만 하며 수정하지 않는다(SPEC §3).

## 5. Decision Points

### D1. AgentState의 종료 상태 표현

- 옵션 A: 명시적 상태 enum(running/final/max steps/error) 필드 하나로 종료 상태를 표현.
- 옵션 B: bool 플래그 조합(`done`, `errored`, `maxedOut`)으로 표현.
- 옵션 C: 에러 유무와 메시지 내용만으로 호출자가 추론(별도 상태 필드 없음).
- 트레이드오프: A는 종료 상태 집합이 코드에 한 점으로 드러나 §5.4의 "final이 아니라 max step
  임을 구분"을 자연스럽게 만족한다. B는 불가능한 조합(done+errored 동시 true)이 표현 가능해져
  불변식 부담이 생긴다. C는 max steps와 final을 구분하기 어렵다(SPEC §5.4 위반 위험).
- 채택: **A(명시적 상태 enum)**. 근거: §5.4·§5.5가 요구하는 "종료 종류 구분"을 단일 필드로
  보장하고, §2에서 enumerate한 종료 상태 집합과 1:1 대응한다.

### D2. 종료 판정 위치

- 옵션 A: loop 본문 안에서 응답을 누적한 직후 `HasToolCalls()`로 판정.
- 옵션 B: 별도 판정 함수/전략 객체로 분리해 주입.
- 트레이드오프: A는 Phase 2 규칙(tool_call 유무 = 최종 답 여부)이 단순해 충분하다. B는 판정
  규칙을 바꿀 여지를 주지만 Phase 2에는 과한 일반화다(범위 너머).
- 채택: **A**. 근거: `message.HasToolCalls()`가 이미 있어 그대로 술어로 쓰면 된다(확인됨,
  message.go line 34). Phase 2 범위에서 판정 규칙은 하나뿐이다(SPEC §5.3).

### D3. max step 강제 방식

- 옵션 A: 매 회전 시작에서 step counter ≥ max step이면 LLM 호출 전에 종료(선검사).
- 옵션 B: LLM 호출 후 누적하고 step을 올린 뒤 검사(후검사).
- 트레이드오프: A는 "max step에 도달하면 LLM을 더 호출하지 않고 종료"(SPEC §5.4)를 글자
  그대로 만족한다. B는 상한에서 한 번 더 호출이 일어날 수 있어 §5.4와 어긋난다.
- 채택: **A(선검사)**. 근거: §5.4의 "더 호출하지 않고", §3의 "어떤 응답 패턴에서도 무한히
  돌지 않는다"를 동시에 보장한다.

### D4. reflection hook 형태

- 옵션 A: 단일 함수 콜백(현재 step·state를 받는 호출 가능 값), 미주입 시 no-op 기본.
- 옵션 B: 메서드 하나짜리 인터페이스, 미주입 시 no-op 구현.
- 옵션 C: 훅 없음(매 step 로깅을 loop가 직접 수행).
- 트레이드오프: A는 가장 가볍고 테스트에서 step·state를 캡처하기 쉬워 §5.6 검증에 적합하다.
  B는 상태를 든 관찰자에 유리하나 Phase 2엔 과하다. C는 spec이 요구한 "확장 지점"을 없앤다
  (SPEC §1, §5.6 위반).
- 채택: **A(함수 콜백 + no-op 기본)**. 근거: 주입하지 않아도 loop가 돌고(기본 no-op), 테스트는
  콜백으로 step별 호출을 결정적으로 관찰할 수 있다(SPEC §5.6). 향후 인터페이스화는 Phase 5
  middleware에서 다룰 일이다(범위 분리).

### D5. error state 표현

- 옵션 A: state에 종료 상태 error + 원인 에러를 함께 보관하고, 실행 표면은 종료된 state를
  반환(에러를 두 번째 반환값으로 또 던지지 않음).
- 옵션 B: 실행 표면이 `(AgentState, error)`로 에러를 별도 반환.
- 트레이드오프: A는 "종료 상태"와 "원인"을 한 곳(state)에 모아 §5.5의 "사실과 원인을 호출자가
  확인"을 단일 표면으로 만족한다. B는 호출자가 에러와 state 두 군데를 봐야 해 종료 상태 모델과
  중복된다. 단, ctx 취소처럼 LLM 호출 단계에서 온 에러도 동일하게 error 상태로 흡수한다(SPEC
  §3, §5.5).
- 채택: **A(state에 흡수)**. 근거: §2의 종료 상태 집합과 일관되고, CLI는 종료 상태만 보고
  stderr/종료코드를 가르면 된다(SPEC §5.5, §5.7).

### D6. tool_call 응답을 Phase 2에서 다루는 방식

- 옵션 A: tool_call을 "최종 답 아님" 신호로만 해석하고 실행하지 않은 채 running 유지(다음
  step으로).
- 옵션 B: tool_call이 오면 즉시 종료(미지원 표시).
- 트레이드오프: A는 §5.8이 요구하는 max step 초과 실패 경로를 자연스럽게 만든다 — stub이 계속
  tool_call을 주면 loop가 소진돼 max steps로 끝난다. 또 Phase 3에서 tool 실행을 끼워 넣을
  자리를 그대로 남긴다. B는 max step 경로를 stub으로 재현하기 어렵고(즉시 끝나버림) loop의 반복
  성격을 가린다.
- 채택: **A(미실행, 신호로만)**. 근거: tool 실행은 Phase 3 범위(SPEC §4)이고, A라야 §5.8의
  "정상 종료/max step 초과" 두 경로를 stub만으로 결정적으로 입증할 수 있다.

### D7. 다단계 stub 주입 수단

- 옵션 A: Phase 2 테스트가 쓸 다단계(step별 순차 응답) stub을 `internal/agent`의 테스트
  코드 안에 둔다(`internal/llm`을 건드리지 않음).
- 옵션 B: `internal/llm/stub.go`의 `StubClient`를 순차 응답을 줄 수 있게 확장한다.
- 트레이드오프: 현 `StubClient`는 단일 응답 고정이라(확인됨) max step 경로를 직접 만들 수 없다.
  A는 Phase 1 산출물의 안정된 경계를 건드리지 않고 Phase 2 테스트 안에서 결정적 다단계 응답을
  구성한다. B는 Phase 1 공용 stub의 의미를 넓혀 기존 `llm`·`main` 테스트에까지 영향이 번질 수
  있다(범위 외 변경 위험, CLAUDE.md §범위).
- 채택: **A(agent 테스트 내 다단계 stub)**. 근거: §5.8의 결정적 검증을 만족하면서 Phase 1
  경계를 보존한다. 구체 형태(시퀀스 슬라이스를 순서대로 반환 등)는 implement.md 소관.

### D8. max step 설정 주입 경로

- 옵션 A: max step을 Agent 생성 인자로 받고, CLI는 상수/하드코딩 기본값을 넘긴다(config 미노출).
- 옵션 B: `internal/config`에 max step 항목을 추가해 환경변수로 노출하고 `main`이 주입한다.
- 트레이드오프: A는 Phase 2 범위에 정확히 들어맞고 config 표면을 늘리지 않는다. B는 운영 유연성을
  주지만 spec이 요구하지 않은 config 확장이다(SPEC 범위 너머; CLAUDE.md §범위).
- 채택: **A(생성 인자 + CLI 기본 상수)**. 근거: spec은 "max step이 강제된다"만 요구하지 설정
  노출을 요구하지 않는다(SPEC §3, §5.4). config 확장은 필요해질 때 별도로 다룬다.

### D9. CLI에서 max step 종료의 표현

- 옵션 A: max steps 종료를 실패로 보아 원인을 stderr에 쓰고 비정상 종료코드를 낸다(error와
  동급 처리).
- 옵션 B: 마지막 누적 메시지를 stdout에 내고 정상 종료코드를 낸다.
- 트레이드오프: §5.4는 "max step 초과가 final이 아님을 호출자가 구분"을 요구한다. A는 그 구분을
  종료코드·출력 스트림으로 사용자에게도 드러내고, ROADMAP 중단 기준의 "관찰 가능한 실패 케이스"와
  맞는다. B는 최종 답이 아닌데 정상처럼 보여 §5.4 취지를 흐린다.
- 채택: **A(실패로 표현)**. 근거: §5.4·SPEC §5.8·ROADMAP 중단 기준이 요구하는 "관찰 가능한
  실패"를 CLI 표면에서 그대로 보이게 한다. 단 error와 메시지 문구는 구분해 원인을 명확히 한다.
