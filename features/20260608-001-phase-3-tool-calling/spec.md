# phase-3-tool-calling

ROADMAP Phase 3(Tool Calling Runtime)의 요구사항 명세다. LLM이 선택한 tool을 Runtime이 안전하게
실행하는 구조를 정의한다. LLM은 tool을 직접 실행하지 않고 tool call을 요청하며, 그 요청을 해석·실행해
결과를 다시 모델에 돌려주는 책임은 Runtime이 진다.

## 1. 범위

- Tool 실행 런타임 계층 (`internal/tool`): tool을 표현하는 추상화, 이름 기반 등록·조회 레지스트리,
  tool call을 실제로 실행하는 경로.
- `Tool` 추상화: 자신의 이름과 입력 schema를 노출하고, 주어진 입력으로 실행되어 결과를 내는 실행 단위.
- tool schema: LLM에 제공해 tool call 생성을 유도하는 각 tool의 입력 명세. 등록된 tool들의 schema를
  모아 chat 요청에 실어 보낸다.
- `ToolRegistry`: tool을 이름으로 등록하고 이름으로 조회하는 단위.
- tool 입력 검증: tool call에 담긴 raw JSON 입력을 tool이 기대하는 형태에 맞게 검증하는 단계.
- tool 실행 timeout: 개별 tool 실행이 무한정 매달리지 않도록 제한하는 장치.
- tool 결과 정규화: 성공·실패를 일관된 tool result 형태로 변환하는 규칙.
- unknown tool 처리: 등록되지 않은 이름의 tool call을 다루는 규칙.
- tool 에러 처리: 실행 중 발생한 에러를 loop를 깨지 않고 결과로 표현하는 규칙.
- 구체 tool 두 개: 기본 calculator tool, 기본 file read tool.
- Agent loop 확장 (`internal/agent`): Phase 2에서 신호로만 남겨 둔 tool_call 응답을 registry로 실제
  실행하고, 그 결과를 `AgentState`에 누적해 loop를 이어가는 경로.

## 2. 목표

- LLM이 tool을 직접 실행하지 않고 tool call을 "요청"하며, 그 요청을 해석·실행하는 주체가 Runtime임을
  코드로 분명히 한다.
- tool schema를 명확히 정의해 안정적인 tool calling이 가능하게 한다.
- tool 실행 결과를 다시 LLM 입력으로 넣어, Phase 2에서 종료 판정·상태 누적까지만 돌던 loop가 비로소
  실질적인 multi-step ReAct로 회전하게 한다.
- unknown tool·입력 검증 실패·실행 에러·timeout 같은 실패가 loop를 깨지 않고 모델에 피드백되어, Runtime이
  tool 실행을 안전하게 통제함을 드러낸다.
- Phase 4(Graph)·Phase 5(Single Agent)가 그 위에 올라설 tool 계층의 골격을 세운다.

## 3. 제약

- LangChain / LangGraph 계열 라이브러리는 사용하지 않는다.
- tool 실행은 `internal/message`의 tool call·tool result·tool schema 타입을 재사용하며, Agent는 구체
  tool 구현체에 직접 의존하지 않고 registry/추상화 뒤에서 tool을 실행한다.
- 모든 tool 실행은 `context.Context`를 받아 취소·timeout을 전파할 수 있어야 한다.
- tool 실행 timeout이 강제된다 — 개별 tool 실행은 어떤 경우에도 무한히 매달리지 않는다.
- unknown tool·입력 검증 실패·실행 에러·timeout은 패닉이나 loop 중단으로 귀결되지 않고, 에러임이 표시된
  tool result로 정규화되어 다음 step의 모델 입력에 포함된다.
- file read tool은 임의 경로를 무제한으로 읽지 않는다 — 허용된 범위 밖의 경로 접근은 에러 결과로 거부한다.
- ROADMAP 중단 기준에 따라, 최소 하나의 실패 케이스(예: unknown tool 호출 또는 입력 검증 실패)가 관찰
  가능해야 한다.
- stub client와 등록된 tool만으로 tool 실행 경로(tool 실행 → 결과 누적 → 최종 답)를 실제 API 호출 없이
  결정적으로 검증할 수 있어야 한다.

## 4. 제외 범위

- Graph State / Node / Edge 기반 실행 구조 (Phase 4).
- Web Search Tool, File Save Tool, Code Execution Tool 등 추가 tool과 그 보안 제한 (Phase 5). 본 Phase의
  구체 tool은 calculator와 file read 둘로 한정한다.
- middleware hook, structured output 파싱, streaming 응답 (Phase 5).
- MCP 등 외부 protocol을 통한 tool 연동 (Phase 9). 본 Phase는 in-process 로컬 tool만 다룬다.
- 한 응답에 여러 tool call이 올 때의 병렬 실행·실행 순서 최적화. 순차 실행으로 충분하며 동시성 정책은
  다루지 않는다.
- 메시지 trimming·요약·세션 메모리 등 장기 상태 관리 (Phase 8).
- 토큰 사용량·latency 등 trace 수집 구조의 정식화 (이후 Phase에서 정리).

## 5. 완료 조건

1. tool을 registry에 이름으로 등록할 수 있고, 같은 이름이 충돌하는 상황이 정의된 방식으로(예: 등록 거부
   또는 명시적 덮어쓰기) 처리되어 호출자가 그 결과를 확인할 수 있다.
2. 이름으로 등록된 tool을 조회할 수 있으며, 등록되지 않은 이름의 조회는 unknown으로 구분되어 결과로
   드러난다.
3. 등록된 tool들의 schema를 모아 LLM chat 요청에 실어 전달할 수 있다.
4. Agent가 tool_call이 담긴 응답을 받으면 registry로 해당 tool을 실행하고, 그 결과를 tool result
   메시지로 `AgentState`의 대화에 누적한 뒤 loop를 이어간다.
5. tool 실행 결과(성공·실패 모두)가 다음 step의 LLM 입력에 tool result로 포함되어, 모델이 그 결과를
   보고 이어서 판단할 수 있다.
6. tool 입력 검증에 실패하면 tool 본체를 실행하지 않고, 에러임이 표시된 tool result로 만들어 loop를
   이어간다.
7. 등록되지 않은(unknown) tool 이름의 tool call은 loop를 깨지 않고 에러 tool result로 표현되어 모델에
   피드백된다.
8. tool 실행 중 에러가 나거나 실행이 timeout을 넘기면, loop가 깨지지 않고 에러임이 표시된 tool result로
   정규화되어 이어진다.
9. 기본 calculator tool이 산술 입력에 대해 계산 결과를 tool result로 반환한다.
10. 기본 file read tool이 허용된 경로의 파일 내용을 tool result로 반환하고, 허용되지 않은 경로나 존재하지
    않는 파일은 에러 tool result로 반환한다.
11. CLI에서 실행되는 Agent가 등록된 tool들과 그 schema를 갖춘 채로 동작해, 사용자 입력이 tool calling을
    거쳐 최종 응답에 이르는 경로가 end-to-end로 성립한다.
12. stub client와 등록된 tool만으로 multi-step tool calling 경로(tool 실행 → 결과 누적 → 최종 답 도달)가
    실제 API 호출 없이 결정적으로 검증된다.
