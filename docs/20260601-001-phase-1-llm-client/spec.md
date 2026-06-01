# phase-1-llm-client

ROADMAP Phase 1(LLM 기반 의사결정 구조)의 요구사항 명세다. LLM을 Agent Runtime의 판단 주체로
사용하기 위한 기본 추상화를 정의한다.

## 1. 범위

- LLM Provider를 Runtime에서 분리하는 추상화 계층 (`internal/llm`).
- Runtime 전반에서 쓰이는 메시지 타입 (`internal/message`): user / assistant / tool / system
  메시지, tool call, tool result.
- LLM 호출의 요청·응답 모델: chat request, chat response, 일반 텍스트 응답과 tool call 응답의 구분.
- 실제 Claude API를 호출하는 Provider 구현체 (초기 Claude 우선).
- 테스트에서 실제 API 대신 끼워 넣을 수 있는 stub client.
- model / api key를 다루는 config (`internal/config`).
- LLM 호출에 대한 context 기반 timeout 처리.
- CLI에서 사용자 입력을 받아 LLM 응답을 출력하는 최소 진입 경로.

## 2. 목표

- LLM Provider가 바뀌어도 이를 호출하는 Runtime 코드는 바뀌지 않도록, Provider를 interface 뒤에 둔다.
- LLM을 "Runtime의 일부 부품"으로 다루는 구조를 코드로 표현한다 — LLM 호출은 Runtime이 소유한 한 단계이지
  Runtime 그 자체가 아니다.
- 일반 응답과 tool call 응답을 호출자가 명확히 구분해 다룰 수 있게 한다 (이후 Phase의 ReAct loop·Tool
  Calling이 이 구분 위에 세워진다).
- 실제 API를 호출하는 실행 경로를 유지하면서도, 테스트는 stub으로 결정적으로 돌릴 수 있게 한다.
- 외부 호출이 무한정 매달리지 않도록 timeout으로 호출 시간을 제한한다.

## 3. 제약

- LangChain / LangGraph 계열 라이브러리는 사용하지 않는다. LLM 호출은 공식 SDK 또는 HTTP로 직접 처리한다.
- Provider 호출 코드는 `LLMClient` interface 뒤에 있어야 하며, Runtime/호출자는 구현체 타입에 직접
  의존하지 않는다.
- API key는 소스에 하드코딩하지 않고 config(환경변수 등)로 주입한다.
- 모든 LLM 호출은 `context.Context`를 받아 취소·timeout을 전파할 수 있어야 한다.
- 초기 실제 Provider는 Claude 하나로 한정한다. 다른 Provider는 interface 수준에서 교체 가능성만
  표현한다 (실제 구현은 이후).
- ROADMAP 중단 기준에 따라, 최소 하나의 실패 케이스(예: timeout 초과, 잘못된 key)가 관찰 가능해야 한다.

## 4. 제외 범위

- ReAct loop, `AgentState`, step 기반 반복 실행 (Phase 2).
- Tool의 실제 등록·실행·검증 런타임 (Phase 3). 본 Phase는 tool call / tool result 메시지 타입과
  응답 구분까지만 다룬다.
- Claude 외 Provider(GPT 등)의 실제 구현체.
- streaming 응답 (Phase 5 optional).
- retry / backoff / rate limit 핸들링 등 호출 안정화 정책.
- structured output 파싱 (Phase 5).
- 토큰 사용량·latency 등 trace 수집 구조의 정식화 (이후 Phase에서 정리).

## 5. 완료 조건

1. CLI에서 사용자가 입력한 프롬프트를 받아 실제 LLM 호출 결과를 stdout으로 출력한다.
2. LLM 호출은 `LLMClient` interface를 통해 이루어지며, 호출자 코드를 바꾸지 않고 구현체(실제 Claude
   client ↔ stub client)를 교체할 수 있다.
3. chat 요청에 대한 응답에서, 일반 텍스트 응답과 tool call 요청 응답을 호출자가 구분해 확인할 수 있다.
4. user / assistant / tool / system 메시지와 tool call·tool result를 표현하는 메시지 타입이 존재하며,
   이를 chat 요청에 담아 전달할 수 있다.
5. api key가 config(환경변수 등)로 주입되며, 소스 변경 없이 다른 key·model 값으로 호출할 수 있다.
6. LLM 호출에 timeout이 적용되어, 제한 시간을 초과하면 호출이 에러로 종료되고 그 사실이 관찰 가능하다.
7. stub client를 사용한 테스트가 실제 API 호출 없이 결정적으로 통과한다.
