# phase-5-2-agent-runtime 명세

## 범위

- Single Agent runner: CLI에 묶인 Agent 실행 구성을 호출자가 재사용할 수 있는 실행 표면으로 정리해,
  provider, model, tool registry, 실행 timeout, 최대 step, middleware, structured output 설정을 한 번의
  실행 단위에서 사용할 수 있게 하는 기능.
- Graph 기반 Single Agent 실행: 기존 Graph 기반 `llm_node → tool_node → llm_node → 종료` 흐름을 유지하면서,
  runner를 통해 동일한 실행 결과와 상태를 관찰할 수 있게 하는 경로.
- Pre-model middleware hook: LLM 호출 직전의 chat 요청, 누적 상태, 등록 tool schema를 관찰하고 필요한 경우
  요청을 변경하거나 호출을 중단할 수 있는 hook.
- Post-model middleware hook: LLM 응답 또는 LLM 호출 에러를 관찰하고 필요한 경우 응답을 변경하거나 실행 실패로
  전환할 수 있는 hook.
- Middleware 실행 규칙: 여러 middleware가 등록될 때 실행 순서, 변경 전파, 에러 전파가 호출자가 예측 가능한
  방식으로 드러나야 한다.
- Structured Output contract: runner 호출자가 이름, 설명, JSON Schema로 기대 최종 출력 구조를 지정하고,
  Agent의 최종 assistant 응답을 해당 contract 기준으로 파싱·검증할 수 있게 하는 기능.
- Structured Output 결과: 파싱·검증 성공 시 raw text와 구조화된 값을 함께 확인할 수 있고, 실패 시 호출자가
  structured output 실패를 LLM 호출 실패나 tool 실행 실패와 구분할 수 있게 하는 결과 표면.
- CLI 연결: 기존 CLI 실행 경로가 runner 기반 실행 구조 위에서 동일한 stdout, stderr, exit code 계약을 유지하는
  경로.

## 목표

- Single Agent 실행을 CLI 전용 조립 코드가 아니라 재사용 가능한 runner 단위로 다룰 수 있게 한다.
- model 호출 전후의 횡단 관심사를 middleware hook으로 분리해 prompt 보강, 요청 조정, 응답 후처리, 관찰을
  Agent loop 본문과 분리한다.
- structured output의 최종 구조를 이번 Phase에서 runner의 output contract로 정의해, 후속 작업이나 숨은 전역
  설정에 의존하지 않게 한다.
- provider별 JSON mode나 constrained decoding 없이도 Runtime이 최종 응답의 JSON 파싱·schema 검증 성공과 실패를
  관찰 가능하게 만든다.
- 기존 Tool Calling Runtime, Graph Runtime, provider-neutral LLM 계약을 유지하면서 Phase 5.1 tool 묶음을
  그대로 실행할 수 있게 한다.

## 제약

- output contract는 전역 설정이나 숨은 package-level 상태가 아니라 runner 호출 또는 runner 구성에서 명시적으로
  제공되어야 한다.
- structured output은 최종 assistant 응답에만 적용한다. 중간 tool call, tool result, middleware 내부 데이터에는
  structured output contract를 적용하지 않는다.
- structured output 검증 실패는 panic이나 조용한 fallback이 아니라 호출자가 구분 가능한 실패 결과로 드러나야
  한다.
- middleware hook은 등록된 순서대로 실행되고, 앞선 hook의 요청·응답 변경 결과가 뒤 hook에 전달되어야 한다.
- middleware hook이 에러를 반환하면 LLM 호출 전이면 호출을 시작하지 않고, 호출 후이면 해당 Agent 실행이 실패로
  관찰되어야 한다.
- middleware와 structured output 추가만으로 `internal/message`, `internal/tool`, `internal/graph`,
  `internal/llm`의 기존 provider-neutral 계약을 불필요하게 깨지 않는다.
- 기존 CLI 사용자는 structured output contract나 middleware를 지정하지 않아도 기존과 같은 방식으로 최종 텍스트
  답변을 stdout에서 확인할 수 있어야 한다.
- streaming 응답은 이번 범위에서 제외한다. runner와 middleware는 비스트림 chat 응답 기준으로 동작한다.

## 제외 범위

- Streaming response와 streaming 중 partial structured output 파싱.
- Claude·Ollama provider별 JSON mode, response format, constrained decoding 강제.
- output schema code generation, schema registry, 여러 output schema 자동 선택.
- 외부 plugin 시스템, 동적 middleware 로딩, middleware 조건식 DSL, hook 우선순위 시스템.
- OpenTelemetry, Datadog, 정식 trace·metric 수집 구조.
- retry, backoff, provider fallback, provider별 장애 복구 정책.
- RAG indexing·retrieval, Memory Runtime, Multi-Agent, MCP/A2A protocol adapter.
- Phase 5.1 tool 묶음의 schema나 실행 정책 변경.

## 완료 조건

1. 호출자가 Single Agent runner를 통해 provider-neutral LLM client, model, tool registry, max step, timeout을
   지정해 Agent를 실행하고 최종 상태와 최종 assistant 메시지를 확인할 수 있다.
2. runner 기반 실행에서도 tool call이 있으면 tool 실행 결과가 대화 state에 누적되고, tool call이 없으면 최종
   응답으로 종료되는 기존 Graph 기반 Single Agent 흐름이 유지된다.
3. CLI는 runner 기반 실행 구조를 사용하면서도 기존처럼 stdin 프롬프트를 받아 최종 답을 stdout에 출력하고,
   실패 시 stderr와 비정상 exit code로 종료한다.
4. pre-model middleware가 LLM 호출 직전의 요청을 관찰하고 변경할 수 있으며, 변경된 요청이 실제 LLM client 호출에서
   확인된다.
5. post-model middleware가 LLM 응답을 관찰하고 변경할 수 있으며, 변경된 응답이 Agent state와 최종 결과에 반영된다.
6. 여러 middleware가 등록 순서대로 실행되고, 앞 hook의 변경 결과가 뒤 hook에 전달되는 것을 호출자가 확인할 수
   있다.
7. middleware가 에러를 반환하면 호출자는 LLM 호출 전 실패와 LLM 호출 후 실패를 구분 가능한 실행 결과로 확인할 수
   있다.
8. 호출자가 output contract(JSON Schema)를 지정하면 최종 assistant 응답의 JSON text가 schema 기준으로 검증되고,
   성공 시 raw text와 structured value를 함께 확인할 수 있다.
9. 최종 assistant 응답이 JSON으로 파싱되지 않거나 output contract를 만족하지 않으면, 호출자는 structured output
   실패를 LLM 호출 실패나 tool 실행 실패와 구분해 확인할 수 있다.
10. output contract를 지정하지 않은 실행은 기존 text-only 최종 응답 경로와 같은 방식으로 동작한다.
