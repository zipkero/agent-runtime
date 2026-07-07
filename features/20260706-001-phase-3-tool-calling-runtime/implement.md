# Phase 3 Tool Calling Runtime 구현

## 체크리스트

- [x] task-001: provider-neutral tool schema 요청 contract
  - 목적: Agent가 사용할 수 있는 Tool 목록을 provider와 무관한 schema로 LLM 요청에 포함할 수 있다.
  - 접근: `internal/message`에 `ToolSchema`를 추가하고 `llm.ChatRequest`에 `Tools []message.ToolSchema`를 추가한다.
    Claude와 Ollama provider는 이 schema를 각 provider의 tools wire format으로 변환하되, `LLMClient.Chat` 메서드
    contract는 유지한다.
  - 검증 조건:
    - 결과: `ChatRequest.Tools`에 담긴 name, description, input schema가 Claude와 Ollama 요청 body에서 provider별
      tools 필드로 보존되고, tool schema가 없는 기존 요청은 기존처럼 동작한다.
    - 확인: `internal/message`와 `internal/llm` 테스트에서 schema 타입 보존, Claude request 변환, Ollama request
      변환을 확인하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.3, SPEC §5.11, ANALYSIS §1, ANALYSIS §3, ANALYSIS §4, ANALYSIS §5.1

- [x] task-002: Tool contract와 registry
  - 목적: 호출자가 Runtime 내부 Tool을 등록하고 이름으로 조회하며, 중복 등록과 unknown tool을 명확히 구분할 수
    있다.
  - 접근: 새 `internal/tool` 패키지에 `Tool`, `Result`, `Registry`를 추가한다. Tool은 name, description, schema,
    validation, execution을 제공하고, registry는 nil/빈 이름/중복 이름을 거부하며 schema 목록을 안정적으로 노출한다.
  - 검증 조건:
    - 결과: Tool 등록, 이름 lookup, schema 목록 조회가 가능하고, 중복 이름과 등록되지 않은 이름은 정상 lookup과
      구분된다.
    - 확인: `internal/tool` 단위 테스트에서 정상 등록/조회, nil Tool, 빈 이름, 중복 이름, unknown lookup, schema
      목록을 확인하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.11, ANALYSIS §1, ANALYSIS §3, ANALYSIS §4, ANALYSIS §5.1,
    ANALYSIS §5.2

- [x] task-003: 정상 tool-use Agent loop
  - 목적: Agent가 assistant tool call을 실행하고 tool result를 메시지에 누적한 뒤, 같은 상태로 다음 LLM 판단을
    이어가 final answer로 종료할 수 있다.
  - 접근: `agent.Options`에 Tool registry와 Tool timeout을 추가하고, registry가 있으면 `needs_action` 종료 대신
    assistant tool call을 순서대로 실행해 `message.Tool`을 append한다. tool result를 포함한 메시지 상태와 schema
    목록을 다음 `LLMClient.Chat` 요청에 전달한다.
  - 검증 조건:
    - 결과: stub LLM이 첫 응답으로 tool call을 반환하면 Agent는 등록된 테스트 Tool을 실행하고, assistant message 뒤에
      tool result message를 추가한 뒤 두 번째 LLM 요청에서 final assistant 응답을 받아 `final` 상태로 종료한다.
    - 확인: `internal/agent` 테스트에서 LLM 호출 횟수, 두 번째 요청 메시지 목록, tool result 내용, final answer,
      기존 registry 없는 `needs_action` 경로 유지를 확인하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.4, SPEC §5.5, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4, ANALYSIS §5.3,
    ANALYSIS §5.5

- [ ] task-004: tool 오류 result, max step, trace
  - 목적: Tool 관련 실패가 Agent process 오류가 아니라 오류 tool result로 누적되고, 반복 제한과 trace에서 실행 흐름을
    확인할 수 있다.
  - 접근: Agent tool 실행 경로에서 unknown tool, validation failure, Tool 실행 error, Tool timeout을
    `ToolResult.IsError=true` 메시지로 정규화한다. tool result 후 다음 LLM 호출 전 `MaxSteps`를 검사하고, trace action과
    event 필드를 확장해 tool call ID, tool name, tool result/error/timeout을 기록한다.
  - 검증 조건:
    - 결과: 잘못된 arguments, unknown tool, Tool error, timeout은 각각 오류 tool result로 append되고 다음 LLM 판단에
      전달된다. `MaxSteps`에 도달하면 추가 LLM 또는 Tool 실행 없이 `max_steps` 상태로 종료하며, trace에서 tool call,
      tool result, 오류 또는 timeout, max step 종료를 확인할 수 있다.
    - 확인: `internal/agent` 테스트에서 각 실패 경로의 messages, status, trace, LLM/Tool 호출 횟수를 확인하고,
      `go test ./...`가 통과한다.
  - 참조: SPEC §5.6, SPEC §5.7, SPEC §5.8, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS §5.3, ANALYSIS §5.4, ANALYSIS §5.5, ANALYSIS §5.7

- [ ] task-005: calculator Tool
  - 목적: 기본 calculator Tool이 명확한 숫자 입력과 연산자로 계산 결과를 반환하고, 잘못된 입력은 오류 result로
    전달될 수 있다.
  - 접근: `internal/tool`에 calculator Tool을 추가한다. 입력은 `{"left": number, "operator": string, "right": number}`
    형태로 검증하고, 허용 연산자만 실행한다.
  - 검증 조건:
    - 결과: 유효한 사칙연산 입력은 계산 결과 content를 반환하고, 잘못된 JSON, 필수 입력 누락, 지원하지 않는
      연산자, 0 나누기는 validation 또는 execution error로 구분된다.
    - 확인: `internal/tool` calculator 테스트에서 정상 계산과 오류 입력을 확인하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.9, SPEC §5.11, ANALYSIS §1, ANALYSIS §3, ANALYSIS §4, ANALYSIS §5.2

- [ ] task-006: file read Tool
  - 목적: 기본 file read Tool이 허용된 root 안의 로컬 파일을 읽고, 허용되지 않은 접근이나 읽기 실패를 오류 result로
    전달할 수 있다.
  - 접근: `internal/tool`에 root directory를 주입받는 file read Tool을 추가한다. 입력은 `{"path": string}`으로
    검증하고, clean/abs 처리 후 root 내부 일반 파일만 읽는다.
  - 검증 조건:
    - 결과: root 내부 일반 파일은 content를 반환하고, 빈 path, root 밖 경로, 절대경로 우회, 디렉터리, 존재하지 않는
      파일은 validation 또는 execution error로 구분된다.
    - 확인: `internal/tool` file read 테스트에서 임시 디렉터리 기반 정상/오류 경로를 확인하고, `go test ./...`가
      통과한다.
  - 참조: SPEC §5.10, SPEC §5.11, ANALYSIS §1, ANALYSIS §3, ANALYSIS §4, ANALYSIS §5.6
