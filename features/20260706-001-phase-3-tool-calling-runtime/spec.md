# Phase 3 Tool Calling Runtime 명세

## 범위

Phase 3는 LLM assistant 응답의 tool call을 Runtime이 직접 실행할 수 있도록 Tool Calling Runtime을 만든다. 대상 범위는
새 `internal/tool` 패키지, 기존 `internal/agent` loop, 기존 `internal/message`의 tool call/result 표현, 그리고
tool schema를 LLM 요청 경계에서 다루는 provider-neutral contract다.

이 단계의 실행 단위는 Phase 2의 Agent run을 확장한 tool-use run이다. Runtime은 등록된 Tool 목록을 Agent가 사용할
수 있게 하고, LLM이 반환한 tool call 이름을 registry에서 찾아 입력을 검증한 뒤 context와 timeout 안에서 실행한다.
실행 결과는 `message.ToolResult`를 담은 tool message로 `AgentState.Messages`에 누적되고, 다음 LLM 요청에는 기존
메시지와 tool result가 함께 전달된다.

Tool 실행 결과가 정상 결과인지 오류 결과인지는 상태와 메시지에서 구분할 수 있어야 한다. 알 수 없는 Tool,
입력 검증 실패, 실행 timeout, Tool 실행 오류는 Agent process를 즉시 중단하는 provider 오류가 아니라 tool result
오류로 표현되어 다음 LLM 판단에 전달되는 것을 기본으로 한다.

Trace는 Phase 2에서 만든 메모리 trace 구조를 확장해 tool call과 tool result 관찰 지점을 남긴다. Phase 3에서도
trace를 파일, stdout, stderr, DB, 외부 observability 시스템으로 내보내는 형식은 고정하지 않는다.

## 목표

LLM이 Tool을 직접 실행하지 않고, Runtime이 등록된 Tool contract를 기준으로 안전하게 실행한다는 경계를 코드로
표현한다. 호출자는 Agent run 결과에서 tool call 요청, tool result 메시지, 최종 assistant 응답, 종료 상태를 확인할 수
있어야 한다.

Tool schema와 registry를 통해 LLM이 사용할 수 있는 Tool 목록을 명확히 하고, 이름 기반 lookup과 입력 검증 실패를
관찰 가능한 결과로 만든다. Tool 실행은 context cancellation과 timeout을 따라야 하며, 실행 결과는 provider-neutral
메시지 형태로 다음 LLM 요청에 다시 포함되어야 한다.

Phase 3는 최소 내장 Tool로 calculator와 file read를 제공한다. 이 Tool들은 Tool Runtime contract가 실제로 동작함을
검증하기 위한 기본 Tool이며, 이후 Phase의 Web Search, File Save, Code Execution Tool과 같은 더 넓은 Tool 묶음의
기반이 된다.

## 제약

Tool Runtime은 provider 구현에 직접 의존하지 않는다. Tool schema를 LLM 요청에 전달하는 provider별 변환은
`internal/llm` 경계에 두고, Tool 실행과 registry 책임은 `internal/tool`에 둔다.

Agent는 Tool 실행 결과를 기존 `message.ToolResult`와 `message.Tool` 메시지 형태로 누적한다. Tool 전용의 별도 대화
저장소나 외부 실행 로그 저장소는 만들지 않는다.

Tool 실행은 등록된 Tool만 허용한다. Tool 이름은 registry의 단일 식별자로 lookup되며, 같은 이름의 Tool을 중복 등록할
수 없거나 명확한 오류로 거부되어야 한다.

입력 검증은 Tool schema와 일관된 기준으로 이루어져야 한다. 잘못된 JSON, 필수 입력 누락, 타입 불일치처럼 Runtime이
확인할 수 있는 입력 문제는 Tool 실행 전에 오류 결과로 관찰 가능해야 한다.

File read Tool은 로컬 파일 읽기를 제공하되, Phase 3에서는 파일 저장, 삭제, 디렉터리 변경, 명령 실행을 포함하지
않는다.
파일 접근 정책의 세부 제한은 analysis 단계에서 기존 프로젝트 안전 기준과 테스트 가능성을 기준으로 확정한다.

## 제외 범위

Web Search Tool, File Save Tool, Code Execution Tool은 포함하지 않는다. 이 Tool 묶음은 Phase 4.1에서 다룬다.

Middleware hook, structured output, streaming runner, Agent Server, HTTP API, CLI를 Agent loop 기반 실행 경로로
전환하는 작업은 포함하지 않는다.

RAG, Memory, Multi-Agent, MCP, A2A 구현은 포함하지 않는다.

Tool 실행 trace의 파일 저장, JSON export, stdout/stderr 출력, DB 저장, 외부 observability 연동은 포함하지 않는다.

LLM이 여러 Tool 중 무엇을 선택해야 하는지 개선하는 prompt template, tool routing 정책, tool ranking은 포함하지
않는다. Phase 3는 등록된 Tool을 LLM 요청 경계에 제공하고, 반환된 tool call을 안전하게 실행하는 Runtime contract에
집중한다.

## 완료 조건
1. Runtime 내부에는 provider-neutral `Tool` contract와 `ToolRegistry`가 존재하며, 호출자는 Tool을 등록하고 이름으로
   조회할 수 있다.
2. 같은 이름의 Tool 중복 등록과 등록되지 않은 Tool lookup은 명확한 오류 또는 오류 결과로 구분되어 확인
   가능하다.
3. Tool schema는 LLM 요청 경계에서 관찰 가능하며, Agent가 LLM을 호출할 때 등록된 Tool 목록이 요청에 포함된다.
4. Agent run은 assistant 응답의 tool call을 registry에서 찾아 실행하고, 정상 실행 결과를 `message.ToolResult` 기반
   tool message로 `AgentState.Messages`에 누적한다.
5. Tool result가 누적된 뒤 Agent는 다음 LLM 요청에 같은 메시지 상태를 전달하며, 후속 assistant 응답에 tool call이
   없으면 final 상태와 final answer로 종료한다.
6. 잘못된 tool arguments, unknown tool, Tool 실행 오류, Tool timeout은 tool result의 오류 상태로 메시지에 보존되고
   다음 LLM 판단에 전달된다.
7. Agent run은 Tool 실행이 포함된 반복에서도 `MaxSteps`를 넘기지 않으며, 제한에 도달하면 LLM 또는 Tool을 추가로
   실행하지 않고 max step 상태로 종료한다.
8. `AgentState.Trace`에는 LLM 요청/응답뿐 아니라 tool call 실행 시작, tool result, tool 오류 또는 timeout을 확인할 수
   있는 메모리 trace event가 남는다.
9. 기본 calculator Tool은 유효한 입력에 대해 계산 결과를 반환하고, 잘못된 입력은 오류 result로 반환한다.
10. 기본 file read Tool은 허용된 로컬 파일 내용을 반환하고, 읽기 실패나 허용되지 않은 접근은 오류 result로
    반환한다.
11. 테스트는 실제 외부 provider 호출 없이 stub `LLMClient`와 테스트 Tool로 registry, schema 전달, 정상 tool 실행,
    오류 tool result, max step, trace 기록을 확인할 수 있다.
