# AI Agent Runtime

LangChain, LangGraph 같은 고수준 프레임워크 없이 AI Runtime의 핵심 구조를 직접 구현하며 익히기 위한 커리큘럼.

---

## 프로젝트 소개

이 프로젝트는 "프레임워크 사용자"가 아닌 "runtime 설계자" 관점으로 agent system을 이해하는 데 목적이 있다.

planner, executor, tool router, state, memory, verifier, retry policy, async task queue + worker 같은 runtime 구성요소를 직접 나누고 연결하면서, agent system이 실제로 어떻게 굴러가는지 손으로 체득하는 커리큘럼이다.

---

## 왜 프레임워크를 쓰지 않는가

LangChain이나 LangGraph 같은 프레임워크는 데모를 빠르게 만드는 데는 편하다. 하지만 아래 질문에 답하려면 오히려 추상화가 방해가 된다.

- agent loop는 어떤 조건에서 반복되고 종료되는가
- planner는 어떤 형식의 결정을 내려야 하는가
- tool router는 planner 결과를 어떻게 실제 실행으로 연결하는가
- session state와 memory는 어떻게 구분해야 하는가
- 실패, retry, timeout, verification은 어디서 처리해야 하는가
- runtime을 서비스 구조로 확장하려면 어떤 경계가 먼저 필요한가

추상화가 두꺼우면 loop가 어떻게 도는지, state가 어떻게 흘러가는지 보이지 않는다. 직접 만들어야 설계 감각이 생긴다.

---

## 학습 목표

이 커리큘럼을 따라가면 최종적으로 아래를 설명하고 구현할 수 있어야 한다.

- 최소 agent loop를 직접 구현할 수 있다
- planner, executor, tool router의 책임을 분리할 수 있다
- request state, session state, working memory, long-term memory를 구분할 수 있다
- tool registry와 tool interface를 직접 설계할 수 있다
- verifier, reflector, retry policy, failure handler로 제어 흐름을 runtime에 녹일 수 있다
- 단일 agent 구조를 multi-agent orchestration으로 확장할 수 있다
- CLI 수준의 실행기를 HTTP API + async task queue + worker 구조로 발전시키는 방향을 설명할 수 있다
- OTel 기반 observability와 policy layer를 runtime에 연동할 수 있다

---

## 비목표

이 커리큘럼은 아래를 우선 목표로 두지 않는다.

- production-grade agent framework를 처음부터 완성하는 것
- 브라우저 자동화 중심 에이전트
- 자율 코딩 에이전트
- 특정 프레임워크 wrapper
- 화려한 데모를 빠르게 만드는 것

핵심은 "runtime 제어 흐름과 상태 모델을 직접 구현하며 이해하는 것"이다.

---

## 최종 구성요소 목록

| 구성요소 | 다루는 Phase |
|---|---|
| Runtime | Phase 1 |
| Agent | Phase 1, 6 |
| Planner | Phase 1, 3 |
| Executor | Phase 1, 3 |
| Tool / Tool Registry / Tool Router | Phase 2 |
| Observability (Structured Logger) | Phase 3 |
| Session | Phase 4 |
| Memory (Working / Long-term) | Phase 4 |
| MemoryManager | Phase 4 |
| Verifier | Phase 5 |
| Reflector | Phase 5 |
| RetryPolicy + FailureHandler | Phase 5 |
| TaskDecomposer | Phase 6 |
| Task / Workflow | Phase 6 |
| HTTP Handler | Phase 7 |
| AsyncTask + TaskQueue + Worker | Phase 7 |
| PolicyLayer | Phase 8 |
| TokenTracker | Phase 8 |
| Observability (OTel Trace) | Phase 8 |

---

## 기술 스택

**언어:** Go

**1차 스택 (Phase 0~6)**

- OpenAI API (ChatGPT)
- Redis
- Postgres
- Docker Compose

**후반 확장 스택 (Phase 7~9)**

- `golang.org/x/sync` — errgroup 기반 병렬 실행 (Phase 5)
- OpenTelemetry (Phase 8 — 필수)
- Kafka (Phase 8 완료 후 선택적 도입 가능, 커리큘럼 필수 범위 아님)
  - Phase 7의 task queue는 InMemoryTaskQueue(buffered channel)로 구현하며, Kafka는 인터페이스 교체 방식으로 추후 선택 도입
- pgvector / Qdrant (Phase 9 이후 선택적 도입 가능)
- Prometheus / Grafana / Kubernetes (커리큘럼 범위 밖)

---

## 전체 로드맵

```
Phase 0  준비 (환경설정 + 용어 + 설계)
Phase 1  최소 Agent Loop
Phase 2  Tool Registry + Tool Router
Phase 3  Planner 고도화 / LLM 연결
Phase 4  Session / State / Memory 분리
Phase 5  Verifier / Retry / Concurrency
Phase 6  Multi-Agent Orchestration
Phase 7  Runtime 서비스화
Phase 8  운영 고도화
Phase 9  문서화 / 포트폴리오
```

> Phase 3(Planner/LLM)이 Phase 4(Session/Memory)보다 먼저인 이유:
> LLM이 없는 상태에서 session을 붙이면 실제 동작 검증이 불가능하다.
> LLM planner가 먼저 동작해야 session 연결 후 대화 맥락이 제대로 흘러가는지 확인할 수 있다.

상세 Task 목록은 [PLAN.md](./PLAN.md)를 참고한다.

---

## 로컬 실행 방법

### 전제 조건

- Go 1.26 이상
- Docker / Docker Compose (통합 테스트 실행 시 필요)
- `.env` 파일 설정 (`.env.example` 참고)
- `make` (Windows는 기본 미포함 — `choco install make` 또는 `scoop install make`로 설치)

### 빌드 및 테스트

```bash
# 빌드 검증
make build

# 단위 테스트 (integration 제외, CI 실행 타겟)
make test-unit

# 통합 테스트 (Redis, Postgres 컨테이너 필요)
docker-compose up -d
make test-integration
```

### 인프라 기동

```bash
# Redis + Postgres 컨테이너 기동
docker-compose up -d

# 컨테이너 종료
docker-compose down
```

> `//go:build integration` 태그가 붙은 테스트 파일은 `make test-unit`에서 제외된다.
> 통합 테스트는 Redis/Postgres 실제 연결이 필요하므로 반드시 `docker-compose up` 이후 실행한다.
