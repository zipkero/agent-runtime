# ollama-provider-support — SPEC

## 승인 전 확인
- 범용 host 설정(`LLM_HOST`)은 Ollama에선 데몬 host, Claude에선 base URL override로 의미가 다르지만
  단일 키로 둔다(Claude는 보통 미설정). provider마다 의미가 다른 값을 한 키로 묶는 것이 맞는가?
  관련 본문: §3
- Ollama tool calling을 완료 조건에 포함하므로, tool calling을 지원하지 않는 모델만 쓰는 환경에서는
  §5.3 을 관찰로 검증할 수 없다. 검증에 tool-capable 모델을 사용할 수 있는가? 관련 본문: §5

## 1. 범위
- agent-runtime의 LLM 호출을 실행 시점에 provider(claude·ollama) 중 하나로 선택하는 구조로 만든다.
- provider 선택과 자격·접속 설정을 provider 접두사 없는 범용 환경변수로 받고, 선택된 provider에
  필요한 값만 조건부로 검증한다.
- Ollama provider가 일반 chat 응답과 tool calling(tool_call → tool_result 왕복)을 처리한다.
- Claude provider는 기존 호출 경로를 범용 설정에 맞춰 유지·연결한다.

## 2. 목표
- 상용 API 키 없이 로컬 Ollama 모델로 구동하는 것을 기본으로 하되, 환경변수 변경만으로 Claude로
  전환할 수 있게 한다.
- provider 차이(접속·프로토콜)를 공통 chat 계약 뒤로 흡수해 호출자(agent loop)가 provider를
  모르게 유지한다.

## 3. 제약
- 한 번의 실행에는 한 provider만 활성화된다.
- provider가 지정되지 않으면 ollama로 동작한다.
- Ollama의 tool calling은 tool calling을 지원하는 모델에서만 동작한다(모델 자체의 한계).
- 기존 provider-neutral chat 입출력 계약은 호출자 코드 변경 없이 그대로 재사용한다.
- 접속·자격 설정은 host XOR api key로 강제하지 않고, 각 provider가 필요한 범용 필드만 검증한다.

## 4. 제외 범위
- streaming 응답 처리.
- Claude·Ollama 외 provider(GPT 등) 추가.
- provider 실패 시 다른 provider로의 자동 fallback.
- 한 실행 안에서 여러 provider 동시 사용 또는 런타임 중 provider 전환.
- chat 외 기능(임베딩 등)의 provider 확장.

## 5. 완료 조건
1. provider를 ollama로(또는 미지정으로) 두고 실행하면 Ollama 모델로부터 응답을 받아 최종 답을
   stdout에 출력한다.
2. provider를 claude로 두고 실행하면 Claude 모델로부터 응답을 받아 최종 답을 stdout에 출력한다.
3. Ollama provider로 실행할 때, agent가 tool을 호출(tool_call)하고 그 결과(tool_result)를 다시
   모델에 전달해 최종 답에 도달하는 왕복이 동작한다(tool calling 지원 모델 기준).
4. 선택된 provider에 필요한 설정값이 없으면 그 provider 기준 오류 메시지를 stderr에 출력하고 비정상
   종료코드로 종료한다. 선택되지 않은 다른 provider의 설정값 부재는 실행을 막지 않는다.
5. 인식할 수 없는 provider 값으로 실행하면 오류 메시지를 stderr에 출력하고 비정상 종료코드로
   종료한다.
