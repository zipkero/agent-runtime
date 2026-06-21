# phase-5-2-agent-runtime 구현

## 체크리스트

- [x] task-001: Single Agent Runner 기본 실행 표면
  - 목적: 호출자가 CLI 밖에서도 provider-neutral LLM client, model, registry, max step, timeout을 지정해
    Single Agent를 실행하고 최종 상태, 최종 assistant 메시지, 최종 text를 확인할 수 있게 한다.
  - 접근: `internal/agent`에 `RunnerConfig`, `Runner`, `RunnerResult`, `RunnerStatus`를 추가하고,
    `NewRunner(cfg)`와 `Run(ctx, prompt)`가 기존 `Agent` graph loop를 감싸도록 구현한다. 기존
    `NewAgent` 호출 경로는 유지하고, output contract가 없으면 text-only 결과만 채운다.
  - 검증 조건:
    - 결과: Runner가 text-only 최종 응답에서 success 결과, `AgentState`, `FinalMessage`, `FinalText`를
      반환한다. tool call 경로에서는 기존처럼 RoleTool 메시지가 누적되고 최종 답까지 도달한다.
    - 확인: `internal/agent` 테스트에 runner text-only 성공, tool call 실행·누적, max step/error 상태 매핑,
      output contract 미지정 경로를 추가하고 `go test ./internal/agent/...` 통과.
  - 참조: SPEC §5.1, §5.2, §5.10 / ANALYSIS §1, §2, §3, §5 D1, D7

- [x] task-002: CLI 실행 경로를 Runner 기반으로 연결
  - 목적: 기존 CLI 사용자가 같은 stdin/stdout/stderr/exit code 계약을 유지하면서 Runner 기반 Single Agent 실행을
    사용하게 한다.
  - 접근: `cmd/agent-runtime.run`이 직접 `agent.NewAgent`를 만들지 않고 `agent.NewRunner`로 실행하도록 바꾼다.
    CLI 기본 경로는 middleware와 output contract를 비워 기존 text 최종 답 출력 방식을 유지한다.
  - 검증 조건:
    - 결과: CLI `run`이 final text를 stdout과 exit code 0으로 반환하고, max step·chat error·context 취소는
      기존처럼 stderr와 non-zero exit code로 반환한다.
    - 확인: 기존 `cmd/agent-runtime` 테스트가 Runner 기반 경로에서도 통과하도록 갱신하고 `go test
      ./cmd/agent-runtime/...` 및 `go test ./...` 통과.
  - 참조: SPEC §5.3, §5.10 / ANALYSIS §1, §2, §3, §4

- [x] task-003: pre/post model middleware 성공 경로
  - 목적: middleware가 LLM 호출 전 요청과 LLM 호출 후 응답을 등록 순서대로 관찰·변경하고, 변경 결과가 실제
    LLM 호출과 Agent state에 반영되게 한다.
  - 접근: `internal/agent`에 `Middleware`, `PreModelInput`, `PostModelInput` 계약과 Agent 옵션 경로를 추가한다.
    `llm_node`에서 pre hook을 `LLMClient.Chat` 전에 실행하고, post hook을 응답 수신 후 state 누적 전에 실행한다.
  - 검증 조건:
    - 결과: pre middleware가 변경한 `ChatRequest`가 실제 LLM client에 전달된다. post middleware가 변경한
      `ChatResponse`가 Agent state와 Runner 최종 결과에 반영된다. 여러 middleware는 등록 순서대로 실행되고 앞
      hook의 변경 결과가 뒤 hook에 전달된다.
    - 확인: `internal/agent` 테스트에 request 변경 캡처, response 변경 반영, pre/post 등록 순서와 변경 전파
      케이스를 추가하고 `go test ./internal/agent/...` 통과.
  - 참조: SPEC §5.4, §5.5, §5.6 / ANALYSIS §1, §2, §3, §5 D2, D3

- [x] task-004: middleware error 구분과 전파
  - 목적: middleware 실패를 LLM 호출 전 실패와 LLM 호출 후 실패로 구분해 호출자가 원인 stage와 middleware 위치를
    확인할 수 있게 한다.
  - 접근: `MiddlewareStage`, `MiddlewareError` 또는 동등한 typed error를 추가하고, pre hook 실패는 LLM 호출 없이
    Agent 실행 실패로 전환한다. post hook 실패는 LLM 호출 후 실패로 전환하되 정상 response fallback은 제공하지
    않는다.
  - 검증 조건:
    - 결과: pre middleware error 시 LLM client가 호출되지 않고 Runner 결과가 실패를 보고한다. post middleware
      error 시 LLM 호출은 발생하지만 Agent state 누적 전 실패로 종료된다. 두 경우 모두 `errors.As` 또는
      RunnerResult로 stage와 index를 확인할 수 있다.
    - 확인: `internal/agent` 테스트에 pre error, post error, LLM error 관찰·전파 케이스를 추가하고
      `go test ./internal/agent/...` 통과.
  - 참조: SPEC §5.7 / ANALYSIS §2, §3, §5 D2, D4

- [x] task-005: structured output contract 파싱·검증
  - 목적: 호출자가 output contract(JSON Schema)를 지정하면 최종 assistant JSON text를 schema 기준으로 검증하고,
    성공과 실패를 text-only 결과, LLM 실패, tool 실패와 구분해 확인할 수 있게 한다.
  - 접근: `OutputContract`와 structured output helper를 `internal/agent`에 추가한다. Runner가 `StatusFinal` 이후
    final text를 JSON으로 파싱하고 JSON Schema validator로 검증해 `StructuredRaw`와 `StructuredValue`를 채운다.
    표준 JSON Schema 검증을 위해 새 직접 의존성이 필요하면 구현 전에 이유와 영향을 보고하고 추가한다.
  - 검증 조건:
    - 결과: 유효한 JSON final text는 raw text, raw JSON, decoded structured value가 함께 반환된다. malformed JSON,
      schema 불일치, 빈 schema 등은 structured output 실패로 분류되고 Agent state와 raw text는 가능한 범위에서
      보존된다. output contract가 없으면 기존 text-only 경로와 동일하게 동작한다.
    - 확인: `internal/agent` 테스트에 structured output 성공, JSON 파싱 실패, schema 검증 실패, contract 미지정
      회귀 케이스를 추가하고 `go test ./internal/agent/...`, `go test ./...` 통과.
  - 참조: SPEC §5.8, §5.9, §5.10 / ANALYSIS §1, §2, §3, §5 D5, D6

- [ ] task-006: output contract 요청 반영
  - 목적: 호출자가 output contract를 지정하면 Runner가 최종 응답 검증만 수행하는 데 그치지 않고, LLM 요청에도
    동일한 contract를 전달해 모델이 요구 JSON 구조를 생성하도록 유도한다.
  - 접근: Runner가 output contract가 있을 때 provider-neutral built-in PreModel 단계를 구성해 `ChatRequest`
    앞쪽에 system message를 추가한다. system message에는 contract 이름, 설명, JSON Schema, 최종 assistant
    응답은 JSON만 출력해야 한다는 지시를 포함하고, provider별 JSON mode나 response format은 사용하지 않는다.
  - 검증 조건:
    - 결과: output contract 지정 시 실제 LLM client가 받는 첫 `ChatRequest.Messages`에 structured output system
      message가 포함된다. 지정하지 않으면 기존 text-only 요청과 동일하다. tool call 이후 재호출되는 LLM 요청에도
      같은 지시가 포함되고, structured output 검증 결과는 task-005와 같은 상태·에러 표면을 유지한다.
    - 확인: `internal/agent` 테스트에 output contract 요청 지시 포함, contract 미지정 회귀, tool call 후 재요청
      지시 유지, structured output 성공·실패 회귀 케이스를 추가하고 `go test ./internal/agent/...`,
      `go test ./...` 통과.
  - 참조: SPEC §5.8, §5.9, §5.10 / ANALYSIS §1, §2, §3, §5 D5, D6, D8
