# phase-5-1-tool-bundle 명세

## 범위

- Web Search Tool: Tavily 검색 API를 호출해 사용자 질문에 필요한 최신 웹 검색 결과를 tool result로 반환하는
  tool.
- Tavily 설정: Web Search Tool이 Tavily API key와 요청 timeout을 사용해 검색을 수행하고, 설정 누락이나
  API 실패를 호출자가 구분 가능한 tool result로 확인할 수 있게 하는 경로.
- File Save Tool: Agent가 생성한 텍스트 또는 JSON 등 문자열 content를 허용된 파일 경로에 저장하는 tool.
- File Save Tool 경로 제한: CLI 실행 작업 디렉터리 하위만 저장 대상으로 허용하고, 절대 경로·상위 경로
  traversal·허용 범위 밖 경로는 거부하는 규칙.
- Code Execution Tool: 명시적으로 허용된 언어 또는 명령만 제한된 로컬 실행 환경에서 실행하고 stdout,
  stderr, exit status를 tool result로 반환하는 tool.
- Code Execution Tool 제한: 실행 timeout, 임시 작업 디렉터리, 출력 크기 제한, 허용 언어·명령 allowlist,
  파일시스템 접근 범위를 강제하는 규칙.
- Tool schema: Web Search, File Save, Code Execution Tool이 LLM에 전달 가능한 `ToolSpec`을 제공하고,
  기존 registry와 dispatcher 경로에서 실행될 수 있게 하는 표면.
- Agent·CLI 연결: CLI에서 생성하는 Single Agent가 Phase 5.1 tool 묶음을 등록해, 실제 API 호출 없이도
  stub LLM과 local tool로 tool calling 경로를 결정적으로 검증할 수 있게 하는 경로.

## 목표

- Single Agent가 검색, 파일 저장, 제한된 코드 실행을 tool calling으로 요청하고 Runtime이 실행하도록 한다.
- Tavily로 web search provider를 고정해 Phase 5.1에서 provider 선택 범위를 넓히지 않는다.
- 파일 저장과 코드 실행처럼 외부 상태를 바꾸거나 위험도가 높은 tool은 허용 범위와 실패 결과를 명확히 한다.
- 기존 `internal/tool`의 Tool, Registry, Dispatcher, ToolResult 정규화 계약을 유지하며 새 tool을 추가한다.
- graph 기반 Agent 실행 흐름에서 새 tool들이 기존 calculator/file read tool과 같은 방식으로 호출되고
  결과가 다음 LLM 입력에 누적되게 한다.

## 제약

- Web Search Tool은 Tavily API만 대상으로 한다. 다른 search provider 추상화나 fallback provider는 만들지
  않는다.
- Tavily API key가 없거나 Tavily 호출이 실패하면 panic이나 loop 중단이 아니라 에러임이 표시된 tool result로
  반환되어야 한다.
- 모든 새 tool 실행은 `context.Context`를 받아 취소와 timeout을 전파해야 한다.
- File Save Tool은 CLI 실행 작업 디렉터리 하위에만 파일을 저장해야 한다. 허용 범위 밖 쓰기, 경로 traversal,
  디렉터리 덮어쓰기, 빈 경로는 에러 tool result로 거부해야 한다.
- Code Execution Tool은 명시적으로 허용된 언어 또는 명령만 실행해야 한다. 허용되지 않은 실행 요청은 실행 전
  거부되어야 한다.
- Code Execution Tool은 실행 timeout, 임시 작업 디렉터리, 출력 크기 제한을 강제해야 한다.
- Code Execution Tool은 제한된 파일시스템 범위에서만 실행되어야 하며, 네트워크 사용은 허용하지 않는 것을
  기본 정책으로 둔다.
- tool 실행 실패, 입력 검증 실패, timeout, 외부 API 실패는 기존 Dispatcher 계약처럼 `IsError=true` tool
  result로 정규화되어 Agent loop를 깨지 않아야 한다.
- 새 tool 추가만으로 `internal/message`, `internal/llm`, `internal/graph`의 공개 역할을 불필요하게 변경하지
  않는다.

## 제외 범위

- Middleware hook, pre/post model hook, structured output, streaming response (Phase 5.2).
- Single Agent runner의 별도 public API 정리 또는 graph 실행 구조 재설계 (Phase 5.2).
- Tavily 외 검색 provider, 검색 결과 ranking 고도화, crawling, page fetch, browser automation.
- 파일 읽기 tool의 정책 변경. 기존 file read tool은 Phase 3 산출물로 유지한다.
- 범용 sandbox, 컨테이너 기반 격리, OS 수준 seccomp/profile, 원격 코드 실행 환경.
- Code Execution Tool의 패키지 설치, 장기 실행 프로세스, interactive stdin, background job, network access.
- 병렬 tool 실행, retry policy, tool 결과 캐시, trace·metric 수집 구조의 정식화.
- RAG indexing·retrieval, Memory Runtime, Multi-Agent, MCP/A2A 외부 protocol adapter.

## 완료 조건

1. Web Search Tool이 Tavily API key가 있는 환경에서 검색 query를 받아 Tavily 검색 결과를 tool result로
   반환하고, 호출자가 결과 content를 확인할 수 있다.
2. Web Search Tool이 Tavily 설정 누락, context 취소, timeout, API 실패를 에러 tool result로 반환하고
   Agent loop를 중단하지 않는다.
3. File Save Tool이 허용된 작업 디렉터리 하위 경로에 content를 저장하고, 호출자가 저장된 파일 경로와
   저장된 내용을 확인할 수 있다.
4. File Save Tool이 절대 경로, 상위 경로 traversal, 허용 범위 밖 경로, 빈 경로, 디렉터리 대상 쓰기를 에러
   tool result로 거부한다.
5. Code Execution Tool이 허용된 언어 또는 명령 요청을 제한된 작업 디렉터리에서 timeout 안에 실행하고,
   stdout, stderr, exit status를 tool result로 반환한다.
6. Code Execution Tool이 허용되지 않은 언어·명령, timeout 초과, 출력 크기 초과, 제한 범위 밖 파일 접근
   요청을 에러 tool result로 반환한다.
7. Web Search, File Save, Code Execution Tool이 각각 `ToolSpec`을 제공하고, registry의 schema 수집 결과에
   포함되어 LLM chat 요청으로 전달될 수 있다.
8. Agent가 stub LLM 응답의 tool call을 통해 Web Search, File Save, Code Execution Tool을 각각 실행하고,
   tool result를 대화 state에 누적한 뒤 최종 답까지 도달할 수 있다.
9. CLI에서 구성되는 Agent가 Phase 5.1 tool 묶음을 등록하고, tool calling을 거쳐 최종 답에 도달하는 경로를
   stdout, stderr, exit code로 기존 CLI 계약에 맞게 관찰할 수 있다.
