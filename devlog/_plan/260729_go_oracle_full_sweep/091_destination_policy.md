# 091 — WP6b: 목적지 정책 3갈래 (감사 4a·4b·4c)

`090`이 **어느 provider로** 가는지를 고쳤다면, 여기는 **그 provider가 가리키는 주소가
허용되는가**다. 090에서 잘라낸 나머지이고, 4b는 보안 우회이므로 이 사이클 1순위다.

## P 재측정 (현재 트리)

### 오라클 구조

`src/lib/destination-policy.ts` 하나가 분류기이고, 세 곳이 그것을 부른다.

| 지점 | 함수 | 오라클 위치 |
| --- | --- | --- |
| config load | `providerDestinationConfigError` | `config.ts:781` |
| 라우팅(요청) | `assertProviderDestinationAllowed` | `router.ts:196,242` |
| 관리 API | `providerDestinationResolvedError` (async, DNS) | `provider-routes.ts:114,270,285` |

분류기는 `DestinationKind`를 돌려주고, **판정 순서가 곧 보안 경계**다
(`destination-policy.ts:134-141`):

```ts
if (assessment.kind === "public" || assessment.kind === "hostname") return null;
if (assessment.kind === "metadata") return "baseUrl targets a blocked metadata endpoint";
if (registryAllowsPrivateNetwork(name)) return null;
if (provider.allowPrivateNetwork === true) return null;
```

**metadata가 allow 스위치보다 위에 있다.** `allowPrivateNetwork: true`를 켜도
169.254.169.254는 여전히 막힌다. 그것이 이 정책의 핵심이다 — 로컬 Ollama를 허용하려고 켠
스위치가 클라우드 자격증명 탈취 경로까지 열어주면 안 된다.

### go의 현재 상태

| 감사 | 판정 | 현재 근거 |
| --- | --- | --- |
| 4a — config load에 정책 없음 | CONFIRMED | `config.go:765-771`이 URL **문법**만 검사한다(scheme, host, user/query/fragment). 목적지 분류가 없다 |
| 4b — 관리 API에서 metadata 차단 우회 | CONFIRMED | `management/provider_destination.go:17-19`가 `AllowPrivateNetwork` 또는 registry-local이면 **즉시 nil을 반환**한다. metadata 판정(`:62`)에 도달하지 못한다 |
| 4c — 요청 시 transport에 검사 없음 | CONFIRMED | 라우팅 경로에 `assertProviderDestinationAllowed` 대응물이 없다 |

4b가 심각한 이유: 오라클에서는 뚫리지 않는 metadata 차단이, go에서는 provider 하나에
`allowPrivateNetwork: true`만 있으면 통과한다. 그 필드는 로컬 모델 쓰려고 켜는 흔한 값이다.

### 뿌리는 하나다: go에 분류기가 없다

go의 `provider_destination.go`는 분류기가 아니라 **관리 API 전용 검사 함수**다. 판정이
`rejectedProviderIP` 안에 섞여 있고 management 패키지에 갇혀 있어 config나 router가 부를 수
없다. 그래서 4a·4c가 "없는" 것이다 — 부를 것이 없었다.

## 이 사이클의 범위

1. **분류기를 공용 위치로 추출한다.** 오라클 `DestinationKind`에 대응하는 분류 함수를 두고,
   판정 순서를 오라클과 동일하게 한다.
2. **4b를 먼저 닫는다.** allow 스위치보다 metadata 판정이 위로 온다.
3. **4a**: config 검증에서 리터럴/localhost 동기 검사를 부른다(오라클도 config load는 동기).
4. **4c**: 라우팅에서 유효 baseURL에 대해 동기 검사를 부른다.

DNS 해석(async)은 관리 API에만 남긴다 — 오라클도 그렇다.

## 이 사이클이 다루지 않는 것

- pinned HTTP(TOCTOU 봉합)는 `130`의 항목이다. 여기는 **정책 판정**이고 그것은 **연결
  고정**이다. 섞으면 둘 다 흐려진다.
- IPv6 분류의 완전 동등성: go `net.IP`가 이미 loopback/private/link-local을 알고 있으므로
  오라클의 수동 hextet 파싱을 그대로 옮기지 않는다. 대신 **오라클이 막는 것을 go도 막는지**를
  케이스로 검증한다(특히 `::ffff:7f00:1` 같은 hex-mapped 형태).

## 검증 계획

- 4b 활성화 증거: `allowPrivateNetwork: true` + metadata 주소가 **지금은 통과한다는 것을
  먼저 재현**한 뒤 거부로 바꾼다.
- 4a: metadata/loopback baseUrl을 가진 config가 load에서 거부되는지.
- 4c: 라우팅 시점 거부.
- 음성 사례: ollama 같은 `allowPrivateNetworkByDefault` 엔트리는 여전히 로컬 허용.
- 어블레이션: 각 호출 지점을 되돌리면 해당 테스트만 깨지는지.

---

## 결과 (커밋 `7036175ff`, `a302bf8bb`)

감사 4a·4b·4c를 모두 닫았다. `go build`/`go vet`/`go test ./...` 통과.

### P가 틀렸던 것: 분류기는 이미 있었다

계획은 "go에 분류기가 없으니 만든다"였는데, 트리를 더 파보니
`internal/lib/destination_policy.go`에 **오라클과 같은 판정 순서를 가진 분류기가 이미
있었다.** metadata를 allow 스위치보다 먼저 본다. `internal/images/ssrf.go`가 쓰고 있다.

그래서 실제 결함은 "분류기 부재"가 아니라 **호출 부재**였다. 관리 API는 자기만의 검사 함수를
따로 갖고 있었고(그쪽 순서가 틀렸다), config와 라우팅은 아무것도 부르지 않았다.

### 왜 패키지를 옮겼나

`internal/lib`가 import 그래프에서 `internal/config` **위에** 있다. 즉 config가 lib를 부를 수
없다 — 이 정책을 가장 필요로 하는 층이 닿지 못하는 위치에 있었다. `internal/destination`으로
내리고 `internal/lib`에는 얇은 alias만 남겼다(기존 호출자 보존).

### 무엇을 바꿨나

| 감사 | 변경 | 위치 |
| --- | --- | --- |
| 4b | metadata 판정을 allow 스위치 **앞으로** | `management/provider_destination.go` |
| 4a | config 검증에 목적지 검사 배선 | `config/config.go` + `destination_helper.go` |
| 4c | resolve된 URL에 대해 요청 시 검사 | `cli/live_config.go` `assertDestinationAllowed` |

### 4c를 처음에 잘못된 층에 넣었다

`registry.ResolveProviderTransport`(공용 헬퍼)에 넣었더니 **12개 넘는 테스트가 깨졌다.**
원인이 분명했다: 테스트나 sidecar가 만드는 합성 registry는 자기 endpoint(httptest loopback)를
직접 이름 짓고 **opt-in할 config 엔트리가 없다.**

옳은 층은 `configBackedRegistry`다 — 거기서만 사용자 config가 권위다. 옮기니 전 스위트가
통과했다. 공용 헬퍼에 정책을 넣으면 "정책의 예외를 표현할 수단이 없는 호출자"까지 함께
막힌다는 것이 이 실패가 알려준 것이다.

### 활성화 증거 (어블레이션)

| 되돌린 것 | 깨진 것 |
| --- | --- |
| 4b 순서 | `TestProviderDestinationBlocksMetadataDespiteAllowPrivateNetwork`, `...ForRegistryLocalProviders` |
| 4a 배선 | `TestValidateRejectsMetadataAndPrivateProviderDestinations`(4 케이스), `TestValidateBlocksMetadataEvenWithAllowPrivateNetwork` |
| 4c 게이트 | `TestResolveTransportRejectsMetadataAndPrivateDestinations`(3 케이스), `TestResolveTransportBlocksMetadataDespiteAllowPrivateNetwork` |

수정 전 재현도 각각 확인했다. 특히 4b는 `allowPrivateNetwork: true`로 169.254.169.254가
**실제로 통과하는 것**을 먼저 보고 고쳤다.

### 기존 테스트 3개를 고쳤다

`server/config_persistence_production_test.go`가 httptest loopback을 `allowPrivateNetwork`
없이 쓰고 있었다. 새 게이트가 정확히 그것을 잡으라고 있는 것이고, 그 픽스처가 원래 의도한 바가
`allowPrivateNetwork: true`이므로 플래그를 세웠다.
