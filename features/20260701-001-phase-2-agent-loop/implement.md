# Phase 2 Agent Loop 구현

## 체크리스트

- [x] task-001: Agent 실행 API와 final 응답 상태
  - 목적: 호출자가 사용자 입력 하나로 Agent run을 실행하고, 초기 user message와 assistant final 응답이 누적된
    최종 상태를 확인할 수 있다.
  - 접근: 새 `internal/agent` 패키지에 `Options`, `Agent`, `AgentState`, `Status`와 `New`, `Run`을 추가한다.
    `Run`은 `message.User(input)`을 상태에 저장하고, 기존 `llm.LLMClient`에 같은 메시지 목록을 전달한 뒤 tool call이
    없는 assistant 응답을 final 상태와 final answer로 보존한다.
  - 검증 조건:
    - 결과: stub `LLMClient`가 받은 요청에는 초기 user message가 들어 있고, 최종 `AgentState.Messages`에는 user와
      assistant message가 순서대로 누적되며, 상태는 final이고 final answer text를 읽을 수 있다.
    - 확인: `internal/agent` 테스트에서 정상 final run을 stub client로 검증하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.8, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4

- [x] task-002: tool call 대기 상태
  - 목적: assistant 응답에 tool call이 있으면 Agent run이 tool을 실행하지 않고 추가 행동 필요 상태로 멈추며, 호출자가
    tool call 정보를 상태와 메시지에서 확인할 수 있다.
  - 접근: `Run`의 assistant 응답 처리에서 `Message.ToolCalls`가 비어 있지 않으면 assistant message를 누적하고,
    같은 tool call 목록을 `AgentState.ToolCalls`에 보존한 뒤 `needs_action` 상태로 종료한다. Tool registry,
    unknown tool 처리, tool result message 생성은 추가하지 않는다.
  - 검증 조건:
    - 결과: tool call 응답 run은 `needs_action` 상태로 끝나고, `ToolCalls`와 마지막 assistant message에 같은
      tool call ID, name, arguments가 남으며, tool 실행이나 tool result message는 발생하지 않는다.
    - 확인: `internal/agent` 테스트에서 tool call 응답을 반환하는 stub client로 상태와 메시지 보존을 검증하고,
      `go test ./...`가 통과한다.
  - 참조: SPEC §5.4, SPEC §5.8, ANALYSIS §1, ANALYSIS §2, ANALYSIS §4, ANALYSIS §5

- [x] task-003: max step과 LLM 오류 종료 상태
  - 목적: Agent run이 설정된 max step을 넘겨 LLM을 호출하지 않고 멈추며, LLM 호출 오류는 상태의 error 경로와 원인
    오류로 확인할 수 있다.
  - 접근: 매 LLM 호출 직전에 `Step >= MaxSteps`를 검사해 초과 시 `max_steps` 상태로 종료하고 client를 호출하지 않는다.
    `LLMClient.Chat`이 error를 반환하면 assistant message를 누적하지 않고 `error` 상태와 `LastError`에 원인을 저장한다.
  - 검증 조건:
    - 결과: `MaxSteps`가 이미 허용량을 소진한 run은 LLM client 호출 없이 `max_steps` 상태가 되고, LLM 오류 run은
      `error` 상태와 `LastError`를 제공하며 assistant message를 추가하지 않는다.
    - 확인: `internal/agent` 테스트에서 호출 횟수를 기록하는 stub client와 오류를 반환하는 stub client로 두 종료
      경로를 검증하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.5, SPEC §5.6, SPEC §5.8, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §5

- [ ] task-004: 메모리 trace 기록
  - 목적: 호출자가 테스트에서 각 step의 주요 action 순서와 run 종료 이유를 `AgentState.Trace`로 확인할 수 있다.
  - 접근: `TraceEvent`에 step, action, status 또는 result, error 참조를 담고, user message 저장, LLM 요청, LLM 응답,
    final 종료, tool 대기 종료, max step 종료, LLM 오류 종료를 메모리 trace에 기록한다. 파일, JSON, stdout, stderr
    출력 형식은 정의하지 않는다.
  - 검증 조건:
    - 결과: final, tool 대기, max step, LLM 오류 run의 trace에서 step 순서, 주요 action, 종료 상태 또는 오류를 확인할
      수 있고 외부 trace 출력 파일이나 로그 contract는 새로 생기지 않는다.
    - 확인: `internal/agent` 테스트에서 stub `LLMClient`로 각 종료 경로의 trace 순서와 종료 이유를 검증하고,
      `go test ./...`가 통과한다.
  - 참조: SPEC §5.7, SPEC §5.8, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §5
