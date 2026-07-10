# Phase 4.2 Agent Execution 명세

## 범위

Phase 4.2는 Phase 3의 Agent tool-use loop와 Phase 4.1의 Tool 묶음을 Single Agent 실행 경로로 조립한다. 대상 범위는
model 호출 전후 middleware, JSON Schema 기반 structured output, provider-neutral Single Agent Runner, 기존 CLI의
Agent loop 기반 실행 전환이다.

Middleware는 Agent loop에서 발생하는 모든 model 호출을 대상으로 한다. `pre-model` middleware는 model 요청을
확인하고 변경할 수 있으며, `post-model` middleware는 Runtime이 정규화한 model 응답을 확인하고 변경할 수 있다.
여러 middleware는 등록 순서대로 실행되고, middleware 오류는 호출자가 Agent 실행 실패로 구분해 확인할 수 있어야
한다.

Structured output은 호출자가 JSON Schema를 지정한 run에 적용한다. Runner는 최종 assistant 응답을 JSON으로
파싱하고 지정된 schema에 맞는지 검증한다. 검증에 성공하면 호출자는 검증된 JSON 결과를 확인할 수 있고, JSON 파싱
실패나 schema 불일치는 일반 model 오류와 구분되는 structured output 오류로 확인할 수 있어야 한다. Schema를 지정하지
않은 run은 기존처럼 일반 text final answer를 반환한다.

Single Agent Runner는 LLM client, model, 실행 제한, Tool registry, middleware, 선택적인 structured output schema를
주입받아 하나의 사용자 요청을 실행한다. Runner는 기존 Agent loop의 메시지 누적, Tool 실행, max step, timeout,
context cancellation을 보존하고 최종 실행 상태와 결과를 호출자에게 반환한다.

기존 CLI는 단발 `LLMClient.Chat` 호출 대신 Single Agent Runner를 사용한다. 진입점에서 조립한 Phase 3·4.1 Tool을
Agent에 제공하고, Tool call이 포함된 요청은 전체 Agent loop를 거쳐 최종 응답을 stdout으로 출력한다. 실행 실패는
stderr와 종료 코드로 확인할 수 있어야 한다.

## 목표

호출자가 provider별 구현을 알지 않고도 middleware와 Tool을 조합해 하나의 Single Agent run을 실행할 수 있게 한다.
Agent의 반복 판단과 Tool 사용은 기존 Runtime contract를 재사용하고, Runner는 실행 의존성과 최종 결과를 다루는
일관된 진입점을 제공해야 한다.

Model 호출 전후의 횡단 관심사를 Agent loop 본체에서 분리한다. Middleware는 요청 보강, 정책 적용, 응답 검사 같은
동작을 수행할 수 있어야 하며, 실행 순서와 실패 결과가 테스트에서 관찰 가능해야 한다.

Structured output을 요청한 호출자는 단순 문자열이 아니라 지정한 JSON Schema를 만족하는 결과를 받거나, 결과가
계약을 만족하지 못했다는 명확한 오류를 받아야 한다. 이 검증은 provider 고유 기능에 의존하지 않고 Claude와 Ollama
등 기존 provider에 동일하게 적용할 수 있어야 한다.

CLI 사용자는 기존 단발 model 응답이 아니라 등록된 Tool을 사용할 수 있는 Agent loop의 최종 결과를 확인할 수 있어야
한다.

## 제약

Runner, middleware, structured output은 기존 provider-neutral `LLMClient`, message, Tool contract 위에서 동작해야
한다. Runtime 본체가 Claude, Ollama 또는 특정 Tool 구현에 직접 의존하지 않아야 한다.

`pre-model`과 `post-model` middleware는 모든 model 호출마다 등록 순서대로 실행되어야 한다. Middleware가 변경한
요청과 응답은 다음 실행 단계에 반영되어야 하며, middleware 오류 이후에는 후속 model 또는 Tool 실행을 계속하지
않아야 한다.

Structured output 검증은 Tool call이 없는 최종 assistant 응답을 대상으로 한다. JSON Schema는 Runner 경계에서
provider-neutral하게 적용하며, provider API의 강제 출력 기능을 Phase 4.2 완료 조건으로 요구하지 않는다. 잘못된
JSON이나 schema 불일치는 자동으로 숨기거나 일반 text 성공 결과로 처리하지 않는다.

Schema를 지정하지 않은 실행은 기존 final answer 동작과 호환되어야 한다. Structured output 지원 때문에 일반 text
응답이나 기존 Tool loop가 별도 provider 기능을 요구해서는 안 된다.

Runner의 LLM client, model, Tool registry, middleware, 실행 제한은 외부에서 주입한다. Runtime 엔진에 특정 도메인의
system prompt나 고정 Tool 구성을 넣지 않으며, 실제 CLI용 구성은 `cmd/agent-runtime` 진입점에서 조립한다.

CLI는 기존 config의 provider, model, timeout과 Tool별 제한을 따라야 한다. Phase 4.2를 이유로 Code Execution Tool의
허용 범위나 파일 Tool의 root 제한을 넓히지 않는다.

## 제외 범위

Provider-neutral streaming LLM contract, Runner streaming event, CLI streaming 출력, streaming 완료 후 final response
조립은 Phase 4.3에서 다룬다.

Provider별 native structured output 또는 constrained decoding 연동, structured output 실패 시 model 자동 재호출이나
응답 복구는 포함하지 않는다.

Tool 실행 전후 middleware, Agent run 전체를 감싸는 middleware, middleware 동시 실행은 포함하지 않는다. Phase 4.2의
middleware는 model 요청과 응답 경계에 한정한다.

새 system prompt template, Tool routing 또는 ranking 정책, 대화형 다중 입력 CLI, HTTP API, Agent Server는 포함하지
않는다.

RAG, Memory, Multi-Agent, MCP, A2A 구현은 포함하지 않는다.

## 완료 조건

1. 호출자는 LLM client, model, 실행 제한, Tool registry를 주입한 Single Agent Runner로 사용자 입력 하나를 실행하고,
   최종 Agent 상태와 응답을 확인할 수 있다.
2. Runner는 등록된 Tool의 schema를 model 요청에 제공하고, model이 요청한 Tool을 기존 Agent loop로 실행한 뒤 Tool
   result를 다음 model 판단에 전달해 final 상태까지 진행할 수 있다.
3. 등록된 `pre-model` middleware는 Tool 사용으로 반복되는 호출을 포함한 모든 model 요청 전에 등록 순서대로
   실행되며, 변경한 요청이 실제 LLM client 호출에서 확인된다.
4. 등록된 `post-model` middleware는 모든 model 응답 후 등록 순서대로 실행되며, 변경한 응답이 Agent의 다음 판단과
   최종 결과에 반영된다.
5. `pre-model` 또는 `post-model` middleware가 오류를 반환하면 Runner 결과에서 middleware 실패를 구분할 수 있고,
   오류 이후 추가 model 또는 Tool 호출은 발생하지 않는다.
6. JSON Schema를 지정한 run에서 최종 assistant 응답이 유효한 JSON이고 schema를 만족하면, 호출자는 검증된 JSON
   결과를 확인할 수 있다.
7. JSON Schema를 지정한 run에서 최종 assistant 응답의 JSON 파싱이 실패하거나 schema와 일치하지 않으면, 호출자는
   structured output 오류를 일반 LLM 호출 오류와 구분해 확인할 수 있다.
8. JSON Schema를 지정하지 않은 run은 기존 일반 text final answer를 반환하며, structured output 기능 없이도 Tool
   loop와 final 상태가 정상 동작한다.
9. CLI에 Tool call을 반환하는 LLM client를 연결하면 CLI는 진입점에서 등록한 Phase 3·4.1 Tool을 Agent loop 안에서
   실행하고 최종 assistant 응답을 stdout에 출력할 수 있다.
10. CLI의 Runner 실행이 final 결과를 만들지 못하면 사용자는 stderr 메시지와 0이 아닌 종료 코드로 실패를 확인할 수
    있다.
11. 테스트는 실제 외부 provider 호출 없이 stub LLM client와 테스트 middleware를 사용해 middleware 순서와 변경
    반영, middleware 오류, Tool loop, structured output 성공·실패, CLI 최종 출력을 확인할 수 있다.
