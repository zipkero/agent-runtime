# Phase 4.1 Tool Bundle 구현

## 체크리스트

- [x] task-001: Tavily Web Search Tool
  - 목적: Web Search Tool이 유효한 검색 입력으로 Tavily 검색 결과를 Tool result content로 반환하고, 설정/입력/provider
    실패는 Tool 오류로 구분한다.
  - 접근: `internal/tool`에 Web Search Tool과 Tavily client 경계를 추가한다. Tool은 기존 `Tool` interface와
    `tool.Error` 분류를 따르고, API key 누락은 생성 실패가 아니라 실행 시 configuration error로 반환한다. Tavily
    응답은 `query`, `answer`, `results`, `request_id`를 가진 JSON 문자열 content로 정규화한다.
  - 검증 조건:
    - 결과: 유효한 query는 title, url, content, score를 포함한 source metadata를 반환한다. 빈 query, 400자를 넘는
      query, 잘못된 `max_results`, API key 누락, provider 오류, 비정상 응답은 Tool 오류로 구분된다.
    - 확인: `internal/tool` Web Search 테스트에서 fake Tavily client로 정상 결과와 실패 경로를 확인하고,
      `go test ./...`가 통과한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.7, SPEC §5.9, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS §5.1, ANALYSIS §5.2

- [ ] task-002: File Save Tool
  - 목적: File Save Tool이 허용된 root 안에 content를 저장하고, root 밖 접근이나 저장 실패를 Tool 오류로 반환한다.
  - 접근: `internal/tool`에 root directory를 주입받는 File Save Tool을 추가한다. Phase 3 `FileRead`와 같은 root 제한
    모델을 사용하고, 입력은 `{"path": string, "content": string, "overwrite": bool}`로 검증한다. parent directory는
    필요 시 생성하고, 기존 파일은 `overwrite: true`일 때만 덮어쓴다.
  - 검증 조건:
    - 결과: root 내부 새 파일은 저장되고 저장 경로, byte 수, overwrite 여부가 result content에 포함된다. 빈 path,
      절대경로, root 밖 경로, 디렉터리 대상, 기존 파일 overwrite 미허용, write 실패는 Tool 오류로 구분된다.
    - 확인: `internal/tool` File Save 테스트에서 임시 디렉터리 기반 정상 저장, parent directory 생성, overwrite 정책,
      실패 경로를 확인하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.3, SPEC §5.4, SPEC §5.7, SPEC §5.9, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS §5.3, ANALYSIS §5.4

- [ ] task-003: Go Code Execution Tool
  - 목적: Code Execution Tool이 root 내부에서 허용된 `go` 명령을 실행하고 stdout, stderr, exit code를 Tool result로
    반환한다.
  - 접근: `internal/tool`에 root directory와 output limit을 주입받는 Code Execution Tool을 추가한다. 입력은
    `{"args": []string, "stdin": string}`으로 검증하고, executable은 사용자 입력으로 받지 않는다.
    `exec.CommandContext`로 `go <args...>`를 실행해 context cancellation과 Agent Tool timeout을 따른다.
  - 검증 조건:
    - 결과: 허용된 `go` args는 stdout, stderr, exit code, timed_out을 포함한 JSON content를 반환한다. 빈 args,
      공백 arg, 허용되지 않은 실행 요청, exit code 0이 아닌 실행 실패, timeout 또는 context 취소는 Tool 오류로
      구분되며 stdout/stderr/exit code는 가능한 범위에서 content에 보존된다.
    - 확인: `internal/tool` Code Execution 테스트에서 임시 Go module 또는 현재 테스트 fixture 기반 정상 실행, 실패
      실행, validation 실패, timeout/cancel 경로를 확인하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.5, SPEC §5.6, SPEC §5.7, SPEC §5.9, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS §5.5, ANALYSIS §5.6

- [ ] task-004: Phase 4.1 Tool registry와 Agent loop 연동
  - 목적: 세 Tool이 기존 registry에 함께 등록되어 Agent LLM 요청 schema에 노출되고, stub LLM tool call을 통해
    실행 결과가 메시지에 누적된다.
  - 접근: `internal/tool` 또는 `internal/agent` 테스트에서 Web Search, File Save, Code Execution Tool을 registry에
    등록하고 schema 목록과 Agent run 메시지 누적을 확인한다. Agent 운영 코드는 Tool별 분기를 추가하지 않고 기존
    Phase 3 registry 실행 경로를 사용한다.
  - 검증 조건:
    - 결과: registry schema 목록에는 세 Tool의 provider-neutral schema가 포함된다. stub LLMClient가 각 Tool call을
      반환하면 Agent는 Tool result message를 append하고, 다음 LLM 요청에 누적 메시지를 전달한 뒤 final 상태로
      종료할 수 있다.
    - 확인: Agent 또는 Tool 통합 테스트에서 schema 노출, tool result message, 오류 result message, 다음 LLM 요청
      메시지 누적을 확인하고, `go test ./...`가 통과한다.
  - 참조: SPEC §5.7, SPEC §5.8, SPEC §5.9, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4
