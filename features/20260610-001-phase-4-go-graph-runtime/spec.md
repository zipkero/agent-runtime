# phase-4-go-graph-runtime 명세

## 범위

- Graph 실행 런타임 계층 (`internal/graph`): 상태를 가진 실행 단위가 node를 거치며 다음 node로 이동하는
  구조를 제공한다.
- `GraphState`: graph 실행 중 node 사이를 이동하며 누적·갱신되는 상태를 표현하는 단위.
- `Node` 추상화: 현재 state를 입력으로 받아 실행 결과와 state 변경을 반환하는 실행 단위.
- `Edge`: 한 node의 실행 이후 다음 node 후보를 표현하는 연결 단위.
- `Router`: 현재 node와 state를 기준으로 다음 node를 결정하는 단위.
- `ConditionalRouter`: 조건에 따라 다음 node 또는 종료를 선택하는 routing 단위.
- `Reducer`: node 실행 결과를 기존 state에 반영하는 규칙.
- `Graph`: 시작 node, 종료 조건, node 집합, edge/routing 규칙을 가진 실행 가능한 graph 단위.
- graph execution loop: graph가 max steps, node 실행, state 갱신, 다음 node 선택, 종료 판정을 일관되게
  수행하는 경로.
- node error handling: node 실행 중 발생한 에러를 panic이 아니라 graph 실행 결과로 관찰할 수 있게 하는
  규칙.
- 기존 Tool Calling Agent 흐름: `llm_node → tool_node → llm_node → end` 구조로 표현 가능한 실행 경로.

## 목표

- Agent 실행 흐름을 hard-coded `for`/`if` loop가 아니라 State, Node, Edge, Conditional Edge로 표현할 수
  있게 한다.
- 정적 workflow와 조건부 agent loop를 같은 graph 실행 엔진으로 다룰 수 있게 한다.
- Tool Calling Agent의 기존 동작을 유지하면서, tool call 여부에 따라 tool 실행 node 또는 종료로 분기하는
  구조를 코드로 분명히 한다.
- max steps, node error, 종료 판정 같은 실행 제어 책임을 graph runtime에서 일관되게 다룬다.
- 이후 RAG, Memory, Multi-Agent, Orchestrator 단계가 node와 edge 조합으로 실행 흐름을 확장할 수 있는
  기반을 마련한다.

## 제약

- LangChain / LangGraph 계열 라이브러리는 사용하지 않는다.
- graph runtime은 특정 LLM provider 구현체에 직접 의존하지 않는다.
- graph runtime은 `context.Context`를 받아 node 실행과 graph 실행 취소를 전파할 수 있어야 한다.
- max steps는 graph 실행 경로에서 반드시 강제되어야 하며, 어떤 routing 결과에서도 무한 loop가 발생하지
  않아야 한다.
- node 실행 에러는 panic으로 전파하지 않고, 호출자가 구분 가능한 graph 실행 결과로 표현되어야 한다.
- 기존 `internal/message`, `internal/llm`, `internal/tool`의 공개 역할은 graph 도입만으로 불필요하게
  변경하지 않는다.
- 기존 CLI의 사용자 관찰 동작은 유지한다. 최종 답은 stdout, max step 초과와 실행 에러는 stderr와
  비정상 종료코드로 관찰 가능해야 한다.
- stub node, stub LLM client, 등록된 local tool만으로 graph 실행 경로를 실제 API 호출 없이 결정적으로
  검증할 수 있어야 한다.

## 제외 범위

- Web Search Tool, File Save Tool, Code Execution Tool 등 새로운 tool 추가와 보안 제한 (Phase 5).
- middleware hook, structured output, streaming 응답 (Phase 5).
- RAG indexing·retrieval node의 실제 구현 (Phase 6).
- Multi-Agent supervisor/worker 구조의 실제 구현 (Phase 7).
- Memory Runtime의 장기 상태 저장·요약·검색 정책 (Phase 8).
- MCP, A2A 등 외부 protocol adapter 연동 (Phase 9, Phase 10).
- trace 저장소, metric, token usage, latency 수집 구조의 정식화. 단, graph 실행 결과에서 현재 node와 step을
  관찰할 수 있는 최소 정보는 Phase 4 범위에 포함한다.
- graph 정의를 외부 YAML/JSON 파일로 로드하는 구성 시스템.
- graph node의 병렬 실행, fan-out/fan-in, retry policy, rollback 같은 고급 workflow 기능.

## 완료 조건

1. 호출자가 시작 node와 node 집합을 가진 graph를 생성하고 실행할 수 있으며, 실행 결과에서 최종 state와
   종료 상태를 확인할 수 있다.
2. graph 실행 중 node가 반환한 변경 사항이 reducer 규칙에 따라 state에 반영되고, 다음 node가 갱신된
   state를 관찰할 수 있다.
3. 정적 edge로 연결된 graph가 지정된 node 순서대로 실행되며, 마지막 node 이후 정상 종료 상태를 반환한다.
4. conditional router가 state를 기준으로 서로 다른 다음 node 또는 종료를 선택하고, 호출자가 선택 결과를
   실행 결과로 확인할 수 있다.
5. graph max steps에 도달하면 다음 node를 더 실행하지 않고 max steps 종료 상태를 반환한다.
6. node 실행이 error를 반환하면 graph 실행이 중단되고, 호출자가 error 종료 상태와 원인 error를 확인할 수
   있다.
7. context가 취소되거나 deadline을 넘기면 graph 실행이 중단되고, 호출자가 취소 원인을 확인할 수 있다.
8. Tool Calling Agent 흐름에서 assistant 응답에 tool call이 있으면 graph가 tool 실행 node로 이동하고,
   tool result가 state에 누적된 뒤 LLM node로 돌아간다.
9. Tool Calling Agent 흐름에서 assistant 응답에 tool call이 없으면 graph가 종료 상태로 끝나고, 호출자가
   최종 assistant 메시지를 확인할 수 있다.
10. 기존 CLI 경로에서 tool calling을 거쳐 최종 답에 도달하는 사용자 관찰 동작이 graph 도입 전과 동일하게
    유지된다.
11. stub node 또는 stub LLM client를 사용한 테스트가 실제 API 호출 없이 정적 edge, conditional router,
    max steps, node error, tool call 분기, 최종 종료 경로를 결정적으로 검증한다.
