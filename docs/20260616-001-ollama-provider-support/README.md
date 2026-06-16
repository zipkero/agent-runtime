# ollama-provider-support

## 요약
agent-runtime의 활성 LLM 호출 경로를 Claude에서 Ollama로 전환한다(직접 HTTP `/api/chat`). 로컬
Ollama 모델로 chat·tool calling을 구동하며, 기존 Claude 코드는 후속 재연결을 위해 비활성으로 둔다.

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
