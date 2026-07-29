# 090 — WP6: 자격증명·목적지 경계

`080_external_audit_remeasure.md`가 첫 구현 사이클로 지목한 유닛. 여기 결함은 **요청과
자격증명이 의도하지 않은 곳으로 가는** 경로라 다른 모든 것의 아래층이다. 전송 의미론을
고쳐도 요청이 엉뚱한 vendor에 닿으면 의미가 없다.

## 이 유닛이 닫는 것

감사 2·3·4a·4b·4c와, 그 뿌리를 공유하는 5의 절반.

### 뿌리는 하나다: `configuredRegistry`가 registry를 config로 덮어쓴다

`go/internal/cli/serve.go:340-348`:

```go
entry := registry.Provider{ID: name, Label: name, Adapter: provider.Adapter,
    BaseURL: provider.BaseURL, ..., StaticHeaders: provider.Headers}
...
preset.Adapter, preset.BaseURL, preset.StaticHeaders = entry.Adapter, entry.BaseURL, entry.StaticHeaders
```

이 세 줄이 세 결함을 동시에 만든다.

- **감사 3**: 영속 config가 canonical built-in의 wire adapter와 endpoint를 덮어쓴다.
  오라클은 `router.ts:231-248`에서 registry endpoint/adapter가 이기고, 템플릿이나
  `allowBaseUrlOverride`가 명시된 엔트리만 예외다.
- **감사 5(절반)**: `provider.Headers`가 nil이면 preset의 static header가 사라진다.
  OpenCode Free의 `x-opencode-client: desktop`이 그렇게 없어진다.
- **감사 2**: 그 위에서 `base`가 전 built-in으로 시작하므로(`serve.go:334`) 설정하지 않은
  provider도 라우팅 후보로 남는다.

오라클은 셋을 각각 다른 곳에서 막는다: 활성 provider 집합(`router.ts:289`),
canonical 강제(`:231-248`), destination policy(`config.ts:773`).

## 이 사이클의 범위 결정

감사 4는 세 갈래이고 2·3·5는 한 함수에 모여 있다. **한 사이클에 다 넣으면 검증이 흐려진다.**
그래서 이 사이클은 **registry 정합성(2·3·5-절반)** 을 닫고, destination policy(4a/4b/4c)는
`wp6b`로 자른다.

고르는 순서의 이유: destination policy는 "어디로 보낼지"의 최종 방어선이고, registry
정합성은 "무엇을 보낼지"를 정한다. 후자가 깨진 채로 전자를 고치면, 올바른 목적지 검사를
통과한 요청이 여전히 잘못된 provider의 자격증명을 달고 나간다.

### 1. 활성 provider 집합만 라우팅한다 (감사 2)

오라클 `src/router.ts:289`:

```ts
const activeProviderEntries = Object.entries(config.providers)
  .filter(([name, p]) => !isLegacyProvider(name) && !p.disabled);
```

go `serve.go:334`는 `base := registry.BuiltinProviders()`로 시작해 config를 덧씌운다.
MODIFY: 설정된·비활성화되지 않은 provider만으로 registry를 구성하고, 그 각각에 대해
canonical 메타데이터를 **병합**한다(덮어쓰기가 아니라).

```go
// before: every built-in is routable whether or not the user configured it
base := registry.BuiltinProviders()
for name, provider := range cfg.Providers { ...overlay... }

// after: the configured set is the routable set
entries := make([]registry.Provider, 0, len(cfg.Providers))
for name, provider := range cfg.Providers {
    if provider.Disabled || registry.IsLegacyProvider(name) {
        continue
    }
    entries = append(entries, canonicalize(name, provider))
}
```

`ListModels`(`registry.go:397`)도 같은 집합을 봐야 한다 — 지금은 전 엔트리의 모델을
광고하므로 쓸 수 없는 provider의 모델이 목록에 뜬다.

### 2. built-in의 adapter/endpoint는 registry가 이긴다 (감사 3)

오라클 `router.ts:231-248`의 규칙을 그대로 옮긴다:

- 커스텀 provider는 설정된 endpoint 유지
- built-in은 registry endpoint가 이김. 단 그 엔트리가 템플릿을 쓰거나
  `allowBaseUrlOverride`가 참이면 설정값 허용
- 버려진 URL은 경고를 남김
- wire adapter는 무조건 registry 값

`canonicalize`가 그 판정을 담당하고, `registry.Provider`에 `AllowBaseURLOverride`
상당 필드가 없으면 추가한다. B의 첫 작업으로 현재 `registry.Provider` 필드를 확인한다.

### 3. static header는 병합한다 (감사 5의 절반)

`preset.StaticHeaders = entry.StaticHeaders`를 병합으로 바꾼다. 설정값이 우선하되,
설정에 없는 preset 키는 살아남는다.

```go
headers := map[string]string{}
maps.Copy(headers, preset.StaticHeaders)
maps.Copy(headers, provider.Headers) // configured wins per key
preset.StaticHeaders = headers
```

**남는 절반**: 런타임 레지스트리(`registry/registry.go:111`)에 `x-opencode-client`가
없다는 것 자체는 별개다. go에 레지스트리가 둘이고(`providers/registry_metadata.go:107`에는
있다) 런타임이 오래된 쪽을 쓴다. 그 이중화 해소는 `wp6c`로 남긴다 — 이 사이클에서 건드리면
범위가 두 배가 된다.

## 이 사이클이 다루지 않는 것

- `wp6b`: destination policy 3갈래(4a config load, 4b 관리 API 메타데이터 순서,
  4c 요청 시 transport). 4b는 **보안 우회**이므로 다음 사이클 1순위다.
- `wp6c`: go 레지스트리 이중화 해소(`internal/registry` vs `internal/providers`).

## 검증 계획

- `go build ./... && go vet ./...`
- **감사 2 활성화 증거**: OpenRouter만 설정한 config에서 bare `claude-*`를 해석했을 때
  built-in Anthropic이 아니라 fallback으로 가는지. 지금 트리에서 먼저 **실패를 재현**한 뒤
  고친다 — 재현 없이 고치면 그 분기가 살아있는지 알 수 없다.
- **감사 3**: 영속 config에 `baseUrl: "https://evil.test"`를 넣은 built-in이 registry
  endpoint로 나가는지. 템플릿 허용 엔트리는 설정값을 유지하는지(음성 사례).
- **감사 5**: `provider.Headers`가 nil인 opencode-free가 preset 헤더를 유지하는지.
- 어블레이션: 각 수정을 되돌리면 해당 테스트만 무너지는지.
- 기존 스위트 전체 통과 — 특히 registry 구성 변경은 파급이 크므로 회귀를 본다.

## 범위 밖

오라클 수정, 라이브 provider 호출, 다른 세션이 작업 중인 파일.

---

## P 재측정 (2026-07-29, wp6 사이클 진입)

이 문서는 `bfbdbcfd1`에서 쓰였고, 그 뒤 트리를 다시 쟀다. 결과: **세 항목 모두 여전히 유효하다.**

| 항목 | 현재 트리 근거 | 판정 |
| --- | --- | --- |
| 감사 2 (미설정 provider가 라우팅 후보) | `serve.go:335` `base := registry.New().Entries()` — 59개 built-in 전체로 시작. `registry.go:344-352`의 family 폴백이 `entries` 전체를 훑음 | 유효 |
| 감사 3 (config가 canonical adapter/endpoint 덮어씀) | `serve.go:345` `preset.Adapter, preset.BaseURL, preset.StaticHeaders = entry.Adapter, entry.BaseURL, entry.StaticHeaders` — 무조건 대입 | 유효 |
| 감사 5 절반 (preset static header 소실) | 같은 줄. `provider.Headers`가 nil이면 preset 헤더가 사라짐 | 유효 |

### 오늘의 provider-scope 스윕과 겹치지 않는다

같은 날 `codex/260729-go-model-list-provider-filter`(`2a8395c9c`..`7ee5c32cc`)가 미설정 provider
누출을 고쳤다. 그러나 **그 브랜치는 `dev2-go`에 머지되지 않았고**(`git merge-base --is-ancestor
7b85e62d5 HEAD` → NO), 고친 층도 다르다: 그것은 `management`/`codex`의 **카탈로그 표시** 필터이고,
여기는 `configuredRegistry`의 **라우팅 집합**이다. 표시를 막아도 bare `claude-*` 요청은 여전히
built-in Anthropic으로 해석된다.

### 문서가 쓴 것과 다른 사실 두 가지

1. **`AllowBaseURLOverride`는 이미 있다** — `registry.go:39`. 필드 추가가 필요하다고 쓴 것은
   틀렸다. `ollama`/`vllm`/`lm-studio`/`litellm`이 `:178,180`에서 참으로 설정된다.
2. **`registry.IsLegacyProvider`는 없다.** go의 legacy id는
   `providers.LegacyChatGPTProviderID`("chatgpt")와 `LegacyOpenAIMultiProviderID`("openai-multi")로
   `providers/openai_tier_migration.go:10`에 있다. registry가 providers를 import하면 순환이 되는지
   B에서 확인하고, 되면 registry 쪽에 상수를 두거나 `serve.go`에서 거른다.

### 템플릿 판정

오라클은 `router.ts:233`에서 `/\{[^}]*\}/`로 registry baseUrl이 템플릿인지 본다. go registry에
같은 판정이 없으므로 `serve.go`(또는 registry 헬퍼)에 동등한 정규식을 둔다.

### 이 사이클의 확정 범위

`configuredRegistry` 한 함수. 세 가지를 바꾼다.

1. 설정된·비활성화되지 않은 provider만 라우팅 집합에 넣는다 (legacy id 제외).
2. built-in의 `Adapter`는 registry가 무조건 이긴다. `BaseURL`은 registry 값이 템플릿이거나
   `AllowBaseURLOverride`일 때만 설정값을 쓰고, 버려질 때 경고한다.
3. `StaticHeaders`는 병합한다 — preset 위에 설정값을 키 단위로 덮는다.

`ListModels` 필터링은 **이 사이클에서 건드리지 않는다**: 위 1번이 registry 집합 자체를 줄이므로
`ListModels`가 자동으로 좁아진다. 그것이 과도한 필터가 되는지는 C의 회귀가 답한다.

### 되돌릴 수 없는 위험 하나

라우팅 집합을 줄이면 **지금 우연히 동작하던 경로가 끊길 수 있다.** 예: config에 provider를
등록하지 않고 built-in default model로 요청하던 사용자. 오라클이 그렇게 동작하므로 파리티상
옳지만, C에서 기존 스위트 전체를 보고 끊긴 것이 있으면 그 테스트가 오라클과 맞는지 먼저 따진다.

---

## 결과 (커밋 `e81e57446`)

감사 2·3·5-절반을 닫았다. `go build`/`go vet`/`go test ./...` 전부 통과.

### 무엇을 바꿨나

| 항목 | 변경 | 위치 |
| --- | --- | --- |
| 감사 2 | 설정된·비활성화되지 않은·legacy 아닌 provider만 라우팅 집합 | `serve.go` `configuredRegistry` |
| 감사 3 | adapter는 registry가 항상 이김. baseURL은 preset이 템플릿이거나 `AllowBaseURLOverride`일 때만 설정값 | `serve.go` `canonicalProviderEntry` |
| 감사 5 절반 | static header 병합(설정값이 키 단위로 우선) | `serve.go` `mergeStaticHeaders` |
| 감사 5 나머지 | 런타임 registry에 `x-opencode-client: desktop` 추가 | `registry.go:181` |

preset 순서를 유지하고 커스텀 provider는 정렬해 붙인다 — Go 맵 순회 순서가 대시보드 목록에
새지 않게 하려는 것이다.

### 어블레이션 (각 가드가 살아있다는 증거)

수정을 하나씩 되돌렸을 때 **해당 테스트만** 깨진다.

| 되돌린 것 | 깨진 테스트 |
| --- | --- |
| baseURL 핀 → 무조건 대입 | `TestConfiguredRegistryKeepsCanonicalAdapterAndEndpoint` |
| disabled/legacy 필터 제거 | `TestConfiguredRegistryDropsDisabledProviders`, `...DropsLegacyProviderIDs` |
| header 병합 → 대입 | `TestConfiguredRegistryMergesPresetStaticHeaders` |

수정 전 트리에서 8개 중 5개가 실패했다는 것도 기록해둔다. 나머지 3개(템플릿 override,
커스텀 provider, 설정 헤더 우선)는 처음부터 통과한 음성 사례다.

### 기존 테스트 2개를 고쳤다 — 그 판단의 근거

`cmd/ocx`의 두 테스트가 canonical `openai` provider의 baseUrl을 로컬 httptest 서버로
돌려놓고 Authorization 헤더를 읽고 있었다. 이번 수정이 그 override를 버리므로 둘 다 깨졌다.

**테스트가 옳은지 오라클에 직접 물었다.** `routeModel`에 `openai` + `127.0.0.1` baseUrl을
주고 실행한 결과:

```
⚠️  config.json provider "openai": configured baseUrl http://127.0.0.1:59841/… is ignored
   because this provider's endpoint is fixed at https://chatgpt.com/backend-api/codex.
provider= openai baseUrl= https://chatgpt.com/backend-api/codex
```

오라클도 버린다. 즉 그 config는 애초에 성립하지 않았고, 두 테스트는 go의 버그에 기대고
있었다. 그래서 가드를 약화시키지 않고 테스트를 옮겼다.

- combo 테스트: `openai` 대신 설정 가능한 provider 둘로. 원래 검증 대상은 provider id가
  아니라 **combo hop이 다음 타겟의 자격증명을 쓰는가**였고 그것은 그대로 남았다.
- pool 테스트: in-process로 내렸다. 풀이 `"openai"` id에 묶여 있어(`serve.go:460`,
  `authcontext.go:143`) 다른 provider로 옮길 수 없다. 대신 resolve된 auth context에
  대해 검증한다 — 레거시 다중 계정이 canonical 풀로 해석되고 thread affinity가 유지되는가.

### 남은 것

- `wp6b`: destination policy 3갈래(4a/4b/4c). 4b가 보안 우회이므로 다음 사이클 1순위.
- `wp6c`: go 레지스트리 이중화(`internal/registry` vs `internal/providers`). 이번에 헤더
  하나를 양쪽에 맞췄을 뿐 이중화 자체는 그대로다. 관리 API 프리셋(`management/providers.go:15`)이
  여전히 `providers` 쪽을 서빙한다.
