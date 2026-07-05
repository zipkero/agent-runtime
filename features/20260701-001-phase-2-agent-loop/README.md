# Phase 2 Agent Loop

## 개요

Agent가 단발 LLM 호출이 아니라 메시지 상태를 유지하며 반복 판단하는 구조임을 Runtime 내부 API로 표현한다.
이 단계는 이후 Tool Calling Runtime이 붙을 수 있도록 final answer, tool call 대기, max step, error 상태와 trace
기반을 마련한다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 이력
- 2026-07-01: SPEC 작성
- 2026-07-02: ANALYSIS 작성
- 2026-07-04: IMPLEMENT 체크리스트 작성
- 2026-07-05: IMPLEMENT 완료
