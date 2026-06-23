# phase-2-agent-react

ROADMAP Phase 2(Agent State와 ReAct 기초)의 요구사항 명세다. Agent가 단발성 LLM 호출이 아니라
상태를 유지하며 반복적으로 판단하는 구조임을 코드로 표현하기 위한 기본 추상화를 정의한다.

## 1. 범위

- Agent 실행 단위와 그 상태를 다루는 추상화 계층 (`internal/agent`).
- `AgentState`: 대화 메시지의 누적, 진행 step 수, 그리고 종료 상태(최종 답 도달 / max step 초과 /
  에러)를 표현하는 실행 상태.
- `Agent`: Phase 1의 `LLMClient`를 주입받아 `AgentState` 위에서 ReAct loop를 실행하는 단위.
- ReAct loop: state를 입력으로 LLM을 반복 호출하고, 응답을 state에 누적하며, 매 반복마다 종료 조건을
  판정하는 실행 루프.
- step counter와 max step: 한 번의 실행에서 진행한 step 수를 세고, 상한을 넘기지 않도록 강제하는 장치.
- final answer detection: tool call이 없는 assistant 응답을 최종 답으로 판정해 loop를 종료하는 규칙.
- error state: LLM 호출이 실패했을 때 loop를 안전하게 종료시키는 종료 상태.
- reflection hook: step 경계에서 진행 상황을 관찰하거나 개입할 수 있는 확장 지점.
- CLI에서 사용자 입력을 단발 Chat 호출이 아니라 Agent loop를 통해 처리해 최종 응답을 출력하는 경로.

## 2. 목표

- Agent가 LLM 1회 호출로 끝나는 것이 아니라, 상태를 유지하며 반복 판단하는 구조임을 코드로 드러낸다.
- LLM 응답을 보고 다음 행동(계속할지 / 종료할지)을 결정하는 주체가 LLM이 아니라 Runtime임을 분명히 한다.
- 무한 루프를 막기 위해 step 상한을 강제하고, 상한 초과·호출 실패가 안전한 종료로 귀결되게 한다.
- Phase 3(Tool Calling)·Phase 4(Graph)가 그 위에 올라설 수 있도록, 상태 누적과 반복 실행의 골격을
  먼저 세운다.

## 3. 제약

- LLM 호출은 Phase 1의 `LLMClient` interface와 `internal/message` 타입을 재사용하며, Agent는 특정
  provider 구현체에 직접 의존하지 않는다.
- LangChain / LangGraph 계열 라이브러리는 사용하지 않는다.
- 모든 Agent 실행은 `context.Context`를 받아 취소·timeout을 LLM 호출까지 전파할 수 있어야 한다.
- max step은 반드시 강제된다 — 어떤 LLM 응답 패턴에서도 loop가 무한히 돌지 않는다.
- ROADMAP 중단 기준에 따라, 최소 하나의 실패 케이스(예: max step 초과로 최종 답에 도달하지 못함)가
  관찰 가능해야 한다.
- stub client만으로 정상 종료·실패 종료 경로를 실제 API 호출 없이 결정적으로 검증할 수 있어야 한다.

## 4. 제외 범위

- Tool의 실제 등록·실행·검증 런타임 (Phase 3). 본 Phase의 loop는 종료 조건 판정과 상태 누적까지만
  다루며, tool call에 대응하는 tool 실행은 수행하지 않는다.
- tool 실행 결과를 다시 LLM 입력으로 넣어 이어가는 실질적 multi-step ReAct (tool 실행이 Phase 3에서
  붙은 뒤에 성립한다).
- Graph State / Node / Edge 기반 실행 구조 (Phase 4).
- streaming 응답, structured output 파싱 (Phase 5).
- 메시지 trimming·요약·세션 메모리 등 장기 상태 관리 (Phase 8).
- 토큰 사용량·latency 등 trace 수집 구조의 정식화 (이후 Phase에서 정리).

## 5. 완료 조건

1. 사용자 입력이 `AgentState`의 초기 대화 메시지로 저장되며, 실행 시작 시점의 state에서 그 입력을
   확인할 수 있다.
2. ReAct loop가 매 step에서 `LLMClient`를 호출하고, 받은 assistant 응답을 `AgentState`의 대화에
   누적한다 — step이 진행될수록 state에 쌓인 메시지가 늘어난다.
3. tool call이 없는 assistant 응답을 최종 답으로 판정해 loop를 종료하고, 그 최종 답을 호출자가 얻을 수
   있다.
4. step counter가 매 step 증가하며, max step에 도달하면 LLM을 더 호출하지 않고 종료한다. 이때의 종료가
   최종 답 도달이 아니라 max step 초과임을 호출자가 구분할 수 있다.
5. LLM 호출이 에러를 반환하면 loop가 error 종료 상태로 안전하게 멈추고, 그 사실과 원인을 호출자가
   확인할 수 있다.
6. step 경계에서 reflection hook이 호출되어, 현재 step과 누적된 state를 관찰할 수 있다.
7. CLI에서 사용자 입력이 단발 Chat 호출이 아니라 Agent loop를 통해 처리되어 최종 응답이 stdout으로
   출력된다.
8. stub client를 사용한 테스트가 실제 API 호출 없이 결정적으로 통과하며, 최소한 정상 종료(최종 답 도달)와
   실패 종료(max step 초과) 두 경로를 각각 검증한다.
