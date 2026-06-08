# phase-3-tool-calling — implement

ROADMAP Phase 3(Tool Calling Runtime) 실행 체크리스트다. analysis.md의 설계 결정(D1~D10)을 전제로 하며,
각 Task는 SPEC §5.N 완료 조건에 매핑된다. Task는 파일 내 위치 순서가 곧 의존 순서다 — 앞 Task의 산출에
의존하는 Task는 반드시 뒤에 온다.

## Section: internal/tool — Tool·Registry·구체 tool

- [x] task-001: tool 실행 단위를 이름·schema 노출과 실행으로 표현하는 추상화를 정의한다.
  - 목적: 모든 tool이 자기 이름·입력 명세를 알리고 입력을 받아 실행되는 공통 형태를 갖춘다.
  - 접근: 새 패키지 `internal/tool`에 `Tool` 인터페이스를 정의한다 — `Spec() message.ToolSpec`,
    `Execute(ctx context.Context, input json.RawMessage) (message.ToolResult, error)`(D1 옵션 A). 성공 Content는
    구현체가 채우고, error 반환의 IsError 정규화와 ToolCallID 결합은 runtime(dispatcher) 책임으로 남긴다 —
    이 Task에서는 계약만 선언하고 정규화 로직은 task-003 소관. `internal/message` 타입을 그대로 입출력에 쓰며
    재정의하지 않는다.
  - 검증 조건 (결과 + 확인):
    - 결과: `internal/tool` 패키지에 `Tool` 인터페이스와 그 doc 주석이 존재한다.
    - 확인: `go build ./internal/tool/...` 통과. `go vet ./internal/tool/...` 통과.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3 / ANALYSIS §3, D1

- [x] task-002: tool을 이름으로 등록·조회하고 등록된 schema를 한데 모으는 registry를 구현한다.
  - 목적: tool을 이름으로 등록하면 충돌 여부를 호출자가 확인할 수 있고, 이름으로 조회하거나 전체 schema를 모을 수 있다.
  - 접근: `internal/tool`에 `Registry` 타입과 `Register(t Tool) error`(같은 이름 충돌 시 등록 거부 + error 반환으로
    호출자가 결과 확인, SPEC §5.1), 이름 lookup(존재/unknown 구분, SPEC §5.2), `Specs() []message.ToolSpec`(등록
    순서 보존 수집, SPEC §5.3)을 둔다. 이름 키는 각 tool의 `Spec().Name`에서 얻는다.
  - 검증 조건 (결과 + 확인):
    - 결과: 정상 등록·중복 등록 거부·존재 조회·unknown 조회·Specs 수집이 동작한다.
    - 확인: `internal/tool`에 단위 테스트 추가 — 중복 이름 등록이 error를 반환함, 미등록 이름 조회가 unknown으로
      구분됨, `Specs()`가 등록한 tool 수만큼 schema를 반환함. `go test ./internal/tool/...` 통과.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3 / ANALYSIS §1, §3, D2, D3

- [ ] task-003: tool_call 하나를 받아 검증·timeout·정규화·unknown을 모두 흡수해 ToolResult로 돌려주는 dispatcher를 구현한다.
  - 목적: 어떤 tool_call이든 unknown·검증 실패·실행 에러·timeout이 발생해도 throw 없이 에러 표시된 결과로 정규화되어 돌아온다.
  - 접근: `internal/tool`에 dispatch 표면을 둔다(registry 메서드 또는 별도 타입, D2 옵션 B) — 입력은 `message.ToolCall`
    하나(또는 묶음)와 loop ctx, 출력은 `message.ToolResult`. 처리 순서: (1) 이름 lookup 실패 → 본체 미실행,
    `IsError=true` "unknown tool" 결과(SPEC §5.7); (2) `context.WithTimeout`으로 loop ctx에서 per-tool ctx 파생
    (timeout 값은 dispatcher/agent 생성 인자로 받음, D5); (3) `Execute` 호출, 반환 error가 non-nil이면(입력 unmarshal
    실패 포함, D4·D6) `IsError=true` 결과로 정규화 — 검증 실패(SPEC §5.6)·실행 에러·timeout(SPEC §5.8)이 모두 같은
    경로로 흡수됨; (4) 성공 시 tool이 채운 Content를 그대로 두고 `IsError=false`. 모든 경로에서 ToolCallID를 대응
    `ToolCall.ID`로 채운다(D6). JSON Schema validator 의존을 새로 들이지 않는다(D4).
  - 검증 조건 (결과 + 확인):
    - 결과: 네 실패 경로(unknown·검증·실행 에러·timeout)가 모두 `IsError=true` ToolResult로 돌아오고 error를 throw하지 않는다.
    - 확인: `internal/tool`에 테스트 추가 — unknown 이름이 IsError 결과로 정규화됨, error 반환 tool이 IsError 결과로
      정규화됨, 짧은 timeout을 주입한 채 매달리는 fake tool이 IsError 결과로 흡수됨(ctx deadline 초과가 error 상태가
      아니라 IsError 결과가 됨), 성공 tool은 IsError=false에 ToolCallID가 대응 call.ID와 일치함. `go test ./internal/tool/...` 통과.
  - 참조: SPEC §5.6, SPEC §5.7, SPEC §5.8 / ANALYSIS §2, §3, D2, D4, D5, D6

- [ ] task-004: 산술 입력을 받아 계산 결과를 돌려주는 calculator tool을 구현한다.
  - 목적: 산술 요청에 대해 계산된 값을 결과로 반환한다.
  - 접근: `internal/tool`에 `Tool`을 구현하는 calculator를 둔다. `Spec()`은 이름·설명·InputSchema(입력 형태를 LLM에 알리는
    용도, SPEC §5.3)를 반환한다. `Execute`는 진입 직후 입력을 unmarshal하고 필드를 검증해(예: `{op, a, b}` 또는
    `{"expression"}` 최소형, D9) 잘못된 식·미지원 연산이면 즉시 error를 반환한다(본체 미실행, task-003이 IsError로 정규화,
    D4). 성공 시 결과 문자열을 Content로 채운 `message.ToolResult`를 반환한다. 복잡한 수식 파서까지 가지 않는다.
  - 검증 조건 (결과 + 확인):
    - 결과: 유효 산술 입력이 계산 결과를 반환하고, 미지원 연산·malformed 입력은 error를 반환한다.
    - 확인: 단위 테스트 — 대표 연산이 기대 결과를 냄, 0 나눗셈·미지원 연산·malformed JSON이 error를 반환함(본체 미실행).
      `go test ./internal/tool/...` 통과.
  - 참조: SPEC §5.9 / ANALYSIS §1, D1, D4, D9

- [ ] task-005: 허용된 base 경로 하위 파일만 읽는 file read tool을 구현한다.
  - 목적: 허용 범위 안의 파일 내용을 반환하고, 범위 밖 경로나 없는 파일은 거부한다.
  - 접근: `internal/tool`에 `Tool`을 구현하는 file read를 둔다. base 디렉터리를 생성 시 고정 인자로 받고(D9 옵션 A),
    `Execute`는 `{"path":"..."}`를 unmarshal한 뒤 입력 경로를 base 기준으로 정규화(`filepath.Clean`/`filepath.Abs`)하고
    base 밖(상위 `..` traversal, 절대경로 이탈)이면 error를 반환한다. 범위 통과 시 파일을 읽어 Content로 반환하고, 읽기
    실패(부재 파일 등)는 error를 반환한다 — 두 경우 모두 task-003이 IsError 결과로 흡수한다(SPEC §5.8 흡수 규칙과 일관).
  - 검증 조건 (결과 + 확인):
    - 결과: base 하위 파일은 내용 반환, base 밖 경로·존재하지 않는 파일은 error 반환.
    - 확인: 단위 테스트(`t.TempDir()`로 base 구성) — base 하위 파일 읽기 성공, `..` traversal·base 밖 절대경로 거부,
      부재 파일 error. `go test ./internal/tool/...` 통과.
  - 참조: SPEC §5.10 / ANALYSIS §1, §2, D1, D4, D9

## Section: internal/agent — loop의 tool 실행 통합

- [ ] task-006: Agent가 registry와 tool timeout을 생성 시 주입받도록 생성 표면을 넓힌다.
  - 목적: Agent가 tool registry와 실행 timeout을 들고 동작하도록 만든다.
  - 접근: `Agent` 구조체에 registry(또는 dispatcher) 필드와 tool timeout 필드를 추가하고, `NewAgent`에 두 인자를 더한다
    (D3 옵션 A, D5). `Run` 시그니처는 바꾸지 않는다(`(ctx, prompt)` 유지). 이 시그니처 변경의 ripple로 `agent_test.go`의
    `NewAgent` 호출 4곳(:85, :136, :176, :219)을 새 인자에 맞춰 갱신한다 — 기존 테스트는 nil registry(또는 빈 registry)와
    임의 timeout으로 호출해 의미를 보존한다.
  - 검증 조건 (결과 + 확인):
    - 결과: `NewAgent`가 registry·timeout을 받아 Agent에 보관하고, 기존 4개 agent 테스트가 변경된 호출로 컴파일·통과한다.
    - 확인: `go build ./...` 통과. `go test ./internal/agent/...` 통과(기존 TestRun_NormalExit / MaxSteps / ErrorExit /
      HookObservation 4 케이스 그대로 통과). LLM 호출 횟수 ≤ maxSteps 불변식(agent_test.go:155 핵심 단언)이 보존됨.
  - 참조: SPEC §5.4 / ANALYSIS §4, D3, D5, D8

- [ ] task-007: loop가 tool_call 응답을 실제로 실행하고 결과를 누적해 다음 회전으로 잇도록 단계 (4)·(6)을 확장한다.
  - 목적: assistant가 tool_call을 요청하면 그 tool을 실행해 결과를 대화에 누적하고, 모델이 결과를 본 뒤 이어서 판단한다.
  - 접근: `agent.go`의 LLM 호출 단계(:104)에서 `llm.ChatRequest`의 `Tools`를 `registry.Specs()`로 채운다(SPEC §5.3·§5.11).
    tool_call 분기(:118~123)에서 신호 취급을 멈추고, assistant 응답의 `BlockTypeToolCall` 블록을 등장 순서대로 하나씩
    task-003 dispatcher에 위임 실행한다(순차, 중간에 LLM 미호출, D7). 받은 모든 `message.ToolResult`를 `NewToolResultBlock`
    으로 감싸 `RoleTool` 메시지로 만들어 `state.Messages`에 누적 append한 뒤 다음 회전으로 넘어간다(SPEC §5.4). 다음 회전의
    LLM 호출이 user+assistant(tool_call)+tool(tool_result)를 재전송하므로 성공·실패 결과가 모두 모델 입력에 포함된다
    (SPEC §5.5). max step 선검사(:98)·LLM 에러 흡수(:108)·step 증가(:116)는 그대로 유지한다(D8). agent는 정규화 책임을 지지
    않고 받은 ToolResult를 메시지로 감싸 append만 한다.
  - 검증 조건 (결과 + 확인):
    - 결과: tool_call 응답에서 tool이 실행되고 tool_result 메시지가 누적되며 loop가 이어지고, 이후 text 응답이 오면 final로 종료된다.
    - 확인: `internal/agent` 테스트에서 `ChatRequest.Tools`가 등록 schema로 채워져 전달됨을 확인, tool_call 한 회전 뒤
      RoleTool 메시지가 누적됨을 확인. 회귀: LLM 호출 횟수 ≤ maxSteps 불변식 보존, ctx 취소가 여전히 StatusError로
      흡수됨(tool 실패는 IsError 결과로 흡수되어 error 상태로 가지 않음과 구분). `go test ./internal/agent/...` 통과. `go vet ./...` 통과.
  - 참조: SPEC §5.3, SPEC §5.4, SPEC §5.5 / ANALYSIS §1, §2, §3, D2, D6, D7, D8

## Section: CLI 통합

- [ ] task-008: CLI가 registry를 구성·tool 등록 후 Agent에 주입해 end-to-end tool calling 경로를 성립시킨다.
  - 목적: CLI로 실행되는 Agent가 등록된 tool과 schema를 갖춘 채 사용자 입력을 tool calling을 거쳐 최종 응답까지 처리한다.
  - 접근: `cmd/agent-runtime/main.go`의 `run`에서 `tool.Registry`를 만들고 calculator·file read tool을 등록한 뒤
    `NewAgent`에 주입한다(SPEC §5.11). tool timeout 기본값은 `defaultMaxSteps`(:19)와 같은 패턴으로 CLI 기본 상수
    (예: `defaultToolTimeout`)를 정의해 넘긴다(D5). file read의 base 경로 결정값도 여기서 정한다(D9). `run`/`main`의 외부
    시그니처와 종료 상태별 출력 분기는 유지한다.
  - 검증 조건 (결과 + 확인):
    - 결과: CLI 경로에서 registry가 Agent에 주입되고 schema가 채워진 채 동작하며, 기존 출력 분기가 보존된다.
    - 확인: `go build ./...` 통과. 기존 `cmd/agent-runtime/main_test.go` 4 케이스(text final / max steps / chat error /
      ctx cancel)가 그대로 통과(tool 미사용 입력에서도 의미 보존). `go test ./...` 통과.
  - 참조: SPEC §5.11 / ANALYSIS §1, §4, D3, D5, D9

## Section: 결정적 multi-step 검증

- [ ] task-009: stub client와 등록 tool만으로 multi-step tool calling 경로와 관찰 가능한 실패 케이스를 결정적으로 검증한다.
  - 목적: 실제 API 없이 tool 실행 → 결과 누적 → 최종 답 도달 경로와, unknown/검증 실패가 loop를 깨지 않음을 한 테스트군으로 관찰한다.
  - 접근: `internal/agent`(또는 신규 테스트 파일)에서 기존 `seqStub` 패턴(agent_test.go:16~42)을 그대로 재사용한다(D10,
    `internal/llm` 미변경). in-test registry에 실제 calculator/file read(또는 결정적 fake) tool을 등록하고, stub이 1회전에
    그 tool의 tool_call을, 2회전에 tool_result를 본 뒤 final text를 반환하도록 시퀀스를 짠다 — tool 실행 경로와 RoleTool
    결과 누적, 최종 StatusFinal 도달을 한 테스트로 관찰(SPEC §5.12). 별도 케이스로 unknown tool 이름 tool_call(SPEC §5.7)
    또는 검증 실패 입력(SPEC §5.6)을 주어, loop가 깨지지 않고 IsError tool_result가 누적된 채 다음 회전으로 이어짐을 관찰한다
    (ROADMAP 중단 기준의 관찰 가능한 실패 케이스). 이 Task는 tool + agent 두 구현을 가로지르므로 별도 Task로 둔다.
  - 검증 조건 (결과 + 확인):
    - 결과: 다단계 tool calling이 결정적으로 final에 도달하고, unknown/검증 실패 케이스에서 loop가 깨지지 않고 IsError 결과가 누적된다.
    - 확인: 신규 테스트 — (a) tool_call→tool_result→final 시퀀스가 StatusFinal로 끝나고 누적 메시지에 RoleTool 결과가 포함됨,
      stub 호출 횟수가 시퀀스 단계 수와 일치함; (b) unknown tool 또는 검증 실패 tool_call이 IsError tool_result로 누적되고
      loop가 계속됨(StatusError 아님). `go test ./...` 통과. 실제 네트워크 호출 없음.
  - 참조: SPEC §5.6, SPEC §5.7, SPEC §5.12 / ANALYSIS §2, D6, D7, D10
