# phase-5-3-streaming-agent-response 구현

## 체크리스트

- [ ] task-001: Provider-neutral streaming LLM contract
  - 목적: 호출자가 provider별 SDK나 wire format을 모르고도 model text chunk와 완료 이벤트를 순차적으로 받을 수
    있게 한다.
  - 접근: `internal/llm`에 기존 `LLMClient.Chat`을 유지한 채 `LLMStreamer`, `ChatStream`, `ChatStreamEvent`,
    event type을 추가한다. stream error는 `Recv` error로 표현하고, 완료된 assistant message는 complete event에서
    전달할 수 있게 한다.
  - 검증 조건:
    - 결과: stub stream으로 text delta, message complete, `Recv` error, `Close` 경로를 구분해 관찰할 수 있다.
      기존 `LLMClient.Chat` 구현과 호출자는 변경 없이 컴파일된다.
    - 확인: `internal/llm` 테스트에 stream event 순서, complete message 전달, error 전파, close 호출 케이스를
      추가하고 새 테스트 함수에는 검증 의도를 주석으로 남긴다. `go test ./internal/llm/...` 통과.
  - 참조: SPEC §5.1, §5.9, ANALYSIS §1, §3, §4, §5 D1

- [ ] task-002: Provider streaming 구현 연결
  - 목적: Ollama와 Claude provider client가 같은 provider-neutral streaming contract로 실제 text chunk와 완료
    이벤트를 반환하게 한다.
  - 접근: `internal/llm/ollama.go`에는 `/api/chat` streaming 응답 변환 경로를 추가하고 기존 `stream:false`
    `Chat` 경로를 유지한다. `internal/llm/claude.go`에는 Anthropic SDK streaming 경로를 provider-neutral event로
    변환하는 경로를 추가한다.
  - 검증 조건:
    - 결과: Ollama와 Claude streaming 호출은 text delta와 complete event를 순서대로 반환한다. provider error,
      malformed stream, context cancellation은 stream 실패로 전파된다. 기존 비스트림 `Chat` 동작은 유지된다.
    - 확인: `httptest` 또는 provider test double로 request 변환, delta 변환, complete 변환, error 전파,
      cancellation을 검증하고 새 테스트 함수에는 provider별 검증 의도를 주석으로 남긴다. `go test
      ./internal/llm/...` 통과.
  - 참조: SPEC §5.1, §5.4, §5.9, ANALYSIS §1, §2, §4, §5 D1

- [ ] task-003: Runner와 Agent streaming 실행 표면
  - 목적: 코드 호출자가 `Runner` streaming 경로로 text delta를 관찰하면서, 완료 후 기존 Runner 결과와 같은 final
    text, final message, Agent state를 받을 수 있게 한다.
  - 접근: `internal/agent`에 `RunnerStreamEvent`, `RunnerStreamSink`, `Runner.Stream`을 추가한다. Agent LLM 호출
    경계에서 `LLMStreamer`를 사용해 stream을 소비하고, 완료 후 조립된 `ChatResponse`를 기존 PostModel과 state
    누적 경로로 전달한다. Graph engine contract는 바꾸지 않는다.
  - 검증 조건:
    - 결과: Runner streaming 경로는 text delta를 sink에 순서대로 전달하고 final result를 반환한다. streaming
      미지원 client, provider stream error, sink error는 실패 결과로 매핑된다. PreModel 변경 request는 실제
      streaming 호출에 전달되고, PostModel 변경 response는 state와 final result에 반영된다.
    - 확인: `internal/agent` 테스트에 event 순서, final result, unsupported streaming, provider error, sink error,
      PreModel request 변경, PostModel response 변경 케이스를 추가하고 새 테스트 함수에는 시나리오 주석을 남긴다.
      `go test ./internal/agent/...` 통과.
  - 참조: SPEC §5.2, §5.4, §5.6, §5.7, §5.8, §5.9, ANALYSIS §1, §2, §3, §4, §5 D2, D3, D6

- [ ] task-004: Streaming tool call 조립 회귀
  - 목적: streaming 응답이 tool call을 포함해도 완료 후 기존 tool node가 실행되고, 다음 LLM streaming step을 거쳐
    최종 답까지 도달하게 한다.
  - 접근: complete event의 assistant message에 tool call block이 있으면 이를 최종 assistant message로 조립하고
    기존 tool dispatcher와 Agent loop를 재사용한다. tool call argument delta의 partial UI는 구현하지 않는다.
  - 검증 조건:
    - 결과: streaming complete message의 tool call은 기존 tool 실행 경로로 이어지고 RoleTool 메시지가 state에
      누적된다. 이후 다음 streaming LLM 호출의 text delta와 final text가 반환된다. 기존 비스트림 tool call 테스트는
      같은 동작을 유지한다.
    - 확인: `internal/agent` 테스트에 stream complete tool call, tool result 누적, 다음 streaming final text,
      기존 비스트림 tool call 회귀 케이스를 추가하고 새 테스트 함수에는 흐름 주석을 남긴다. `go test
      ./internal/agent/...` 및 `go test ./...` 통과.
  - 참조: SPEC §5.2, §5.6, §5.9, ANALYSIS §2, §4, §5 D6

- [ ] task-005: Streaming structured output final 검증
  - 목적: output contract가 있는 streaming 실행에서 stream 완료 후 조립된 final text만 JSON Schema로 검증하고,
    성공과 structured output 실패를 기존 Runner status로 구분하게 한다.
  - 접근: `Runner.Stream` final result 매핑에서 기존 structured output helper를 재사용한다. text delta 수신 중에는
    partial JSON 파싱이나 검증을 수행하지 않고, output contract가 없으면 기존 text-only final text를 보존한다.
  - 검증 조건:
    - 결과: 유효한 final JSON은 structured output success로 반환된다. malformed JSON과 schema mismatch는
      structured output 실패로 반환되고 raw final text와 Agent state는 가능한 범위에서 보존된다. contract 미지정
      streaming은 text-only 결과와 동일하다.
    - 확인: `internal/agent` 테스트에 streaming valid JSON, malformed JSON, schema mismatch, contract 미지정
      text-only 회귀 케이스를 추가하고 새 테스트 함수에는 final-only 검증 이유를 주석으로 남긴다. `go test
      ./internal/agent/...` 및 `go test ./...` 통과.
  - 참조: SPEC §5.5, §5.6, ANALYSIS §1, §2, §3, §5 D4, D5

- [ ] task-006: CLI streaming mode
  - 목적: CLI 사용자가 streaming mode를 명시하면 model text chunk를 stdout에서 순차 확인하고, 완료 또는 실패를
    exit code와 stderr로 판단할 수 있게 한다.
  - 접근: `cmd/agent-runtime`에 streaming mode 선택 입력을 추가하고 기본값은 false로 둔다. streaming mode에서는
    `Runner.Stream`에 stdout sink를 연결하고, final success는 exit code 0으로 반환한다. provider, middleware,
    sink, structured output 실패는 stderr와 non-zero exit code로 매핑한다.
  - 검증 조건:
    - 결과: CLI 기본 실행은 기존처럼 final text를 한 번 출력한다. streaming mode는 text chunk를 순서대로 stdout에
      출력하고 성공 시 exit code 0으로 종료한다. streaming 실패와 structured output 실패는 stderr와 non-zero exit
      code로 관찰된다.
    - 확인: `cmd/agent-runtime` 테스트에 기본 비스트림 회귀, streaming stdout 순서, success exit code, provider
      error stderr, structured output error stderr 케이스를 추가하고 새 테스트 함수에는 사용자 관찰 결과 주석을
      남긴다. `go test ./cmd/agent-runtime/...` 및 `go test ./...` 통과.
  - 참조: SPEC §5.3, §5.4, §5.5, §5.9, ANALYSIS §1, §2, §3, §4, §5 D2, D5
