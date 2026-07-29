# 120 — WP9: 관리 표면

네 번째 층. 대시보드 계약이라 사용자에게 즉시 보이지만, 데이터 경로가 아니므로 앞의 셋보다
뒤에 온다.

## 담는 항목 (감사 13, 14, 15, 16)

### 1. provider 덮어쓰기가 API 키 풀을 삭제한다 (감사 14) — 가장 먼저

**데이터 손실**이라 이 유닛에서 1순위다. `providers.go:59-63`이 제출된 provider로 맵
엔트리를 통째 교체한다. 오라클은 제출값에 `apiKeyPool`이 없으면 기존 것을 보존하고
(`provider-routes.ts:120-124`), 새 키를 그 풀에 활성 키로 추가한다(`:127-129`).

대시보드에서 provider를 편집·재활성화·재import하면 **폴백 키가 전부 사라진다.** 다음 rate
limit에서 회전할 자격증명이 없다. go에 `config.AddAPIKey`(`api_keys.go:67-92`)가 이미
있는데 POST 핸들러가 부르지 않는다.

### 2. `/api/providers/test`가 프로덕션에서 미배선 (감사 13)

`server.go:519`가 `FetchModels`를 넘기지 않아 항상 501이다(`providers.go:231-237`).
대시보드의 연결 테스트가 정상 provider에도 "not implemented"를 낸다.

주입해도 계약이 다르다: 프로브 실패가 502인데 오라클은 **HTTP 200 + `{ok:false}`** 다
(`provider-routes.ts:312-367`). 대시보드가 평범한 연결 실패를 API 장애로 오해한다.

머지가 가져온 pinned transport(`src/lib/pinned-http.ts`)의 go 대응이 없으므로, 이 프로브를
배선할 때 어떤 클라이언트를 쓸지가 130과 얽힌다. 순서상 130을 기다릴지 이 사이클의 P가 정한다.

### 3. provider DTO 필드 누락 (감사 15)

`GET /api/providers`: `apiKeyTransport` 없음, `discovery`는 순서 목록에만 있고 채워지지
않음, `codexAccountMode`는 영속 값이 비면 안 나옴(`shared.go:116-125`, `providers.go:35-37`).

safeConfig는 16개 필드가 누락됐다 — `apiKeyTransport`, `freeTier`, 컨텍스트/출력 맵,
OpenRouter 라우팅, reasoning effort 맵, 각종 능력 제외 목록, registry `note`, 정규화된
Codex 모드.

대시보드가 provider 상태를 부정확하게 보여주고, 그 위에서 한 편집이 불완전한 스냅샷에
기반한다.

### 4. provider PATCH 필드·부수효과 누락 (감사 16)

`apiKeyTransport`, `note`, `codexAccountMode`를 모두 거부한다(`providers.go:252-304`).
Anthropic bearer/x-api-key 전환과 Codex pool/direct 전환이 go에서 불가능하다.

부수효과도 없다: 오라클은 모드 전환 시 쿼터 캐시를 비우고 스레드 친화성을 지우고 pool
모드면 쿼터를 prime한다(`provider-routes.ts:144-174`). go에 `ClearThreadAccountMap`
(`routing.go:178`)이 있는데 PATCH가 부르지 않아, 다른 경로로 모드가 바뀌면 낡은 결정이 남는다.

## 검증 계획

JSON 키 이름으로 단언한다 — 구조체 필드명이 아니라. 대시보드가 읽는 것이 키 이름이고,
타입이 컴파일되는 rename도 페이지를 깨뜨린다. 14번은 **풀이 실제로 보존되는지** 왕복
테스트가 필요하다.

## 범위 밖

오라클 수정, GUI 코드 수정.

---

## 결과 (커밋 `cf280990a`, `a128d9045`, `a2c72cc6d`)

감사 13·14·15·16을 닫았다. 16의 **codexAccountMode 부수효과**만 `wp9b`로 남겼다.

| 감사 | 처리 | 커밋 |
| ---: | --- | --- |
| 14 API 키 풀 삭제 | 닫음 | `cf280990a` |
| 13 `/api/providers/test` | 닫음 | `a128d9045` |
| 15 DTO 필드 누락 | 닫음 (`apiKeyTransport`, `note`) | `a2c72cc6d` |
| 16 PATCH 필드 | 닫음 (`apiKeyTransport`, `note`) | `a2c72cc6d` |
| 16 codexAccountMode 부수효과 | **wp9b로 분리** | — |

### 14번은 이 스윕에서 나온 유일한 데이터 손실이다

대시보드 편집 폼에 키 풀 입력이 없으므로, provider를 저장하면 제출된 값으로 엔트리가 통째
교체되면서 **풀 전체가 사라졌다.** provider를 잠깐 껐다 켜는 것만으로도 폴백 키가 전부 지워지고
디스크에서도 사라진다 — 되돌릴 방법이 없다.

go에 `config.AddAPIKey`가 처음부터 있었다. POST 핸들러가 부르지 않았을 뿐이다.

명시적으로 제출된 풀은 여전히 이긴다. "보존" 규칙이 덮어쓰기를 막아버리면 그것도 버그이므로
음성 사례로 고정했다.

### 13번: 왜 `FetchProviderModels`를 쓰지 않았나

오라클이 주석으로 이유를 밝혀뒀다(`provider-routes.ts:312-317`). 카탈로그 집계 경로는 실패 시
캐시·stale·configured로 **degrade한다.** 그것이 카탈로그 조립에는 옳지만 연결 테스트에는
정반대다 — 키가 폐기된 provider가 정적 카탈로그 덕분에 "통과"한다.

그래서 `codex.ProbeProviderModels`를 따로 뒀다. 실제 업스트림 증거만 보고한다.

계약도 바꿨다. 업스트림에 닿지 못한 것은 **실패한 연결에 대한 성공적인 보고**이므로 200 +
`ok:false`다. 502를 주면 대시보드가 평범한 잘못된 키를 API 장애로 표시한다.

오라클이 정직하게 답하고 go가 답하지 않던 경우 셋도 옮겼다: passthrough provider는 프로브할
`/models`가 없고, 정적 카탈로그 provider는 업스트림 미검증이며, 토큰을 읽을 수 없는 OAuth
provider는 **익명 요청을 보내 provider 탓을 하지 않고** "not logged in"이라고 말한다.

### 테스트가 두 번 나를 교정했다

- `apiKeyTransport` 픽스처를 `openai-chat`으로 썼더니 config 검증이 거부했다. 코드가 아니라
  픽스처가 틀렸다 — 그 필드는 anthropic 와이어 전용이고 검증이 제 일을 하고 있었다.
- 키 풀 픽스처의 ID를 `"one"`/`"two"` 같은 임의 문자열로 썼더니 `AddAPIKey`가 같은 키를
  중복 추가했다. 풀 엔트리 ID는 **키의 해시**이고, 그것이 오라클이 재제출된 키를 알아보는
  방식이다(`apiKeyPoolEntryId`). 픽스처를 실제 ID로 고쳤다.

### wp9b로 남긴 것

오라클의 `codexAccountMode` PATCH는 전용 부수효과 경로를 갖는다(`provider-routes.ts:144-175`):
다른 필드와 **상호 배타**, `openai` 전용, canonical provider 확인, 그리고 저장 후
`clearProviderQuotaCache` / `clearThreadAccountMap` / `primeCodexPoolQuotas`.

go의 `codex.Router`에 `ClearThreadAccountMap`은 있지만(`routing.go:178`) 관리 API가 라우터에
접근하는 경로와 쿼터 프라이밍 배선이 이 사이클의 범위를 넘는다. 필드만 받고 부수효과를
빠뜨리면 **모드는 바뀌었는데 스레드가 이전 계정에 계속 붙어 있는** 상태가 되므로, 반쪽으로
넣는 것이 넣지 않는 것보다 나쁘다.
