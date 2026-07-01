# Phase 2 Agent Loop 명세

## 범위

Phase 2는 Agent Runtime이 단발 LLM 호출을 넘어서, 메시지 상태를 유지하며 반복적으로 LLM 판단을 진행하는 기본
Agent loop를 만든다. 대상 범위는 새 `internal/agent` 패키지와 기존 `internal/message`, `internal/llm` contract의
사용 경계다.

이 단계의 실행 단위는 사용자 입력 하나에서 시작하는 Agent run이다. Runtime은 사용자 메시지를 Agent 상태에
저장하고, 설정된 `LLMClient`를 호출해 assistant 응답을 상태에 누적한다. assistant 응답에 tool call이 없으면 final
answer로 종료하고, tool call이 있으면 Phase 2에서는 tool을 실행하지 않고 추가 행동이 필요하다는 상태로 멈춘다.

Agent 상태는 메시지 목록, 현재 step, 종료 상태, 마지막 오류, trace를 포함한다. Trace는 각 step에서 Runtime이
어떤 action을 수행했고 어떤 결과나 오류가 관찰됐는지 메모리 안에서 확인할 수 있는 구조로 둔다. 파일 저장,
로그 출력, JSON export 같은 외부 trace 출력 형식은 이 단계에서 고정하지 않는다.

## 목표

LLM 호출 한 번으로 끝나는 Phase 1 실행 경로와 달리, Agent가 상태를 들고 반복 판단하는 구조를 코드로 표현한다.
이후 Tool Calling Runtime이 붙을 수 있도록, LLM 응답의 tool call을 final answer와 구분하고 안전하게 멈추는 상태를
제공한다.

Agent loop는 최대 step 수를 기준으로 무한 반복을 막을 수 있어야 한다. 각 step의 LLM 요청과 응답은 메시지 상태에
누적되어야 하며, 실패나 종료 이유는 호출자가 테스트와 런타임 조립 코드에서 확인할 수 있어야 한다.

## 제약

Agent loop는 기존 `internal/llm.LLMClient` contract를 사용하고, Claude나 Ollama 같은 provider 구현에 직접
의존하지 않는다. Provider별 요청 변환, timeout 분류, 설정 검증은 Phase 1의 `internal/llm` 경계에 둔다.

Phase 2는 Agent loop의 내부 상태와 실행 결과를 테스트 가능한 API로 제공하는 것을 우선한다. 기존
`cmd/agent-runtime`의 단발 CLI 실행 contract는 이 단계에서 바꾸지 않는다.

Final answer 감지는 Phase 2 기준으로 단순하게 정의한다. Assistant 응답에 tool call이 없으면 final answer이며,
tool call이 있으면 tool 실행을 기다리는 상태로 종료한다. 더 정교한 final answer marker, structured output,
streaming 기반 종료 판단은 이후 단계에서 다룬다.

Trace는 이후 Phase에서 필드를 확장할 수 있도록 단일 구조로 둔다. 다만 Phase 2에서는 trace의 외부 저장 형식이나
공개 로그 형식을 요구사항으로 고정하지 않는다.

## 제외 범위

Tool registry, tool schema 검증, tool 실행, unknown tool 처리, tool result를 다시 LLM 입력으로 전달하는 흐름은
포함하지 않는다.

기존 CLI를 Agent loop 기반 실행 경로로 전환하는 작업은 포함하지 않는다. Phase 2가 끝난 뒤에도
`cmd/agent-runtime`은 Phase 1의 단발 LLM 호출 경로를 유지할 수 있다.

Streaming 응답, middleware hook, structured output, system prompt template, memory trimming, RAG, Multi-Agent,
MCP, A2A 구현은 포함하지 않는다.

Trace를 파일, JSON, stdout, stderr, 외부 observability 시스템으로 내보내는 기능은 포함하지 않는다.

## 완료 조건

1. 새 Agent 실행 API는 사용자 입력을 받아 초기 user message를 `AgentState`에 저장하고, LLM 호출에 같은 메시지
   상태를 전달한다.
2. LLM assistant 응답은 `AgentState`의 메시지 목록에 순서대로 누적되며, 호출자는 최종 상태에서 누적 메시지를
   확인할 수 있다.
3. Assistant 응답에 tool call이 없으면 Agent run은 final 상태로 종료되고, final answer text를 호출자가 확인할 수
   있다.
4. Assistant 응답에 tool call이 있으면 Agent run은 tool 실행 없이 추가 행동이 필요한 상태로 종료되고, tool call
   정보가 상태와 메시지에 보존된다.
5. Agent run은 설정된 max step을 넘기지 않고 종료하며, max step 초과 상황은 별도 상태나 오류로 호출자가 구분할 수
   있다.
6. LLM 호출 오류는 Agent 상태의 error 경로로 반영되고, 호출자는 실패 상태와 원인 오류를 확인할 수 있다.
7. 각 step의 주요 action과 결과는 메모리 안의 trace 구조에 기록되며, 호출자는 테스트에서 step 순서와 종료 이유를
   확인할 수 있다.
8. 테스트는 실제 외부 provider 호출 없이 stub `LLMClient`로 정상 종료, tool call 대기, max step 종료, LLM 오류,
   trace 기록을 확인할 수 있다.
