# Phase 1 LLM Client 명세

## 범위

Phase 1은 Agent Runtime이 LLM을 교체 가능한 판단 주체로 사용할 수 있도록 기본 호출 contract와 실제 provider
구현을 만든다. 대상 범위는 `internal/message`, `internal/llm`, 기존 `internal/config`, 그리고
`cmd/agent-runtime`의 단발 CLI 실행 경로다.

이 단계의 실행 결과는 CLI에서 사용자 입력 하나를 받아 설정된 LLM provider로 전송하고, provider 응답을 출력하는
것이다. Claude와 Ollama를 실제 provider로 지원하며, provider 선택, 모델, host, API key, timeout은 Phase 0의
설정 로딩 경계를 확장해 사용한다.

메시지와 LLM 응답 contract에는 이후 Agent loop와 Tool Calling Runtime이 재사용할 수 있는 user, assistant,
system 메시지와 tool call, tool result 표현을 포함한다. 다만 Phase 1은 tool을 실행하거나 agent 상태를 반복 갱신하는
단계가 아니라, provider 응답 안의 tool call 정보를 잃지 않고 표현할 수 있는 LLM 호출 계층까지만 다룬다.

## 목표

LLM provider와 Runtime 본체를 분리해 Claude와 Ollama를 같은 호출 contract 뒤에서 다룰 수 있게 한다.
CLI에서 단발 prompt를 입력하면 설정된 provider를 실제 호출하고, 응답 text 또는 provider 오류를 관찰할 수 있게 한다.
실행 경로는 실제 provider를 호출하되, 테스트는 provider interface 뒤에서 stub으로 교체할 수 있어야 한다.
요청별 timeout을 적용해 provider 지연이나 연결 문제를 Runtime 경계에서 제어할 수 있게 한다.

## 제약

Runtime 코드는 하나의 코드베이스 안에서 발전시키며 Phase 1을 위한 별도 예제 프로젝트를 만들지 않는다.
새 런타임 패키지는 `internal/llm`과 `internal/message`를 중심으로 제한하고, 기존 설정 확장은 `internal/config`에서
처리한다.
LLM 호출은 provider별 client 구현 안에 격리하고, Agent loop, Tool Runtime, RAG, Memory, Multi-Agent 계층은 LLM
client에 의존할 수 있어도 LLM client가 그 계층에 의존하지 않는다.
Claude API key 같은 비밀값은 로그와 일반 출력에 노출하지 않는다.
Provider별 필수 설정이 부족하면 실제 호출 전에 명확한 오류로 종료해야 한다.
Provider 호출에는 요청 timeout이 적용되어야 하며, timeout 오류는 일반 provider 오류와 구분 가능해야 한다.
Ollama는 로컬 host 설정을 사용하고, Claude는 API key 기반 원격 호출을 사용한다.

## 제외 범위

Agent loop, step 상태, final answer 감지, max step 제어는 포함하지 않는다.
Tool registry, tool schema 검증, tool 실행, unknown tool 처리, tool result를 다시 model 입력으로 넘기는 흐름은
포함하지 않는다.
Streaming 응답 출력, structured output 검증, middleware hook은 포함하지 않는다.
RAG, Memory, Multi-Agent, MCP, A2A 구현은 포함하지 않는다.
GPT 등 Claude와 Ollama 외 provider 구현은 포함하지 않는다.
HTTP API, daemon, Agent Server 실행 방식은 포함하지 않는다.

## 완료 조건
1. 저장소 루트에서 CLI를 실행해 사용자 prompt를 전달하면 설정된 LLM provider가 실제 호출되고 응답 text가
   표준 출력에 표시된다.
2. `LLM_PROVIDER=claude`와 관련 설정을 사용하면 Claude provider가 선택되고, 필수 설정 누락 또는 provider 오류가
   비밀값 노출 없이 명확한 오류로 반환된다.
3. `LLM_PROVIDER=ollama`와 관련 설정을 사용하면 Ollama provider가 선택되고, 설정된 host와 model로 요청이 전송된다.
4. Runtime 내부에는 provider-neutral `LLMClient` 호출 contract와 메시지·응답 타입이 존재하며, Claude와 Ollama
   구현은 같은 contract 뒤에서 교체 가능하다.
5. LLM 요청에는 설정된 timeout이 적용되고, timeout 상황은 테스트 또는 실행 결과로 확인 가능하다.
6. 테스트는 실제 외부 provider 호출 없이 stub client나 local test server로 request/response 변환, provider 선택,
   설정 검증, timeout 처리를 확인할 수 있다.
