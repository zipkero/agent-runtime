# phase-5-1-tool-bundle 구현

## 체크리스트

- [x] task-001: Tavily Web Search Tool
  - 목적: Agent가 `web_search` tool call로 Tavily 검색을 실행하고, 검색 결과 또는 설정·API 실패를 tool
    result로 관찰할 수 있게 한다.
  - 접근: `internal/tool`에 `WebSearch`, Tavily HTTP client, 검색 입력 schema를 추가하고,
    `TAVILY_API_KEY`를 optional config 값으로 읽는다. SDK 의존성은 추가하지 않고 `net/http`와 주입 가능한
    client 인터페이스로 테스트한다.
  - 검증 조건:
    - 결과: API key가 있으면 query 기반 검색 결과가 content로 반환되고, API key 누락·context 취소·timeout·
      non-2xx/API 실패는 `IsError=true` tool result로 정규화될 수 있다.
    - 확인: `internal/tool` 단위 테스트에서 Tavily 성공 응답, 설정 누락, API 실패, context 취소/timeout,
      `ToolSpec` 포함을 단언하고 `go test ./internal/tool ./internal/config`가 통과한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.7, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS D1, ANALYSIS D2

- [x] task-002: File Save Tool
  - 목적: Agent가 `file_save` tool call로 허용된 작업 디렉터리 하위 파일을 저장하고, 위험한 경로 쓰기는
    tool result 에러로 관찰할 수 있게 한다.
  - 접근: `internal/tool`에 `FileSave`를 추가하고 기존 `FileRead`의 base 경로 정규화 패턴을 재사용한다.
    입력은 `path`, `content`, 선택 `overwrite`를 사용하며 기본은 기존 파일 덮어쓰기를 거부한다.
  - 검증 조건:
    - 결과: 상대 경로 파일 저장은 저장 경로와 byte 수를 content로 반환하고, 절대 경로·상위 경로 traversal·
      허용 범위 밖 경로·빈 경로·디렉터리 대상 쓰기·명시되지 않은 덮어쓰기는 에러로 정규화될 수 있다.
    - 확인: `internal/tool` 단위 테스트에서 저장 성공, 하위 디렉터리 생성, overwrite 허용/거부, 경로 거부
      케이스와 `ToolSpec` 포함을 단언하고 `go test ./internal/tool`이 통과한다.
  - 참조: SPEC §5.3, SPEC §5.4, SPEC §5.7, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS D3

- [ ] task-003: 제한된 Code Execution Tool
  - 목적: Agent가 `code_execute` tool call로 허용된 command profile을 제한된 환경에서 실행하고, 허용되지
    않은 실행 요청과 제한 위반을 tool result 에러로 관찰할 수 있게 한다.
  - 접근: `internal/tool`에 `CodeExecution`, `CommandProfile`, 출력 제한 로직을 추가한다. shell 문자열 실행은
    허용하지 않고, profile allowlist, 인자 검증, 임시 작업 디렉터리, `exec.CommandContext`, output cap을
    사용한다.
  - 검증 조건:
    - 결과: 허용된 profile은 stdout, stderr, exit status를 content로 반환하고, 미허용 profile/인자,
      timeout, output cap 초과, 제한 범위 밖 파일 접근 시도는 에러로 정규화될 수 있다.
    - 확인: `internal/tool` 단위 테스트에서 성공 실행, non-zero exit 관찰, 미허용 profile/인자 거부,
      timeout, output cap 초과, `ToolSpec` 포함을 단언하고 `go test ./internal/tool`이 통과한다.
  - 참조: SPEC §5.5, SPEC §5.6, SPEC §5.7, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS D4, ANALYSIS D5

- [ ] task-004: CLI registry와 Agent tool bundle 회귀
  - 목적: CLI에서 구성되는 Agent가 Phase 5.1 tool 묶음을 schema로 노출하고, stub LLM tool call을 통해 각
    tool을 실행한 뒤 기존 stdout/stderr/exit code 계약으로 최종 답에 도달하게 한다.
  - 접근: `cmd/agent-runtime.buildRegistry`가 calculator, file read와 함께 web search, file save,
    code execution tool을 등록하도록 갱신한다. `internal/agent` graph loop는 변경하지 않고 기존
    Registry/Dispatcher 경로를 재사용한다.
  - 검증 조건:
    - 결과: 등록된 schema에 Web Search, File Save, Code Execution Tool이 포함되고, stub LLM 응답의 tool call로
      각 tool result가 대화 state에 누적된 뒤 최종 답이 stdout과 종료코드 0으로 관찰된다. tool 실패 경로는
      stderr/비정상 종료가 아니라 RoleTool 에러 result로 loop에 남는다.
    - 확인: `cmd/agent-runtime`와 `internal/agent` 테스트에서 schema 포함, 각 tool calling 후 최종 답 도달,
      CLI stdout/stderr/exit code 회귀를 단언하고 `go test ./cmd/agent-runtime ./internal/agent` 및
      `go test ./...`가 통과한다.
  - 참조: SPEC §5.7, SPEC §5.8, SPEC §5.9, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4
