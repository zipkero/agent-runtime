# Phase 4.2 Agent Execution 구현

## 체크리스트

- [x] task-001: Single Agent Runner 실행 경계 추가
  - 목적: 호출자가 provider-neutral 의존성과 실행 제한을 주입한 Runner로 기존 Tool loop를 실행하고, 일반 text 최종
    응답과 Agent 종료 상태를 일관된 결과로 확인할 수 있다.
  - 접근: `RunnerOptions`, `RunnerResult`, `Runner`를 추가하고 Runner가 model 호출별 timeout을 적용하는 client를 조립해 기존
    `Agent`를 내부 loop 엔진으로 사용한다. 기존 `Agent`, `Options`, `Run` contract와 Tool 실행 흐름은 유지한다.
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
  - 접근: 이름과 선택적 pre/post hook을 가진 `ModelMiddleware`를 provider-neutral client decorator로 구현한다. 요청의 message,
    Tool schema, Tool call argument 등 중첩 참조값을 복제하고, middleware typed error와 전용 trace action을 기존 Agent 오류
    처리에 연결한다.
  - 검증 조건:
    - 결과: Tool loop의 모든 model 호출에서 pre/post hook이 등록 순서대로 실행되고 앞 hook의 변경값이 다음 hook,
      provider와 Agent 판단에 반영된다. pre 실패 시 provider가 호출되지 않고 post 실패 시 응답이 상태에 누적되지
      않으며, 실패 이후 model 또는 Tool 호출이 발생하지 않는다.
    - 확인: 테스트 middleware와 stub client로 hook 순서, 요청·응답 변경, 중첩값 alias 방지, middleware 이름·stage·원인
      오류, 오류 이후 호출 중단, middleware trace를 확인하고 `go test ./internal/agent`를 실행한다.
  - 참조: SPEC §5.3, SPEC §5.4, SPEC §5.5, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3

- [ ] task-003: JSON Schema structured output 검증 추가
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

- [ ] task-004: CLI를 Runner 기반 Agent 실행으로 전환
  - 목적: CLI 사용자가 현재 작업 디렉터리 범위의 Phase 3·4.1 Tool을 Agent loop에서 사용하고 final assistant 응답 또는
    명확한 실패 출력을 확인할 수 있다.
  - 접근: `cmd/agent-runtime`에서 calculator, file read, web search, file save, code execution Tool registry를 조립하고 Runner에
    주입한다. `LLM_TIMEOUT`은 model 호출별 timeout으로, CLI max step은 10으로 적용하며 기존 positional args와 stdin 입력
    contract를 유지한다.
  - 검증 조건:
    - 결과: Tool call을 반환하는 client에서 CLI가 Tool을 실행하고 Tool result를 다음 model 요청에 전달한 뒤 final text만
      stdout에 출력한다. Runner 생성 실패와 error, max steps, needs action 상태는 stdout을 비우고 stderr와 0이 아닌 종료
      코드로 구분되며, File·Code Tool root와 기존 Tool 제한은 현재 작업 디렉터리 기준으로 유지된다.
    - 확인: 순차 응답 stub client와 임시 작업 디렉터리를 사용한 CLI 테스트로 다섯 Tool schema, Tool loop, 최종 출력,
      호출별 deadline, 비-final 종료를 확인하고 `go test ./cmd/agent-runtime`와 `go test ./...`를 실행한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.8, SPEC §5.9, SPEC §5.10, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §4
