# Phase 4.1 Tool Bundle 분석

## 근거

확인한 사실:

- `spec.md`는 Phase 4.1 범위를 Web Search Tool, File Save Tool, Code Execution Tool 추가로 정의한다.
- `SPEC §5.1`과 `SPEC §5.2`는 Tavily 기반 Web Search Tool의 정상 결과, source metadata, API key 누락, provider
  오류, 잘못된 입력, 외부 호출 없는 단위 테스트를 요구한다.
- `SPEC §5.3`과 `SPEC §5.4`는 허용된 root 안에 content를 저장하는 File Save Tool과 저장 실패/허용되지 않은 경로의
  오류 result를 요구한다.
- `SPEC §5.5`와 `SPEC §5.6`은 stdout, stderr, exit code를 포함한 Code Execution Tool 결과와 잘못된 입력, 허용되지
  않은 실행 요청, timeout, 실행 실패의 오류 result를 요구한다.
- `SPEC §5.7`, `SPEC §5.8`, `SPEC §5.9`는 세 Tool의 schema 노출, registry 등록, Agent loop 연동, 외부 provider 호출
  없는 테스트 가능성을 요구한다.
- 현재 `internal/tool`은 `Tool` interface, `Result`, `Registry`, 공통 `tool.Error`와 `ErrorKind`를 제공한다.
- 현재 `internal/agent`는 registry에 등록된 Tool schema를 LLM 요청에 싣고, assistant tool call을 실행해
  `message.ToolResult` 기반 tool message로 누적한다.
- 현재 `internal/config.Config`와 `.env.example`에는 `TAVILY_API_KEY`가 이미 있다.
- Tavily 공식 문서는 Search API 응답에 `query`, `answer`, `results`, `response_time`, `request_id`가 포함될 수 있고,
  각 result에는 `title`, `url`, `content`, `score` 등이 있음을 설명한다.
- Tavily 공식 best practices는 query를 400자 아래로 유지하고, `max_results`와 `include_raw_content` 같은 응답 크기
  제어 값을 명시적으로 다루라고 안내한다.
- 사용자는 Phase 4.1 Code Execution Tool을 프로젝트 root 안에서 `go` 명령 실행으로 제한하는 방향을 확인했다.

추정:

- Phase 4.1에서도 기존 CLI는 단발 LLM 호출 경로를 유지한다. spec이 Single Agent runner와 CLI 전환을 Phase 4.2
  제외 범위로 두기 때문이다.
- Code Execution Tool의 OS 수준 네트워크 차단이나 컨테이너 격리는 Phase 4.1에서 제공하지 않는다. spec이
  production-grade sandbox를 제외하기 때문이다.

## 1. 구조

Phase 4.1의 새 Tool들은 기존 `internal/tool` 패키지 안에 둔다. Phase 3가 `internal/tool`에 provider-neutral Tool
contract와 기본 Tool을 모았고, spec도 새 Tool들이 기존 Tool interface와 공통 오류 분류를 따르도록 요구한다
(SPEC §5.7). Web Search, File Save, Code Execution 모두 `Tool` interface를 구현하고 `Validate`와 `Execute`에서
각자의 입력 검증과 실행을 분리한다.

Web Search Tool은 HTTP 호출 책임을 Tool 내부의 작은 client 경계로 분리한다. Tool 자체는 schema, 입력 검증, 결과
정규화, 공통 오류 분류를 맡고, Tavily 요청/응답 변환은 `tavilySearchClient` 성격의 내부 interface 뒤에 둔다. 이렇게
하면 실제 API key가 없는 단위 테스트에서 fake client로 정상 결과와 provider 오류를 검증할 수 있다(SPEC §5.1,
SPEC §5.2, SPEC §5.9).

File Save Tool은 Phase 3의 `FileRead`와 같은 root 제한 모델을 따른다. 생성자는 root directory를 받고 절대 경로로
정규화한다. 실행 시 입력 path를 clean/abs 처리한 뒤 root 내부인지 확인하고, 일반 파일 저장만 허용한다. 저장 경계는
로컬 filesystem이며, 파일 삭제/이동/복사/탐색은 별도 Tool 책임으로 확장하지 않는다(SPEC §5.3, SPEC §5.4).

Code Execution Tool은 범위를 의도적으로 좁힌다. Phase 4.1에서는 임의 shell을 실행하지 않고, 주입된 root 내부
working directory에서 `go` executable만 직접 실행한다. 입력은 `go`의 하위 args만 받으며, 실행 파일 이름이나 shell
문자열을 사용자 입력으로 받지 않는다. 이 구조는 현재 Go 프로젝트 검증 capability를 제공하면서 spec의 제한된 실행
요구를 만족하고, production-grade sandbox 제외 범위를 넘지 않는다(SPEC §5.5, SPEC §5.6).

세 Tool 모두 오류를 Agent `StatusError`로 올리지 않는다. `Validate` 실패, 설정 누락, provider 오류, 파일 저장 실패,
실행 실패는 `tool.Error` 또는 context 오류를 반환하고, Agent의 기존 tool 실행 경로가 이를 오류 tool result로
정규화한다(SPEC §5.2, SPEC §5.4, SPEC §5.6, SPEC §5.8).

## 2. 데이터 흐름

Web Search Tool의 입력은 검색 query와 응답 크기 제어 옵션으로 들어온다. `Validate`는 query 공백, query 길이,
`max_results` 범위, 지원하는 검색 depth/topic 같은 값의 타입과 허용값을 확인한다. `Execute`는 API key와 HTTP client
설정을 확인한 뒤 context를 전달해 Tavily Search API를 호출한다. 성공 응답은 검색 결과 목록을 JSON 문자열 content로
정규화하고, 각 결과의 title, url, content, score를 source metadata로 보존한다(SPEC §5.1). API key 누락, HTTP 오류,
비정상 응답, JSON decode 실패는 execution/configuration 성격의 Tool 오류로 반환한다(SPEC §5.2).

File Save Tool의 입력은 `path`와 `content`다. `Validate`는 JSON 형식, path 공백, 절대경로, root 밖 escape를 확인한다.
`Execute`는 같은 경로 해석을 반복해 저장 대상이 root 내부인지 다시 확인하고, 필요하면 parent directory 생성 여부를
정책에 맞게 적용한 뒤 파일을 쓴다. 성공 시 저장된 상대 경로, byte 수, overwrite 여부를 JSON 문자열 content로
반환한다(SPEC §5.3). root 밖 경로, 디렉터리 대상, parent 생성 실패, write 실패는 Tool 오류 result로 전달된다
(SPEC §5.4).

Code Execution Tool의 입력은 `args`와 선택적인 `stdin`이다. `Validate`는 args 배열이 비어 있지 않은지, 각 arg가
공백이 아닌지, 허용되지 않은 작업 디렉터리나 executable 지정이 없는지 확인한다. `Execute`는 `exec.CommandContext`로
`go <args...>`를 실행하고, context cancellation과 Agent Tool timeout을 따른다. 성공과 실패 모두 stdout, stderr,
exit code를 result content로 반환할 수 있어야 한다. timeout이나 context 취소는 오류 result이고, exit code가 0이
아닌 실행 실패도 오류 result로 반환한다(SPEC §5.5, SPEC §5.6).

Agent 연동 흐름은 Phase 3를 그대로 사용한다. Registry에 세 Tool을 등록하면 Agent의 LLM 요청 schema 목록에 포함되고,
stub LLMClient가 반환한 tool call 이름에 따라 Agent가 해당 Tool을 실행한다. Tool result message가 누적된 뒤 다음 LLM
요청으로 전달되는지 확인하면 Phase 4.1 Tool이 기존 Agent loop와 호환되는지 검증할 수 있다(SPEC §5.7, SPEC §5.8).

## 3. 인터페이스

세 Tool은 기존 `tool.Tool` interface를 그대로 구현한다. 새 공통 Tool interface나 Agent 전용 분기 인터페이스는 만들지
않는다. `Schema()`는 `message.ToolSchema`를 반환하며, input schema는 provider wire format이 아니라 provider-neutral
JSON Schema 형태를 유지한다(SPEC §5.7).

Web Search Tool 생성자는 API key, endpoint, HTTP client 또는 client interface, 기본 max results를 받을 수 있어야
한다. API key가 비어 있어도 생성 자체를 실패시킬지, 실행 시 configuration error를 반환할지는 일관되어야 한다.
채택안은 생성은 허용하고 실행 시 configuration error를 반환하는 방식이다. 이렇게 해야 `.env.example`의 설명처럼
API key가 없어도 tool 호출만 오류 result로 관찰할 수 있다(SPEC §5.2).

File Save Tool 생성자는 `NewFileSave(root string)` 형태로 root를 받는다. root 자체가 없거나 디렉터리가 아니면
configuration error로 반환한다. 실행 입력은 `{"path": string, "content": string}`을 기본 contract로 둔다. 저장 결과
content는 JSON 문자열로 정규화해 LLM이 저장 경로와 byte 수를 읽을 수 있게 한다(SPEC §5.3, SPEC §5.4).

Code Execution Tool 생성자는 root, executable path 또는 executable name, 기본 output limit을 받을 수 있다. Phase
4.1의 공개 입력 contract는 `{"args": []string, "stdin": string}`이다. executable은 입력으로 받지 않는다. 결과
content는 `stdout`, `stderr`, `exit_code`, `timed_out`를 가진 JSON 문자열로 정규화한다(SPEC §5.5, SPEC §5.6).

Tavily 응답 content도 JSON 문자열로 반환한다. 구조는 최소 `query`, `answer`, `results`, `request_id`를 담고,
`results`의 각 항목은 `title`, `url`, `content`, `score`를 보존한다. `include_raw_content`는 기본 false로 두어 응답
크기를 제한하고, raw content가 필요한 정책은 후속 단계에서 확장한다(SPEC §5.1).

## 4. 영향 범위

주 변경 대상은 `internal/tool`이다. 이 패키지에 Web Search, File Save, Code Execution Tool과 테스트용 fake client
또는 helper가 추가된다. 기존 `Registry`, `Tool`, `Result`, `tool.Error`의 public contract는 유지한다.

`internal/agent`는 기존 Agent loop 호환 테스트가 추가될 수 있지만, Tool별 실행 분기를 추가하지 않는다. Agent는
registry와 `Tool` interface만 알고, Web Search/File Save/Code Execution의 내부 타입은 알지 않는다(SPEC §5.8).

`internal/config`는 이미 `TavilyAPIKey`를 제공하므로 Phase 4.1 필수 변경 대상이 아니다. 다만 실제 조립 계층에서
Web Search Tool을 만들 때 config 값을 주입할 수 있게 문서나 이후 Phase에서 사용될 수 있다.

`.env.example`은 이미 `TAVILY_API_KEY`를 설명한다. Phase 4.1 구현 중 설명이 실제 behavior와 어긋나면 갱신하지만,
analysis 기준으로는 새 환경변수 추가가 필수는 아니다.

외부 contract는 Tavily Search API다. 구현은 공식 Tavily endpoint와 응답 필드를 대상으로 하되, 단위 테스트는 fake
client 또는 stub HTTP server로 외부 네트워크 없이 수행한다(SPEC §5.2, SPEC §5.9).

## 5. Decision Points

1. Web Search Tool의 Tavily 경계
   - 옵션 A: Tool이 `net/http` client로 Tavily API 요청/응답을 직접 처리하되, 테스트에서는 HTTP client나 round tripper
     를 교체한다.
   - 옵션 B: 별도 Tavily client interface를 만들고 Tool은 그 interface에만 의존한다.
   - trade-off: 옵션 A는 파일 수가 적고 실제 wire format이 가까이 있지만 테스트 fixture가 HTTP 세부사항에 묶인다.
     옵션 B는 Tool 테스트가 provider wire format에서 분리되고 provider 오류를 쉽게 만들 수 있지만 작은 interface가
     하나 늘어난다.
   - 채택안: 옵션 B.
   - 근거: spec은 실제 Tavily 호출 없는 단위 테스트와 provider 오류 구분을 요구한다(SPEC §5.2, SPEC §5.9).

2. Web Search API key 누락 처리
   - 옵션 A: 생성자에서 API key 누락을 오류로 반환한다.
   - 옵션 B: 생성은 허용하고 `Execute`에서 configuration error를 반환한다.
   - trade-off: 옵션 A는 잘못된 설정을 조립 시점에 빨리 잡지만, API key 없는 환경에서 tool 호출이 오류 result로
     전달되는 behavior를 보기 어렵다. 옵션 B는 런타임 오류 result 검증이 쉽고 `.env.example` 설명과 맞지만 조립
     시점 실패는 늦어진다.
   - 채택안: 옵션 B.
   - 근거: spec은 API key 누락이 Tool 오류 result로 구분되어야 한다고 요구한다(SPEC §5.2).

3. File Save parent directory 정책
   - 옵션 A: parent directory가 없으면 생성한다.
   - 옵션 B: parent directory가 없으면 오류로 반환한다.
   - trade-off: 옵션 A는 Agent가 새 파일 구조를 만들 수 있어 File Save Tool의 실용성이 높다. 옵션 B는 동작이
     보수적이지만 파일 저장 capability가 기존 디렉터리에 묶인다.
   - 채택안: 옵션 A.
   - 근거: spec은 로컬 파일 생성 capability를 목표로 하며, 파일 삭제/이동/탐색은 제외하지만 parent 생성은 저장을
     완료하기 위한 좁은 보조 동작이다(SPEC §5.3).

4. File Save overwrite 정책
   - 옵션 A: 기본은 기존 파일 덮어쓰기를 거부하고, 입력 `overwrite: true`일 때만 덮어쓴다.
   - 옵션 B: 항상 덮어쓴다.
   - 옵션 C: 항상 덮어쓰기를 거부한다.
   - trade-off: 옵션 A는 의도치 않은 데이터 손실을 줄이면서 명시적 갱신도 허용한다. 옵션 B는 단순하지만 위험하고,
     옵션 C는 기존 파일 수정 capability가 없다.
   - 채택안: 옵션 A.
   - 근거: spec은 허용된 root 안 저장을 요구하지만 저장 실패와 허용되지 않은 대상을 구분해야 하므로, 명시적
     overwrite 입력이 관찰 가능성과 안전성의 균형이 좋다(SPEC §5.3, SPEC §5.4).

5. Code Execution 실행 범위
   - 옵션 A: `go` executable만 허용하고 args만 입력받는다.
   - 옵션 B: `python`, `go`, `node` 같은 여러 executable allowlist를 둔다.
   - 옵션 C: 임의 shell command를 실행한다.
   - trade-off: 옵션 A는 현재 Go 프로젝트 검증에 충분하고 공격면이 작다. 옵션 B는 범용성이 높지만 언어별 제한과
     테스트 matrix가 늘어난다. 옵션 C는 가장 유연하지만 Phase 4.1의 제한된 실행 요구와 충돌한다.
   - 채택안: 옵션 A.
   - 근거: 사용자가 root 내부 `go` 명령 제한을 확인했고, spec은 production-grade sandbox가 아닌 최소 제한 실행 Tool을
     요구한다(SPEC §5.5, SPEC §5.6).

6. Code Execution 실패 결과 표현
   - 옵션 A: exit code가 0이 아니면 오류 result로 반환하되 stdout/stderr/exit code를 content에 포함한다.
   - 옵션 B: process가 실행만 됐으면 exit code와 무관하게 정상 result로 반환한다.
   - trade-off: 옵션 A는 Agent가 실패를 오류 result로 인식하면서도 디버깅 출력을 볼 수 있다. 옵션 B는 프로세스 결과를
     중립적으로 보존하지만 spec의 실행 실패 오류 result 요구를 약화한다.
   - 채택안: 옵션 A.
   - 근거: spec은 실행 실패를 Tool 오류 result로 구분해야 한다고 요구한다(SPEC §5.6).
