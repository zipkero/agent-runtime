# Phase 4.3 Streaming Agent Response 구현

## 체크리스트

- [ ] task-001: Provider-neutral stream 계약과 Claude adapter 구현
  - 목적: 호출자가 Claude의 SSE 형식을 알지 않고도 model text를 생성 순서대로 받고, 완성된 응답이나 구분 가능한
    stream 오류를 확인할 수 있다.
  - 접근: 기존 `LLMClient`는 유지하고 `StreamingLLMClient`와 공통 stream event를 추가한다. Claude adapter는
    `bufio.Reader`로 SSE를 해석해 text delta를 즉시 전달하고, Tool input·usage·완료 사유를 완성된
    `ChatResponse`로 조립한다.
  - 검증 조건:
    - 결과: Claude stream은 text delta 뒤에 완성 응답을 정확히 한 번 반환하고, 부분 Tool JSON은 content block 완료
      후에만 Tool call이 된다. Ping과 알 수 없는 event는 무시하며 SSE 오류, 잘못된 순서, 불완전 stream과 취소는
      기존 provider 오류 체계로 종료된다. 기존 `Chat` 호출은 동일하게 동작한다.
    - 확인: local HTTP test server로 SSE 순서, multi-line data, text·Tool call·usage·stop reason 조립, ping·unknown
      event, provider error, 비정상 EOF, timeout, context 취소와 소비 중단을 테스트하고 `go test ./internal/llm`을
      실행한다.
  - 참조: SPEC §5.1, SPEC §5.3, SPEC §5.9, SPEC §5.11, SPEC §5.14, SPEC §5.15, ANALYSIS §1,
    ANALYSIS §2, ANALYSIS §3, ANALYSIS §5.1, ANALYSIS §5.2, ANALYSIS §5.4, ANALYSIS §5.11,
    ANALYSIS §5.13

- [ ] task-002: Ollama streaming adapter 구현
  - 목적: 호출자가 Ollama의 NDJSON 형식을 처리하지 않고 text와 완성된 Tool call을 공통 stream 계약으로 받아
    Agent 실행에 사용할 수 있다.
  - 접근: Ollama streaming 요청에 `stream: true`를 보내고 `json.Decoder`로 연속 chunk를 소비한다. 도착 순서대로
    content와 Tool call을 누적하고 `done: true`의 usage와 완료 사유를 반영해 완성된 `ChatResponse`를 만든다.
  - 검증 조건:
    - 결과: Ollama stream은 text delta 순서를 보존하고 여러 chunk의 Tool call을 완성한 뒤 응답을 정확히 한 번
      반환한다. Decode 오류, final marker 없는 EOF, provider 오류, timeout과 취소는 성공 응답으로 숨겨지지 않으며
      기존 non-streaming `Chat`은 유지된다.
    - 확인: local HTTP test server로 streaming request, text·Tool call chunk 조립, usage·done reason, decode 오류,
      비정상 EOF, timeout, context 취소와 소비 중단을 테스트하고 `go test ./internal/llm`을 실행한다.
  - 참조: SPEC §5.1, SPEC §5.3, SPEC §5.4, SPEC §5.9, SPEC §5.11, SPEC §5.14, SPEC §5.15,
    ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §5.2, ANALYSIS §5.4, ANALYSIS §5.6,
    ANALYSIS §5.12, ANALYSIS §5.13

- [ ] task-003: Agent와 Runner의 공통 streaming 실행 경로 구현
  - 목적: Programmatic 호출자가 모든 model step의 text delta를 순서대로 관찰하면서 기존 Tool loop, middleware,
    실행 제한과 Agent 종료 상태가 반영된 final 또는 error 결과를 정확히 한 번 받을 수 있다.
  - 접근: Agent 상태 머신을 공통 내부 loop로 추출하고 non-streaming caller와 streaming caller를 연결한다.
    Runner에는 `iter.Seq` 기반 `RunStream`과 text·final·error event를 추가하되, Tool은 adapter가 완성한 call만
    기존 실행 경로로 전달한다.
  - 검증 조건:
    - 결과: 여러 model step의 delta는 step과 생성 순서를 보존한다. 각 호출의 pre/post-model middleware, Tool result
      누적, 완료 사유, max step, Tool 호출 수·result 크기, model·Tool timeout, 전체 deadline이 기존 `Run`과 같은
      상태 전이를 사용한다. 실패 뒤에는 추가 model·Tool 호출이 없고, 완전 소비 시 terminal event는 하나뿐이다.
      Streaming 미지원 client는 외부 호출 없이 error event로 끝나며 기존 `Runner.Run` 결과는 유지된다.
    - 확인: stub streaming client와 등록 Tool로 일반 text, 여러 step, 완성 Tool call 실행, middleware 순서·변경·실패,
      길이 제한·차단·알 수 없는 완료 사유, provider 오류, 모든 실행 제한, context 취소와 iterator 조기 중단을
      테스트한다. `go test -race ./internal/agent`와 `go test ./...`를 실행한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.4, SPEC §5.5, SPEC §5.6, SPEC §5.9, SPEC §5.10,
    SPEC §5.11, SPEC §5.14, SPEC §5.15, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §5.2,
    ANALYSIS §5.3, ANALYSIS §5.5, ANALYSIS §5.6, ANALYSIS §5.8

- [ ] task-004: Streaming structured output 최종 검증 연결
  - 목적: Output schema를 사용하는 호출자가 생성 중 text를 먼저 관찰하되, 전체 final answer가 JSON Schema를
    통과한 경우에만 검증된 structured output과 성공 결과를 받는다.
  - 접근: `Run`과 `RunStream`이 기존 structured output finalization helper를 공유하게 한다. Delta는 임시 event로
    그대로 전달하고, 완성된 Agent final answer만 JSON 파싱과 schema 검증에 통과시켜 terminal event를 결정한다.
  - 검증 조건:
    - 결과: 유효한 JSON은 모든 delta 뒤에 검증된 structured output을 포함한 final event가 된다. JSON 파싱이나
      schema 검증 실패는 이미 전달된 delta와 별개로 structured output error event 하나가 되며 성공 final은
      발생하지 않는다. Schema가 없는 실행과 기존 `Runner.Run`의 검증 의미는 유지된다.
    - 확인: stub streaming client로 유효 JSON, malformed JSON, schema 불일치, post-model이 변경한 final 응답,
      schema 미지정과 non-streaming 회귀를 테스트하고 `go test ./internal/agent`와 `go test ./...`를 실행한다.
  - 참조: SPEC §5.2, SPEC §5.7, SPEC §5.8, SPEC §5.15, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3,
    ANALYSIS §5.5, ANALYSIS §5.7, ANALYSIS §5.8

- [ ] task-005: CLI 기본 streaming과 final-only renderer 구현
  - 목적: CLI 사용자가 별도 flag 없이 생성 중 text를 interactive terminal에서 확인하고, 실행이 끝난 화면과
    redirect 출력에서는 최종 answer 또는 명확한 실패만 확인할 수 있다.
  - 접근: CLI가 `Runner.RunStream`을 기본 소비하도록 바꾸고 stdout의 character device 여부를 실행 경계에서
    판별한다. Interactive renderer는 cursor 저장·복원·영역 지우기로 임시 delta를 정리하며, redirect renderer는
    delta를 무시하고 성공 final만 한 번 출력한다. 구현 완료 내용은 `README.md`와 `ROADMAP.md`에 반영한다.
  - 검증 조건:
    - 결과: Interactive terminal은 delta를 도착 순서대로 표시한 뒤 정상 종료 시 임시 영역을 지우고 final answer만
      남긴다. Redirect stdout에는 ANSI 제어 문자와 중간 text가 없고 final answer만 한 번 기록된다. 오류 시 임시
      영역은 복구되고 성공 final 없이 stderr와 0이 아닌 종료 코드가 사용된다. 기존 positional argument·stdin과
      성공 종료 코드 계약은 유지된다.
    - 확인: 주입한 interactive 판정, 기록 writer와 stub streaming client로 delta 출력 순서, final 화면 sequence,
      redirect bytes, 오류 reset, stdout·stderr와 종료 코드를 테스트한다. 문서 diff를 확인하고
      `go test ./cmd/agent-runtime`와 `go test ./...`를 실행한다.
  - 참조: SPEC §5.12, SPEC §5.13, SPEC §5.15, SPEC §5.16, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3,
    ANALYSIS §4, ANALYSIS §5.9, ANALYSIS §5.10
