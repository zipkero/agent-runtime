# Phase 1 LLM Client 분석

## 근거

확인한 사실:

- `spec.md`는 Phase 1 범위를 `internal/message`, `internal/llm`, 기존 `internal/config`,
  `cmd/agent-runtime`의 단발 CLI 실행 경로로 제한한다.
- `SPEC §5.1`은 저장소 루트에서 CLI를 실행해 사용자 prompt를 전달하면 설정된 provider가 실제 호출되고 응답 text가
  표준 출력에 표시되어야 한다고 요구한다.
- `SPEC §5.2`와 `SPEC §5.3`은 Claude와 Ollama provider 선택, 필수 설정 검증, provider 오류, host와 model 사용을
  관찰 가능하게 요구한다.
- `SPEC §5.4`는 provider-neutral `LLMClient` contract와 메시지·응답 타입을 요구하고, `SPEC §5.6`은 외부 provider
  호출 없는 테스트 경계를 요구한다.
- `SPEC §5.5`는 요청 timeout 적용과 timeout 상황 확인을 요구한다.
- 현재 `internal/config`는 `LLM_PROVIDER`, `LLM_MODEL`, `LLM_HOST`, `LLM_API_KEY`, `LLM_TIMEOUT`, `LOG_LEVEL`을
  로딩하고, 실제 환경변수가 `.env`보다 우선하도록 구현되어 있다.
- 현재 `cmd/agent-runtime`은 config를 로딩하고 Phase 0 시작 로그를 출력한 뒤 종료한다.
- 현재 `go test ./...`는 통과한다.
- Claude Platform Docs의 Messages API는 message 생성 API를 제공하고, Anthropic-compatible schema는 `model`,
  `messages`, `max_tokens`, `x-api-key`, `anthropic-version` 같은 요청 요소와 `text`, `tool_use` content block,
  `stop_reason`을 사용한다.
- Ollama 공식 API 문서는 `POST /api/chat`이 `model`과 `messages`를 받고, 응답 `message`에 `content`와
  `tool_calls`를 포함할 수 있으며, `stream` 기본값이 `true`라고 설명한다.
- 사용자는 Phase 1 CLI가 추후 제대로 된 CLI loop Runtime으로 확장될 전제에 동의했고, Phase 1에서는 positional
  argument 우선, 인자가 없으면 stdin 전체를 읽는 단발 입력 방식을 승인했다.

추정:

- Phase 1은 streaming을 제외하므로 provider 요청은 non-streaming 응답으로 맞추는 편이 완료 조건 검증에 적합하다.
- Claude 호출에는 응답 token 상한이 필요하므로 Phase 1에서는 사용자 설정을 새로 늘리지 않고 provider adapter 내부
  기본 `max_tokens` 값을 둔다.

## 1. 구조

Phase 1 구조는 CLI 조립 계층, 설정 계층, provider-neutral LLM 계층, 메시지 타입 계층으로 나눈다. CLI는 사용자
prompt를 읽고 config를 로딩한 뒤 LLM client를 선택해 한 번 호출한다. `internal/config`는 provider 선택과 호출에
필요한 설정값을 제공하고, provider별 필수값 검증은 LLM client factory 또는 provider adapter 경계에서 수행한다
(SPEC §5.1, SPEC §5.2, SPEC §5.3).

`internal/message`는 Runtime 전체에서 공유할 메시지 표현을 소유한다. Phase 1에서 필요한 role은 `system`, `user`,
`assistant`, `tool`이며, text와 tool call, tool result를 표현할 수 있어야 한다. 이 패키지는 provider API 모양이 아니라
Runtime 내부 의미를 기준으로 타입을 정의한다. Claude의 content block과 Ollama의 `tool_calls`는 provider adapter가
이 내부 타입으로 변환한다. 이렇게 해야 이후 Agent loop와 Tool Calling Runtime이 provider별 JSON 구조에 묶이지 않고
같은 메시지 contract를 재사용할 수 있다(SPEC §5.4).

`internal/llm`은 `LLMClient` interface와 provider adapter를 소유한다. 권장 contract는
`Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)` 형태다. `ChatRequest`는 model과 messages를 담고,
`ChatResponse`는 assistant message, provider 이름, model, stop reason, usage metadata를 담는다. Phase 1에서 tool을
실행하지는 않지만, provider 응답의 tool call은 assistant message 안에 보존한다. 이는 tool 실행을 제외하면서도
응답 contract가 tool call 정보를 잃지 않아야 한다는 spec 범위를 만족한다(SPEC §5.4).

Provider adapter는 Claude와 Ollama를 각각 별도 파일 또는 하위 구조로 둔다. 두 adapter 모두 표준 `net/http` client와
`encoding/json`을 사용해 외부 HTTP API를 호출한다. 새 SDK 의존성을 추가하면 provider별 편의는 얻지만 Phase 1의
핵심인 request/response mapping과 timeout 검증 경계가 흐려질 수 있다. 표준 HTTP adapter는 local test server로
요청 body, header, timeout, 오류 변환을 안정적으로 검증할 수 있다(SPEC §5.2, SPEC §5.3, SPEC §5.5, SPEC §5.6).

CLI는 최종 대화형 Runtime이 아니라 Phase 1 검증용 얇은 실행 경로다. `cmd/agent-runtime`은 positional argument가
있으면 이를 공백으로 합쳐 prompt로 사용하고, 인자가 없으면 stdin 전체를 prompt로 읽는다. prompt가 비어 있으면
provider를 호출하지 않고 명확한 사용 오류로 종료한다. 이 방식은 Phase 1 단발 호출을 충족하면서 이후 CLI loop나
Agent runner 도입 시 입력 모드만 확장하면 된다(SPEC §5.1).

## 2. 데이터 흐름

정상 흐름은 저장소 루트에서 `go run ./cmd/agent-runtime "질문"` 또는 stdin 입력으로 시작한다. CLI는 prompt를
확정한 뒤 `config.Load()`를 호출한다. config 로딩은 Phase 0 규칙에 따라 `.env`와 실제 환경변수를 병합하고, 실제
환경변수를 우선한다. CLI는 provider 이름으로 LLM client를 만든 뒤 `context.WithTimeout`으로 `LLM_TIMEOUT`을 적용한
context를 만들고 `LLMClient.Chat`을 한 번 호출한다(SPEC §5.1, SPEC §5.5).

Claude 흐름은 provider adapter가 내부 `ChatRequest`를 Anthropic Messages request로 변환한다. 요청에는 model,
messages, adapter 기본 `max_tokens`를 포함하고, API key와 API version header를 붙인다. system message가 있으면
Claude request의 system 영역으로 보내고, user/assistant/tool 관련 content는 Claude content block으로 변환한다.
응답은 content block을 순회해 text는 assistant text로 합치고, tool use block은 내부 tool call로 보존한다.
Provider가 4xx/5xx 또는 error body를 반환하면 API key 값 없이 status와 provider 오류 메시지만 감싸서 반환한다
(SPEC §5.2, SPEC §5.4).

Ollama 흐름은 provider adapter가 내부 `ChatRequest`를 `POST {LLM_HOST}/api/chat` 요청으로 변환한다. Phase 1은
streaming 출력이 제외 범위이므로 `stream: false`를 명시한다. 요청에는 model과 messages를 포함하고, 응답
`message.content`를 assistant text로 옮기며 `message.tool_calls`는 내부 tool call로 보존한다. host 연결 실패,
잘못된 model, Ollama 오류 응답은 provider 오류로 변환한다(SPEC §5.3, SPEC §5.4).

Timeout 흐름은 CLI 또는 provider factory가 만든 context에서 시작한다. HTTP request는 그 context를 사용해야 하며,
context deadline 초과는 일반 provider 오류와 구분되는 timeout 오류로 감싼다. 테스트는 `httptest.Server` 지연 응답
또는 context timeout으로 이 경로를 확인한다(SPEC §5.5, SPEC §5.6).

실패 흐름은 provider 호출 전에 확인 가능한 설정 오류와 호출 후 provider 오류를 분리한다. 알 수 없는 provider,
빈 model, Claude API key 누락, 빈 prompt는 외부 호출 전에 사용 오류로 종료한다. Provider가 반환한 HTTP 오류,
응답 JSON decode 오류, 연결 오류는 provider 오류로 종료한다. 모든 오류 출력은 stderr로 보내고, API key와 전체
request body 같은 비밀 또는 불필요한 payload는 출력하지 않는다(SPEC §5.1, SPEC §5.2, SPEC §5.3).

## 3. 인터페이스

CLI 외부 인터페이스는 단발 prompt 입력이다. positional argument가 있으면 `os.Args[1:]`를 공백으로 합쳐 prompt로
사용하고, 인자가 없으면 stdin 전체를 읽는다. 성공 시 표준 출력에는 provider 응답 text를 출력한다. 로그나 오류는
stderr로 분리해 스크립트에서 응답 text를 재사용할 수 있게 한다(SPEC §5.1).

설정 인터페이스는 기존 `.env`와 실제 환경변수다. Phase 1에서 필요한 공개 설정은 `LLM_PROVIDER`, `LLM_MODEL`,
`LLM_HOST`, `LLM_API_KEY`, `LLM_TIMEOUT`이다. `LLM_PROVIDER`는 `claude`와 `ollama`를 지원하고, 기본값은 기존
`ollama`를 유지한다. Claude는 `LLM_API_KEY`와 `LLM_MODEL`이 필요하고, Ollama는 `LLM_MODEL`과 `LLM_HOST`가 필요하다
(SPEC §5.2, SPEC §5.3).

내부 LLM interface는 provider 구현이 아니라 Runtime 호출 의미를 표현한다. `LLMClient`는 context와 `ChatRequest`를
받고 `ChatResponse`를 반환한다. `ChatRequest`는 messages와 model을 필수로 다루며, provider별 header, endpoint,
response JSON은 interface 밖으로 노출하지 않는다. `ChatResponse`는 assistant message와 stop reason, token usage
metadata를 포함하되, Phase 1 완료 조건에 필요하지 않은 streaming event나 structured output 결과는 포함하지 않는다
(SPEC §5.4).

테스트 인터페이스는 실제 외부 provider 대신 stub client와 `httptest.Server`다. CLI 입력 처리와 client 호출 조립은
stub client로 확인하고, Claude/Ollama adapter는 local test server로 request body, header, 오류 응답, timeout을
검증한다. 이 경계는 실제 API key나 로컬 Ollama 설치 없이 `go test ./...`를 재현 가능하게 만든다(SPEC §5.6).

## 4. 영향 범위

새로 추가되는 주된 파일은 `internal/message`와 `internal/llm` 하위 Go 파일 및 테스트다. `internal/message`는 이후
Agent loop와 Tool Runtime이 재사용할 공통 타입을 만들기 때문에 공개 내부 contract 영향이 있다. `internal/llm`은
provider-neutral interface와 Claude/Ollama adapter, provider 선택 함수, 오류 타입을 포함한다(SPEC §5.4).

기존 `internal/config`는 Phase 1 호출에 필요한 provider별 필수 설정 검증을 지원하도록 확장될 수 있다. 다만 `.env`
파서나 우선순위 규칙은 Phase 0 완료 조건이므로 바꾸지 않는다. `LLM_MODEL`은 Phase 0에서 선택 항목처럼 로딩되지만,
Phase 1 실제 호출 경로에서는 provider 호출 전에 필수값으로 검증한다(SPEC §5.2, SPEC §5.3).

기존 `cmd/agent-runtime`은 Phase 0 시작 로그만 출력하는 실행 경로에서 단발 prompt 실행 경로로 바뀐다. 출력 contract는
사용자 관찰 지점이므로 응답 text는 stdout, 오류와 진단은 stderr로 분리한다. Phase 1에서 대화형 loop, command
subcommand, streaming UI를 추가하지 않는다(SPEC §5.1).

문서 영향은 `.env.example`, `README.md`, `ROADMAP.md`에 제한된다. 구현 결과가 환경변수 설명, 실행 방식, Phase 진행
상태와 불일치하면 해당 문서를 갱신해야 한다. 외부 provider와 실제 통신이 생기므로 README나 `.env.example`에는
비밀값을 커밋하지 않는 기준과 provider별 필수값을 사용자가 판단할 수 있게 남겨야 한다(SPEC §5.2, SPEC §5.3).

외부 contract 영향은 Claude Messages API와 Ollama Chat API다. Claude는 원격 API key 기반 호출이므로 테스트에서는
실제 호출을 기본 검증으로 삼지 않는다. Ollama는 로컬 host 기반 호출이므로 로컬 설치 여부에 따라 실행 검증
가능성이 달라진다. 둘 다 adapter 경계에서 HTTP request/response mapping을 테스트해 외부 환경 없이 기본
정확성을 확인한다
(SPEC §5.6).

## 5. Decision Points

1. Provider 호출 구현 방식
   - 옵션 A: 표준 라이브러리 `net/http`로 Claude와 Ollama adapter를 직접 구현한다.
   - 옵션 B: provider SDK를 추가한다.
   - trade-off: 옵션 A는 새 의존성 없이 request/response mapping과 timeout을 명확히 통제하고, local test server로
     검증하기 쉽다. 옵션 B는 provider 변화 대응과 편의 기능에는 유리하지만, Phase 1 범위보다 넓은 SDK surface를
     끌어오고 테스트가 SDK 동작에 더 의존한다.
   - 채택안: 옵션 A.
   - 근거: README는 외부 연결을 SDK 또는 HTTP로 처리한다고 허용하고, Phase 1 완료 조건은 mapping, provider 선택,
     timeout, 오류 노출을 직접 검증해야 한다(SPEC §5.2, SPEC §5.3, SPEC §5.5, SPEC §5.6).

2. CLI prompt 입력 방식
   - 옵션 A: positional argument를 우선 사용하고, 인자가 없으면 stdin 전체를 읽는다.
   - 옵션 B: positional argument만 지원한다.
   - trade-off: 옵션 A는 단발 prompt와 pipe 입력을 모두 지원하고, 이후 interactive loop 도입 시 입력 모드 확장이
     쉽다. 옵션 B는 구현은 더 단순하지만 긴 prompt나 파일 입력 사용성이 떨어진다.
   - 채택안: 옵션 A.
   - 근거: 사용자가 Phase 1 이후 제대로 된 CLI loop Runtime이 구축되는 전제에 동의했고, Phase 1 CLI는 얇은 단발
     호출 경로로 두기로 확인했다(SPEC §5.1).

3. 메시지 타입 소유 위치
   - 옵션 A: `internal/message`가 Runtime 내부 메시지 타입을 소유하고 `internal/llm`은 이를 요청·응답 contract로
     사용한다.
   - 옵션 B: `internal/llm` 안에 LLM 전용 메시지 타입을 둔다.
   - trade-off: 옵션 A는 이후 Agent loop와 Tool Runtime이 같은 메시지 타입을 재사용하기 좋고 provider adapter를
     내부 타입 변환 경계로 제한한다. 옵션 B는 Phase 1 구현은 작아지지만, Phase 2 이후 message 타입 이동이나 중복
     변환이 생길 가능성이 높다.
   - 채택안: 옵션 A.
   - 근거: README는 `internal/message`를 Runtime 전체 메시지 타입 계층으로 설명하고, spec은 이후 Agent loop와 Tool
     Calling Runtime이 재사용할 메시지·응답 contract를 요구한다(SPEC §5.4).

4. Claude `max_tokens` 처리
   - 옵션 A: Phase 1 adapter 내부 기본값을 사용하고 사용자 설정은 추가하지 않는다.
   - 옵션 B: `LLM_MAX_TOKENS` 같은 새 환경변수를 추가한다.
   - trade-off: 옵션 A는 spec에 없는 설정 surface를 늘리지 않으면서 Claude 호출에 필요한 request field를 채운다.
     옵션 B는 응답 길이를 사용자가 조정할 수 있지만, Phase 1 spec과 `.env.example`의 공개 설정 contract를 넓힌다.
   - 채택안: 옵션 A.
   - 근거: Phase 1 완료 조건은 provider 호출과 timeout, 오류, 교체 가능한 contract이며 응답 token 상한 설정은
     사용자 관찰 요구사항으로 확정되어 있지 않다(SPEC §5.1, SPEC §5.2, SPEC §5.4).

5. Tool call 처리 수준
   - 옵션 A: provider 응답의 tool call을 내부 response에 보존하지만 tool 정의 전달과 tool 실행은 하지 않는다.
   - 옵션 B: Phase 1에서 provider에 tool schema를 전달할 수 있는 request field까지 만든다.
   - trade-off: 옵션 A는 spec의 제외 범위를 지키면서 provider 응답 정보를 잃지 않는다. 옵션 B는 Phase 3에 가까운
     contract를 미리 고정해 이후 수정 비용을 줄일 수 있지만, Tool Runtime이 없는 상태에서 schema 의미를 먼저
     확정하게 된다.
   - 채택안: 옵션 A.
   - 근거: spec은 tool 실행과 Tool Runtime을 제외하면서도 provider 응답 안의 tool call 정보를 표현할 수 있는 LLM
     호출 계층을 요구한다(SPEC §5.4).
