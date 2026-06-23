# phase-5-3-streaming-agent-response 명세

## 범위

- Provider-neutral streaming LLM contract: Runtime이 provider별 SDK 세부를 직접 알지 않고도 model 응답 chunk를
  순차적으로 받을 수 있는 streaming 호출 표면.
- Streaming event: 호출자와 CLI가 text chunk, 완료, error, 최종 조립 결과를 구분해 관찰할 수 있는 이벤트 표면.
- Runner streaming 실행: 기존 비스트림 `Runner.Run` 경로를 유지하면서, 호출자가 streaming event를 받을 수 있는
  별도 실행 경로.
- Agent streaming 조립: streaming chunk를 최종 assistant message와 final text로 조립해 기존 Agent state와
  Runner 결과 의미를 유지하는 경로.
- CLI streaming 출력: CLI 사용자가 streaming mode를 켜면 model text chunk를 stdout에서 순차적으로 확인할 수 있는
  실행 경로.
- Structured output과 streaming의 관계: streaming 중 partial JSON은 검증하지 않고, stream 완료 후 조립된 final
  text에 기존 output contract 검증을 적용하는 정책.
- Middleware와 streaming의 관계: 기존 PreModel middleware는 streaming 요청에도 적용하고, 기존 PostModel
  middleware는 stream 완료 후 조립된 final response에 적용하는 경로.

## 목표

- CLI 사용자가 긴 model 응답을 최종 완료까지 기다리지 않고 stdout에서 순차적으로 확인할 수 있게 한다.
- 코드 호출자가 Runner를 통해 streaming event를 관찰하면서도, stream 완료 후 기존과 같은 최종 상태와 final text를
  확인할 수 있게 한다.
- 기존 비스트림 `LLMClient.Chat`, `Runner.Run`, CLI 기본 실행 계약을 깨지 않고 streaming을 선택 기능으로 추가한다.
- structured output contract가 있는 실행에서도 streaming 완료 후 최종 JSON text를 기존 schema 검증 표면으로
  판정할 수 있게 한다.
- provider별 streaming wire format은 provider 구현 내부로 숨기고, Agent Runtime은 provider-neutral event만 다룬다.

## 제약

- streaming은 명시적으로 선택한 실행에서만 동작한다. 기존 CLI 기본 경로와 `Runner.Run`은 비스트림 동작을
  유지한다.
- streaming 중간 chunk는 최종 assistant message로 간주하지 않는다. Agent state에는 stream 완료 후 조립된 assistant
  message만 누적된다.
- output contract가 있는 경우에도 partial JSON chunk를 검증하지 않는다. 검증은 stream 완료 후 조립된 final text에만
  적용한다.
- 기존 PreModel middleware는 streaming LLM 호출 전 `ChatRequest` 관찰·변경에 동일하게 적용되어야 한다.
- 기존 PostModel middleware는 streaming 완료 후 조립된 `ChatResponse`에 적용되어야 한다.
- streaming 추가만으로 `internal/message`, `internal/tool`, `internal/graph`의 기존 비스트림 contract를
  불필요하게 깨지 않는다.
- provider별 JSON mode, response format, constrained decoding은 이번 범위에서 사용하지 않는다.
- CLI streaming 출력은 text chunk 기준이다. tool call이나 structured output 검증 결과를 token 단위로 노출하지 않는다.

## 제외 범위

- Streaming 중 partial structured output 파싱·검증.
- Provider별 JSON mode, response format, constrained decoding 연동.
- Tool call argument delta의 고급 partial 조립 정책과 tool call streaming UI.
- Multi-Agent streaming relay, supervisor streaming, worker-to-worker streaming.
- RAG retrieval 진행 상황 streaming, Memory Runtime streaming, MCP/A2A streaming protocol 연동.
- Retry, backoff, provider fallback, stream 재연결 정책.
- Web UI, TUI, progress bar, rich terminal rendering.

## 완료 조건

1. 호출자가 provider-neutral streaming LLM contract를 통해 text chunk와 완료 이벤트를 순차적으로 받을 수 있다.
2. Runner는 streaming 실행 경로에서 event를 호출자에게 전달하고, 완료 후 기존 Runner 결과와 같은 final text와
   Agent state를 반환한다.
3. CLI는 streaming mode에서 model text chunk를 stdout에 순차 출력하고, 완료 시 exit code 0으로 종료한다.
4. CLI streaming 실패는 stderr와 non-zero exit code로 관찰되며, 기존 비스트림 실패 출력 계약을 깨지 않는다.
5. output contract가 있는 streaming 실행은 stream 완료 후 조립된 final text를 JSON Schema로 검증하고, 성공과
   structured output 실패를 기존 Runner status로 구분한다.
6. output contract가 없는 streaming 실행은 기존 text-only 최종 응답 경로와 같은 final text를 보존한다.
7. PreModel middleware는 streaming 요청에도 적용되고, 변경된 `ChatRequest`가 실제 streaming LLM 호출에 전달된다.
8. PostModel middleware는 stream 완료 후 조립된 response에 적용되고, 변경 결과가 Agent state와 최종 결과에 반영된다.
9. streaming을 사용하지 않는 기존 `LLMClient.Chat`, `Runner.Run`, CLI 기본 실행, tool call 실행 테스트는 기존과
   같은 동작을 유지한다.
