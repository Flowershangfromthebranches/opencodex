# 070 — WP4: 데이터플레인 아티팩트 경로 (A 감사 FAIL 반영 개정본)

`030_synthesis.md`의 070 표는 5항목이다. 앞의 사이클들이 보여준 대로 한 번에 다 넣으면
검증이 흐려지므로, **이 사이클은 아티팩트 라우트 하나**를 닫는다.

고르는 기준: 나머지 넷(clear-cooldown ×2, pool-strategy, injection-model 형태)은 전부 관리
API 계층이고 서로 비슷한 모양이라 한 유닛으로 묶는 편이 낫다. 아티팩트 라우트는 **데이터
플레인**이고, 인증·CORS 경계를 지나며, 모델이 보는 URL을 실제로 서빙한다 — 성격이 다르다.

## P 재검증

### `GET /v1/opencodex/artifacts/{id}`가 go 서버에 없다 (UNWIRED, user-visible)

오라클 `src/server/index.ts:505-532`:

1. `requireApiAuth(req, config, "data-plane")` — 데이터 플레인 인증
2. `isAllowedRequestOrigin` 실패 시 403 `origin_rejected`
3. `resolveArtifactPath(id)` — 없으면 404 `not_found`
4. 확장자로 content-type 결정, `cache-control: private, max-age=3600`,
   `x-content-type-options: nosniff`

go 쪽 상태:

- `internal/images/artifacts.go:174` `ResolveArtifactPath(dir, id)` — **있다.** traversal을
  두 번 막고(`:183-186`) 정규 파일인지도 본다(`:188-191`).
- `:195` `ArtifactContentType(path)` — **있다.** png/jpeg/webp/gif + 폴백.
- `:32` `ArtifactHTTPPrefix = "/v1/opencodex/artifacts"` — **있다.**
- `:161` `ArtifactHTTPURL`이 그 접두사로 모델에게 보여줄 URL을 만든다.
- 라우트 등록: **없다.** `server.go:475-478`은 images/search POST만 등록하고,
  요청은 `server.go:523`의 미지 `/v1/*` JSON 404로 떨어진다.

### 감사가 무너뜨린 전제, 그리고 그것이 드러낸 더 큰 결함

초판은 "go 브리지가 이미 이 URL을 모델에게 건넨다"고 썼다. **틀렸다.** `ArtifactHTTPURL`은
프로덕션 호출자가 없고, 이미지 브리지는 `loop.go:222-228`에서 `fileURI(primary)`로
`file://` 링크를 만든다. 라우트만 만들면 아무도 쓰지 않는 엔드포인트가 된다.

그런데 그 전제를 검증하다 **오라클의 진짜 소비자**를 찾았다. `artifactHttpUrl`을 쓰는 곳은
이미지 브리지가 아니라 **Gemini 어댑터**다:

```
src/adapters/google.ts:265  artifactMarkdownUrl = artifactHttpUrl(...).replace(/([()])/g, "\\$1")
src/adapters/google.ts:499-508   스트림: inlineData -> materializeInlineImage -> ![image](URL)
src/adapters/google.ts:669-677   비스트림: 같은 경로
```

Gemini가 응답에 인라인 이미지를 실어 보내면 오라클은 그것을 디스크에 쓰고, **호스트 경로가
아닌 불투명 HTTP URL**로 모델/사용자에게 보여준다. 주석이 이유를 밝힌다: 원격·컨테이너
클라이언트가 호스트 파일시스템 경로 없이 이미지를 가져올 수 있어야 한다.

go에는 그 경로가 **통째로 없다.** `emitParts`(`google.go:714-738`)는 `text`와 `functionCall`만
보고 `inlineData`를 그냥 버린다. `internal/images/download.go:41`에 `MaterializeInlineImage`가
있는데도 Gemini 어댑터에서 부르지 않는다. 즉 **Gemini가 생성한 이미지가 go에서는 사라진다.**
(`google.go:412`, `:489`의 `inline_data`는 반대 방향 — 사용자가 보낸 이미지를 요청에 싣는 것이다.)

이것이 L1이 "Google 스트림 파싱의 인라인 이미지 materialization"을 OK로 판정하면서
"아티팩트 서빙 엔드투엔드는 확인하지 않았다"고 남긴 바로 그 구멍이다.

## 이 사이클의 범위 (감사 반영)

라우트만으로는 사용자 가치가 없고, materialization만으로는 URL이 404다. **둘은 한 쌍이다.**

오라클과 go의 시그니처 차이 하나: 오라클 `resolveArtifactPath(id)`는 디렉터리를 내부에서
구한다. go는 `dir`을 인자로 받으므로 서버가 그 값을 알아야 한다. 규칙은
`internal/images/wiring.go:21,43`에 있다 — `<home>/artifacts`. 서버는 이미 같은 방식으로
home을 구한다(`server.go:490-492`, `:508-511`: `config.StorageHome`이 비면
`codex.ResolveCodexHome`).

### 0. 아티팩트 홈을 하나로 못박는다 (감사 라운드2 블로커 1 — 가장 중요)

**두 절반이 서로 다른 디렉터리를 보면 엔드투엔드가 조용히 404가 된다.** 그리고 테스트는
라우트 쪽 디렉터리에 파일을 심어놓으면 통과해버리므로, 이 실수는 테스트로 잡히지 않는다.

현재 상태:

| 소비자 | 홈 | 근거 |
| --- | --- | --- |
| 이미지 브리지(쓰기) | `OPENCODEX_HOME` 또는 `~/.opencodex` | `cli/image_bridge.go:29-38` `opencodexHome()` |
| 서버 `StorageHome` | `CODEX_HOME` | `cli/serve.go:183` `StorageHome: os.Getenv("CODEX_HOME")` |
| 서버 폴백 | `codex.ResolveCodexHome` = `~/.codex` | `server.go:490-492` |
| 오라클 | `getConfigDir()/artifacts` = `OPENCODEX_HOME` 또는 `~/.opencodex` | `config.ts:408-413`, `artifacts.ts:64` |

초판이 "서버가 이미 같은 방식으로 home을 구한다"고 쓴 것은 **틀렸다.** `StorageHome`은
Codex 홈이지 OpenCodex 홈이 아니다. 기본 설정에서 Gemini는 `~/.opencodex/artifacts`에 쓰고
라우트는 `~/.codex/artifacts`에서 찾게 된다.

**결정(B로 미루지 않는다):** `server.Config`에 `ArtifactsHome` 필드를 새로 만들고,
`cli/serve.go`가 `opencodexHome()`의 값을 넣는다. 그 함수가 이미 오라클 규칙의 유일한
구현체이므로 그것이 단일 진실이다. `StorageHome`에서 파생하지 않는다.

```go
// server.Config
	// ArtifactsHome is the OpenCodex home, which is NOT StorageHome: artifacts
	// live under OPENCODEX_HOME while StorageHome tracks CODEX_HOME. Deriving one
	// from the other would serve from a directory nothing writes to.
	ArtifactsHome string
```

`cli/serve.go:183`의 `server.Config{...}`에 `ArtifactsHome: opencodexHome()`을 추가하고,
Gemini 어댑터도 같은 값을 받는다.

### 1. MODIFY `go/internal/server/server.go` — 라우트 등록

### 2. MODIFY `go/internal/adapter/google/google.go` — 인라인 이미지 저장

`emitParts`(`:714`)의 part 루프에 `inlineData` 분기를 넣는다. 오라클
`google.ts:499-508`과 같은 순서: 크기 상한 초과면 오류 이벤트, 아니면 디스크에 쓰고
마크다운 텍스트 델타를 낸다.

```go
		if inline, ok := part["inlineData"].(map[string]any); ok {
			if data := stringValue(inline["data"]); data != "" {
				// Gemini can answer with an image. Dropping it loses the answer;
				// writing the host path into the transcript leaks the filesystem
				// to a model and to remote clients. The opaque HTTP route is what
				// the oracle hands over instead (src/adapters/google.ts:506).
				...
			}
		}
```

필요한 것: 아티팩트 디렉터리, `ImageBudget`(턴당 바이트 예산), 그리고 마크다운 이스케이프
(`artifactMarkdownUrl`이 괄호를 이스케이프한다 — 파일명에 괄호가 들어갈 수 있으면 링크가
깨지므로).

**예산 수명 (감사 라운드2 블로커 2):** 오라클은 `ParseStream`/`ParseUnary` **각 1회**
`imageBudget`을 만들어 그 턴의 모든 인라인 이미지가 공유한다(`google.ts:493`, `:660`).
go도 같아야 한다 — part 분기 안에서 만들면 이미지마다 상한을 새로 쓰고, 어댑터 필드로 두면
서로 다른 파싱이 예산을 잘못 공유한다. **`ParseStream`과 `ParseUnary`가 각각 하나씩 만들어
`collectGeminiCandidates`(`google.go:689-697`)를 통해 넘긴다.**

크기 상한도 오라클 순서를 따른다: `MaterializeInlineImage` 호출 **전에**
`len(data) > images.MaxEncodedBytesPerImage`를 검사해 오류 이벤트를 낸다
(`google.ts:501-503`). `DecodeValidatedImageBase64`가 내부에서도 검사하지만, 오류 분류를
오라클과 같게 하려면 바깥 검사가 필요하다.

괄호 이스케이프(`google.ts:266`)는 현재 go 아티팩트 파일명에 괄호가 들어갈 수 없으므로
(`artifactIDPattern`, `artifacts.go:40-44`) 실질 효과가 없다. 파리티를 위해 넣되 죽은
코드임을 주석에 남긴다.

어댑터가 `ArtifactsHome`을 받는 방법은 `google.NewAdapter`(`google.go:64`) 시그니처를 넓히거나
`Adapter` 필드로 둔다 — 배선은 `cli/serve.go:646`.

## 이 사이클이 다루지 않는 것 (`wp4b-management`로 이월)

`POST /api/oauth/accounts/clear-cooldown`, `POST /api/codex-auth/accounts/clear-cooldown`,
`PUT/PATCH /api/codex-auth/pool-strategy`, `/api/injection-model`의
`syncCodexSubagentDefaults` + `available` 객체 형태. 넷 다 관리 API 계층이라 한 유닛으로 묶는다.

(`POST /api/system/restart` 등록은 이 사이클 이전에 다른 세션이 이미 고쳤다 — L5 발견과
일치하는 수정이 `management/api.go`에 들어와 있다.)

## 검증 계획

- `go build ./... && go vet ./...`
- NEW 서버 테스트: 임시 디렉터리에 아티팩트를 놓고 GET → 200 + 정확한 content-type +
  두 헤더. 존재하지 않는 id → 404. **traversal 시도**(`../`, 절대경로, 구분자 포함) → 404.
- NEW Gemini 테스트: `inlineData`를 담은 응답이 아티팩트를 쓰고 `/v1/opencodex/artifacts/...`
  마크다운을 내는지. 크기 상한 초과는 오류 이벤트인지.
- **엔드투엔드**: Gemini가 낸 URL을 그 라우트로 GET 했을 때 200이 나오는지 — 두 절반이
  실제로 만나는지 증명한다. 이것이 이 사이클의 핵심 증거다.
- 미들웨어 경계 확인: 인증이 필요한 구성에서 토큰 없이 부르면 200이 아님을 단언.
  (감사가 `middleware.go:44-47,60-89,115-137`을 읽고 전역 적용을 확인했다 — 핸들러에
  별도 인증을 만들지 않는다.)
- 어블레이션: 라우트 등록을 빼면 그 테스트만 실패하는지.
- 라이브 확인은 데몬 재빌드가 필요하므로 소스 수준 증거로 대체한다.

## 범위 밖

오라클 수정, 이미지 생성 자체, 다른 세션이 작업 중인 파일.
