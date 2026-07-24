# Phase 4.3 Streaming Agent Response 명세

## 범위

Phase 4.3은 Phase 4.2의 provider-neutral Single Agent Runner와 Agent loop에 streaming model 응답 경로를 추가한다.
Claude와 Ollama가 전송하는 provider별 streaming 형식을 Runtime 공통 계약으로 정규화하고, 호출자가 model text
조각을 생성 순서대로 관찰하면서 최종 Agent 상태와 응답도 확인할 수 있게 한다.

Runner streaming은 Agent loop의 모든 model 호출을 대상으로 한다. Model이 Tool을 요청하면 streaming 응답에서
완성된 Tool call을 조립해 기존 Tool 실행 경로로 처리하고, Tool result를 누적한 다음 model streaming을 이어간다.
Phase 4.3에서 호출자에게 공개하는 streaming 정보는 model text 조각과 최종 성공 또는 오류 결과로 제한한다.
Tool 호출·결과·오류·timeout의 공개 lifecycle event는 Phase 4.4에서 다룬다.

CLI는 별도 활성화 옵션 없이 streaming 실행을 기본으로 사용한다. Interactive terminal에서는 생성 중인 model text를
임시 영역에 순서대로 표시하고, 정상 종료하면 임시 내용을 정리한 뒤 최종 answer만 남긴다. Stdout이 pipe나 파일로
redirect된 경우에는 terminal 제어 문자를 쓰지 않고 최종 answer만 한 번 출력한다. Streaming 오류는 stderr와 0이 아닌
종료 코드로 확인할 수 있어야 한다.

Structured output schema가 지정된 streaming run은 text 조각을 검증 전 임시 데이터로 전달한다. Runner는 전체
최종 응답을 조립한 뒤 Phase 4.2와 같은 JSON 파싱과 schema 검증을 수행한다. 검증 성공 시에만 최종 성공 결과를
반환하고, 실패하면 이미 전달된 조각과 별개로 structured output 오류를 반환한다.

기존 non-streaming `LLMClient.Chat`과 `Runner.Run` 경로는 유지한다. Streaming 지원은 호출자가 전체 응답만 필요한
programmatic 실행을 제거하거나 provider별 streaming 형식에 의존하게 만들지 않는다.

### 입력 맥락

- `ROADMAP.md`의 Phase 4.3 범위는 provider-neutral streaming LLM contract, Runner streaming event, CLI streaming
  출력, 최종 응답 조립, structured output final 검증 관계다.
- `features/20260710-001-phase-4-2-agent-execution/spec.md`는 streaming을 Phase 4.3으로 제외하고, Runner,
  middleware, structured output, Tool loop와 실행 제한의 기존 계약을 정의한다.
- `internal/llm.LLMClient`는 현재 완성된 `ChatResponse`를 반환하고, Claude와 Ollama adapter는 non-streaming
  응답만 처리한다.
- `internal/agent.Runner`는 현재 `Run`이 완료된 뒤 `RunnerResult`를 반환하며, CLI는 최종 text를 한 번에
  stdout으로 출력한다.
- Claude 공식 문서는 Messages API의 SSE text·Tool input delta와 최종 message 조립 방식을 제공한다.
- Ollama 공식 문서는 REST API streaming이 기본 활성화이고 SDK에서는 선택적으로 활성화됨을 설명한다.

## 목표

대화형 CLI 사용자가 긴 model 응답의 완료를 기다리지 않고 생성되는 text를 즉시 확인하되, 실행이 끝난 화면에는 최종
answer만 명확하게 남도록 한다. Tool을 사용하는 여러 step의 Agent run에서도 provider별 전송 형식을 알 필요 없이
같은 streaming 동작을 제공해야 한다.

Programmatic 호출자는 provider-neutral streaming 정보와 정확히 한 번의 최종 성공 또는 오류 결과를 순서대로
관찰할 수 있어야 한다. 최종 성공 결과는 기존 non-streaming Runner가 제공하는 Agent 상태, final answer와 검증된
structured output 의미를 유지해야 한다.

Streaming 전송과 최종 상태 판정을 분리한다. Text 조각은 응답 생성 중의 임시 관찰값이고, middleware 적용,
완료 사유 판정, Tool call 조립과 structured output 검증을 마친 결과만 최종 성공으로 취급해야 한다.

## 제약

Streaming contract와 Runner event는 Claude, Ollama 또는 SSE·NDJSON 같은 provider 전송 형식을 외부에 노출하지
않아야 한다. Provider adapter가 raw event를 Runtime 공통 text와 최종 응답 의미로 변환해야 한다.

각 model 호출 전에 Phase 4.2의 `pre-model` middleware를 적용하고, streaming으로 완성한 정규화 응답에는
`post-model` middleware를 등록 순서대로 적용해야 한다. Streaming 중 전달된 text 조각은 임시 값이며,
`post-model`이 변경한 최종 응답과 다를 수 있다. Middleware 실패 후에는 후속 model 또는 Tool 실행을 계속하지
않고 최종 오류로 종료해야 한다.

Tool call arguments는 provider가 전달한 조각을 완성한 뒤 기존 Tool schema와 실행 계약에 따라 처리해야 한다.
완성되지 않았거나 잘못된 Tool call을 실행해서는 안 된다. Tool loop의 max step, 최대 Tool 호출 수, result 크기,
model·Tool timeout, caller cancellation과 전체 run deadline은 streaming 경로에서도 동일하게 적용해야 한다.

Provider가 길이 제한, 차단 또는 알 수 없는 완료 사유를 반환하면 이미 전달된 text가 있더라도 정상 final로 숨기지
않아야 한다. Streaming 연결 오류, provider 오류, middleware 오류, 실행 제한과 structured output 오류도 최종 성공과
구분해야 한다.

Streaming 결과는 text 조각의 순서를 보존하고 최종 성공 또는 오류로 한 번만 끝나야 한다. 호출자가 context를
취소하거나 streaming 소비를 중단할 때 provider 요청과 Runner 실행이 취소 가능해야 하며, 종료 후 event 생산이나
goroutine이 background에 남지 않아야 한다.

CLI는 positional argument 우선, argument가 없을 때 stdin 사용, final 성공 시 종료 코드 0, 실패 시 stderr와
0이 아닌 종료 코드라는 기존 입력·종료 contract를 유지한다. Interactive terminal의 임시 출력은 정상 종료 시
정리해야 하며, redirect된 stdout은 정상 final 이외의 중간 text나 terminal 제어 문자를 포함하지 않아야 한다.

테스트는 실제 외부 provider 호출 없이 local HTTP test server와 stub streaming client를 사용해 event 순서,
응답 조립, Tool loop, middleware, structured output, 취소·timeout과 CLI 출력을 확인할 수 있어야 한다.

## 제외 범위

Tool 호출 시작, Tool result, Tool 오류, Tool timeout을 호출자에게 공개하는 Runner streaming lifecycle event는
포함하지 않는다. 이 event와 inline·process-backed Tool 실행의 공통 관찰 계약은 Phase 4.4에서 다룬다.

Tool execution backend, process-backed executor, cancel grace period, process kill·wait, Tool process crash 분류와
보안 sandbox는 포함하지 않는다.

CLI streaming 활성화·비활성화 flag, 대화형 다중 입력 UI, full-screen TUI, HTTP streaming API, WebSocket,
Agent Server는 포함하지 않는다. Interactive terminal의 임시 text 표시와 최종 화면 정리에 필요한 최소 renderer는
Phase 4.3 범위에 포함한다.

Provider별 native structured output 또는 constrained decoding, 부분 JSON schema 검증, 검증 전 JSON 조각의
안전성 보장, structured output 실패 자동 재호출과 복구는 포함하지 않는다.

Streaming 재연결, 중단 지점부터 자동 재개, provider fallback, retry, rate-limit backoff는 포함하지 않는다.

Provider가 제공하는 thinking·reasoning 조각의 공개 출력, token 단위 latency 통계와 외부 observability 전송은
포함하지 않는다.

RAG, Memory, Multi-Agent, MCP, A2A 구현은 포함하지 않는다.

## 완료 조건

1. 호출자는 provider-neutral streaming Runner로 사용자 입력 하나를 실행하고, 각 model text 조각을 생성 순서대로
   받은 뒤 정확히 한 번의 최종 성공 결과를 확인할 수 있다.
2. Streaming 최종 성공 결과는 같은 완성 응답을 처리한 non-streaming Runner와 동일한 Agent 종료 상태,
   final answer, 메시지 누적과 완료 사유를 제공한다.
3. Claude와 Ollama streaming adapter는 provider별 전송 형식을 Runtime 공통 text 조각과 완성된 응답으로
   변환하며, 호출자는 SSE·NDJSON이나 provider raw event를 처리하지 않아도 된다.
4. Model이 streaming 응답으로 Tool call을 반환하면 Runner는 완성된 Tool 이름과 arguments만 기존 Tool loop에서
   실행하고, Tool result를 다음 streaming model 요청에 누적해 final 상태까지 진행할 수 있다.
5. `pre-model` middleware는 모든 streaming model 호출 전에 등록 순서대로 실행되고, `post-model` middleware는
   조립된 각 model 응답에 등록 순서대로 실행되며 변경 결과가 Agent 상태와 다음 판단에 반영된다.
6. Middleware가 실패하면 호출자는 middleware 오류를 최종 오류 결과로 확인할 수 있고, 실패 이후 추가 model 또는
   Tool 호출은 발생하지 않는다.
7. Output schema가 지정된 run은 검증 전 text 조각을 순서대로 전달한 뒤 완성된 final answer를 JSON Schema로
   검증하며, 성공한 경우에만 검증된 structured output과 최종 성공 결과를 반환한다.
8. Streaming structured output의 JSON 파싱이나 schema 검증이 실패하면 호출자는 이미 전달된 조각과 별개로
   structured output 최종 오류를 확인할 수 있고, 성공 final 결과는 반환되지 않는다.
9. 길이 제한, 차단, 알 수 없는 완료 사유, provider stream 오류와 불완전 Tool call은 이미 text가 전달됐더라도
   정상 final로 처리되지 않고 구분 가능한 최종 오류로 종료된다.
10. Streaming 경로는 max step, 최대 Tool 호출 수, Tool result 크기, model·Tool timeout, caller cancellation과
    전체 run deadline을 기존 Runner와 동일하게 지키며 제한 초과를 최종 성공과 구분한다.
11. 호출자가 context를 취소하거나 event 소비를 중단한 뒤에는 provider 요청과 Runner 실행이 종료되고, event 생산
    또는 goroutine이 background에서 계속되지 않는다.
12. CLI는 별도 flag 없이 streaming Runner를 기본으로 사용한다. Interactive terminal에서는 model text 조각을
    도착 순서대로 임시 표시하고, 정상 종료하면 임시 영역을 정리해 최종 answer만 남긴 뒤 종료 코드 0을 반환한다.
13. Stdout이 pipe나 파일로 redirect된 CLI 실행은 terminal 제어 문자와 중간 model text를 출력하지 않고, 정상
    final answer만 한 번 출력한다.
14. 기존 `LLMClient.Chat`과 `Runner.Run` non-streaming 호출자는 변경 없이 전체 응답과 최종 결과를 받을 수 있다.
15. 테스트는 실제 Claude·Ollama 호출 없이 local HTTP test server와 stub streaming client로 text 조각 순서,
    Tool call 조립과 loop, middleware, structured output, 오류, 취소·timeout, CLI 기본 streaming 출력을 확인한다.
16. CLI streaming 도중 오류가 발생하면 interactive terminal의 임시 영역을 종료 가능한 상태로 정리하고,
    redirect된 stdout에는 성공 final을 출력하지 않는다. 사용자는 stderr와 0이 아닌 종료 코드로 실패를 확인한다.
