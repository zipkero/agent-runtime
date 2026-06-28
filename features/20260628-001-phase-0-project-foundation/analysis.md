# Phase 0 Project Foundation 분석

## 근거

확인한 사실:

- `spec.md`는 Phase 0의 범위를 Go module, `cmd/agent-runtime`, `internal/config`, `.env` 로딩, 기본 로그 출력,
  `.env.example`, `.gitignore`, 최상위 `README.md`와 `ROADMAP.md`로 제한한다.
- `README.md`는 하나의 Runtime 코드베이스를 점진적으로 발전시키고, 도메인 성격은 진입점에서 주입한다는 방향을
  설명한다.
- `README.md`는 `cmd/agent-runtime`을 초기 CLI 진입점으로 두고, HTTP API와 Agent Server를 로드맵 밖 확장으로 둔다.
- `README.md`는 `internal/config`가 `.env` 및 환경변수 로딩, LLM 관련 설정, timeout 기본값, 외부 연동 설정,
  logger 설정 값을 제공한다고 설명한다.
- `ROADMAP.md`는 Phase 0의 주요 패키지를 `cmd/agent-runtime`과 `internal/config`로 한정하고,
  완료 기준을 `go run ./cmd/agent-runtime`, 환경변수 로딩, 기본 로그 출력, README 정리로 둔다.
- `.env.example`은 실제 환경변수가 `.env`보다 우선한다고 설명하고, 현재 LLM·Tavily 관련 환경변수 이름을 제공한다.
- `.gitignore`는 `.env`를 무시하고, `/agent-runtime` 실행 파일과 일반 Go 산출물을 무시한다.
- git remote는 `https://github.com/zipkero/agent-runtime.git`이고, 로컬 Go 버전은 `go1.26.4 darwin/arm64`다.

추정:

- 아직 `go.mod`, `cmd/agent-runtime`, `internal/config`는 없으므로, Phase 0 구현에서 새로 생성해야 한다.
- Phase 0의 설정 로딩은 현재 `.env.example`의 단순 `KEY=VALUE`, 빈 줄, 주석 형식을 기준으로 충분하다.
  따옴표 escape나 shell expansion 같은 풍부한 dotenv 문법은 `spec.md`에 요구되어 있지 않다.

## 1. 구조

Phase 0 구조는 CLI 진입점과 설정 로딩 계층만 만든다. `cmd/agent-runtime`은 실행 조립 지점으로서 config 로딩,
logger 초기화, 시작 로그 출력을 담당한다. `internal/config`는 환경변수 이름, 기본값, `.env` 파일 읽기,
실제 환경변수 우선순위 적용을 소유한다. 이 경계는 `README.md`의 "도메인 성격은 진입점에서 주입" 원칙과 맞고,
Phase 1 이후 LLM provider나 tool 구성 추가 시 Runtime 본체가 아닌 진입점 조립으로 확장할 수 있게 한다
(SPEC §5.1, SPEC §5.2, SPEC §5.3).

`go.mod`의 module path는 git remote와 일치하는 `github.com/zipkero/agent-runtime`을 채택한다. 로컬 전용
`agent-runtime` module path는 짧지만, 이후 GitHub 기준 import와 문서·태그 추적에 불리하다. 원격 저장소가
확인되었으므로 repository path를 module path로 쓰는 편이 이후 패키지 import 기준을 안정적으로 만든다
(SPEC §5.1, SPEC §5.4).

`internal/config`는 하나의 `Config` 값을 반환하는 패키지로 둔다. Phase 0의 책임은 실행 설정을 읽고 검증 가능한
형태로 제공하는 것이며, provider별 client 생성이나 외부 서비스 연결은 Phase 1 이후 책임이다. 따라서 `Config`에는
현재 `.env.example`에 있는 LLM·Tavily 설정과 로그 레벨, timeout 기본값을 담되, Claude API key 필수성이나 Tavily
미설정 시 tool error 같은 provider·tool별 의미 검증은 수행하지 않는다. 이 구분은 Phase 0 제외 범위를 지키면서
Phase 1 이후 설정 재사용을 가능하게 한다(SPEC §5.2, SPEC §5.5).

logger는 별도 패키지를 만들지 않고 `cmd/agent-runtime`에서 표준 라이브러리 logger를 설정한다. 설정값 로딩은
`internal/config`가 제공하고, 실제 logger 객체 생성과 출력은 진입점 책임으로 둔다. 이렇게 하면 Phase 0에서
logger 공통 추상화를 새로 만들지 않으면서도 기본 로그 출력 완료 조건을 만족한다(SPEC §5.3).

## 2. 데이터 흐름

실행 흐름은 저장소 루트에서 `go run ./cmd/agent-runtime`을 호출하는 것으로 시작한다. Go toolchain은 `go.mod`를
기준으로 CLI main package를 빌드하고 실행한다. main 함수는 먼저 `internal/config`의 로딩 함수를 호출해 `.env`
값을 읽고 실제 환경변수와 병합한 `Config`를 받는다(SPEC §5.1, SPEC §5.4).

설정 로딩 흐름은 `.env` 파일을 선택적으로 읽은 뒤, 현재 process environment를 우선 적용하는 순서다. `.env`가
없으면 오류로 종료하지 않고 실제 환경변수와 기본값만으로 `Config`를 만든다. `.env` 파싱은 빈 줄과 `#` 주석을
무시하고, `KEY=VALUE` 형식을 읽는다. 동일한 key가 `.env`와 실제 환경변수에 모두 있으면 실제 환경변수가 이긴다.
이 흐름은 `.env.example`의 주석과 일치하며 SPEC §5.2, SPEC §5.5의 관찰 기준이 된다.

`Config` 생성 뒤 main 함수는 로그 레벨 설정을 해석해 기본 logger를 초기화하고 시작 정보를 출력한다. 출력에는
프로그램 시작, 선택된 provider, model, timeout처럼 비밀값이 아닌 설정만 포함한다. `LLM_API_KEY` 같은 비밀값은
로그에 출력하지 않는다. Phase 0은 외부 provider를 호출하지 않으므로, 설정값이 비어 있어도 provider별 필수성으로
실행 실패시키지 않는다. 실행은 성공 상태로 종료되어 `go run ./cmd/agent-runtime` 완료 조건을 만족한다
(SPEC §5.1, SPEC §5.3).

실패 흐름은 설정 파일 접근 자체가 아니라 파싱 가능한 형식과 timeout 값처럼 Phase 0에서 직접 해석하는 값에만
둔다. `.env` 파일이 없으면 정상 진행하고, `.env` 파일에 잘못된 줄이나 해석 불가능한 duration 값이 있으면
오류 메시지를 stderr 또는 로그로 출력하고 non-zero 상태로 종료한다. 이 경계는 "환경변수 로딩 가능"을
확인 가능한 동작으로 만들되,
외부 서비스 검증을 Phase 0에 끌어오지 않는다(SPEC §5.2).

## 3. 인터페이스

외부 실행 인터페이스는 `go run ./cmd/agent-runtime` 하나다. 인자나 subcommand는 Phase 0 완료 조건에 없으므로
추가하지 않는다. 실행 결과는 성공 상태와 기본 로그 출력으로 관찰한다(SPEC §5.1, SPEC §5.3).

환경 설정 인터페이스는 `.env.example`에 공개된 환경변수 이름과 process environment다. 실제 환경변수는 `.env`
값보다 높은 우선순위를 갖는다. 이 우선순위는 `.env.example`에 이미 문서화되어 있으므로 구현과 문서가 같은
contract를
따라야 한다(SPEC §5.2, SPEC §5.5).

패키지 내부 인터페이스는 `internal/config`가 제공하는 설정 로딩 함수와 `Config` 구조체다. main package는
환경변수 이름이나 `.env` 파싱 규칙을 직접 알지 않고, `Config` 값만 받아 logger 초기화와 로그 출력을 수행한다.
이 방향은 설정 책임을 한 곳에 모으고, 이후 Phase에서 provider client가 config 값을 재사용할 수 있게 한다
(SPEC §5.2, SPEC §5.3).

문서 인터페이스는 최상위 `README.md`, `ROADMAP.md`, `.env.example`, `.gitignore`다. Phase 0 구현이 환경변수 이름,
우선순위, 실행 방식, 생성 파일을 바꾸면 이 문서들이 같은 의미를 설명해야 한다(SPEC §5.5, SPEC §5.6).

## 4. 영향 범위

새로 생기는 런타임 파일은 `go.mod`, `cmd/agent-runtime/main.go`, `internal/config` 하위 Go 파일이다. 기존 문서는
Phase 0 구현과 맞지 않는 부분이 확인될 때만 갱신한다. `.env.example`은 설정 key가 추가되거나 기본값 설명이 구현과
달라질 때만 수정 대상이다. `.gitignore`는 현재 `.env`와 Go 산출물을 이미 무시하므로, Phase 0 구현 중 새 산출물
경로가 생기지 않는 한 변경하지 않는다(SPEC §5.4, SPEC §5.5, SPEC §5.6).

외부 서비스 영향은 없다. Phase 0은 Claude, Ollama, Tavily, Postgres, MCP, A2A와 통신하지 않는다. 네트워크 호출이
없는 구조가 `go run ./cmd/agent-runtime`의 재현성을 높이고, 외부 API key 없이도 완료 조건을 검증할 수 있게 한다
(SPEC §5.1, SPEC §5.2).

저장소 상태 영향은 Go module이 루트에 생기고, 이후 모든 내부 패키지 import 기준이 그 module path에 묶인다는
점이다.
이 결정은 이후 Phase의 패키지 import와 테스트 작성에 영향을 준다(SPEC §5.4).

## 5. Decision Points

1. Go module path
   - 옵션 A: `github.com/zipkero/agent-runtime`을 사용한다.
   - 옵션 B: `agent-runtime` 같은 로컬 module path를 사용한다.
   - trade-off: 옵션 A는 GitHub remote와 일치해 이후 import, tag, 외부 참조가 자연스럽다. 옵션 B는 짧고 로컬에서
     단순하지만, 원격 저장소 기준 문서와 패키지 import가 다시 바뀔 수 있다.
   - 채택안: 옵션 A.
   - 근거: git remote가 `https://github.com/zipkero/agent-runtime.git`로 확인되었고, 프로젝트는 docs, commit, tag로
     진행 상태를 추적한다.

2. `.env` 로딩 구현 방식
   - 옵션 A: 표준 라이브러리로 현재 `.env.example`에 맞는 단순 parser를 구현한다.
   - 옵션 B: `joho/godotenv` 같은 외부 라이브러리를 추가한다.
   - trade-off: 옵션 A는 새 의존성 없이 Phase 0 범위의 `KEY=VALUE` 파일을 처리할 수 있다. 옵션 B는 더 넓은 dotenv
     문법을 제공하지만, 현재 spec에 없는 문법 지원을 위해 의존성을 추가한다.
   - 채택안: 옵션 A.
   - 근거: Phase 0은 외부 연동보다 프로젝트 기반이 목적이고, `.env.example`은 주석, 빈 줄, 단순 key-value 형식만
     사용한다.

3. provider별 필수값 검증 위치
   - 옵션 A: Phase 0에서는 형식 검증과 기본값 적용만 하고, provider별 필수값 검증은 해당 provider 구현 Phase로
     미룬다.
   - 옵션 B: Phase 0에서 `LLM_PROVIDER=claude`이면 `LLM_API_KEY` 필수 같은 의미 검증까지 수행한다.
   - trade-off: 옵션 A는 외부 통신 없는 실행 가능한 기반을 보장하고, Phase 0 제외 범위를 지킨다. 옵션 B는 조기
     오류 발견에는 유리하지만, Phase 1 이후 provider contract를 Phase 0에서 먼저 고정한다.
   - 채택안: 옵션 A.
   - 근거: spec은 Phase 0에서 LLM provider 호출과 provider별 세부 설정 검증을 제외한다.

4. logger 구조
   - 옵션 A: `cmd/agent-runtime`에서 표준 라이브러리 logger를 초기화하고 출력한다.
   - 옵션 B: `internal/logger` 같은 별도 패키지를 만든다.
   - trade-off: 옵션 A는 `README.md`와 spec의 "별도 logger 패키지 없음" 제약을 따른다. 옵션 B는 재사용 지점은 만들지만,
     Phase 0 주요 패키지 범위를 넓힌다.
   - 채택안: 옵션 A.
   - 근거: Phase 0 주요 패키지는 `cmd/agent-runtime`과 `internal/config`이고, 로그 초기화는 진입점 책임으로
     명시되어 있다.
