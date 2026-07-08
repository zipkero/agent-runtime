# Phase 4.1 Tool Bundle 명세

## 범위

Phase 4.1은 Phase 3에서 만든 Tool contract, registry, Agent tool-use loop 위에 Single Agent용 기본 Tool 묶음을
추가한다. 대상 Tool은 Web Search Tool, File Save Tool, Code Execution Tool이다.

Web Search Tool은 Tavily 검색 API를 외부 검색 provider로 사용해 사용자 질의에 대한 검색 결과를 Runtime의 Tool
result로 반환한다. API key 또는 provider 호출 실패는 Agent process 오류가 아니라 Tool 오류 result로 관찰 가능해야
한다.

File Save Tool은 허용된 root 안에 로컬 파일을 저장한다. 저장 대상 경로와 content를 입력으로 받고, 허용 범위를
벗어난 경로나 저장 실패는 Tool 오류 result로 반환한다.

Code Execution Tool은 제한된 실행 환경에서 코드 또는 명령을 실행하고 stdout, stderr, exit code, timeout 같은 실행
결과를 Tool result로 반환한다. 실행 제한은 host 전체에 대한 무제한 명령 실행이 아니라 분석 단계에서 확정되는
명시적 정책을 따라야 한다.

이 단계는 새 Tool들이 Phase 3의 provider-neutral Tool contract를 따르고 Agent registry에 등록되어 호출될 수 있음을
확인하는 데 집중한다. Single Agent runner, middleware, structured output, streaming은 후속 Phase에서 다룬다.

## 목표

Single Agent가 외부 정보 검색, 로컬 파일 생성, 제한된 코드 실행이라는 기본 작업 capability를 Tool 형태로 사용할 수
있게 한다.

각 Tool은 명확한 schema, 입력 검증, context cancellation, timeout, 오류 result 표현을 제공해야 한다. 정상 result와
오류 result는 기존 `message.ToolResult` 기반 메시지 흐름으로 다음 LLM 판단에 전달될 수 있어야 한다.

Web Search, File Save, Code Execution Tool은 실제 외부 provider 또는 로컬 I/O를 다루더라도 테스트 가능한 경계를
가져야 한다. 외부 API 호출이 필요한 경로는 설정 누락과 provider 오류를 명확히 구분하고, 단위 테스트는 실제 Tavily
호출 없이 검증할 수 있어야 한다.

## 제약

Tool 구현은 `internal/tool`의 기존 Tool interface와 공통 오류 분류를 따른다. Agent나 provider 구현이 특정 Tool의
내부 타입에 직접 의존하지 않아야 한다.

Web Search Tool은 Tavily API 연결을 사용하되, 실제 API key가 없는 환경에서도 설정 오류가 Tool 오류 result로
전달되어야 한다. 단위 테스트는 stub HTTP client 또는 동등한 테스트 대역으로 외부 네트워크 호출 없이 실행 가능해야
한다.

File Save Tool은 명시적으로 주입된 root 밖에 파일을 저장하지 않는다. 경로 정리 후 root 밖으로 벗어나는 입력,
디렉터리 덮어쓰기, 빈 content 정책처럼 저장 안전성에 영향을 주는 세부 판단은 분석 단계에서 확정한다.

Code Execution Tool은 timeout과 context cancellation을 따라야 하며, 장기 실행 또는 무제한 host 접근을 허용하지
않는다. 실행 언어, 허용 명령, 작업 디렉터리, 환경변수, 네트워크 접근, 출력 크기 제한은 분석 단계에서 확정한다.

모든 Tool 오류는 Agent의 `StatusError`가 아니라 Tool result의 오류 상태로 표현되어야 한다. 단, Tool 생성에 필요한
설정 자체가 잘못된 경우에는 생성자 또는 등록 시점의 오류로도 확인 가능해야 한다.

## 제외 범위

Middleware hook, structured output, Single Agent runner, agent loop 기반 CLI 전환은 Phase 4.2에서 다룬다.

Provider-neutral streaming LLM contract, Runner streaming event, CLI streaming 출력, streaming final response 조립은
Phase 4.3에서 다룬다.

RAG, Memory, Multi-Agent, MCP, A2A 구현은 포함하지 않는다.

웹 검색 provider routing, 여러 검색 provider 지원, 검색 결과 ranking 고도화, cache, retry/backoff 정책은 포함하지
않는다.

File Save Tool은 파일 저장만 다루며 파일 삭제, 이동, 복사, 디렉터리 탐색, 파일 diff/patch 적용은 포함하지 않는다.

Code Execution Tool은 production-grade sandbox, 컨테이너 격리, 권한 모델, 비용/자원 quota 시스템을 포함하지 않는다.
Phase 4.1은 제한 정책이 코드로 표현되고 테스트로 확인되는 최소 실행 Tool을 목표로 한다.

## 완료 조건
1. Runtime에는 Web Search Tool이 존재하며, 유효한 검색 입력에 대해 검색 결과 content와 source metadata를 Tool
   result로 반환할 수 있다.
2. Web Search Tool은 API key 누락, provider 오류, 잘못된 입력을 Tool 오류 result로 구분해 반환할 수 있고, 실제
   Tavily 호출 없이 단위 테스트로 검증할 수 있다.
3. Runtime에는 File Save Tool이 존재하며, 허용된 root 안의 파일에 content를 저장하고 저장된 경로 또는 저장 결과를
   Tool result로 반환할 수 있다.
4. File Save Tool은 빈 경로, root 밖 경로, 허용되지 않은 대상, 저장 실패를 Tool 오류 result로 구분해 반환할 수
   있다.
5. Runtime에는 Code Execution Tool이 존재하며, 허용된 실행 입력에 대해 stdout, stderr, exit code를 포함한 실행
   결과를 Tool result로 반환할 수 있다.
6. Code Execution Tool은 잘못된 입력, 허용되지 않은 실행 요청, timeout, 실행 실패를 Tool 오류 result로 구분해
   반환할 수 있다.
7. 세 Tool은 모두 provider-neutral Tool schema를 노출하고, 기존 Tool registry에 등록되어 Agent의 LLM 요청 schema
   목록에 포함될 수 있다.
8. Agent run은 stub LLMClient와 등록된 Phase 4.1 Tool을 사용해 tool call을 실행하고, 각 Tool result를 메시지에
   누적한 뒤 다음 LLM 판단으로 전달할 수 있다.
9. Phase 4.1 테스트는 외부 provider 호출 없이 Web Search, File Save, Code Execution Tool의 정상 결과, 입력 검증
   실패, 실행 실패, timeout 또는 취소 경로를 확인할 수 있다.
