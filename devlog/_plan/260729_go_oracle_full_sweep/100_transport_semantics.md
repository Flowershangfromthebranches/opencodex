# 100 — WP7: 전송 계층 의미론

`080_external_audit_remeasure.md`의 두 번째 층. 매 턴 지나가는 경로라 잘못된 판정이 곧바로
"턴이 깨졌다"로 보인다. 090(자격증명·목적지)이 먼저인 이유는 요청이 올바른 provider에
가야 이 층의 수정이 의미를 갖기 때문이다.

## 담는 항목 (감사 1, 6, 7, 9, 10, 12)

의존 순서로 정렬했다. 1번이 가장 크고 나머지를 포함할 수도 있다 — 패스스루가 복원되면
파서를 지나지 않는 경로가 늘어나므로, 1번을 먼저 재고 나머지 범위를 다시 자른다.

### 1. 성공한 Responses를 재조립하지 않고 패스스루 (감사 1, P0)

오라클 `src/server/responses/core.ts:1392`가 routed compaction이 아닌 성공 Responses를
패스스루 분기로 보내고, `:1518`이 Content-Type이 없어도 스트리밍이면 SSE로 취급하며,
`:1730,1743`이 업스트림 본문을 그대로 돌려준다.

go는 `streamMode == "eager-relay"` **그리고** SSE content-type일 때만 무변환 릴레이한다
(`responses_core_port.go:690,842`). 그 외에는 파싱 후 재구성한다(`:737,802,945,990,1013`).
파서는 `function_call`만 툴콜로 인식하므로(`responses.go:746`) `custom_tool_call`,
`tool_search_call`, `web_search_call`이 유실된다.

**사용자가 겪는 것**: 모델이 정상 완료한 것처럼 보이는데 요청한 도구 동작이 사라진다.

두 방향이 가능하다.
- (a) auto 모드에서도 성공 패스스루를 복원한다 — 오라클과 같은 구조.
- (b) 파서가 미지 output item을 보존하게 한다.

오라클을 따르는 것은 (a)다. (b)는 새 item type이 나올 때마다 또 뒤처진다. B의 첫 작업으로
go의 relay 경로가 상태 기록·검사 side effect를 어디서 하는지 확인하고, 패스스루에서도 그것이
유지되는지 본다.

### 2. 중간 스트림 실패 꼬리 (감사 6)

오라클 `relay.ts:67-78`: 선행 `\n\n` → `event: response.failed` →
`{type:"upstream_error", code:"upstream_reset"}` + `error`와 `last_error` 둘 다 → `[DONE]`.
go `relay.go:158-166`은 선행 빈 줄이 없고 `server_error` + 숫자 502를 쓰며 `last_error`가 없다.

선행 빈 줄이 없으면 **부분 전송된 `data:` 프레임에 우리 `event:` 줄이 이어붙어** 별도
이벤트로 파싱되지 않는다. 프레이밍 문제라 클라이언트가 오류를 아예 못 본다.

### 3. 직접 스트림 preflight (감사 7)

오라클은 combo/failover일 때만 preflight한다(`core.ts:1976-1989`). go는 모든 non-eager
스트림을 preflight하고 실패 시 SSE 헤더 **전에** 502 JSON을 쓴다(`responses_core_port.go:764-772`).

감사의 "regardless"는 과장이었다 — disconnect류는 이미 면제된다(`:764-765`). 남은 것은
업스트림이 200 SSE로 시작해 첫 이벤트가 provider 오류인 경우다. 클라이언트는 이미 스트리밍
프로토콜을 골랐는데 JSON을 받는다.

### 4. Chat 오류가 code/type/status 유실 (감사 9)

go는 스트림·단항 모두 메시지 문자열로 축약하고(`chat.go:281-285,393-395`), bridge가
`event.Code`를 복사하지 않는다(`bridge.go:421-437`). `insufficient_quota`,
`model_not_found`, `cyber_policy` 같은 코드가 사라져 재시도/치명 분류가 달라진다.

`types.AdapterEvent`에 `Code` 필드는 이미 있다 — 채우지 않을 뿐이다.

### 5. 툴콜 조립 손상 (감사 10)

두 가지가 섞여 있다.
- index/id가 둘 다 없는 continuation을 **index 0**으로 폴백한다(`chat.go:336-339`).
  오라클은 마지막 활성 콜에 붙인다(`openai-chat.ts:657`). 병렬 툴콜에서 인자가 엉뚱한
  도구에 붙는다.
- 손상된 JSON을 `{}`로 대체한다(`chat.go:376-378,415-417`, `anthropic.go:568-570`,
  `responses.go:752-754`). 오라클은 **비어 있을 때만** `{}`를 쓴다(`bridge.ts:378`).
  잘린 호출이 기본값으로 실행될 수 있다.

### 6. service_tier를 일반 Chat provider에 전달 (감사 12)

`chat.go:203-204`가 무조건 넣는다. 오라클 Chat 본문 구성에는 없다. 엄격한
OpenAI 호환 provider에서 400을 만든다. 가장 작은 수정이자 가장 명확한 결함.

## 검증 계획

각 항목마다 활성화 증거를 요구한다. 특히 5번은 **음성 사례가 핵심**이다: 손상 JSON이
`{}`가 되지 않고 오류로 드러나는지, 그리고 정상 병렬 툴콜이 여전히 올바르게 조립되는지.

## 범위 밖

오라클 수정, 라이브 provider 호출.
