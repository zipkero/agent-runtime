# Phase 1 LLM Client 구현

## 체크리스트

- [x] task-001: 메시지와 LLM 호출 contract 작성
  - 목적: Runtime 내부에서 Claude와 Ollama를 같은 방식으로 호출하고, 이후 Agent loop와 Tool Runtime이 재사용할
    메시지·응답 타입을 제공한다.
  - 접근: `internal/message`에 role, text, tool call, tool result 표현을 만들고, `internal/llm`에
    `LLMClient`, `ChatRequest`, `ChatResponse`, provider 선택과 오류 타입을 추가한다. 실제 provider HTTP 호출은
    이 Task에서 구현하지 않고, 외부 호출 없는 contract 단위 테스트로 고정한다.
  - 검증 조건:
    - 결과: 메시지 타입은 user, assistant, system, tool 메시지와 tool call/tool result 정보를 표현할 수 있고,
      `LLMClient` contract는 provider별 구현을 교체 가능한 형태로 노출한다.
    - 확인: `go test ./internal/message ./internal/llm`로 메시지 생성, response의 tool call 보존, provider 선택
      오류, 필수 설정 검증의 contract 수준 동작을 확인한다.
  - 참조: SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.6, ANALYSIS §1, ANALYSIS §3, ANALYSIS §5.3,
    ANALYSIS §5.5

- [x] task-002: Claude provider adapter 작성
  - 목적: `LLM_PROVIDER=claude` 설정에서 Claude Messages API를 실제 호출할 수 있고, 필수 설정 누락과 provider 오류를
    비밀값 노출 없이 확인할 수 있게 한다.
  - 접근: `internal/llm`에 표준 `net/http` 기반 Claude adapter를 추가하고, 내부 `ChatRequest`를 Messages API request로
    변환한다. API key와 API version header, model, messages, adapter 내부 기본 `max_tokens`, text/tool_use 응답 변환,
    HTTP 오류와 timeout 오류 변환을 구현한다.
  - 검증 조건:
    - 결과: Claude adapter는 model, message, header를 올바르게 전송하고, text와 tool_use 응답을 내부
      `ChatResponse`로 변환하며, API key 누락·HTTP 오류·timeout 오류에서 비밀값을 출력하지 않는다.
    - 확인: `go test ./internal/llm`에서 `httptest.Server`로 Claude request body/header, 성공 응답 변환, 필수 설정
      오류, provider 오류, timeout 오류를 확인한다.
  - 참조: SPEC §5.2, SPEC §5.4, SPEC §5.5, SPEC §5.6, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3,
    ANALYSIS §5.1, ANALYSIS §5.4, ANALYSIS §5.5

- [x] task-003: Ollama provider adapter 작성
  - 목적: `LLM_PROVIDER=ollama` 설정에서 설정된 host와 model로 Ollama Chat API를 실제 호출하고, 응답 text를
    provider-neutral response로 받을 수 있게 한다.
  - 접근: `internal/llm`에 표준 `net/http` 기반 Ollama adapter를 추가하고, 내부 `ChatRequest`를
    `POST {LLM_HOST}/api/chat`의 non-streaming request로 변환한다. `stream: false`, message content, tool_calls 보존,
    host/model 설정 검증, HTTP 오류와 timeout 오류 변환을 구현한다.
  - 검증 조건:
    - 결과: Ollama adapter는 설정된 host의 `/api/chat`에 model과 messages, `stream: false`를 전송하고,
      `message.content`와 `message.tool_calls`를 내부 `ChatResponse`로 변환한다.
    - 확인: `go test ./internal/llm`에서 `httptest.Server`로 Ollama request path/body, 성공 응답 변환, model/host
      검증, provider 오류, timeout 오류를 확인한다.
  - 참조: SPEC §5.3, SPEC §5.4, SPEC §5.5, SPEC §5.6, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3,
    ANALYSIS §5.1, ANALYSIS §5.5

- [x] task-004: CLI 단발 LLM 실행 경로 연결
  - 목적: 저장소 루트에서 CLI에 prompt를 전달하면 설정된 provider를 한 번 호출하고 응답 text를 stdout에 출력한다.
  - 접근: `cmd/agent-runtime`을 Phase 0 시작 로그 출력에서 단발 LLM 호출 경로로 바꾼다. positional argument를
    우선 prompt로 사용하고 인자가 없으면 stdin 전체를 읽으며, prompt 공백, provider 설정 오류, provider 오류는
    stderr와 non-zero exit로 처리한다. 호출에는 `LLM_TIMEOUT` 기반 context를 적용한다.
  - 검증 조건:
    - 결과: `go run ./cmd/agent-runtime "질문"` 형태와 stdin 입력 형태가 prompt를 만들고, 성공 시 provider 응답 text만
      stdout에 표시하며, 오류 출력에는 비밀값이 포함되지 않는다.
    - 확인: `go test ./cmd/agent-runtime ./...`로 CLI 입력 처리, stub client 기반 호출 조립, stdout/stderr 분리,
      timeout context 적용을 확인하고, `go run ./cmd/agent-runtime`의 빈 prompt 오류를 확인한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.5, SPEC §5.6, ANALYSIS §1, ANALYSIS §2,
    ANALYSIS §3, ANALYSIS §5.2

- [x] task-005: Phase 1 문서와 설정 예시 정합성 확인
  - 목적: Phase 1 이후 로컬 실행자가 provider별 필수 설정과 단발 CLI 실행 방식을 문서에서 확인할 수 있게 한다.
  - 접근: 구현 결과와 `.env.example`, `README.md`, `ROADMAP.md`를 비교해 provider별 필수값, 실행 예시, Phase 진행 상태가
    다르면 요청 범위 안에서 갱신한다. 비밀값 파일인 `.env` ignore 규칙은 유지한다.
  - 검증 조건:
    - 결과: `.env.example`은 Claude와 Ollama 실행에 필요한 환경변수와 우선순위를 설명하고, 최상위 문서는 Phase 1의
      단발 LLM 호출 방식과 진행 상태를 구현 결과와 일치하게 설명한다.
    - 확인: `git status --short`, `.env.example`, `.gitignore`, `README.md`, `ROADMAP.md`,
      `features/20260628-002-phase-1-llm-client/README.md`를 확인하고, `go test ./...`로 문서 갱신 후 코드 검증이
      유지되는지 확인한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.6, ANALYSIS §4
