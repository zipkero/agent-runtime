# ollama-provider-support

## 요약
agent-runtime의 LLM 호출을 실행 시점에 provider(claude·ollama)로 선택하는 구조로 만든다. 범용
`LLM_*` 설정 + provider별 조건부 검증을 두고, Ollama는 직접 HTTP `/api/chat`(chat·tool calling),
Claude는 기존 SDK 경로를 유지한다. 기본 provider는 ollama.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [ ] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-06-16: SPEC 작성
- 2026-06-16: ANALYSIS 작성
- 2026-06-16: SPEC·ANALYSIS 재작성 (Ollama 전용 교체 + 직접 HTTP 전환)
- 2026-06-16: IMPLEMENT 체크리스트 작성
- 2026-06-17: SPEC·ANALYSIS·IMPLEMENT 재작성 (범용 provider 선택 설계로 전환, task-001 재작업)
