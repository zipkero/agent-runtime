# Phase 4.2 Agent Execution 구현

## 체크리스트

- [x] task-001: Single Agent Runner 실행 경계 추가
  - 목적: 호출자가 provider-neutral 의존성과 실행 제한을 주입한 Runner로 기존 Tool loop를 실행하고, 일반 text 최종
    응답과 Agent 종료 상태를 일관된 결과로 확인할 수 있다.
  - 접근: `RunnerOptions`, `RunnerResult`, `Runner`를 추가하고 Runner가 model 호출별 timeout을 Agent 실행 옵션으로 전달해 기존
    `Agent`를 내부 loop 엔진으로 사용한다. Agent는 각 `LLMClient.Chat`에 timeout context를 적용하고 기존 Tool 실행 흐름을 유지한다.
  - 검증 조건:
    - 결과: schema와 middleware가 없는 Runner가 Tool schema를 모든 model 요청에 전달하고, Tool result를 다음 요청에 누적해
      final text를 반환한다. 각 model 호출에는 독립된 timeout이 적용되고 max step, caller cancellation, Agent 오류 상태가
      보존된다.
    - 확인: stub LLM client와 등록 Tool을 사용한 Runner 단위 테스트로 일반 text, 반복 Tool 호출, 호출별 deadline, max step,
      context cancellation, LLM 오류를 확인하고 `go test ./internal/agent`를 실행한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.8, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3

- [x] task-002: Model middleware 순서·변경·실패 계약 구현
  - 목적: 호출자가 등록한 `pre-model`과 `post-model` middleware가 모든 model 호출을 순서대로 관찰·변경하며, 실패한
    middleware와 실행 중단 지점을 Runner 결과에서 구분할 수 있다.
  - 접근: 이름과 선택적 pre/post hook을 가진 `ModelMiddleware`를 Agent loop의 각 model 호출 전후에 명시적으로 적용한다.
    Agent는 상태 메시지를 복제하고 registry가 분리해 반환한 Tool schema를 인수해 model 요청을 만든다. Hook 반환값은
    소유권을 이전해 순서대로 전달하고, 작업값의 중첩 참조 변경은 최초 복사 경계 안에서 허용한다. 순회와 typed error
    생성은 provider-neutral helper로 분리한다.
  - 검증 조건:
    - 결과: Tool loop의 모든 model 호출에서 pre/post hook이 등록 순서대로 실행되고 앞 hook의 변경값이 다음 hook,
      provider와 Agent 판단에 반영된다. pre 실패 시 provider가 호출되지 않고 post 실패 시 응답이 상태에 누적되지
      않으며, 실패 이후 model 또는 Tool 호출이 발생하지 않는다.
    - 확인: 값 반환형 테스트 middleware와 stub client로 hook 순서, 요청·응답 변경, Agent 상태와 model 요청의 중첩값
      alias 방지, middleware 이름의 공백·중복 검증, 오류 stage·원인과 호출 전후 trace, 오류 이후 실행 중단을 확인하고
      `go test ./internal/agent`를 실행한다.
  - 참조: SPEC §5.3, SPEC §5.4, SPEC §5.5, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3

- [x] task-005: Tool timeout 실행 수명 안정화
  - 목적: Tool timeout이나 caller cancellation 뒤에 해당 Tool 실행이 background에 남지 않고, Agent가 Tool 반환 이후에만
    다음 상태로 전이한다.
  - 접근: Agent의 goroutine 기반 Tool 호출을 timeout context를 전달하는 동기 호출로 바꾸고 Runtime timeout을 cooperative
    cancellation contract로 명시한다. 내장 Tool은 context 취소를 관찰하고 반환하도록 맞춘다.
  - 검증 조건:
    - 결과: 정상·오류·timeout Tool은 `Execute`가 반환된 뒤에만 result와 trace가 기록되며, Agent 반환 후 Tool 상태나
      filesystem 부작용이 추가로 바뀌지 않는다.
    - 확인: 반환 시점을 제어하는 test Tool로 정상·timeout·caller cancellation과 후속 model 호출 순서를 검증하고,
      `go test -race ./internal/agent` 및 `go test ./...`를 실행한다.
  - 참조: SPEC §5.2, SPEC §5.11, SPEC §5.12, ANALYSIS §2, ANALYSIS §5.8

- [x] task-007: Tool 호출·result·전체 run 예산 적용
  - 목적: 한 model 응답의 다수 Tool call이나 큰 result가 실행 시간, 메모리, 다음 model context를 무제한으로 키우지
    않으며 호출자가 제한 초과를 일반 성공과 구분한다.
  - 접근: Agent와 Runner에 0일 때 각각 20회와 64KiB를 적용하는 `MaxToolCalls`, `MaxToolResultBytes`를 추가하고 음수는
    생성 오류로 거부한다. 전체 run 시간은 caller context가 소유하며 CLI는 10분 deadline을 사용한다. File Read와 Web
    Search는 읽기 단계부터 64KiB 상한을 적용한다.
  - 검증 조건:
    - 결과: 20회까지의 Tool 호출과 64KiB 이하 result는 기존 흐름을 유지한다. 호출 수나 전체 deadline 초과는
      `execution_limit` 오류 상태와 trace로 종료되고, 큰 result는 전체 payload를 다음 model 요청에 싣지 않는 크기 제한
      오류 result가 된다.
    - 확인: 한 응답의 21개 Tool call, 여러 step 누적 호출, 경계 크기의 File Read·Web Search·일반 Tool result, caller
      deadline, 기본값·음수 옵션을 단위 테스트하고 `go test -race ./...`를 실행한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.10, SPEC §5.11, SPEC §5.14, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3,
    ANALYSIS §5.10

- [x] task-008: Provider 완료 사유와 Agent 종료 상태 정규화
  - 목적: 길이 제한, 차단, 알 수 없는 provider 종료를 완성된 final 응답으로 숨기지 않고 호출자가 불완전 응답으로
    구분한다.
  - 접근: raw `StopReason`을 보존하면서 `complete`, `tool_call`, `length_limit`, `blocked`, `unknown`의 provider-neutral
    `FinishReason`을 `llm.ChatResponse`에 추가하고 Claude·Ollama adapter에서 정규화한다. 빈 값은 custom client 호환을
    위해 `complete`로 해석한다.
  - 검증 조건:
    - 결과: 정상 완료와 Tool 호출은 기존 final·Tool 흐름을 유지한다. 불완전 완료는 assistant 원문을 보존하되 Tool을
      실행하지 않고 `FinalAnswer`를 비운 `incomplete_response` 오류 상태와 trace를 반환한다.
    - 확인: 두 provider의 raw reason 매핑, 빈 값 호환, length limit·blocked·unknown 응답, 불완전 응답 속 Tool call을
      단위 테스트하고 `go test ./internal/llm ./internal/agent` 및 `go test ./...`를 실행한다.
  - 참조: SPEC §5.1, SPEC §5.8, SPEC §5.10, SPEC §5.11, SPEC §5.15, ANALYSIS §2, ANALYSIS §3,
    ANALYSIS §5.11

- [x] task-010: Timeout과 실행 제한 설정 검증 통일
  - 목적: 잘못된 비양수 설정이 CLI에서는 즉시 timeout, Runner에서는 무제한으로 다르게 동작하지 않고 실행 전에
    일관된 오류로 확인된다.
  - 접근: config의 `LLM_TIMEOUT`은 양수만 허용한다. Agent와 Runner 생성 시 음수 model·Tool timeout 및 Tool 호출·result
    제한을 거부하되, programmatic `ModelTimeout=0`은 미지정, `ToolTimeout=0`은 기존 30초 기본값으로 유지한다.
  - 검증 조건:
    - 결과: `.env`와 process 환경의 0·음수 `LLM_TIMEOUT`은 config 오류가 되고, 음수 Runner·Agent 옵션은 외부 호출 전에
      생성 오류가 된다. 양수와 문서화된 0 기본값은 기존 동작을 유지한다.
    - 확인: config와 Agent·Runner의 table test로 양수·0·음수 경계를 확인하고 `go test ./internal/config ./internal/agent`
      및 `go test ./...`를 실행한다.
  - 참조: SPEC §5.1, SPEC §5.9, SPEC §5.10, SPEC §5.11, SPEC §5.17, ANALYSIS §2, ANALYSIS §3,
    ANALYSIS §5.13

- [x] task-003: JSON Schema structured output 검증 추가
  - 목적: output schema를 지정한 호출자가 최종 assistant JSON을 검증된 원문으로 받거나, schema compile·JSON parse·validation
    실패를 일반 LLM 오류와 구분해 확인할 수 있다.
  - 접근: `github.com/santhosh-tekuri/jsonschema/v6@v6.0.2`를 내부 구현 의존성으로 추가하고 Runner 생성 시 self-contained schema를
    compile해 재사용한다. final text만 엄격하게 파싱·검증하고 외부 library 타입은 공개 Runner API에 노출하지 않는다.
  - 검증 조건:
    - 결과: 유효한 final JSON은 schema를 만족할 때 `StructuredOutput`에 원문 bytes로 보존된다. 잘못된 schema는 model
      호출 전에 Runner 생성 오류가 되고, JSON parse 또는 validation 실패는 assistant 원문 메시지를 남긴 채 Agent 상태를
      error로 바꾸고 `FinalAnswer`를 비우며 typed error와 전용 trace를 제공한다. schema 미지정 실행은 일반 text 동작을
      유지한다.
    - 확인: valid JSON, malformed JSON, schema 불일치, 빈·잘못된 schema, local `$ref`, 차단된 외부 `$ref`, schema 미지정 경로를
      단위 테스트하고 `go test ./internal/agent`와 `go test ./...`를 실행한다.
  - 참조: SPEC §5.6, SPEC §5.7, SPEC §5.8, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4

- [x] task-006: File Tool root 격리 강화
  - 목적: File Read와 File Save가 symbolic link나 경로 변경을 통해 주입된 root 밖을 읽거나 변경하지 않으며, 거부된
    저장이 root 밖에 디렉터리나 파일을 남기지 않는다.
  - 접근: lexical path 검사와 사후 symlink 순회를 보안 경계로 사용하지 않고, 각 실행에서 Go 1.26 표준 `os.OpenRoot`로
    root를 열어 `os.Root` 연산으로 읽기, parent 생성, 파일 쓰기를 수행한 뒤 닫는다.
  - 검증 조건:
    - 결과: root 내부 일반 파일의 기존 읽기·저장·overwrite 동작은 유지되고, root 밖을 가리키는 중간·최종 symlink와
      symlink 아래 새 parent 경로는 오류로 거부되며 root 밖에는 어떤 항목도 생성되지 않는다.
    - 확인: 임시 root와 외부 디렉터리를 사용해 File Read·File Save의 symlink escape, nested parent 생성, overwrite,
      일반 경로 회귀를 테스트하고 `go test ./internal/tool` 및 `go test ./...`를 실행한다.
  - 참조: SPEC §5.2, SPEC §5.9, SPEC §5.11, SPEC §5.13, ANALYSIS §2, ANALYSIS §5.9

- [ ] task-009: CLI Code Execution opt-in과 자식 환경 제한
  - 목적: CLI가 명시적 활성화 없이 host code 실행 capability를 노출하지 않고, 활성화된 Code Execution도 LLM·Tavily
    secret을 포함한 process 환경 전체를 자식 Go process에 전달하지 않는다.
  - 접근: config와 `.env.example`에 기본 false인 `ENABLE_CODE_EXECUTION`을 추가하고 true일 때만 Tool을 등록한다. 자식
    환경은 `PATH`, `TMPDIR`, `GOROOT`, `GOCACHE`, `GOMODCACHE`, `GOPATH`, `GOOS`, `GOARCH`, `CGO_ENABLED` allowlist와
    강제된 `GOWORK=off`로 구성한다.
  - 검증 조건:
    - 결과: 기본 CLI schema에는 Code Execution이 없고 opt-in 때만 추가된다. 허용된 Go 명령은 필요한 allowlist 환경에서
      동작하며 `LLM_API_KEY`, `TAVILY_API_KEY`와 임의 환경변수는 자식 process에서 관찰되지 않는다.
    - 확인: config 기본값·boolean parsing, CLI Tool 등록 목록, 자식 process 환경 allowlist와 secret 부재를 테스트하고
      `go test ./internal/config ./internal/tool ./cmd/agent-runtime` 및 `go test ./...`를 실행한다.
  - 참조: SPEC §5.9, SPEC §5.10, SPEC §5.11, SPEC §5.16, ANALYSIS §1, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS §5.12

- [ ] task-004: CLI를 Runner 기반 Agent 실행으로 전환
  - 목적: CLI 사용자가 현재 작업 디렉터리 범위의 Phase 3·4.1 Tool을 Agent loop에서 사용하고 final assistant 응답 또는
    명확한 실패 출력을 확인할 수 있다.
  - 접근: `cmd/agent-runtime`에서 calculator, file read, web search, file save를 기본 등록하고 Code Execution은
    `ENABLE_CODE_EXECUTION=true`일 때만 추가해 Runner에 주입한다. `LLM_TIMEOUT`은 model 호출별 timeout으로, CLI max
    step은 10, Tool call은 20회, result는 64KiB, 전체 run은 10분으로 적용하며 기존 positional args와 stdin 입력
    contract를 유지한다.
  - 검증 조건:
    - 결과: Tool call을 반환하는 client에서 CLI가 Tool을 실행하고 Tool result를 다음 model 요청에 전달한 뒤 final text만
      stdout에 출력한다. Runner 생성 실패와 error, max steps, needs action 상태는 stdout을 비우고 stderr와 0이 아닌 종료
      코드로 구분되며, File·Code Tool root와 기존 Tool 제한은 현재 작업 디렉터리 기준으로 유지된다.
    - 확인: 순차 응답 stub client와 임시 작업 디렉터리를 사용한 CLI 테스트로 기본 네 Tool과 opt-in Code Execution
      schema, Tool loop, 최종 출력, 호출별·전체 deadline, 비-final 종료를 확인하고 `go test ./cmd/agent-runtime`와
      `go test ./...`를 실행한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.8, SPEC §5.9, SPEC §5.10, SPEC §5.11, SPEC §5.14, SPEC §5.16,
    ANALYSIS §1, ANALYSIS §2, ANALYSIS §4
