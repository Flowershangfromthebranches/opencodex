# 010 — L1 인벤토리: `src/adapters/` 최상위 (cursor·kiro 제외)

22파일 / 5,703줄. 대응 go: `adapter/{anthropic,google,openai}`, `adapter/preflight.go`.

## 확정 결함 3건 (전부 DIVERGENT)

### 1. Azure 어댑터가 다른 wire 경로를 쓴다 (user-visible)

오라클 `src/adapters/azure.ts:5`는 Responses 패스스루를 재사용하고, `:32`에서 v1 API는
`api-version`이 필요 없다고 명시한다. go `adapter/openai/azure.go:27`은 Chat Completions를
감싸고 `:42-50`에서 `api-version`을 쿼리/헤더로 주입한다. Codex Responses 형태 페이로드에서
엔드포인트·본문 의미가 달라진다.

최소 수정: `AzureAdapter`가 go Responses 어댑터 동작을 감싸도록 바꾸고, `api-key` 유지,
bearer 삭제, forward auth와 미해결 placeholder 거부는 그대로 두고, `api-version` 주입 제거.

### 2. identity 중립화가 새 Codex 문구를 놓친다 (user-visible)

오라클 `identity.ts:27-40`은 "a coding agent"와 최신 "an agent" 두 문구를 모두 정규식으로
치환한다. go `adapter/openai/identity.go:6-13`은 옛 문장만 정확히 일치할 때 치환한다.
최신 Codex 클라이언트가 보내는 `You are Codex, an agent based on GPT-5.`가 그대로 통과해
라우팅된 비-OpenAI 모델이 자신을 GPT-5로 오인할 수 있다.

최소 수정: `strings.Replace`를 `a coding agent|an agent` + 선택적 마이너 접미사를 덮는 좁은
정규식으로 교체.

### 3. Anthropic 이미지 정규화가 오라클이 받아들이는 범위를 버린다 (user-visible)

go `adapter/anthropic/imagenormalize.go:27`이 TS에 없는 `maxDecodedImageBytes` 48MiB를 두고
`:371`에서 초과 대상을 드롭한다. TS는 base64 길이(`anthropic-image-normalize.ts:55`)와
픽셀 수(`:64`)만 본다. base64가 64MiB 이하지만 디코드 길이가 48MiB를 넘는 이미지는 TS에서는
정규화되고 go에서는 텍스트로 대체된다.

최소 수정: 그 캡을 제거하거나 TS 기준에 맞추고, go 메모리 안전상 유지해야 한다면 파리티
범위 밖의 의도적 go 정책으로 문서화.

## 스윕에서 처분한 미참조 export

`ReadUpstreamHTTPError`(L1 프로덕션 호출자 없음, google은 패키지 로컬 처리),
`TurnQueue.Collect`(go는 `Stream`/`Send` 사용), `PreflightAdapterEvents`(별칭, 실제로는
`adapter.PreflightEvents` 사용), `SafeVertexHTTPErrorMessage`/`SafeAntigravityHTTPErrorMessage`
(라벨 인자로 공통 포매터 호출), `CompileGoogleWireBody`(프로덕션은 소유권 보존 비공개 변형),
`ParseDataURL`/`ParseImageDimensions`/테스트 전용 통계 함수.

## OK로 확인된 주요 축

Anthropic 이미지 가드(8000px·5MiB·100장·20MiB 총량·오래된 것부터 텍스트화), 도구 결과 인접성
복구, thinking 예산, Antigravity UA·thought signature 필터·세션 id·Claude 서명 살균·Gemini
replay 캐시, Google 재시도와 invalid-body 복구, Gemini 도구 스키마 살균, Vertex truncation
fail-closed, MiMo JWT 수명주기, OpenAI Chat 변환·스트림 파싱, Responses 요청 빌드와 파서,
tool catalog nudge.

## 검증하지 않은 것

테스트/빌드 미실행, 라이브 provider 호출 없음. Gemini 인라인 이미지의 아티팩트 서빙
엔드투엔드는 L4가 확인(라우트 부재 확정).
