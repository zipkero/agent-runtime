# phase-4-go-graph-runtime 구현

## 체크리스트

- [x] task-001: generic graph core와 정적 edge 실행
  - 목적: 호출자가 시작 node와 node 집합을 가진 graph를 실행하고, 정적 edge 순서대로 변경된 최종 state와
    종료 상태를 확인할 수 있게 한다.
  - 접근: `internal/graph`에 generic `Graph[S]`, `Node[S]`, `NodeResult[S]`, `Reducer[S]`, `Router[S]`,
    `Result[S]`, `NodeID`를 추가하고, 기본 replace reducer와 정적 edge router를 제공한다.
  - 검증 조건:
    - 결과: 두 개 이상의 stub node가 정적 edge 순서대로 실행되고, 각 node가 반환한 state 변경이 reducer를
      거쳐 다음 node와 최종 result에 반영된다.
    - 확인: `internal/graph` 단위 테스트에서 실행 순서, 최종 state, result status, step 수를 단언하고
      `go test ./internal/graph`가 통과한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.11, ANALYSIS §1, ANALYSIS §3, ANALYSIS D1, ANALYSIS D2

- [x] task-002: conditional router 실행
  - 목적: graph가 reducer 반영 이후의 최신 state를 기준으로 다음 node 또는 종료를 선택할 수 있게 한다.
  - 접근: `internal/graph`에 `Router[S]` 공통 표면을 따르는 conditional router adapter를 추가하고, 정적
    edge router와 같은 graph execution loop에서 사용한다.
  - 검증 조건:
    - 결과: 같은 graph가 초기 state 값에 따라 서로 다른 다음 node로 이동하거나 즉시 종료하며, 호출자가
      result의 최종 state와 종료 상태로 선택 결과를 확인할 수 있다.
    - 확인: `internal/graph` 단위 테스트에서 true/false 조건의 서로 다른 경로와 end 선택 경로를 단언하고
      `go test ./internal/graph`가 통과한다.
  - 참조: SPEC §5.4, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3

- [ ] task-003: graph max steps, node error, context 취소 처리
  - 목적: graph 실행이 무한 loop, node error, context 취소 상황에서 panic 없이 구분 가능한 결과로 멈추게
    한다.
  - 접근: graph execution loop에 node 실행 횟수 기준 max steps 선검사, node error result, context
    cancellation/deadline result를 추가하고, graph status를 agent status와 분리해 유지한다.
  - 검증 조건:
    - 결과: max steps 도달 시 다음 node를 실행하지 않고 max steps status를 반환하며, node error와 context
      취소는 원인 error가 담긴 error/canceled status로 반환된다.
    - 확인: `internal/graph` 단위 테스트에서 max steps 초과 호출 방지, sentinel error 보존,
      canceled/deadline context 결과를 단언하고 `go test ./internal/graph`가 통과한다.
  - 참조: SPEC §5.5, SPEC §5.6, SPEC §5.7, SPEC §5.11, ANALYSIS §2, ANALYSIS §3, ANALYSIS D3, ANALYSIS D6

- [ ] task-004: Tool Calling Agent를 graph adapter로 재배치
  - 목적: 기존 `Agent.Run` 호출자가 API 변경 없이 tool call 분기, tool result 누적, 최종 답 종료를 같은
    방식으로 관찰하게 한다.
  - 접근: `internal/agent`에서 `Graph[AgentState]`를 구성해 `llm_node`, `tool_node`,
    `route_after_llm`로 기존 loop를 표현하고, graph result를 기존 `AgentState`와 `Status`로 변환한다.
  - 검증 조건:
    - 결과: assistant 응답에 tool call이 있으면 tool node가 실행되어 RoleTool 메시지가 누적된 뒤 LLM node로
      돌아가고, tool call이 없으면 최종 assistant 메시지로 종료된다.
    - 확인: 기존 `internal/agent` 테스트가 통과하고, tool call 분기 테스트에서 LLM 호출 횟수, RoleTool
      메시지, final message, `AgentState.Steps`가 기존 기대값과 일치한다.
  - 참조: SPEC §5.8, SPEC §5.9, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §4,
    ANALYSIS D3, ANALYSIS D4, ANALYSIS D5, ANALYSIS D6

- [ ] task-005: CLI 관찰 동작 회귀 확인
  - 목적: graph 도입 후에도 CLI 사용자는 기존과 같이 최종 답은 stdout으로, max step과 실행 에러는 stderr와
    비정상 종료코드로 확인할 수 있게 한다.
  - 접근: `cmd/agent-runtime`의 `run` 상태 분기 계약을 유지하고, graph 기반 `Agent.Run` 결과가 기존
    `agent.StatusFinal`, `agent.StatusMaxSteps`, `agent.StatusError`로 관찰되는지 확인한다.
  - 검증 조건:
    - 결과: tool calling을 거쳐 최종 답에 도달하는 경로, max step 경로, LLM/context error 경로의 stdout,
      stderr, exit code가 graph 도입 전 계약과 동일하다.
    - 확인: 기존 `cmd/agent-runtime` 테스트가 통과하고, 전체 회귀 확인으로 `go test ./...`를 실행한다.
  - 참조: SPEC §5.10, SPEC §5.11, ANALYSIS §1, ANALYSIS §2, ANALYSIS §4, ANALYSIS D4, ANALYSIS D6
