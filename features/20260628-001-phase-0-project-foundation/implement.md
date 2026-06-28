
# Phase 0 Project Foundation 구현

## 체크리스트

- [x] task-001: 설정 로딩 패키지 작성
  - 목적: 실행 설정을 `.env` 파일과 실제 환경변수에서 읽고, 실제 환경변수가 `.env`보다 우선하는 동작을
    제공한다.
  - 접근: `internal/config`에 `Config`와 로딩 함수를 만들고, 표준 라이브러리 기반 단순 parser로 주석, 빈 줄,
    `KEY=VALUE` 형식을 처리한다. provider별 필수값 검증은 하지 않고, Phase 0에서 직접 해석하는 duration과 기본값만
    검증한다.
  - 검증 조건:
    - 결과: `.env`가 없어도 기본값과 실제 환경변수로 설정을 만들 수 있고, `.env`와 실제 환경변수가 같은 key를
      제공하면 실제 환경변수 값이 사용된다.
    - 확인: `go test ./internal/config`로 기본값, `.env` 로딩, 실제 환경변수 우선순위, 잘못된 duration 오류를 확인한다.
  - 참조: SPEC §5.2, SPEC §5.4, SPEC §5.5, ANALYSIS §1, ANALYSIS §2, ANALYSIS §5

- [x] task-002: Go module과 CLI 진입점 작성
  - 목적: 저장소 루트에서 `go run ./cmd/agent-runtime`을 실행하면 성공 상태로 종료되고 기본 로그가 출력된다.
  - 접근: `go.mod`의 module path를 `github.com/zipkero/agent-runtime`으로 만들고, `cmd/agent-runtime/main.go`에서
    `internal/config`를 호출한다. logger는 별도 패키지 없이 진입점에서 초기화하고, 비밀값을 제외한 시작 정보를
    출력한다.
  - 검증 조건:
    - 결과: `cmd/agent-runtime` CLI가 config 로딩 결과를 사용해 기본 로그를 출력하고 외부 provider 호출 없이 종료된다.
    - 확인: `go test ./...`와 `go run ./cmd/agent-runtime`을 실행해 빌드, 테스트, 실행 성공, 기본 로그 출력을 확인한다.
  - 참조: SPEC §5.1, SPEC §5.3, SPEC §5.4, ANALYSIS §1, ANALYSIS §2, ANALYSIS §3, ANALYSIS §5

- [x] task-003: Phase 0 문서와 ignore 규칙 정합성 확인
  - 목적: 로컬 실행에 필요한 환경변수 이름과 실행 방식이 문서에 남고, `.env`가 git 추적 대상에서 제외된다.
  - 접근: 구현 결과와 `.env.example`, `.gitignore`, `README.md`, `ROADMAP.md`를 비교해 불일치가 있으면 요청 범위 안에서
    갱신한다. 새 환경변수나 새 산출물 경로가 없으면 기존 문서를 유지한다.
  - 검증 조건:
    - 결과: `.env.example`은 로컬 실행에 필요한 환경변수 이름과 우선순위 설명을 제공하고, `.gitignore`는 `.env`를
      무시한다. 최상위 문서는 Phase 0 이후에도 프로젝트 목적, 단일 Runtime 진행 방식, Phase 진행 상태를 설명한다.
    - 확인: `git status --short`, `.env.example`, `.gitignore`, `README.md`, `ROADMAP.md`를 확인해 구현 결과와 문서가
      일치하는지 검토한다.
  - 참조: SPEC §5.5, SPEC §5.6, ANALYSIS §3, ANALYSIS §4
