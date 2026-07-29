# 013 — L4 인벤토리: `src/server/` 최상위

28파일 / 8,364줄. 대응 go: `go/internal/server`. exported 291개를 스윕했다.

## 확정 결함 1건

### `GET /v1/opencodex/artifacts/{id}`가 go 서버에 등록되지 않았다 (UNWIRED, user-visible)

오라클 `src/server/index.ts:505-532`가 auth/CORS 게이트 뒤에서 이미지 아티팩트를 서빙한다
(content type, `Cache-Control: private, max-age=3600`, `X-Content-Type-Options: nosniff`).

go에는 헬퍼가 **있다**: `internal/images/artifacts.go:174` `ResolveArtifactPath`,
`:195` `ArtifactContentType`, `:32` `ArtifactHTTPPrefix = "/v1/opencodex/artifacts"`.
참조 스윕 결과 셋 다 자기 선언과 URL 조립(`:166`)뿐이고, `server.go:477-479`는 images/search
POST만 등록한다. 요청은 `server.go:523`의 미지 `/v1/*` JSON 404로 떨어진다.

사용자가 겪는 것: go 이미지 브리지가 만들어 모델에게 보여주는 아티팩트 URL이 프록시에서
404가 된다.

최소 수정: `server.go`에 접두사 핸들러를 등록하고 `ResolveArtifactPath`/`ArtifactContentType`를
호출, 기존 auth/CORS 미들웨어 아래에서 헤더 두 개와 함께 파일 서빙.

## 처분한 미참조 export

`Ready`(go는 `/health/startup` 사용), `PublicProviderBaseURL`(management에 동등물),
`SplitHostPortDefault`/`ValidateConfiguredPort`(테스트 지원), `NewManagementAPI`/
`ManagementRoutes`(L5 소관), `CompletedResponse`(실제 재조립은 `SSEInspector` 소비자 경유),
`SealIdentity`/`NoteSendEstimate`(추적은 `responses_request_tracking.go:63-91`),
`Latest`(활성 표면은 `Snapshot`).

## OK로 확인된 축

라우트 구성 전반, WebSocket 비활성 시 426, auth/CORS 신뢰 경계(go가 더 엄격하나 동등 이상),
요청 압축 해제(256MiB 캡), draining 503 + `Retry-After: 5`, `/healthz`, GUI 정적 서빙과 SPA
폴백, Chat Completions·Claude Messages 호환, 이미지/검색 사이드카 릴레이, live/realtime,
SSE 릴레이와 하트비트·터미널 감지·합성 incomplete 꼬리, 요청 로그 링과 대화 필터, 메모리
워치독, 포트 선택/회수, 런타임 liveness, 시작 액션과 Windows 트레이(go에서는 CLI 런타임
백엔드 소유), Windows CLOSE_WAIT 정리.

정적 grep 오탐 재확인: `wsbridge_protocol.go:98`이 `"response." + status`로 이벤트명을
조립하므로 문자열 검색의 부재 보고는 무효.

## 검증하지 않은 것

라이브 HTTP 호출·테스트 미실행. management/responses 하위는 L5 담당.
