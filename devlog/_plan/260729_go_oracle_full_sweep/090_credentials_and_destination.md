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
