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

---

## 결과 1차 (커밋 `9c67cd17d`, `494ed13fe`, `69fcbeb67`)

6개 항목 중 **4개를 닫았다.** 감사 1(패스스루)과 7(preflight)은 남았다 — 이유는 아래.

| 감사 | 처리 | 커밋 |
| ---: | --- | --- |
| 6 SSE 프레이밍 | 닫음 | `9c67cd17d` |
| 12 service_tier | 닫음 | `9c67cd17d` |
| 10 툴콜 조립 | 닫음 | `494ed13fe` |
| 9 오류 분류 | 닫음 | `69fcbeb67` |
| 1 패스스루 | **wp7b로 분리** | — |
| 7 preflight | **wp7b로 분리** | — |

### 감사 10에서 문서가 틀렸던 것

계획은 "`{}` 대체 3곳"을 모두 결함으로 봤다. 오라클을 직접 읽으니 **셋 중 하나는 옳았다.**

- `chat.go` 스트리밍, `anthropic.go` 스트리밍 → 오라클은 raw 인자를 그대로 흘린다
  (`openai-chat.ts:666`, `anthropic.ts:803`). 결함 맞다.
- `responses.go:752` → **history item 파서**다. 오라클도 `{}`로 기본값을 주고 경고를 남긴다
  (`parser.ts:442-452`). 이유가 문서화돼 있다: 오염된 history item 하나가 이후 모든 턴을
  400으로 만든다. 그대로 두고 주석으로 의도를 남겼다.

"같은 패턴이니 같은 결함"이 아니었다. 세 곳의 **맥락**이 달랐다.

### 감사 10의 실제 피해는 문서보다 컸다

문서는 "인자가 엉뚱한 도구에 붙는다"고 썼는데, 재현해보니 index/id 없는 continuation이
index 0으로 가면서 **두 콜이 아예 하나로 합쳐졌다.** 병렬 툴콜 하나가 통째로 사라진다.

부수적으로 mixed-keying 구조 탐색이 Go 맵을 순회하고 있었다 — 어느 콜이 id-only
continuation을 흡수하는지가 맵 순회 순서에 달려 있었다. 기록된 order를 걷도록 바꿨다.

### 감사 9는 두 층이었다

adapter가 code/type/status를 버리는 것이 절반이고, **bridge가 `event.Code`를 복사하지 않는
것이 나머지 절반**이다. 어느 한쪽만 고치면 분류가 여전히 클라이언트에 닿지 않는다.
bridge 쪽은 `classifyError`가 status로부터 type을 다시 유도하므로, 업스트림이 스스로 밝힌
type을 그 뒤에 복원해야 한다(오라클 `bridge.ts:73`이 같은 순서로 한다).

### 테스트를 한 번 잘못 썼다

`accumulateChatCalls` 테스트를 Go `int`로 `index`를 넣어 만들었더니 실패했다. 원인은 코드가
아니라 테스트였다 — 실제 입력은 `encoding/json`에서 오므로 모든 숫자가 `float64`다. int로는
`index`가 읽히지 않아 모든 콜이 한 키로 뭉친다. 픽스처를 `float64`로 고치고 그 이유를
주석으로 남겼다.

### 1과 7을 분리한 이유

둘은 `responses_core_port.go`의 같은 분기 구조를 건드리고, 문서 스스로 "1번을 먼저 재고
나머지 범위를 다시 자른다"고 적어뒀다. 앞의 4개는 서로 독립적이라 한 사이클에 묶는 것이
검증을 흐리지 않았지만, 1·7은 relay 경로의 상태 기록·검사 side effect를 어디서 하는지부터
확인해야 하므로 자기 P가 필요하다. `wp7b-passthrough-preflight`로 등록했다.

---

## 결과 2차 — 감사 1·7 (커밋 `07eb21136`, `14d4f54b0`)

### 감사 1: 문서가 제안한 두 방향 중 어느 것도 아니었다

문서는 (a) 성공 패스스루 복원 또는 (b) 파서가 미지 item을 보존, 둘 중 (a)를 권했다.
재측정하니 **범위가 문서보다 훨씬 좁았다.**

go의 bridge는 이미 `custom_tool_call`·`tool_search_call`을 **내보낸다**
(`bridge.go:542,548`). 클라이언트가 선언한 tool 종류로부터 item type을 재구성하기 때문이다
(`FreeformTools`/`ToolSearchTools`). 즉 우리 쪽에서 시작된 호출은 이미 정상 왕복한다.

실제 유실은 **업스트림이 그 item type으로 응답할 때**뿐이었다. `toolCallFromResponseItem`이
`function_call`만 통과시켰다(`responses.go:748`). 요청 파서는 셋 다 알고 있는데
(`responses.go:349`) 응답 경로만 눈이 멀어 있었다.

그래서 (a)의 아키텍처 변경 없이 파서 게이트만 열어 닫았다. `web_search_call`은 오라클이
의도적으로 버리므로(`parser.ts:492-497`) 제외를 유지하고 그 사실을 테스트로 고정했다.

### 그 과정에서 인자 처리 버그 둘이 더 나왔다

- 완료 item 병합이 누적된 delta를 **유효한 JSON일 때만** 채택했다. 잘린 스트림이 `{}`가
  된다 — chat 경로에서 이미 고친 것과 같은 결함이다.
- item이 열릴 때 `{}` 자리표시자를 저장하면서 delta가 거기에 이어붙어 `{}{"q":"x"}`가 됐다.

**494ed13fe에서 내가 쓴 주석도 틀렸다.** `toolCallFromResponseItem`은 history 전용이 아니라
라이브 스트림도 처리한다. 빈 인자는 여전히 `{}`(no-arg 케이스)이고, **비어 있지 않은데
유효하지 않은** 것만 그대로 보존한다.

### 감사 7: "regardless"가 과장이라던 080의 지적이 맞았다

오라클은 preflight를 **combo attempt에서만** 한다(`core.ts:1980`, `options.comboAttempt` 게이트).
go는 모든 non-eager 스트림에서 했다. 그래서 평범한 429가 클라이언트가 요청한 SSE 스트림 대신
JSON 502로 나갔다 — 프록시 자체가 깨진 것처럼 보이고, 원인을 설명할 `response.failed` 이벤트도
사라진다.

**그런데 한 경우는 pre-stream이어야 한다.** 기존 테스트
`TestResponsesCorePreflightsBeforeCommittingSSEHeaders`가 그것을 지키고 있었다.

잘못된 chunked body는 **유효한 HTTP 응답이 아예 없었던** 것이다. Bun은 `fetch()` 안에서
거부하므로 오라클은 스트림 없이 502를 준다. go의 http 클라이언트는 응답을 받아들이고 첫
read에서야 실패한다 — 그래서 여기서 adapter 오류로 나타난다. 같은 업스트림 조건에 대해 같은
클라이언트 가시 결과를 내려면 첫 이벤트에서 잡아야 한다. 이 파일의 `normalizePreflightError`가
정확히 그 번역을 위해 존재했다는 것이 근거다.

게이트는 `combo attempt || transport-level failure`가 됐고, `isTransportLevelPreflightError`는
chunked 인코딩 마커 둘로 좁게 유지했다. 그 테스트는 **수정 없이 통과한다** — 버그에 기댄
테스트가 아니라 런타임 차이를 지키는 테스트였다.
