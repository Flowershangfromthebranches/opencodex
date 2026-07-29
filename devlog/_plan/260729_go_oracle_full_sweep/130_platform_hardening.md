# 130 — WP10: 플랫폼 하드닝 (머지가 드러낸 층)

앞의 넷과 성격이 다르다. 이것은 **외부 감사가 볼 수 없었던 층**이다 — `origin/dev`가
가져온 보안 하드닝이고, 감사는 머지 이전 스냅샷을 봤다. 앞선 `010`~`021` 레인들도 그
코드가 존재하기 전에 조사됐다.

14건 중 **9건이 go에 대응물이 없다.**

## 우선순위 (보안 영향 순)

### 1. 관리/데이터 자격증명 분리 (`src/server/management-auth.ts`, `src/lib/admin-secrets.ts`)

go 관리 API가 공용 admission 경로를 그대로 쓴다(`middleware.go:60-87`).
`Authorize`는 옵션이고 서버 구성에서 nil이다(`server.go:519`). 즉 데이터 플레인 키로
관리 API에 접근할 수 있다 — 오라클은 이것을 분리했다.

admin 토큰/GUI 세션/CSRF 분리 개념이 go에 통째로 없다. **이 유닛에서 가장 큰 작업이자
가장 큰 보안 간극**이다.

### 2. pinned HTTP (`src/lib/pinned-http.ts`) + provider outbound (`src/lib/provider-outbound.ts`)

go discovery에 DNS 사전 거부는 있다(`catalog_provider_fetch.go:75,250,260`). 없는 것은
**해석 결과를 실제 연결까지 고정**하는 것이다 — 평범한 `DialContext`/`client.Do`를 쓴다
(`fetch.go:28,45`). 사전검사와 연결 사이에 TOCTOU가 남는다.

pinned peer, 수동 리다이렉트 차단, 프록시 경계 의미도 없다. 090의 destination policy
(`wp6b`)와 같은 계열이므로 그것과 함께 설계해야 한다.

### 3. npm 호출 하드닝 (`src/update/npm-invocation.mjs`) + cwd 하이재킹 방지

go 업데이트가 맨 `npm`/`npm.cmd`를 실행하고(`runtime_management.go:536-540`),
Windows PATH 해석이 cwd를 건너뛰지 않는다(`winexec.go:30-40`). 신뢰되지 않은 디렉터리에서
업데이트를 돌리면 그 디렉터리의 `npm.cmd`가 실행될 수 있다.

### 4. config 소유권 (`src/lib/config-ownership.ts`) + manifest 소유 상태만 제거

go에 `.opencodex-owner.json` 개념이 없어 제거 경계를 그을 수 없다. 언인스톨이 사용자가
직접 만든 상태까지 지울 수 있다.

### 5. 빈 hostname에서 config 보존

go는 빈 hostname을 거부한다(`config.go:641-642`). 오라클은 보존·강등한다. 영속 config가
어떤 이유로 빈 hostname을 갖게 되면 go는 시작을 거부하고 오라클은 계속 동작한다.

### 6. 부분 대응 2건

- `proxy-env`: go가 HTTP/HTTPS/NO_PROXY만 미러링(`environment.go:67-100`),
  `ALL_PROXY` 등이 빠졌다.
- 프레이밍 거부: go는 `X-Frame-Options: DENY`만(`middleware.go:128-130`),
  CSP `frame-ancestors 'none'`이 없다.

## 이 유닛의 성격

앞의 넷은 "오라클을 따라잡는" 작업이고, 이것은 **보안 경계를 새로 세우는** 작업이다.
1번은 특히 설계 결정이 필요하다 — 관리 토큰을 어디에 저장하고 어떻게 회전할지는
go 런타임의 자체 결정이므로, 그 P에서 오라클 설계를 읽고 go 구조에 맞게 옮긴다.

## 검증 계획

보안 경계는 **음성 사례가 본체**다. 데이터 키로 관리 API를 부르면 거부되는지, 신뢰되지 않은
cwd의 `npm.cmd`가 실행되지 않는지, manifest에 없는 파일이 지워지지 않는지.

Windows 항목은 macOS에서 정적 대조만 가능하다 — 그 사실을 해당 work-phase가 명시한다.

## 범위 밖

오라클 수정, 릴리스, 푸시.
