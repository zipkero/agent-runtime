# ollama-provider-support — SPEC

## 승인 전 확인
- 이 feature는 런타임의 활성 LLM 호출 경로를 Ollama로 전환하고, 기존 Claude 호출 코드는 제거하지
  않되 비활성 상태로 둔다. Claude를 다시 활성 경로로 붙이는 일을 별도 후속 작업으로 미루는 것이
  맞는가? 관련 본문: §1·§4
- Ollama tool calling을 완료 조건에 포함하므로, tool calling을 지원하지 않는 모델만 쓰는 환경에서는
  §5.2 를 관찰로 검증할 수 없다. 검증에 tool-capable 모델을 사용할 수 있는가? 관련 본문: §5

## 1. 범위
- agent-runtime의 활성 LLM 호출 경로를 Claude에서 Ollama로 전환한다.
- Ollama provider가 일반 chat 응답과 tool calling(tool_call → tool_result 왕복)을 처리한다.
- Ollama 구동에 필요한 설정(host·model)을 환경변수로 받아 검증한다.
- 기존 Claude 호출 코드는 제거하지 않고 비활성 상태로 유지한다(후속 재연결 대비).

## 2. 목표
- 상용 API 키 없이 로컬 Ollama 모델만으로 agent-runtime을 구동할 수 있게 한다.
- 향후 Claude를 다시 붙일 수 있도록, LLM 호출을 provider 교체 가능한 형태(공통 chat 계약 뒤)로
  유지한다.

## 3. 제약
- 한 번의 실행에는 Ollama 한 provider만 활성화된다.
- Ollama의 tool calling은 tool calling을 지원하는 모델에서만 동작한다(모델 자체의 한계).
- 기존 provider-neutral chat 입출력 계약은 호출자 코드 변경 없이 그대로 재사용한다.

## 4. 제외 범위
- streaming 응답 처리.
- Claude를 활성 호출 경로로 다시 연결하는 작업(코드는 유지하되 이번 범위 밖).
- 실행 시점에 여러 provider 중 하나를 고르는 선택 메커니즘(LLM_PROVIDER 등 provider 스위칭).
- Claude·Ollama 외 provider(GPT 등) 추가.
- chat 외 기능(임베딩 등)의 provider 확장.

## 5. 완료 조건
1. 설정된 Ollama host·model로 실행하면 Ollama 모델로부터 응답을 받아 최종 답을 stdout에 출력한다.
2. agent가 tool을 호출(tool_call)하고 그 결과(tool_result)를 다시 모델에 전달해 최종 답에 도달하는
   왕복이 동작한다(tool calling 지원 모델 기준).
3. Ollama 구동에 필요한 설정값(host 또는 model)이 없으면 오류 메시지를 stderr에 출력하고 비정상
   종료코드로 종료한다.
