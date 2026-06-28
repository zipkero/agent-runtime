# Phase 0 Project Foundation 명세

## 범위

Phase 0은 Go 기반 Agent Runtime 프로젝트를 이후 단계가 누적될 수 있는 실행 가능한 기본 상태로 만든다.
대상 범위는 Go module, 기본 디렉터리 구조, `cmd/agent-runtime` CLI 진입점, `internal/config` 설정 로딩,
`.env` 로딩, 기본 로그 출력, `.env.example`, `.gitignore`, 최상위 `README.md`와 `ROADMAP.md`의 프로젝트 기준 문서다.

이 단계의 결과물은 단계별 예제 폴더가 아니라 하나의 Runtime 코드베이스의 시작점이어야 한다.
엔진 본체는 특정 도메인에 묶이지 않으며, 이후 단계에서 추가될 prompt, tool, worker 구성은 진입점 주입으로
확장될 수 있어야 한다.

## 목표

Go 명령으로 실행 가능한 프로젝트 골격을 제공한다.
개발자가 로컬 환경변수와 `.env` 파일을 통해 실행 설정을 로딩할 수 있게 한다.
별도 logger 패키지 없이 CLI 진입점에서 설정값을 이용해 기본 로그를 출력할 수 있게 한다.
최상위 문서가 프로젝트 목적, 진행 방식, Phase 진행 상태, 최종 목표 구조를 설명하도록 유지한다.

## 제약

Runtime 코드는 단계별 예제 디렉터리로 분리하지 않고 하나의 코드베이스 안에서 발전시킨다.
Phase 0에서 새로 만드는 런타임 패키지는 `cmd/agent-runtime`과 `internal/config`를 중심으로 제한한다.
`.env`는 로컬 비밀값 파일로 취급해 git 추적 대상에서 제외하고, 공유 가능한 값의 이름과 기본 사용법은
`.env.example`에 둔다.
실제 환경변수와 `.env` 값의 우선순위는 문서화되고 일관되게 동작해야 한다.
로그 초기화는 진입점 책임으로 두며, 별도 logger 패키지를 만들지 않는다.

## 제외 범위

LLM provider 호출, Agent loop, Tool Calling Runtime, RAG, Memory, Multi-Agent, MCP, A2A 구현은 포함하지 않는다.
Claude, Ollama, Tavily, Postgres 같은 외부 서비스와 실제로 통신하는 client 구현은 포함하지 않는다.
HTTP API, Agent Server, daemon 실행 방식은 포함하지 않는다.
단계별 예제 프로젝트나 Phase별 별도 실행 폴더는 만들지 않는다.
Phase 1 이후에 필요한 provider별 세부 설정 검증이나 tool별 필수값 검증은 포함하지 않는다.

## 완료 조건

1. `go run ./cmd/agent-runtime` 명령이 저장소 루트에서 실행 가능하고 성공 상태로 종료된다.
2. 실행 시 `internal/config`를 통해 실제 환경변수와 `.env` 파일의 값을 로딩할 수 있으며,
   우선순위가 문서화된 기준과 일치한다.
3. 실행 시 CLI 진입점에서 설정값을 사용해 기본 로그를 출력한다.
4. 저장소에는 `go.mod`, `cmd/agent-runtime`, `internal/config`, `.env.example`, `.gitignore`가 존재한다.
5. `.env`는 git 추적 대상에서 제외되고, `.env.example`은 로컬 실행에 필요한 환경변수 이름과 사용법을 제공한다.
6. 최상위 `README.md`와 `ROADMAP.md`가 프로젝트 목적, 단일 Runtime 진행 방식, Phase 진행 상태를 설명한다.
