# 021 — L12 인벤토리: chat / combos / grok / images / vision / web-search

32파일 / 7,212줄. 대응 go: `internal/{chat,combos,grok,images,vision,search}`.

## 확정 결함

### 1. 비디오 브리지가 통째로 미이식이다 (MISSING, user-visible)

오라클은 이미지와 비디오를 한 미디어 루프에서 다룬다(`images/loop.ts:242`, 비디오 분기
`:595-668`). 합성 도구 `video_gen`(`synthetic-tool.ts:91,111`), 계획(`plan.ts:92`),
xAI 비디오 submit/poll(`xai-video-client.ts:65`), 하트비트와 유료 호출 3회 상한
(`fulfill-video.ts:40,83`)이 전부 있다.

go는 이미지 루프만 구현한다(`images/bridge.go:194-282`). `video_gen`, `videoBridgeEnabled`,
`BuildVideoTool`, xAI 비디오 API 모두 비테스트 프로덕션 참조가 없다.

사용자가 겪는 것: `images.videoBridgeEnabled`를 켜도 **아무 일도 일어나지 않는다.** 오류가
아니라 기능 부재라 신고되지 않는다.

### 2. vision 데이터 URL 검증이 오라클보다 엄격하다 (DIVERGENT, user-visible)

go `vision/validate.go:95-109`는 이미지 치수를 디코드하고 MIME이 실제 바이트와 맞는지
요구한다. 오라클 `vision/describe.ts:28-41`은 허용 MIME과 추정 크기만 본다. TS가 사이드카로
넘길 데이터 URL을 go는 사이드카 실행 전에 거부/제거할 수 있다.

fail-closed 방향이라 위험하진 않지만 동작이 다르므로 의도를 정해야 한다.

### 3. combo 기본 effort 적용 조건 (관찰 필요)

오라클 `combos/request.ts:17`은 대상이 지원할 때만 기본 effort를 적용하고, go
`combos/request.go:12`는 생략됐을 때 적용한다. effort 정책 계층에서 완화될 수 있어 결함으로
확정하지 않고 관찰 항목으로 남긴다.

## 사이드카 루프 동작 대조

| 축 | 결과 |
| --- | --- |
| 이미지 루프 | 정지 조건(`maxRounds + 1`), 최종 턴에서 합성 도구 제거, 실제 도구콜 시 종료, 턴당 이미지 예산 10 — 의미 일치 |
| 웹검색 루프 | `maxSearches` 상한, 강제 답변 패스, 실패 쿼리 중복 제거, 429 키 회전 — 의미 일치. 강제 답변 nudge 문구만 짧음 |
| vision | 트리거 조건 일치(텍스트 전용 모델 + 이미지 포함 요청). 검증 엄격도만 다름 |
| combos failover | 재시도 판정 일치(499/cyber/origin rejected/context/invalid는 비재시도, 401/403/404/408/429/5xx와 quota/auth/rate/overload는 재시도) |

`combos.Next` 미배선 의심은 오탐이었다 — `chat/handler.go:478`과
`server/responses_core_port.go:683`이 호출한다.

## 처분한 미참조 export

`chat.EncodeCompactionSummary`(디코드/replay 경로는 배선됨), `combos.Cooldown`/`InCooldown`
(프로덕션은 `Next`와 `pickLocked` 내부 검사), `search.FormatWebSearchResult(s)`(이름 파리티용
별칭, 프로덕션은 `FormatResults`), `search.NewSearchMiddleware`(chat/server가 `Loop`를 직접
배선), `search.ShouldResolveOpenAISidecar`(프로덕션 경계는 `BuildSidecarPlan`).

## OK로 확인된 축

Chat Completions 인바운드 정규화와 아웃바운드 변환, combo id/별칭 검증, Grok 관리 블록·
루프백 정책·strip/apply·sync·상태, 호스티드 이미지 도구 감지와 합성 `image_gen`,
이미지 브리지 활성 게이트와 xAI 키 전용 인증, 아티팩트 쓰기/다운로드/SSRF/보존 예산,
웹검색 합성 도구·계획·OpenAI/Anthropic 실행기·파싱·출처 포맷·진행 스트림,
vision 두 백엔드 describer와 사이드카 없을 때 이미지 제거.

## 검증하지 않은 것

라이브 이미지/비디오/vision/검색 호출 없음. 테스트 미실행. 생성 메타데이터 행 단위 미검사.
