# CLAUDE.md

이 파일은 Claude Code가 이 프로젝트에서 작업할 때 따라야 할 지침이다.

---

## 구현 원칙

1. **처음엔 단일 프로세스** — 처음부터 MSA로 쪼개지 않는다. 핵심 loop와 상태 모델을 이해하는 것이 먼저다.
2. **프레임워크보다 인터페이스** — 라이브러리 사용보다 planner / executor / tool / memory 경계를 직접 정의하는 것을 우선한다.
3. **상태는 반드시 분리** — request state / session state / working memory / long-term memory를 하나로 뭉개지 않는다.
4. **LLM은 부품이다** — 핵심은 LLM 호출 자체가 아니라 orchestration이다.
5. **단계마다 동작하는 결과물** — 문서만 쌓지 않는다. 각 Phase마다 CLI 또는 API로 실제 동작해야 다음 단계로 간다.

---

## 작업 방식

- 요청된 범위만 수정한다. 관련 없는 리팩토링이나 구조 변경은 하지 않는다.
- 새 의존성은 명확한 이유가 있을 때만 추가한다.
- 각 Phase의 Exit Criteria를 충족했는지 확인한 뒤 다음 단계로 넘어간다.
- 현재 진행 중인 Phase와 Task는 [PLAN.md](./PLAN.md)에서 확인한다.

### Task 완료 시

1. `PLAN.md`에서 해당 Task의 체크박스를 `[x]`로 업데이트한다.
2. 구현한 내용에 대해 상세 설명을 제공한다.
   - 무엇을 만들었는지 (파일, 타입, 함수 등)
   - 왜 이렇게 설계했는지 (인터페이스 경계, 의존 방향 등)
   - 다음 Task와 어떻게 연결되는지

---

## 패키지 경계 규칙

- `internal/planner` — Plan 결정만. State를 수정하지 않는다.
- `internal/executor` — Tool 실행 위임만. Planner 결과(`PlanResult`)를 입력으로 받는다.
- `internal/tools` — Tool 구현 + Registry + Router. Runtime 루프를 알지 않는다. `MemoryManager`가 필요한 tool은 인터페이스를 주입받아 사용하며, `internal/memory` 패키지에 직접 의존하지 않는다.
- `internal/state` — 상태 타입 정의만. 비즈니스 로직을 포함하지 않는다.
- `internal/agent` — Runtime loop + finish 조건 + retry 정책. 최상위 조율자.
- `internal/memory` — Session + Long-term memory. 저장소를 인터페이스로 분리한다.
- `internal/verifier` — Verifier + Reflector 인터페이스 및 구현체. `internal/agent`에 주입되며, `internal/agent`에 직접 의존하지 않는다.
- `internal/observability` — structured logger + context 키 관리 + OTel 초기화. 다른 internal 패키지를 참조하지 않는다. `log/slog`를 직접 사용하는 것은 이 패키지만 허용한다. 다른 패키지는 반드시 `observability.New()` / `observability.FromContext()`를 통해 logger를 사용한다.
- `internal/orchestration` — Multi-agent 조율. `internal/agent`(Runtime)를 재사용한다(`orchestration → agent`). `internal/agent`는 `internal/orchestration`을 알지 않는다(역방향 의존 금지).
- `internal/api` — HTTP 핸들러 + AsyncTask 타입 + 저장소 인터페이스. `internal/agent`와 `internal/queue`를 직접 알지 않는다. 핸들러는 `TaskQueue` 인터페이스와 `AsyncTaskRepository` 인터페이스만 주입받는다.
- `internal/queue` — TaskQueue 인터페이스 + Worker. Worker는 `internal/agent`(Runtime)와 `internal/api`(AsyncTaskRepository)에 의존한다(`queue → agent`, `queue → api`).
- `testutil/` — 테스트 전용 mock 구현체(MockLLMClient 등). 프로덕션 코드에서 import 금지. `internal/` 패키지를 참조할 수 있으나 참조 방향은 항상 단방향(testutil → internal).

순환 참조가 발생할 경우 `internal/types` 공유 타입 패키지로 해결한다 (`docs/architecture-overview.md` 참고).
