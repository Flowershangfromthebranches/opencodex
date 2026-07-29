# 110 — WP8: 요청 정규화

세 번째 층. 요청/응답 **본문 변형**이라 전송 계층(100)이 안정된 뒤에 다뤄야 실패 원인을
구분할 수 있다.

## 담는 항목 (감사 5-잔여, 8, 17, 18, 19)

### 1. image_gen 클라이언트 툴 네임스페이스 정규화 (감사 8)

go는 `stripConflictingHostedTools`만 있다(`responses.go:230-240,432-457`) — 최상위
`image_gen` / `image_gen.` 접두사를 보고 hosted 엔트리를 지우는 게 전부다.

오라클(`openai-responses.ts:504-668`, 적용 `:701-782`)이 하는 것 중 go에 없는 것:
네임스페이스 평탄화, dotted 선언의 `image_gen__` 별칭화, `tool_choice` 재작성,
중첩 `additional_tools` 순회, 입력 `function_call`의 replay 인코딩, 별칭 중복 제거,
그리고 **"쓸 만한 별칭이 대체할 때만 hosted를 제거한다"** 는 규칙.

마지막 규칙이 중요하다. go는 조건 없이 지우므로, 대체할 별칭이 없으면 이미지 생성 능력이
그냥 사라진다.

오라클 테스트가 계약을 이미 문서화해뒀다(`tests/openai-responses-passthrough.test.ts:631-908`) —
그 케이스들을 go 테스트로 옮기는 것이 가장 정확한 이식 경로다.

### 2. reasoning_summary_delivery 요청시 카탈로그 가드 (감사 17)

오라클은 매 요청 활성 카탈로그를 읽어 지원하지 않는 모델이면
`stream_options.reasoning_summary_delivery`를 제거한다(`openai-responses.ts:61-71`,
적용 `:934`). 주석이 이유를 밝힌다: **이미 실행 중인 클라이언트의 카탈로그가 오래됐을 때**를
막는 것이다.

go는 provider config의 `ModelSupportsReasoningSummaries`에 명시적 false가 있을 때만
제거하고(`responses_config.go:56-73`), 카탈로그 접근 자체가 없다. 카탈로그가 갱신돼
미지원으로 바뀌어도 go는 계속 보낸다 → 업스트림 400.

### 3. Gemini thought:true 재분류 (감사 18)

`google.go:721-728`이 `thought:true` 텍스트를 `EventReasoning`으로 낸다. 오라클에는 그
분기가 아예 없고 항상 `text_delta`다(`google.ts:488-498,656-668`). reasoning을 숨기는
클라이언트에서 **오라클이라면 보였을 텍스트가 사라진다.**

### 4. 환경변수 치환 범위 (감사 19)

`environment.go:27-64`가 config 전체를 JSON 왕복하며 모든 문자열을 치환한다. 오라클은
선택된 값(provider API key, proxy)에만 적용한다(`router.ts:188-190`, `config.ts:1561-1565`).

`$`로 시작하는 리터럴 헤더 값이나 모델명이 **조용히 빈 문자열이 된다.** 헤더 값이 사라지면
인증이나 라우팅이 바뀔 수 있다.

### 5. OpenCode Free 헤더 잔여 (감사 5)

090이 static header 병합을 고치면 절반이 닫힌다. 남는 것은 **go 레지스트리 이중화**다:
`providers/registry_metadata.go:107`에는 헤더가 있고 런타임이 쓰는
`registry/registry.go:111`에는 없다. 이 P가 090 이후 상태를 다시 재고 남은 범위를 정한다.

## 검증 계획

1번은 오라클 테스트 케이스 이식이 가장 정확하다. 3번은 회귀 위험이 있으니
(reasoning을 텍스트로 바꾸면 기존 기대가 깨질 수 있음) 기존 Gemini 테스트를 먼저 읽는다.
4번은 음성 사례가 핵심: `$`로 시작하는 리터럴이 보존되는지.

## 범위 밖

오라클 수정, 라이브 provider 호출.

---

## 결과 (커밋 `482229421`, `410f44682`)

5개 항목 중 **3개를 닫았고, 1개는 090에서 이미 닫혔으며, 감사 8은 자기 사이클로 분리했다.**

| 감사 | 처리 | 커밋 |
| ---: | --- | --- |
| 19 env 치환 범위 | 닫음 | `482229421` |
| 18 Gemini thought:true | 닫음 | `482229421` |
| 17 카탈로그 가드 | 닫음 | `410f44682` |
| 5 OpenCode Free 헤더 | **090에서 이미 닫힘** | `e81e57446` |
| 8 image_gen 정규화 | **wp8b로 분리** | — |

### 감사 19: 오라클이 무엇을 푸는지가 핵심이었다

문서는 "선택된 값에만 적용"이라고 썼는데, 그 목록을 정확히 재야 했다. 오라클을 훑으니
`resolveEnvValue` 호출 지점이 provider `apiKey`(`router.ts:189`, `quota.ts:553`,
`openai-sidecar.ts:125,163`, `images/plan.ts:38`, `oauth/index.ts:467`)와
`proxy`(`config.ts:1562`, `doctor.ts:340`)뿐이다.

go에는 오라클에 대응물이 없는 값이 둘 더 있다: `authToken`과 `apiKeys[].key`. 둘 다
자격증명이고 `Load`가 디스크에 미확장 상태로 유지하므로 같은 취급이 옳다. 실제로 기존 테스트
`TestConfigLoadPreservesEnvironmentReferencesAndResolvesRuntimeCopy`가 `authToken` 확장을
기대하고 있었다 — 좁히기만 했다면 그것이 깨졌을 것이다.

### 감사 18: `thought`와 `thoughtSignature`는 다른 것이다

오라클 `google.ts`에 `thought` 분기가 **아예 없다.** `thoughtSignature`는 검색에 걸리지만
그것은 reasoning **연속성**을 위한 별개 메커니즘이고 이번 수정과 무관하다. 혼동하지 않도록
주석에 남겼다.

### 감사 17: import 사이클 때문에 훅이 필요했다

오라클은 `catalogModelSupportsReasoningSummaries`를 직접 부른다. go에서는
`internal/codex`가 **`internal/adapter/openai`를 이미 import**하므로 역방향 직접 호출이
사이클이다. 패키지 레벨 훅을 두고 `serve` 시작 시 주입한다.

훅이 nil일 때(유닛 테스트, 요청을 처리하지 않는 CLI 경로)는 이전 동작 그대로다 —
그것을 테스트로 고정했다. 회귀 위험이 있는 변경이라 명시적 음성 사례가 필요했다.

카탈로그 읽기는 path+mtime으로 캐시한다. 이 sanitizer는 **요청마다** 돌기 때문에 매번
파일을 파싱하면 디스크 IO가 핫 패스에 올라간다.

판정 규칙 둘은 의도적으로 보수적이다:
- 카탈로그가 모르는 모델은 `unknown`이지 `unsupported`가 아니다. 추측하면 멀쩡한 모델에서
  필드를 떼어낸다.
- routed slug는 **모든 엔트리가 일치할 때만** bare id를 대신 답한다. 갈리면 답하지 않는다.

### 감사 8을 분리한 이유

오라클 `openai-responses.ts:504-668`이 **165줄**이고 서로 다른 동작이 7개다: 네임스페이스
평탄화, dotted 별칭화, `tool_choice` 재작성, 중첩 `additional_tools` 순회, replay call 인코딩,
별칭 중복 제거, "쓸 만한 별칭이 대체할 때만 hosted 제거". 게다가 오라클 테스트
(`tests/openai-responses-passthrough.test.ts:631-908`)가 계약을 이미 문서화해뒀으므로 그
케이스 이식이 가장 정확한 경로다.

앞의 셋과 한 사이클에 묶으면 검증이 흐려진다. `wp8b-image-gen-normalization`으로 등록했다.
