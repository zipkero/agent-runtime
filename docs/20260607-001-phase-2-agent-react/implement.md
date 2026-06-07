# phase-2-agent-react — 실행 체크리스트

이 체크리스트는 `analysis.md`의 구조와 Decision Points(D1–D9)를 실행 단위로 옮긴 것이다.
Task는 위치 순서가 곧 의존성 순서다. 각 Task는 목적 / 접근 / 검증 조건 / 참조로만 구성된다.

## Section: agent 패키지 — 상태와 loop

- [ ] task-001: AgentState와 종료 상태 모델
  - 목적: Agent 실행 결과를 담는 상태 값이 종료 종류(진행중·최종답·max step 초과·에러)를
    하나의 명시적 상태로 구분하고, 누적 메시지·진행 step 수·원인 에러를 함께 보관한다.
  - 접근: 새 패키지 `internal/agent`에 `agent.go`를 만든다. 종료 상태를 나타내는 enum 타입
    (예: `Status` — `StatusRunning`/`StatusFinal`/`StatusMaxSteps`/`StatusError`)을 정의한다(D1 옵션 A).
    `AgentState` 구조체에 누적 대화 메시지 슬라이스(`[]message.Message`), 진행 step 수(`int`),
    현재 종료 상태(`Status`), 원인 에러(`error`, error 상태일 때만 채움)를 둔다(D5: 에러를 state에 흡수).
    호출자가 종료 후 (a) 종료 상태 식별 (b) final이면 최종 답 메시지 취득 (c) error면 원인 취득
    (d) 누적 메시지·step 수 취득이 가능하도록 관찰 표면(필드 또는 메서드)을 노출한다. 최종 답 메시지는
    마지막 assistant 응답이다. import는 `internal/message`만 사용한다.
  - 검증 조건:
    - 결과: `internal/agent` 패키지가 컴파일되고, `AgentState`가 종료 상태 enum·누적 메시지·step
      수·원인 에러를 모두 표현한다. `go build ./...`가 통과한다.
    - 확인: `go vet ./internal/agent/...` 통과. 종료 상태 enum의 네 값이 §2 terminal 집합
      (running 제외 final/max steps/error)과 1:1 대응한다.
  - 참조: SPEC §5.1, §5.2, §5.4, §5.5. ANALYSIS §2(종료 상태 집합), D1, D5.

- [ ] task-002: Agent와 ReAct loop 실행
  - 목적: 주입된 LLM client와 max step·reflection hook을 들고, 사용자 입력으로 시작한 loop를
    매 step LLM 호출·응답 누적하며 돌려 최종답·max step 초과·에러 중 하나로 종료된 상태를 반환한다.
  - 접근: `internal/agent`에 `Agent` 구조체와 생성 함수를 둔다. 생성 함수는 `llm.LLMClient`(interface,
    구현체 아님)·`model string`·`maxSteps int`·reflection hook을 인자로 받는다(D8 옵션 A: max step은
    생성 인자, config 미노출). reflection hook은 현재 step 번호와 누적 state를 받는 단일 함수 콜백
    타입으로 정의하고, 미주입(nil) 시 no-op으로 동작하게 한다(D4 옵션 A). 실행 표면
    `Run(ctx context.Context, prompt string) AgentState`를 둔다 — 에러를 두 번째 반환값으로 던지지
    않고 state에 흡수한다(D5). loop 본문은 ANALYSIS §2 "loop 한 회전" 순서를 그대로 따른다:
    (1) prompt를 `RoleUser` 메시지로 만들어 state 대화에 넣고 step 0·상태 running으로 초기화,
    (2) 매 회전 시작에서 reflection hook을 현재 step·state로 호출(nil이면 no-op),
    (3) step counter가 maxSteps 이상이면 LLM을 호출하지 않고 max steps 상태로 종료(D3 옵션 A: 선검사),
    (4) 누적 메시지로 `ChatRequest`를 만들어 `client.Chat(ctx, ...)` 호출 — ctx를 그대로 전파하고,
    에러를 반환하면 그 에러를 state에 담아 error 상태로 종료(ctx 취소 에러도 동일하게 흡수),
    (5) 받은 assistant 응답을 state 대화에 append하고 step counter 1 증가,
    (6) `resp.Message.HasToolCalls()`가 false면 final 상태로 종료, true면 실행하지 않고(D6: 신호로만)
    running 유지하며 (2)로 회전. tool_call 응답에 대해 tool 실행·tool_result 생성은 하지 않는다(SPEC §4).
  - 검증 조건:
    - 결과: `Agent.Run`이 final/max steps/error 세 종료 상태 중 하나로 끝난 `AgentState`를 반환한다.
      어떤 응답 패턴에서도 loop가 maxSteps를 넘겨 LLM을 호출하지 않는다(선검사). `go build ./...` 통과.
    - 확인: reflection hook 미주입 시에도 loop가 정상 동작한다(nil 안전). max step 검사가 LLM 호출
      앞에 위치해, tool_call만 반복하는 응답에서도 LLM 호출 횟수가 maxSteps를 넘지 않는다. ctx 취소
      에러가 error 상태로 흡수된다. 회귀: `go test ./...`가 깨지지 않는다(이 시점 main_test.go는
      아직 단발 run 가정이므로 통과해야 한다).
  - 참조: SPEC §5.1, §5.2, §5.3, §5.4, §5.5, §5.6. ANALYSIS §2(loop 한 회전·tool_call 처리·hook
    시점), §3(인터페이스), D2, D3, D4, D5, D6, D8.

- [ ] task-003: agent loop 결정적 테스트 (정상 종료·max step 두 경로)
  - 목적: 실제 API 호출 없이 stub만으로 정상 종료(최종 답 도달)와 실패 종료(max step 초과) 두 경로,
    그리고 에러 종료·hook 관찰·메시지 누적·step 증가가 결정적으로 검증된다.
  - 접근: `internal/agent`에 `agent_test.go`를 만든다. step마다 다른 응답을 순서대로 반환하는 다단계
    stub을 이 테스트 코드 안에 둔다(D7 옵션 A: `internal/llm`을 수정하지 않는다). 다단계 stub은
    `llm.LLMClient`를 구현하고, 응답 시퀀스를 순서대로 반환하며 `stub_test.go`처럼 ctx 취소를 먼저
    존중한다. 다음 케이스를 결정적으로 검증한다:
    - 정상 종료: 첫 응답이 tool_call 없는 text → 즉시 final, 최종 답 메시지가 그 text, 종료 상태가
      final, 누적 메시지에 user 입력 + assistant 응답이 모두 있고 step 수가 증가했음(SPEC §5.1·§5.2·§5.3).
    - max step 초과: 매 응답이 tool_call(D6에 따라 미실행·신호로만) → loop가 final에 못 닿고 maxSteps
      소진 후 max steps 상태로 종료, 이것이 final이 아님을 종료 상태로 구분, LLM 호출 횟수가 maxSteps와
      일치(선검사로 초과 호출 없음)(SPEC §5.4·§5.8).
    - 에러 종료: stub이 에러를 반환 → 종료 상태 error, 원인 에러가 state에서 확인됨(SPEC §5.5).
    - hook 관찰: 콜백이 step 경계마다 호출되며, 캡처한 step 번호·state로 호출 사실을 확인(SPEC §5.6).
  - 검증 조건:
    - 결과: `go test ./internal/agent/...`가 실제 API 호출 없이 통과하고, 정상 종료·max step 초과
      두 경로를 각각 별도 테스트로 검증한다.
    - 확인: max step 케이스에서 stub의 호출 횟수가 maxSteps를 초과하지 않음을 단언한다. 에러 케이스에서
      원인 에러가 `errors.Is` 또는 동등 비교로 확인된다. `go test ./...` 전체 통과.
  - 참조: SPEC §5.2, §5.3, §5.4, §5.5, §5.6, §5.8. ANALYSIS §2(tool_call 처리), D6, D7.

## Section: CLI 진입점 교체

- [ ] task-004: run을 Agent loop로 교체하고 종료 상태별 출력 분기
  - 목적: CLI가 단발 Chat 호출 대신 Agent loop로 입력을 처리해, 최종 답을 stdout에 출력하고
    max step 초과·에러·취소는 원인을 stderr에 쓰며 비정상 종료코드로 끝낸다.
  - 접근: `cmd/agent-runtime/main.go`의 `run`을 Agent 기반으로 바꾼다. `run` 시그니처에 max step을
    전달할 경로를 마련하고, CLI는 하드코딩 기본 상수(예: `defaultMaxSteps`)를 Agent 생성에 넘긴다
    (D8 옵션 A: config 미노출). `run`은 `agent` 패키지로 Agent를 만들어(reflection hook 미주입 또는
    no-op) `Run(ctx, prompt)`를 호출하고, 반환된 `AgentState`의 종료 상태로 출력을 가른다:
    - final: 최종 답 메시지를 stdout에 출력하고 종료코드 0. 기존 `printResponse`를 최종 답 메시지
      출력에 재사용하거나 종료 상태별 분기에 맞게 조정한다.
    - error: 원인 에러를 stderr에 쓰고 비정상 종료코드(1). ctx 취소도 error 상태로 들어와 동일 처리.
    - max steps: 실패로 표현한다(D9 옵션 A) — max step 초과 원인을 stderr에 쓰고 비정상 종료코드(1).
      문구는 error와 구분해 "max step 초과로 최종 답에 도달하지 못함"임을 드러낸다.
    `main` 함수는 `run` 호출 형태만 새 시그니처에 맞춰 조정하고, config에 max step 항목은 추가하지 않는다.
  - 검증 조건:
    - 결과: `run`이 Agent loop를 통해 입력을 처리하고, final이면 최종 답이 stdout에, max step·error·
      취소면 원인이 stderr에 출력되며 종료코드가 갈린다. `go build ./...` 통과.
    - 확인: `cmd/agent-runtime/main_test.go`의 기존 4개 테스트를 새 동작에 맞게 갱신한다 —
      text 응답 → final·stdout·종료코드 0; 단발 tool_call을 stdout에 찍던 `TestRun_ToolCall_...`은
      loop 의미와 충돌하므로 재정의한다(예: 다단계 stub으로 max step 초과 → stderr·비정상 종료코드,
      또는 text 응답으로 바꿔 final 검증); chat 에러 → error·stderr·비정상 종료코드;
      ctx 취소 → error·stderr·비정상 종료코드. `go test ./...` 전체가 통과한다.
  - 참조: SPEC §5.7. ANALYSIS §1(CLI), §3(CLI와의 계약), §4(영향 범위), D8, D9.
