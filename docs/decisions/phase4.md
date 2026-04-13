# Phase 4 설계 결정 기록

## 1. RequestState / SessionState 분리

### 결정: 단일 Run 범위와 세션 범위를 별도 struct로 분리

`RequestState`는 단일 `Run()` 호출 범위의 데이터(RequestID, UserInput, ToolResults, ReasoningSteps)를 담고, `SessionState`는 여러 `Run()` 호출을 넘어 지속되는 데이터(SessionID, RecentContext, ActiveGoal)를 담는다. `AgentState`가 두 struct를 합성한다.

**왜:**
- 단일 요청에서만 유효한 데이터(ToolResults, ReasoningSteps)와 세션 전체에 걸친 데이터(RecentContext, ActiveGoal)의 생명주기가 다르다. 하나로 뭉개면 "이번 요청의 ToolResults"와 "이전 요청의 RecentContext"를 같은 시점에 직렬화해야 하는데, 저장 시점과 TTL이 다르므로 관리가 복잡해진다.
- Phase 6 multi-agent에서 각 agent가 자신의 RequestState를 독립적으로 갖되 SessionState는 공유할 수 있다. 분리하지 않으면 agent별 격리가 어렵다.
- Phase 7 HTTP API에서 RequestState는 단일 요청 lifecycle과 1:1로 매핑되고, SessionState는 클라이언트가 session ID를 통해 연속성을 유지하는 데 사용된다.

**대안으로 고려한 방식:**
- 단일 State struct에 모든 필드를 넣고 `IsSessionField` 태그로 구분 → 직렬화/역직렬화 로직이 태그 기반으로 복잡해지고, 실수로 세션 필드를 요청 범위에서 덮어쓸 위험이 있다.

**구현 포인트:**
- `AgentState.Session`은 `*SessionState` (포인터)로 nil 허용 → anonymous 요청(세션 없음)을 자연스럽게 표현한다.
- `SessionState`는 `SessionRepository` 인터페이스를 통해 저장소에 영속화된다. `RequestState`는 영속화하지 않는다.

---

## 2. Memory struct의 `internal/types` 배치

### 결정: `types.Memory`를 `internal/types` 패키지에 정의

`Memory` struct(ID, UserID, Content, Tags, CreatedAt)를 `internal/types/memory.go`에 배치한다.

**왜:**
- 직접적인 원인은 `internal/state` 패키지이다. `AgentState.RelevantMemories []types.Memory` 필드가 있으므로, Memory를 `internal/memory`에 정의하면 `state → memory` 의존이 생긴다. `internal/memory`는 이미 `state.SessionRepository`를 통해 `state`를 참조하므로 순환참조가 발생한다.
- `internal/memory`(저장소 구현)와 `cmd/agent-cli/main.go`(Memory 생성·저장)도 이 타입을 참조한다. `internal/agent`(Runtime)는 `AgentState`를 통해 간접 참조하며 직접 import하지 않는다.
- `internal/types`는 순환참조 해소를 위한 공유 타입 패키지로 이미 `ToolResult`, `PlanResult` 등을 보유하고 있다. Memory도 같은 위치에 두면 일관성이 유지된다.

**대안으로 고려한 방식:**
- `internal/memory` 패키지 내부에 정의 → `internal/state`가 `internal/memory`를 import해야 하고, `internal/memory`도 `internal/state`를 import하므로 순환참조.
- `internal/state`에 정의 → Memory는 상태(state)가 아니라 장기 저장 데이터이므로 의미가 맞지 않는다.

**향후 확장:**
- `internal/types`에 타입이 과도하게 쌓이면 `internal/types/memory`, `internal/types/plan` 등 하위 패키지로 분리할 수 있다. 현재 규모에서는 단일 패키지로 충분하다.

---

## 3. MemoryManager 파사드 패턴

### 결정: SessionRepository + MemoryRepository를 단일 인터페이스로 캡슐화

`MemoryManager` 인터페이스가 `LoadSession`, `SaveSession`, `SaveMemory`, `LoadRelevantMemory` 4개 메서드를 노출한다. `DefaultMemoryManager` 구현체가 내부적으로 `SessionRepository`와 `MemoryRepository`에 위임한다.

**왜:**
- Runtime이 세션 로드, 메모리 조회, 메모리 저장을 각각 별도 저장소에 요청하면 Runtime의 의존 대상이 3개 이상으로 늘어난다. 파사드로 묶으면 Runtime은 `MemoryManager` 하나만 의존한다.
- 테스트 시 `MemoryManager` mock 하나만 준비하면 되므로 테스트 setup이 단순해진다.
- 저장소 교체(InMemory → Redis, InMemory → Postgres)가 `DefaultMemoryManager` 생성자 인자만 바꾸면 되므로 Runtime 코드에 영향이 없다.

**대안으로 고려한 방식:**
- Runtime이 `SessionRepository`와 `MemoryRepository`를 개별 필드로 보유 → 두 저장소의 조합 로직(예: 세션 로드 시 관련 Memory도 함께 조회)을 Runtime에 넣어야 한다. Runtime의 책임이 늘어난다.

**파사드 내부 구현:**
- `LoadRelevantMemory(ctx, userInput)` 내부에서 `extractTags()` 함수로 userInput에서 2자 초과 단어를 태그로 추출한다. 이는 단순 키워드 매칭이며, 향후 임베딩 기반 유사도 검색으로 교체할 수 있다.
- 기본 조회 제한: `defaultRelevantMemoryLimit = 10`.

---

## 4. RedisSessionRepository AOF 설정

### 결정: Redis `--appendonly yes`로 AOF 활성화

Docker Compose에서 Redis 7 Alpine 이미지에 `--appendonly yes` 플래그를 적용한다.

**왜:**
- SessionState는 사용자의 대화 연속성을 보장하는 핵심 데이터이다. Redis 프로세스가 재시작되면 RDB 스냅샷 방식은 마지막 스냅샷 이후의 데이터를 잃는다. AOF는 모든 write 명령을 로그에 기록하므로 데이터 손실이 최소화된다.
- Phase 4 Exit Criteria에 "Redis 재시작 후 세션 복원 확인"이 명시되어 있다. RDB만으로는 재시작 시점에 따라 복원이 보장되지 않는다.

**구현 포인트:**
- TTL 없음 (`Set(ctx, key, data, 0)`) → 세션은 명시적 삭제까지 유지된다. 향후 세션 만료 정책이 필요하면 TTL을 설정하거나 별도 정리 작업을 추가한다.
- Key prefix `"session:"` → Redis 내에서 세션 데이터를 네임스페이싱한다.
- `redis.Nil` 처리 → 존재하지 않는 세션 ID로 조회 시 에러 대신 빈 `SessionState`를 반환한다. 이를 통해 신규 세션과 기존 세션을 호출자가 구분할 수 있다(SessionID가 빈 문자열이면 신규).

**대안으로 고려한 방식:**
- RDB 스냅샷만 사용 → 재시작 시 데이터 손실 가능. Exit Criteria 불충족.
- AOF + RDB 혼합 → 복원 안정성은 높지만, 현재 규모에서 과도한 설정. 필요 시 추가 가능.

---

## 5. Long-term Memory 주입 방식: C안(Run() 시작 시 1회 주입) 채택

### 결정: `Runtime.Run()` 진입 직후 `MemoryManager.LoadRelevantMemory()` 1회 호출, 결과를 `AgentState.RelevantMemories`에 저장

Loop 내에서 재조회하지 않는다. Memory 저장은 `Run()` 외부(호출자, 예: `main.go`)에서 수행한다.

**왜:**
- **A안(Tool로 구현)**: LLM이 `recall_memory` tool을 호출해야 Memory를 조회하는 방식. LLM이 tool 호출을 빠뜨리면 Memory 없이 응답할 수 있고, 매 step마다 불필요하게 호출할 수도 있다. 조회 시점이 LLM의 판단에 의존하므로 예측 불가능하다.
- **B안(매 Step 자동 조회)**: Loop의 매 step마다 자동으로 Memory를 조회하는 방식. DB 접근이 step 수에 비례해 증가한다. 대부분의 step에서 동일한 userInput 기반 조회이므로 중복 호출이다.
- **C안(Run 시작 시 1회)**: userInput이 확정된 시점에 1회만 조회하므로 DB 접근이 최소화된다. 조회 결과가 `AgentState.RelevantMemories`에 저장되어 Planner가 system prompt에 포함할 수 있다. Loop 내에서 Memory가 변하지 않으므로 동작이 결정적이다.

**Memory Save 분리 근거:**
- `Run()` 내부에서 저장하면 Runtime이 "어떤 결과를 Memory로 남길지" 판단해야 한다. 이는 Runtime의 책임(loop 조율)을 넘어선다.
- 호출자(`main.go`, 향후 HTTP handler)가 `Run()` 결과를 보고 저장 여부를 결정하면, 저장 정책을 Runtime 수정 없이 변경할 수 있다.
- 실패/중단된 요청의 결과를 저장하지 않는 로직도 호출자에서 자연스럽게 처리된다(`FinalAnswer`가 빈 문자열이면 저장하지 않음).

**Planner 연동:**
- `LLMPlanner.Plan()` 호출 시 `AgentState.RelevantMemories`가 비어 있지 않으면 system prompt에 "참고할 과거 기억" 섹션으로 포함된다.
- 이를 통해 LLM이 과거 상호작용의 맥락을 반영한 응답을 생성할 수 있다.
