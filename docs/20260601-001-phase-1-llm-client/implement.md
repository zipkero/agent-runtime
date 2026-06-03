# implement: phase-1-llm-client

`analysis.md`의 설계와 7개 Decision을 실행 가능한 Task 체크리스트로 분해한다. Task는 의존
방향(아래에서 위로 쌓이는 계층)에 따라 파일 내 위치로 순서를 표현한다 — 위에 있을수록 먼저
수행한다. 각 Task는 spec §5 완료 조건을 최소 하나 참조한다.

전제: `go mod init`(Go module 초기화)은 사용자가 선행으로 완료했으며 implement 시작 시점에
`go.mod`가 이미 존재한다. 따라서 module 초기화는 Task가 아니다. Anthropic Go SDK를 `go get`
으로 추가하는 작업은 Claude 구현체 Task 안에서 수행한다.

## Section: 의존성

- [x] task-001: Anthropic Go SDK 의존성 추가
  - 목적: 프로젝트가 공식 Anthropic Go SDK를 사용할 수 있는 상태가 된다.
  - 접근: 기존 go.mod에 `go get github.com/anthropics/anthropic-sdk-go`로 SDK를 추가하고
    `go mod tidy`로 go.mod / go.sum을 정리한다. SDK import는 이후 Claude 구현체에만 둔다.
  - 검증 조건:
    - 결과: go.mod의 require 블록에 Anthropic Go SDK가 명시되고, go.sum에 체크섬이 기록된다.
    - 확인: `go build ./...`(빌드 통과) + go.mod diff로 SDK 라인 추가 확인.
  - 참조: SPEC §5.1, ANALYSIS §5(Decision 1)

## Section: message

- [x] task-002: provider-neutral 메시지 타입 정의
  - 목적: user / assistant / tool / system 메시지와 tool call·tool result를 하나의 모델로
    표현할 수 있고, 한 assistant 메시지가 텍스트와 tool call을 함께 담을 수 있다.
  - 접근: `internal/message`에 content-block 스타일 타입을 정의한다. Role(user/assistant/
    tool/system), Message{Role, Content []ContentBlock}, ContentBlock(text/tool_call/
    tool_result 중 하나), ToolCall(ID/Name/Input json.RawMessage), ToolResult(ToolCallID/
    Content/IsError), 그리고 tool 정의 schema(ToolSpec)를 둔다. 이 패키지는 다른 internal
    패키지를 import하지 않는다(최하위 leaf, 순환 방지).
  - 검증 조건:
    - 결과: assistant 메시지의 Content에 tool_call 블록이 있으면 "tool call 응답", text
      블록만 있으면 "텍스트 응답"으로 판별 가능한 형태가 존재한다. tool 메시지가 tool_result를
      담을 수 있다.
    - 확인: stub 없이도 검증 가능한 순수 타입 단위 테스트 — text-only 메시지와 tool_call을
      가진 메시지를 각각 구성해 블록 종류 판별(예: HasToolCalls 헬퍼 또는 Content 순회)이
      기대대로 동작함을 `go test ./internal/message/...`로 확인.
  - 참조: SPEC §5.4, SPEC §5.3, ANALYSIS §5(Decision 2)

## Section: config

- [x] task-003: 환경변수·`.env` 기반 config 로더 정의
  - 목적: api key·model·timeout이 환경변수 또는 프로젝트 루트 `.env`로 주입되며, 소스 변경
    없이 다른 key·model 값으로 실행을 바꿀 수 있다.
  - 접근: `internal/config`에 Config{AnthropicAPIKey, Model, Timeout time.Duration}와
    `Load() (Config, error)`를 둔다. `go get github.com/joho/godotenv`로 의존성을 추가하고,
    Load 진입 시 `godotenv.Load()`를 먼저 호출(파일 없으면 에러 무시)한 뒤 os.Getenv로 읽는다.
    `godotenv.Load`는 이미 설정된 실제 환경변수를 덮어쓰지 않아 실제 환경변수가 `.env`보다
    우선한다. api key·model은 필수라 누락 시 error를 반환한다(model 기본값 없음). timeout은
    미지정 시 기본값을 적용한다. `.env`는 `.gitignore`로 무시되며, 키 목록을 담은
    `.env.example` 템플릿을 함께 둔다.
  - 검증 조건:
    - 결과: 환경변수 설정에 따라 Config 필드가 채워지고, 필수 api key·model이 없으면 Load가
      error를 반환한다. 서로 다른 환경변수 값으로 Load하면 서로 다른 Config가 나온다. `.env`가
      있으면 그 값이 로딩되되 실제 환경변수가 우선한다.
    - 확인: t.Setenv로 환경변수를 세팅한 단위 테스트 — key·model 존재 시 성공·필드 값 일치,
      key 또는 model 누락 시 error, model 값 반영을 `go test ./internal/config/...`로 확인. `.env`
      자동 로딩과 실제 환경변수 우선은 임시 디렉터리에 `.env`를 쓰고 작업 디렉터리를 바꿔
      검증한다.
  - 참조: SPEC §5.5, ANALYSIS §5(Decision 6)

## Section: llm

- [x] task-004: LLMClient interface와 ChatRequest / ChatResponse 모델 정의
  - 목적: 호출자가 단일 Chat 호출 계약 하나에만 의존하고, 요청에 메시지 목록과 tool 정의를
    담아 전달하며, 응답으로 assistant 메시지를 받을 수 있다.
  - 접근: `internal/llm`에 `LLMClient` interface(단일 메서드 `Chat(ctx context.Context,
    req ChatRequest) (ChatResponse, error)`)와 ChatRequest{Model, Messages []message.
    Message, Tools []message.ToolSpec}, ChatResponse{Message message.Message}를 정의한다.
    context를 첫 인자로 두어 취소·timeout을 전파한다. 구체 구현체 타입을 계약에 노출하지 않는다.
  - 검증 조건:
    - 결과: 호출자는 LLMClient interface와 ChatRequest/ChatResponse 타입만으로 컴파일되는
      코드를 작성할 수 있고, 구현체 타입 import 없이 Chat을 호출하는 형태가 성립한다.
    - 확인: `go build ./internal/llm/...`(interface·타입 컴파일) + 이후 stub/Claude 두
      구현체가 같은 interface를 만족함을 컴파일 타임 assertion(예: `var _ LLMClient = ...`)
      으로 확인.
  - 참조: SPEC §5.2, SPEC §5.3, SPEC §5.4, ANALYSIS §5(Decision 3)

- [x] task-005: StubClient 구현 및 결정적 stub 기반 테스트
  - 목적: 실제 API 없이도 미리 정한 응답(텍스트 응답·tool call 응답 모두)을 돌려주는
    교체용 클라이언트로 테스트를 결정적으로 통과시킬 수 있다.
  - 접근: `internal/llm`에 `StubClient`를 두어 LLMClient를 만족시키고, 미리 구성한
    ChatResponse(또는 응답/에러 시퀀스)를 반환하도록 구성 가능하게 한다. 매핑 없이 internal
    타입을 직접 반환해 결정성을 보장한다. timeout 관찰 테스트를 위해 ctx 취소·deadline을
    감지해 에러를 반환하는 모드도 지원한다.
  - 검증 조건:
    - 결과: StubClient를 LLMClient 자리에 주입한 테스트가 실제 Anthropic API 호출 없이
      통과한다. 텍스트 응답 stub은 text 블록 응답을, tool call 응답 stub은 tool_call 블록
      응답을 반환해 호출자가 §5.3 구분을 결정적으로 검증할 수 있다.
    - 확인: stub 주입 단위 테스트 — (a) 텍스트 응답 stub→텍스트로 판별, (b) tool call 응답
      stub→tool call로 판별, (c) deadline 지난 ctx 전달→에러 반환을 `go test ./internal/
      llm/...`로 확인. 테스트는 네트워크 없이 통과해야 한다.
  - 참조: SPEC §5.7, SPEC §5.2, SPEC §5.3, ANALYSIS §5(Decision 4)

- [ ] task-006: Claude 구현체 (SDK 호출 + internal↔Claude wire 매핑 + 관찰 가능한 에러)
  - 목적: config로 주입된 api key·model로 실제 Claude를 호출해 응답을 받고, deadline 초과나
    잘못된 key 같은 실패가 에러로 표면화된다.
  - 접근: `internal/llm`에 Claude 구현체를 두어 LLMClient를 만족시킨다. config(api key·model·
    timeout)를 받아 생성 시 주입한다. internal message ↔ Claude content block/tool_use
    매핑(role, text/tool_call/tool_result)을 이 구현체 안에만 가둔다. Chat은 전달받은 ctx를
    SDK 호출에 그대로 흘려보내 취소·timeout을 전파하고, deadline 초과(context.DeadlineExceeded
    계열)와 인증 실패(401)를 error로 반환한다. Anthropic SDK import는 이 파일에만 존재한다.
  - 검증 조건:
    - 결과: 유효한 config로 생성한 클라이언트가 Claude 응답을 ChatResponse로 매핑해 반환하고,
      그 응답이 텍스트인지 tool call인지 호출자가 구분 가능하다. deadline이 지난 ctx 또는
      잘못된 key로 호출하면 Chat이 error를 반환한다.
    - 확인: `go build ./internal/llm/...`로 SDK 매핑 컴파일 확인 + 매핑 로직(internal↔wire
      변환)을 SDK 네트워크 없이 검증 가능한 부분은 단위 테스트로 분리해 확인. 실제 API
      경로(실 호출·401)는 수동 확인 또는 build-tag 격리 — 결정적 테스트(§5.7)는 stub으로
      커버하고 본 Task는 매핑·에러 표면화 형태를 보장한다.
  - 참조: SPEC §5.5, SPEC §5.6, SPEC §5.3, SPEC §5.4, ANALYSIS §5(Decision 1, 5, 7)

## Section: cli

- [ ] task-007: CLI 진입점 — 프롬프트 입력→LLM 호출→응답 출력, 실패는 stderr
  - 목적: 사용자가 CLI에 입력한 프롬프트로 LLM을 호출해 결과를 stdout으로 출력하고, timeout
    초과·잘못된 key 같은 실패는 stderr로 관찰 가능하게 출력한다.
  - 접근: `cmd/agent-runtime/main.go`에서 config.Load → Claude LLMClient 생성(config 주입)
    → 사용자 입력을 user message로 감싼 ChatRequest 구성 → config.Timeout으로
    context.WithTimeout한 ctx로 Chat 호출. main은 LLMClient interface에만 의존하고 구체
    타입을 노출하지 않는다. 응답 메시지의 Content를 보고 텍스트면 텍스트를, tool call이면
    요청된 tool call을 stdout에 구분 출력한다(실행은 Phase 3 범위라 표시까지). Chat이 error를
    반환하면 stderr에 출력하고 비정상 종료코드로 종료한다.
  - 검증 조건:
    - 결과: 프롬프트 입력 시 LLM 텍스트 응답이 stdout에 나오고, tool call 응답은 tool call로
      구분 표시된다. deadline 초과나 잘못된 key 시 에러 메시지가 stderr에 나오고 종료코드가
      0이 아니다.
    - 확인: `go build ./cmd/agent-runtime/...`(빌드 통과) + 실제 호출 경로는 수동 확인
      (유효 key로 정상 출력, 잘못된 key 또는 짧은 timeout으로 stderr 에러 관찰). 응답 구분·
      출력 분기 로직은 stub 주입이 가능하도록 추출했다면 `go test`로 결정적 확인.
  - 참조: SPEC §5.1, SPEC §5.6, SPEC §5.3, SPEC §5.2, ANALYSIS §1, ANALYSIS §2

## 빌드 게이트

모든 Task 완료 후 `go build ./...`와 `go test ./...`(stub 기반, 네트워크 불요)가 통과해야
한다. 실제 Claude 호출이 필요한 검증(§5.1 정상 출력, §5.6 관찰 가능 실패)은 유효/무효 key와
짧은 timeout으로 수동 확인한다.
