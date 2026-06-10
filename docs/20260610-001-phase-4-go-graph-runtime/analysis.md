# phase-4-go-graph-runtime 분석

## 근거

읽은 기준 문서:

- `docs/20260610-001-phase-4-go-graph-runtime/spec.md` 전체. 본 분석의 범위 상한은 이 spec이며,
  Phase 5 이후의 tool 추가, middleware, RAG, Multi-Agent, MCP/A2A는 spec §4 제외 범위에 따라 다루지
  않는다.
- `ROADMAP.md` Phase 4 섹션. State / Node / Edge / Conditional Edge를 Go로 직접 구현하고,
  Tool Calling Agent를 `llm_node → tool_node → llm_node → end` 구조로 표현한다는 목표를 확인했다.

코드베이스에서 확인한 사실:

- `internal/graph` 패키지는 아직 없다. Phase 4의 신규 graph 실행 엔진은 새 패키지로 추가되어야 한다.
- `internal/agent/agent.go`는 `AgentState`, `Status`, `ReflectionHook`, `Agent`, `NewAgent`, `Run`을
  가진다. `Run`은 user 메시지로 state를 초기화하고, `for` loop 안에서 hook 호출, max step 선검사,
  LLM 호출, assistant 메시지 누적, tool call 여부 분기, tool 실행 결과 누적을 직접 수행한다.
- 현재 `AgentState.Steps`는 LLM 호출 성공 후 증가한다. tool 실행은 step을 증가시키지 않는다.
- `AgentState.Status`는 `running`, `final`, `max_steps`, `error` 네 값이다. CLI는 이 값을 기준으로 stdout,
  stderr, exit code를 분기한다.
- `internal/tool`에는 이미 `Tool`, `Registry`, `Dispatcher`가 있다. `Dispatcher.Dispatch`는 unknown tool,
  검증 실패, 실행 에러, timeout을 모두 `message.ToolResult{IsError:true}`로 정규화하고 error를 반환하지
  않는다.
- `internal/llm.LLMClient`는 `Chat(ctx, ChatRequest) (ChatResponse, error)`만 요구한다.
  `ChatRequest`는 `Tools []message.ToolSpec`를 이미 가진다.
- `cmd/agent-runtime/main.go`는 registry를 생성해 calculator와 file read tool을 등록하고,
  `agent.NewAgent`에 `maxSteps`, registry, tool timeout을 넘긴다. CLI의 사용자 관찰 계약은
  final이면 stdout, max step 또는 error이면 stderr와 exit code 1이다.
- `internal/agent/agent_test.go`는 정상 종료, max steps, LLM error, hook 호출, tool 실행·누적,
  tool schema 전달, nil registry, IsError tool result 흡수를 검증한다.

추정으로 분리:

- Graph Runtime의 구체 타입 이름과 메서드 시그니처는 아직 코드에 없다. 아래 인터페이스는 spec §5를
  만족하기 위한 설계 기준이며, 최종 세부 타입명은 implement.md에서 Task로 고정할 수 있다.
- 이후 Phase의 RAG, Memory, Multi-Agent node는 아직 없으므로 이 분석에서는 실제 구현을 전제하지 않는다.
  다만 graph core가 특정 agent/tool 구현에 묶이지 않도록 경계를 둔다.

## 1. 구조

Phase 4는 `internal/graph`와 `internal/agent`의 책임을 분리한다. `internal/graph`는 순수 graph 실행
엔진이고, `internal/agent`는 기존 Tool Calling Agent를 graph 위에 올리는 adapter 역할을 맡는다.

`internal/graph`의 책임:

- node 이름, node 집합, 시작 node, edge/router, reducer, max steps를 가진 graph를 실행한다.
- graph 실행 상태(`running`, `completed`, `max_steps`, `error`, `canceled`)와 최종 state를 결과로 돌려준다.
  이는 spec §5.1, §5.5, §5.6, §5.7을 만족하기 위한 graph 수준의 관찰 표면이다.
- `Node` 실행, `Reducer` 적용, `Router`를 통한 다음 node 선택, 종료 판정을 한 loop에서 일관되게 처리한다.
  node가 반환한 변경 사항은 reducer를 통해 state에 반영되고 다음 node가 그 state를 본다(SPEC §5.2).
- 정적 edge와 conditional router를 같은 routing 표면으로 다룬다. 정적 workflow는 고정 mapping router,
  agent loop는 state 기반 conditional router로 표현한다(SPEC §5.3, §5.4).
- `context.Context` 취소와 node error를 graph 결과로 흡수한다. graph core는 panic 기반 흐름 제어를 쓰지
  않는다(SPEC §5.6, §5.7).

`internal/graph`가 알지 않아야 하는 것:

- `llm.LLMClient`, `message.Message`, `tool.Registry`, Claude/GPT provider, calculator/file read 같은 agent
  도메인 타입.
- tool 실패 정규화 정책. unknown tool과 tool 실행 실패는 이미 `internal/tool.Dispatcher`가
  `ToolResult.IsError`로 정규화하므로 graph error로 취급하지 않는다.
- CLI 출력 정책. graph는 결과를 반환하고, CLI stdout/stderr/exit code 분기는 기존 `cmd/agent-runtime`과
  `internal/agent` 경계에 남긴다.

`internal/agent`의 책임:

- 기존 `AgentState`와 `Status`를 public-facing 상태로 유지한다. graph core의 상태와 agent의 상태를
  같은 타입으로 합치지 않는다. CLI와 테스트가 관찰하는 계약을 보존하기 위해서다(SPEC §5.10).
- `llm_node`와 `tool_node`를 구성한다. `llm_node`는 hook 호출, agent max step 선검사, LLM 호출,
  assistant 메시지 누적, final 판정을 담당한다. `tool_node`는 마지막 assistant의 tool call을
  `tool.Dispatcher`로 실행하고 RoleTool 메시지를 누적한다(SPEC §5.8, §5.9).
- graph 결과를 기존 `AgentState`로 변환해 `Agent.Run(ctx, prompt) AgentState` 시그니처를 유지한다.
  graph node error 또는 context 취소는 기존처럼 `AgentState{Status: StatusError, Err: err}`로 관찰되게 한다.

이 구조에서는 Phase 4가 `Agent.Run`의 동작을 바꾸는 것이 아니라, 현재 loop를 graph 실행 모델로
재배치한다. 새 기능 tool이나 새 provider를 추가하지 않는다.

## 2. 데이터 흐름

Graph core의 일반 실행 흐름:

```text
initial state
→ current = start node
→ max step / context 확인
→ node 실행
→ reducer로 state 갱신
→ router가 다음 node 선택
→ next가 end이면 completed
→ 아니면 current = next로 반복
```

정상 경로에서 graph의 step counter는 node 실행 횟수를 센다. 이는 graph runtime의 무한 loop 방지 장치다.
Agent의 `AgentState.Steps`는 기존처럼 LLM 호출 횟수를 센다. 두 counter를 분리해야 기존 max step 의미와
graph 안전장치를 동시에 보존할 수 있다(SPEC §5.5, §5.10).

정적 edge workflow:

```text
start_node
→ first_node
→ second_node
→ end
```

이 경로에서는 router가 현재 node 이름으로 다음 node를 고정 lookup한다. 마지막 node의 다음 값이 end이면
graph는 `completed` 상태와 최종 state를 반환한다(SPEC §5.3).

조건부 routing:

```text
node_a
→ conditional router(state)
    → state.flag == true: node_b
    → state.flag == false: end
```

conditional router는 reducer가 반영한 최신 state를 기준으로 다음 node를 고른다. 호출자는 graph 결과의
최종 state, 종료 상태, 마지막 node 또는 step 정보를 통해 선택 결과를 검증할 수 있다(SPEC §5.4).

Tool Calling Agent graph:

```text
initial AgentState
→ llm_node
→ route_after_llm
    → StatusFinal: end
    → StatusMaxSteps: end
    → StatusError: end
    → 마지막 assistant에 tool_call 있음: tool_node
→ tool_node
→ llm_node
```

`llm_node`의 흐름:

1. 현재 `AgentState.Steps`와 state를 `ReflectionHook`에 전달한다. nil이면 no-op이다.
2. `AgentState.Steps >= maxSteps`이면 LLM을 호출하지 않고 `StatusMaxSteps`를 설정한다.
3. registry가 있으면 `ChatRequest.Tools`에 registry의 specs를 싣고 `LLMClient.Chat`을 호출한다.
4. 호출 에러 또는 context 취소는 node error로 반환한다. `Agent.Run`은 graph error 결과를 기존
   `StatusError`와 `Err`로 변환한다.
5. 성공하면 assistant 메시지를 누적하고 `AgentState.Steps`를 1 증가시킨다.
6. assistant 메시지에 tool call이 없으면 `StatusFinal`로 바꾼다. tool call이 있으면 `StatusRunning`을
   유지한다.

`tool_node`의 흐름:

1. 마지막 assistant 메시지에서 tool call 블록만 순서대로 찾는다.
2. 각 tool call을 기존 `tool.Dispatcher.Dispatch`에 넘긴다.
3. 반환된 `ToolResult`들을 `message.NewToolResultBlock`으로 감싸 하나의 RoleTool 메시지에 누적한다.
4. tool 실패는 `ToolResult.IsError`로 state에 누적되며 graph error나 agent `StatusError`가 되지 않는다.
5. router는 항상 `llm_node`로 돌아간다.

이 흐름은 현재 `Agent.Run`의 사용자 관찰 동작을 유지한다. tool call이 있으면 tool result가 state에
누적된 뒤 다음 LLM 호출로 이어지고(SPEC §5.8), tool call이 없으면 최종 assistant 메시지를 얻는다
(SPEC §5.9). CLI는 기존처럼 최종 답을 stdout에 출력하고, max step과 LLM/context error는 stderr로
출력한다(SPEC §5.10).

실패 경로:

- graph max steps: graph node 실행 횟수가 graph max에 도달하면 다음 node를 실행하지 않고
  `max_steps` 결과를 반환한다(SPEC §5.5). Agent adapter는 이를 기존 `StatusMaxSteps`로 매핑한다.
  기존 agent max step은 `llm_node` 안에서 LLM 호출 상한으로 별도 보존한다.
- node error: node가 error를 반환하면 graph는 중단되고 `error` 결과와 원인 error를 반환한다(SPEC §5.6).
- context 취소: node 실행 전 또는 node가 반환한 error가 context 취소/timeout이면 graph는 취소 원인을
  결과에 담는다(SPEC §5.7). Agent public API에서는 기존과 같이 `StatusError`와 `Err`로 관찰된다.
- tool 실행 실패: `tool.Dispatcher`가 `ToolResult.IsError`로 정규화하므로 graph 실행 실패가 아니다.

## 3. 인터페이스

경계를 가로지르는 표면만 정의한다. 세부 타입명은 implement.md에서 조정할 수 있지만, 책임은 아래 기준을
따른다.

Graph core:

```go
type NodeID string

type Status string

type Result[S any] struct {
    State   S
    Status  Status
    Steps   int
    Current NodeID
    Err     error
}

type Node[S any] interface {
    Run(ctx context.Context, state S) (NodeResult[S], error)
}

type NodeResult[S any] struct {
    Update S
}

type Reducer[S any] interface {
    Reduce(current S, result NodeResult[S]) (S, error)
}

type Router[S any] interface {
    Next(current NodeID, state S) (NodeID, error)
}
```

`S`는 graph state의 구체 타입이다. `internal/graph`는 `map[string]any` 같은 약한 공용 state를 강제하지
않고, 호출자가 가진 타입을 그대로 graph state로 사용한다. `internal/agent`는 `agent.AgentState`를
`S`로 넘긴다. 이렇게 해야 기존 `AgentState` 계약을 유지하면서 graph core가 agent 도메인을 import하지
않는다(SPEC §5.1, §5.10).

`Reducer`는 node 결과를 현재 state에 반영하는 유일한 위치다. 기본 구현은 replace reducer로 충분하다.
테스트용 workflow나 Agent adapter가 node에서 갱신된 state 전체를 반환하면 replace reducer가 이를 최종
state로 삼는다. 이후 더 세밀한 patch merge가 필요해지면 reducer 구현만 바꿀 수 있다(SPEC §5.2).

`Router`는 정적 edge와 conditional router의 공통 표면이다. 정적 edge는 `map[NodeID]NodeID` 기반 router로,
conditional router는 `func(current NodeID, state S) (NodeID, error)` 형태의 adapter로 표현할 수 있다.
end는 예약된 `NodeID` 값 또는 별도 sentinel로 표현하되, graph core 밖으로 명확히 노출되어야 한다
(SPEC §5.3, §5.4).

Graph 실행 표면:

```go
type Graph[S any] struct { ... }

func (g *Graph[S]) Run(ctx context.Context, initial S) Result[S]
```

`Run`은 error를 두 번째 반환값으로 던지지 않고 `Result`에 흡수한다. 현재 `Agent.Run`이 error를
`AgentState`에 흡수하는 방식과 같은 사용성을 유지하기 위해서다. 단 graph 구성 오류처럼 실행 전에 잡히는
문제는 graph 생성 또는 node 등록 단계에서 error로 돌려 호출자가 즉시 확인하게 한다.

Agent adapter:

- `Agent.Run(ctx, prompt) AgentState`는 유지한다. 호출자는 graph 도입 여부를 몰라도 된다.
- `Agent`는 내부에서 `Graph[AgentState]`를 구성하거나 보관한다. `llm_node`는 `llm.LLMClient`,
  model, registry, hook, maxSteps를 캡처한다.
- `tool_node`는 기존 `tool.Dispatcher`를 사용한다. graph core가 `internal/tool`을 import하지 않도록
  node 구현은 `internal/agent`에 둔다.

## 4. 영향 범위

신규:

- `internal/graph` 패키지와 테스트. graph core, 정적 router, conditional router, reducer, result/status,
  max steps, context/error 처리를 검증한다(SPEC §5.1~§5.7, §5.11).

변경:

- `internal/agent/agent.go`: 현재 `for` loop를 graph 구성과 실행으로 재배치한다. public-facing
  `AgentState`, `Status`, `FinalMessage`, `ReflectionHook`, `NewAgent`, `Run`의 관찰 계약은 유지한다.
  변경 핵심은 LLM 호출 node, tool 실행 node, route_after_llm 조건부 router 구성이다(SPEC §5.8~§5.10).
- `internal/agent/agent_test.go`: 기존 테스트가 graph 도입 후에도 같은 결과를 관찰해야 한다. 특히 hook
  호출 시점, `AgentState.Steps`, nil registry, IsError tool result 흡수 테스트가 회귀 방지 기준이다.
- `cmd/agent-runtime/main.go`: CLI 분기 자체는 유지한다. 필요하다면 `Agent.Run` 내부가 graph 기반이 되어도
  `run`의 상태 처리 코드는 그대로 둘 수 있다. CLI 관찰 동작은 SPEC §5.10의 회귀 기준이다.

재사용:

- `internal/tool.Tool`, `Registry`, `Dispatcher`: tool node에서 그대로 사용한다. graph core로 옮기지 않는다.
- `internal/llm.LLMClient`, `ChatRequest`, `ChatResponse`: LLM node에서 그대로 사용한다.
- `internal/message`: `Message`, `ContentBlock`, `ToolCall`, `ToolResult` 타입을 그대로 사용한다.

변경하지 않는 범위:

- provider 구현(`internal/llm/claude.go`)은 graph를 몰라도 된다.
- calculator/file read tool 구현은 변경 대상이 아니다.
- README/ROADMAP의 Phase 문구 갱신은 Phase 4 구현 검증 이후의 정리 작업으로 남긴다.

## 5. Decision Points

### D1. Graph state를 generic으로 둘지, 공용 map으로 둘지

- 옵션 A: `map[string]any` 기반 `GraphState`를 graph core가 강제한다.
- 옵션 B: `Graph[S any]`, `Node[S]`, `Reducer[S]`, `Router[S]`처럼 호출자가 가진 타입을 state로 쓰는
  generic graph를 둔다.
- 옵션 C: `GraphState` interface에 clone/merge 같은 메서드를 요구한다.
- 트레이드오프: A는 구현이 단순하지만 Go 타입 안정성을 잃고, 기존 `AgentState`를 map으로 변환하는
  adapter가 필요하다. C는 모든 state 타입에 graph 전용 메서드를 강제해 agent 도메인 타입을 오염시킨다.
  B는 타입 파라미터가 늘지만 `AgentState`를 그대로 사용할 수 있고 graph core가 domain-neutral하게 남는다.
- 채택: **B**. `GraphState`는 graph가 다루는 역할 이름이고, 코드 표면에서는 `S any`가 그 역할을
  나타낸다. 이는 SPEC §5.1, §5.2, §5.10에 가장 잘 맞는다.

### D2. node 결과와 reducer의 책임

- 옵션 A: node가 state를 직접 mutate하고 reducer를 두지 않는다.
- 옵션 B: node가 갱신된 state 전체를 `NodeResult`로 반환하고, reducer가 current state와 result를 합쳐
  다음 state를 만든다.
- 옵션 C: node마다 서로 다른 patch 타입을 반환하고 reducer가 patch merge를 수행한다.
- 트레이드오프: A는 spec의 reducer 요구를 약화하고, state 변경 지점이 node 내부로 흩어진다. C는 장기적으로
  유연하지만 Phase 4 범위에는 과하다. B는 replace reducer만으로 현재 Agent 흐름을 표현하면서도 reducer
  경계를 명확히 둘 수 있다.
- 채택: **B**. Phase 4에서는 기본 replace reducer를 제공하고, 필요한 경우 테스트에서 custom reducer로
  state 누적을 검증한다(SPEC §5.2, §5.11).

### D3. graph step과 agent step의 의미

- 옵션 A: graph step과 `AgentState.Steps`를 같은 counter로 합친다.
- 옵션 B: graph step은 node 실행 횟수, `AgentState.Steps`는 기존처럼 LLM 호출 횟수로 분리한다.
- 옵션 C: graph step을 router 전이 횟수로 센다.
- 트레이드오프: A는 단순하지만 tool node까지 agent step으로 세게 되어 기존 max step 의미가 바뀐다.
  C는 node 실행과 step이 어긋나 max steps 검증이 모호하다. B는 counter가 둘이지만 graph의 안전장치와
  기존 Agent public contract를 모두 보존한다.
- 채택: **B**. graph max steps는 runtime 안전장치이고, agent max step은 `llm_node` 안에서 기존처럼
  LLM 호출 상한으로 유지한다(SPEC §5.5, §5.10).

### D4. Agent migration 방식

- 옵션 A: `Agent.Run`을 유지하되 내부 구현만 graph 기반으로 바꾼다.
- 옵션 B: 새 `GraphAgent`를 만들고 CLI가 이를 직접 사용하게 한다.
- 옵션 C: 기존 `Agent`와 graph 기반 agent를 둘 다 유지한다.
- 트레이드오프: B와 C는 전환 범위를 키우고 public API가 둘로 갈라진다. A는 기존 테스트와 CLI 계약을
  회귀 기준으로 삼을 수 있고, Phase 4의 목적도 "표현 방식 변경"에 집중된다.
- 채택: **A**. `internal/agent`가 graph adapter를 품고, 외부 호출자는 기존 `Agent.Run`만 본다.
  이는 SPEC §5.8~§5.10의 기존 Tool Calling Agent 동작 보존에 맞다.

### D5. tool 실패와 node error의 경계

- 옵션 A: unknown tool, validation error, tool execution error를 graph node error로 올린다.
- 옵션 B: tool 실패는 기존 dispatcher처럼 `ToolResult.IsError`로 state에 누적하고, graph node error는
  LLM 호출 실패, context 취소, graph/node 구현 실패에만 사용한다.
- 트레이드오프: A는 graph error 처리를 재사용하지만 Phase 3에서 확정한 "tool 실패는 loop를 깨지 않는다"는
  계약을 깨뜨린다. B는 graph error와 tool result error의 의미를 분리한다.
- 채택: **B**. tool node는 dispatcher 결과를 state에 누적하고 계속 진행한다. graph error는 node 실행
  자체를 계속할 수 없는 경우에만 사용한다(SPEC §5.6, §5.8).

### D6. graph 결과 status와 agent status 매핑

- 옵션 A: graph status를 agent status와 같은 타입으로 통합한다.
- 옵션 B: graph status는 graph package 전용으로 두고, `Agent.Run`이 agent status로 변환한다.
- 옵션 C: graph는 status 없이 error와 end 여부만 반환한다.
- 트레이드오프: A는 타입 수가 줄지만 graph core가 agent 의미(`final`, `tool_call 없음`)를 알게 된다.
  C는 SPEC §5.1, §5.5, §5.6, §5.7의 관찰 가능한 종료 상태를 약화한다. B는 변환이 필요하지만 경계가
  명확하다.
- 채택: **B**. graph core는 `completed`, `max_steps`, `error`, `canceled` 같은 실행 상태를 반환한다.
  agent adapter는 graph `completed`일 때 state에 이미 담긴 agent status를 신뢰하고, graph `max_steps`는
  `StatusMaxSteps`, graph `error`와 `canceled`는 `StatusError`로 변환한다.
