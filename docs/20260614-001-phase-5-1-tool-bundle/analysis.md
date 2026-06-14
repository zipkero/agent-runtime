# phase-5-1-tool-bundle 분석

## 근거

읽은 기준 문서:

- `docs/20260614-001-phase-5-1-tool-bundle/spec.md` 전체. 범위는 Web Search Tool(Tavily), File Save Tool,
  Code Execution Tool과 CLI 등록 경로이며, Phase 5.2의 middleware, structured output, runner 정리는 제외다.
- `ROADMAP.md` Phase 5.1. Tool 묶음으로 Web Search Tool, File Save Tool, Code Execution Tool을 먼저
  구현하고, Phase 5.2가 그 위에서 Single Agent 실행 구조를 완성한다는 선행 관계를 확인했다.
- `docs/20260608-001-phase-3-tool-calling/spec.md`와 `docs/20260610-001-phase-4-go-graph-runtime/spec.md`.
  기존 Tool/Registry/Dispatcher 계약과 graph 기반 Agent loop 보존이 Phase 5.1의 기반임을 확인했다.
- Tavily 공식 문서. Search API는 `POST https://api.tavily.com/search`, `Authorization: Bearer <api key>`,
  JSON body의 필수 `query`, 선택 `search_depth`, `max_results` 등을 사용한다는 점을 확인했다.

코드베이스에서 확인한 사실:

- `internal/tool.Tool`은 `Spec() message.ToolSpec`과
  `Execute(ctx context.Context, input json.RawMessage) (message.ToolResult, error)` 계약을 가진다. tool이
  error를 반환하면 `Dispatcher`가 `IsError=true` result로 정규화한다.
- `internal/tool.Registry`는 tool 이름으로 등록·조회하고, 등록 순서대로 `ToolSpec`을 수집한다.
- `internal/tool.Dispatcher`는 unknown tool, 실행 error, timeout을 모두 tool result로 흡수한다. 단 timeout은
  `context.WithTimeout(ctx, timeout)`으로 적용하므로, `timeout == 0`이면 즉시 deadline이 걸릴 수 있다.
- `internal/tool.FileRead`는 base 디렉터리를 절대경로로 정규화하고, base 밖 경로와 prefix 위장 경로를 거부한다.
  File Save Tool은 이 경로 검증 패턴을 재사용할 수 있다.
- `internal/config.Config`는 현재 Anthropic API key, model, LLM timeout만 가진다. Tavily API key와 tool별
  제한 설정은 아직 없다.
- `cmd/agent-runtime.buildRegistry`는 현재 calculator와 file read tool만 등록한다. CLI 실행 작업 디렉터리를
  file read base로 사용한다.
- `internal/agent.Agent`는 graph 기반 loop로 tool call을 실행하고 RoleTool 메시지를 누적한다. 새 tool은
  registry에 등록되면 기존 흐름을 그대로 탄다.
- `go.mod`에는 Tavily SDK가 없다. 현재 외부 HTTP 호출은 Anthropic SDK만 사용한다.

추정으로 분리:

- Tavily 응답의 최종 JSON 필드 전체는 분석 단계에서 고정하지 않는다. 구현에서는 Search API의 `results` 배열에서
  title, url, content, score처럼 tool result에 필요한 필드만 구조화해 파싱하면 충분하다.
- Code Execution Tool의 OS 수준 sandbox는 현재 코드와 spec 제외 범위상 제공하지 않는다. Phase 5.1의 제한은
  shell 미사용, allowlist, 인자 검증, 임시 작업 디렉터리, timeout, 출력 크기 제한으로 달성한다.

## 1. 구조

Phase 5.1은 기존 `internal/tool` 계층을 확장한다. 새 tool 세 개는 모두 `tool.Tool` 구현체로 추가하고,
`internal/agent`, `internal/message`, `internal/graph`의 공개 계약은 바꾸지 않는다(SPEC §5.7, §5.8).

`internal/tool`의 책임:

- Web Search Tool은 Tavily Search API 호출을 감싼다. tool 입력 JSON을 검증하고, Tavily API key와 HTTP
  client를 사용해 검색한 뒤, 모델이 읽기 쉬운 문자열 content를 `ToolResult.Content`로 반환한다(SPEC §5.1).
- File Save Tool은 base 디렉터리 하위 파일 쓰기만 담당한다. 경로 정규화와 이탈 검사는 tool 내부에서 수행하고,
  실패는 error로 반환해 Dispatcher가 `IsError=true`로 정규화한다(SPEC §5.3, §5.4).
- Code Execution Tool은 allowlist 기반 로컬 명령 실행을 담당한다. shell을 거치지 않고 `exec.CommandContext`로
  허용된 명령 profile만 실행하며, stdout/stderr/exit status를 content로 직렬화한다(SPEC §5.5, §5.6).
- 새 tool들은 각자 JSON Schema를 `ToolSpec.InputSchema`로 제공한다. Registry는 기존 방식대로 schema를 수집한다
  (SPEC §5.7).

`internal/config`의 책임:

- Tavily API key를 환경변수에서 읽어 `Config`에 보관한다. Anthropic 설정처럼 CLI 기동을 막는 필수값으로
  만들지 않고, 비어 있으면 Web Search Tool 실행 시 에러 tool result가 나오게 한다(SPEC §5.2).
- Code Execution과 File Save의 제한값은 Phase 5.1에서 config 파일 형식으로 확장하지 않는다. CLI 기본 상수와
  tool 생성자 인자로 제한을 주입한다. 설정 표면을 넓히는 일은 필요할 때 별도 phase에서 다룬다.

`cmd/agent-runtime`의 책임:

- CLI 실행 작업 디렉터리를 File Read, File Save, Code Execution의 base 또는 작업 기준으로 넘긴다.
- `buildRegistry`에서 calculator, file read에 더해 web search, file save, code execution tool을 등록한다
  (SPEC §5.9).
- `run`의 stdout/stderr/exit code 분기 계약은 바꾸지 않는다. 새 tool의 실패는 기존 tool 실패처럼 RoleTool
  메시지에 누적되고, Agent loop 자체의 error가 아니다(SPEC §5.8, §5.9).

## 2. 데이터 흐름

Web Search 정상 흐름:

```text
assistant tool_call(web_search)
→ Dispatcher.Dispatch
→ WebSearch.Execute(ctx, input)
→ 입력 JSON 검증(query 필수, max_results 등 선택)
→ Tavily Search API POST /search
→ 응답 results를 간결한 text content로 변환
→ ToolResult.Content 반환
→ Dispatcher가 ToolCallID와 IsError=false 결합
→ AgentState.Messages에 RoleTool 메시지 누적
```

Tavily API key 누락, HTTP 실패, non-2xx 응답, 응답 파싱 실패, context 취소는 Web Search Tool이 error를
반환하고, Dispatcher가 `IsError=true` result로 만든다(SPEC §5.2). Tavily 실패는 Agent graph error로 올리지
않는다.

File Save 정상 흐름:

```text
assistant tool_call(file_save)
→ Dispatcher.Dispatch
→ FileSave.Execute(ctx, input)
→ path/content/overwrite 입력 검증
→ base 기준 target 경로 정규화
→ 절대경로·traversal·base 밖 경로·디렉터리 대상 거부
→ parent directory 생성
→ 파일 쓰기
→ 저장 경로와 byte 수를 ToolResult.Content로 반환
```

File Save는 path가 비어 있거나 base 밖으로 나가면 파일 시스템을 변경하기 전에 error를 반환한다(SPEC §5.4).
기본 정책은 기존 파일 덮어쓰기를 막고, 입력의 `overwrite`가 true일 때만 덮어쓰기를 허용하는 방식이 안전하다.

Code Execution 정상 흐름:

```text
assistant tool_call(code_execute)
→ Dispatcher.Dispatch
→ CodeExecution.Execute(ctx, input)
→ command/profile과 args 입력 검증
→ 허용 profile 조회
→ 임시 작업 디렉터리 준비
→ exec.CommandContext(ctx, command, args...) 실행
→ stdout/stderr를 제한 크기까지 수집
→ exit status와 출력 요약을 ToolResult.Content로 반환
```

허용되지 않은 command/profile, 검증 실패, timeout, 출력 크기 초과는 error로 반환해 `IsError=true` result가
된다(SPEC §5.6). 범용 sandbox가 제외되어 있으므로 파일시스템과 네트워크 제한은 shell 미사용, 임시 작업
디렉터리, command profile allowlist, 인자 검증으로 구현한다.

Agent·CLI 흐름:

```text
CLI main
→ config.Load
→ buildRegistry(workDir, cfg)
→ Agent.Run(graph 기반 loop)
→ llm_node가 tool schema 전달
→ tool_node가 Dispatcher로 새 tool 실행
→ RoleTool 메시지 누적
→ 최종 assistant text
→ run이 stdout/exit code 분기
```

이 흐름은 Phase 4의 `llm_node → tool_node → llm_node → end` 구조를 그대로 사용한다. 새 tool은 registry에
등록된 실행 단위일 뿐이므로 `internal/agent`의 graph adapter를 바꿀 필요가 없다(SPEC §5.8, §5.9).

## 3. 인터페이스

Web Search Tool 생성 표면:

```go
type TavilyClient interface {
    Search(ctx context.Context, req TavilySearchRequest) (TavilySearchResponse, error)
}

func NewWebSearch(apiKey string, client TavilyClient) *WebSearch
```

구현은 `net/http` 기반 `TavilyClient`를 제공하고, 테스트는 fake client를 주입한다. 새 SDK 의존성은 추가하지
않는다. `apiKey == ""`이면 `Execute`가 설정 누락 error를 반환한다(SPEC §5.1, §5.2).

Web Search 입력은 최소 `query`를 필수로 두고, `max_results`, `search_depth`, `topic` 정도만 Phase 5.1에서
허용한다. Tavily의 전체 옵션을 노출하지 않아 provider 세부가 tool schema를 과도하게 키우지 않게 한다.

File Save Tool 생성 표면:

```go
func NewFileSave(base string) (*FileSave, error)
```

입력 JSON은 `path`, `content`, 선택 `overwrite`를 가진다. `path`는 base 기준 상대경로만 허용한다. 절대경로는
base 하위라도 SPEC §5.4에 따라 거부한다. 반환 content는 저장된 상대경로, byte 수, overwrite 여부를 포함한다.

Code Execution Tool 생성 표면:

```go
type CommandProfile struct {
    Name    string
    Command string
    Args    []string
}

func NewCodeExecution(base string, profiles []CommandProfile, maxOutputBytes int) (*CodeExecution, error)
```

입력 JSON은 `profile`과 선택 `args`를 가진다. Phase 5.1의 기본 profile은 shell을 거치지 않는 명령만 둔다.
구현 Task에서 `go_test`처럼 프로젝트 검증에 필요한 profile부터 추가하고, profile별로 허용 인자를 정규식 또는
명시적 목록으로 검증한다. 임의 shell 문자열은 인터페이스에 넣지 않는다(SPEC §5.5, §5.6).

Config 표면:

```go
const EnvTavilyAPIKey = "TAVILY_API_KEY"

type Config struct {
    AnthropicAPIKey string
    Model           string
    Timeout         time.Duration
    TavilyAPIKey    string
}
```

`TAVILY_API_KEY`는 optional이다. 값이 없어도 `config.Load`는 성공하고, Web Search Tool 실행에서 에러 tool
result가 나오게 한다(SPEC §5.2).

## 4. 영향 범위

신규:

- `internal/tool/web_search.go`와 테스트. Tavily client, 입력 검증, 응답 포맷, 설정 누락·API 실패·timeout
  정규화 경로를 다룬다(SPEC §5.1, §5.2, §5.7).
- `internal/tool/file_save.go`와 테스트. base 경로 제한, 저장 성공, traversal/절대경로/디렉터리 대상 거부를
  다룬다(SPEC §5.3, §5.4, §5.7).
- `internal/tool/code_execution.go`와 테스트. command profile allowlist, timeout, output cap, exit status
  반환을 다룬다(SPEC §5.5, §5.6, §5.7).

변경:

- `internal/config/config.go`와 테스트. `TAVILY_API_KEY` optional 로딩을 추가한다(SPEC §5.2).
- `cmd/agent-runtime/main.go`와 테스트. `buildRegistry`가 Phase 5.1 tool 묶음을 등록하고, CLI 회귀 테스트가
  새 tool schema와 tool calling 경로를 확인한다(SPEC §5.8, §5.9).

재사용:

- `internal/message.ToolCall`, `ToolResult`, `ToolSpec`: 새 tool 입출력과 schema 표면으로 그대로 사용한다.
- `internal/tool.Registry`, `Dispatcher`: 새 tool 등록, lookup, timeout, error result 정규화를 그대로 맡긴다.
- `internal/agent.Agent`: graph 기반 tool execution loop를 그대로 사용한다.
- `internal/tool.FileRead`의 경로 정규화 패턴: File Save에서 같은 base 하위 검증 방식을 재사용한다.

변경하지 않는 범위:

- `internal/llm`의 provider 변환과 Claude client는 새 tool 추가만으로 변경하지 않는다.
- `internal/graph`의 Node, Router, Reducer, Result 표면은 변경하지 않는다.
- Phase 5.2 대상인 middleware, structured output, streaming, Single Agent runner public API는 다루지 않는다.

## 5. Decision Points

### D1. Tavily 연동을 SDK로 할지, `net/http` client로 할지

- 옵션 A: Tavily Go SDK 또는 별도 SDK 의존성을 추가한다.
- 옵션 B: 표준 `net/http`로 Tavily Search API만 호출하고, 내부 `TavilyClient` 인터페이스로 테스트를 분리한다.
- 트레이드오프: A는 SDK가 제공하는 타입과 편의 기능을 얻지만 Phase 5.1 범위에 새 의존성과 SDK 버전 관리를
  추가한다. B는 Search API에 필요한 최소 표면만 직접 구현해 의존성을 늘리지 않지만 요청/응답 타입을 직접
  관리해야 한다.
- 채택: **B**. Phase 5.1은 Tavily Search만 필요하고, 공식 문서의 HTTP contract가 단순하다. 새 SDK 없이도
  SPEC §5.1, §5.2를 검증할 수 있다.

### D2. Tavily API key 누락을 CLI 시작 실패로 볼지, tool 실행 실패로 볼지

- 옵션 A: `config.Load`에서 `TAVILY_API_KEY`를 필수로 요구해 CLI 시작을 실패시킨다.
- 옵션 B: `TAVILY_API_KEY`를 optional로 읽고, Web Search Tool 실행 시 설정 누락 error를 반환한다.
- 트레이드오프: A는 설정 문제를 빨리 드러내지만 web search를 쓰지 않는 CLI 사용까지 막는다. B는 calculator,
  file read, file save, code execution만 쓰는 경로를 유지하면서 SPEC §5.2의 "설정 누락을 tool result로
  확인"에 맞다.
- 채택: **B**. Tavily 설정 누락은 Agent loop를 깨지 않는 tool 실패로 관찰되어야 한다.

### D3. File Save의 덮어쓰기 정책

- 옵션 A: 기존 파일을 항상 덮어쓴다.
- 옵션 B: 기본은 덮어쓰기를 거부하고, 입력의 `overwrite=true`일 때만 덮어쓴다.
- 옵션 C: 기존 파일이 있으면 항상 실패한다.
- 트레이드오프: A는 사용성이 좋지만 의도치 않은 파일 손상 위험이 크다. C는 안전하지만 agent가 파일을
  수정하거나 재시도하기 어렵다. B는 기본 안전성과 명시적 수정 가능성을 함께 제공한다.
- 채택: **B**. File Save는 외부 상태를 바꾸므로 기본 거부가 안전하고, 명시적 overwrite는 실사용성을 보존한다.

### D4. Code Execution을 source snippet 실행으로 볼지, command profile 실행으로 볼지

- 옵션 A: 입력 source code를 임시 파일에 쓰고 언어별 interpreter/compiler로 실행한다.
- 옵션 B: 미리 정의한 command profile만 실행하고, 인자는 profile별 검증 규칙을 통과한 값만 허용한다.
- 옵션 C: shell command 문자열을 받아 allowlist로 필터링한다.
- 트레이드오프: A는 "코드 실행"이라는 이름에 직관적이지만 OS sandbox 없이 네트워크와 파일 접근을 강하게
  막기 어렵다. C는 구현이 쉽지만 shell injection과 인자 검증 위험이 크다. B는 임의 코드 실행보다 기능은
  좁지만 Phase 5.1의 제한 요구와 범용 sandbox 제외 범위를 가장 잘 맞춘다.
- 채택: **B**. Phase 5.1의 Code Execution Tool은 allowlist command profile runner로 정의한다. 임의 shell
  또는 arbitrary source execution은 다루지 않는다.

### D5. Code Execution의 파일시스템·네트워크 제한 수준

- 옵션 A: OS 수준 sandbox, container, seccomp 등으로 강제한다.
- 옵션 B: shell 미사용, 임시 작업 디렉터리, command profile allowlist, 인자 검증, timeout, output cap으로
  제한한다.
- 트레이드오프: A는 강한 격리를 제공하지만 spec 제외 범위의 범용 sandbox에 해당하고 구현 비용이 크다. B는
  OS 수준의 완전 격리는 아니지만 현재 프로젝트 의존성 없이 테스트 가능한 제한을 제공한다.
- 채택: **B**. 분석과 구현에서는 이를 "Phase 5.1 제한 실행"으로 명확히 다루고, 강한 sandbox는 제외 범위로
  유지한다.
