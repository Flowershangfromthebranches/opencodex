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
