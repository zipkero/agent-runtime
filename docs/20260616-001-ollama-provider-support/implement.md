# ollama-provider-support — IMPLEMENT

> 순수 실행 체크리스트. 각 Task의 설계 근거는 analysis.md, 요구사항 레벨 완료 조건은 spec.md §5에 둔다.
> 순서는 의존성(line order)으로 표현한다. 체크박스 전환은 verify가 approved로 판단한 뒤에만 한다.

## Section: 설정 경계 (internal/config)

- [ ] task-001: config가 Ollama host·model을 읽고 검증하도록 전환
  - 목적: Ollama host·model이 설정되면 그 값으로 Config가 채워지고, model이 비어 있으면 설정 로딩이 오류를 반환한다. host는 미지정 시 기본값으로 채워진다.
  - 접근: Ollama host·model env 상수와 Config 필드를 추가하고, Load()의 무조건 ANTHROPIC_* 검증을 Ollama host·model 검증으로 바꾼다. host는 미지정 시 `http://localhost:11434` 기본값, model은 부재 시 error. 활성 model은 공용 `Model` 필드로 읽고, `AnthropicAPIKey` 필드는 검증 없이 유지한다.
  - 검증 조건:
    - 결과: Ollama model 부재 시 Load()가 error를 반환하고, host 미지정 시 기본 host가 채워지며, ANTHROPIC_* 부재만으로는 더 이상 실패하지 않는다. `cfg.Model`은 Ollama model에서 읽힌다.
    - 확인: config_test.go의 ANTHROPIC_* 필수 가정 케이스(`TestLoadRequiresAPIKey`, `TestLoadRequiresModel`, `TestLoadReadsModelFromEnv`, `TestLoadReadsFromDotEnvFile`, `TestRealEnvTakesPrecedenceOverDotEnv` 등 ANTHROPIC_* 필수/`Model=ANTHROPIC_MODEL`을 가정하는 케이스)를 새 검증 규칙(model 필수, host 기본값, ANTHROPIC_* 비필수)에 맞게 갱신하고, model 부재 error·host 기본값을 검증하는 케이스를 포함해 `go test ./internal/config/...` 통과.
  - 참조: SPEC §5.3 / ANALYSIS §1, §2(부팅 흐름), §3, §4, §5 D2

## Section: LLM provider 경계 (internal/llm)

- [ ] task-002: Ollama `/api/chat` 호출로 일반 chat 응답을 반환하는 OllamaClient 추가
  - 목적: 설정된 Ollama host·model로 Chat을 호출하면 Ollama 모델의 응답 텍스트가 assistant 메시지로 반환되며, host·model 부재 시 생성자가 오류를 반환한다.
  - 접근: net/http로 `POST {host}/api/chat`(body `{model, messages, stream:false}`)을 호출하는 LLMClient 구현체를 추가한다. 공개 생성자 `NewOllamaClient(cfg)`는 host·model 부재를 error로 반환하고, 테스트 주입용 내부 생성자가 base host·http.Client를 받는다. system/user/assistant text 블록을 wire 메시지로 변환하고, ctx.Err()를 먼저 존중하며 ctx 취소·연결 오류·비정상 status를 error로 표면화한다. 응답 `message.content`를 text 블록으로 환원한다.
  - 검증 조건:
    - 결과: httptest로 가로챈 `/api/chat`에 system·user 메시지와 `stream:false`가 실린 요청이 도달하고, 응답 text가 assistant 메시지로 반환된다. host·model이 비면 생성자가 error를 반환하고, ctx 취소 시 context error가 표면화된다.
    - 확인: httptest로 요청 변환(system 메시지·stream:false·model 사상)과 응답 변환(text 블록)을 관찰하는 테스트, 생성자 필수값 부재 error 테스트, ctx 취소 error 테스트를 추가하고 `go test ./internal/llm/...` 통과.
  - 참조: SPEC §5.1, §5.3 / ANALYSIS §1, §2(chat 왕복 흐름), §3, §5 D1, D4, D5

- [ ] task-003: OllamaClient의 tool calling 왕복 변환(tools·tool_calls·tool 결과·ID 매칭) 구현
  - 목적: agent가 Ollama 응답의 tool 호출을 받아 tool 결과를 다시 모델에 전달하고 최종 답에 도달하는 왕복이 동작하며, Ollama 응답에 호출 식별자가 없어도 호출-결과 매칭이 유지된다.
  - 접근: req.Tools를 `tools[{type:"function", function:{name, description, parameters}}]`로 변환(InputSchema에서 type/properties/required 분리·나머지 보존)한다. assistant 메시지의 tool_call 블록을 같은 메시지의 `tool_calls[]`로, RoleTool 메시지의 각 tool_result 블록을 개별 `{role:"tool", content, tool_name, tool_call_id}` 메시지로 1:N 변환한다. 응답 `message.tool_calls[]`를 tool_call 블록으로 환원하되, id가 비면 응답 내 등장 순번 기반 결정적 ID(`call_<n>`)를 채워 internal ToolCall.ID에 싣고, 요청 변환 때 그 ID를 wire로 되싣어 왕복 동안 동일 ID를 유지한다.
  - 검증 조건:
    - 결과: httptest로 가로챈 요청에 tools·assistant tool_calls·tool 결과 메시지가 올바로 사상되고, 응답 tool_call이 internal 모델로 환원되며, id 부재 응답에서도 결정적 ID가 채워져 ID 기반 매칭이 성립한다.
    - 확인: httptest로 tool 요청 변환(tools schema 분리·tool_calls·tool 결과 1:N)과 응답 변환(tool_call 환원·id 부재 시 결정적 ID 부여) + 동일 ID 왕복 유지를 관찰하는 테스트를 추가하고 `go test ./internal/llm/...` 통과.
  - 참조: SPEC §5.2 / ANALYSIS §1, §2(tool calling 왕복·ID 매칭 흐름), §3, §5 D3, D5

## Section: 조립 경계 (cmd/agent-runtime)

- [ ] task-004: 활성 LLM 호출 경로를 OllamaClient로 교체
  - 목적: 실행 시 client 생성이 Ollama로 이뤄져 정상 설정에서는 Ollama 모델 응답이 stdout에 출력되고, host·model 설정이 없으면 오류가 stderr에 출력되며 비정상 종료코드로 종료한다.
  - 접근: main의 client 생성 한 줄을 `NewClaudeClient`에서 `NewOllamaClient`로 교체한다. provider 분기·팩토리는 두지 않는다. `run`에 넘기는 `cfg.Model`은 Ollama model이 되며 buildRegistry·readPrompt·run 본문은 그대로 둔다.
  - 검증 조건:
    - 결과: 정상 설정에서 final 응답이 stdout·종료코드 0으로 관찰되고, host·model 부재 시 stderr 오류·비정상 종료코드로 끝난다. claude.go는 변경 없이 비활성으로 잔존한다.
    - 확인: `run()`에 stub을 주입하는 기존 main_test.go가 그대로 통과함을 확인하고, `go build ./...`와 `go test ./...` 전체 통과.
  - 참조: SPEC §5.1, §5.2, §5.3 / ANALYSIS §1, §2(부팅 흐름), §4
