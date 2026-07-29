# 015 — L6 인벤토리: `src/codex/**`

47파일 / 14,773줄. 대응 go `internal/codex` — exported 375개 중 비참조 44개.

## 오래된 의심 클러스터 재측정 (`DEAD_EXPORT_AUDIT.md`는 `e5ba7b7b` 기준으로 낡음)

| 심볼 | 현재 상태 |
| --- | --- |
| `ResolveAndPersistCodexRuntime` | 해소 (`cli/startup_health.go:190`) |
| `LoadLastEffortClamp` | 해소 (`:191`) |
| `ClassifyCodexRouting` | 해소 (`codex/inject.go:533`) |
| `PersistEffortClamp` | **UNWIRED 확정** (`runtime.go:149`) |
| `FormatRuntimeLogLine` | **UNWIRED 확정** (`runtime_resolve.go:159`) |
| `FormatClampLogLines` | **UNWIRED 확정** (`:166`) |
| `DedupeRelatedProjectConfigWarnings` | **UNWIRED 확정** (`warnings.go:68`) |
| `FormatProjectConfigWarningsForConsole` | **UNWIRED 확정** (`warnings.go:146`) |
| `MarkJournalInjectedState` | **UNWIRED 확정** (`journal.go:66`) |
| `GetAgentsMaxThreads` / `GetMaxConcurrentThreads` | 패키지 헬퍼. 프로덕션은 `GetLogicalMaxThreads` |
| `BuildUnixShim` | 헬퍼. 프로덕션은 `BuildUnixShimWithToken`(`shim.go:188`) |

절반이 해소됐고 절반은 살아 있다. 문서를 믿지 않고 재측정한 것이 정확했다.

## 확정 결함

### 1. `ocx sync`가 프로젝트 설정 우회 경고를 내지 않는다 (MISSING, user-visible)

오라클 `src/codex/sync.ts:113`은 sync 직후 신뢰된 프로젝트 `.codex/config.toml`이 Codex를
OpenCodex 밖으로 라우팅하고 있으면 경고한다. go `cli/sync.go:19`는
`CollectProjectCodexConfigWarnings`도 `FormatProjectConfigWarningsForConsole`도 부르지 않는다.
사용자는 프록시가 우회되고 있다는 사실을 모른 채 sync가 성공했다고 본다.

최소 수정: `cli/sync.go:100-107` 주입 직후 수집 → `DedupeRelatedProjectConfigWarnings` →
`FormatProjectConfigWarningsForConsole` 출력.

### 2. effort-clamp 진단 3종이 배선되지 않았다 (UNWIRED, user-visible)

오라클은 `catalog/effort.ts:269,320,347`에서 클램프 상태를 기록하고 런타임 선택 라인과
"지원되지 않는 reasoning effort 제거" 경고를 출력한다. go는 세 함수를 다 갖고 있으나 호출자가
없어 `ocx doctor`/런타임 상태가 마지막 클램프 진단을 놓친다.

### 3. journal injected-state 표시가 호출되지 않는다 (UNWIRED, user-visible)

오라클 `inject.ts:597`은 config/profile 기록 후 정확한 주입 상태 메타데이터를 남겨 restore가
재현할 수 있게 한다. go는 `WriteJournal`/`RestoreJournal`은 쓰지만 `MarkJournalInjectedState`는
부르지 않아 restore 모호성이 커진다.

### 4. 수동 Codex 계정 import 검증이 다르다 (DIVERGENT, 보안 인접)

오라클 `auth-api.ts:669,672`는 JWT `exp`를 쓰고 warmup 검증 후 저장한다.
go `cli/codex_auth_management.go:248`은 `Expires: now+1h`를 쓰고 동등한 warmup 검증이 없다.

### 5. 계정 로그 라벨이 로그인/import 시 부여되지 않는다 (UNWIRED, 프라이버시)

오라클 `account-label.ts:25`, `auth-api.ts:686,1139`는 안정적인 불투명 라벨을 만든다.
go `upsertConfigAccount`는 `WithCodexAccountLogLabel`을 부르지 않아 폴백 해시 라벨에 의존한다.

### 6. Windows 번들 플러그인 마켓플레이스 진단 부재 (MISSING, user-visible)

오라클 `plugins-doctor.ts:152`. go 상태는 `codexPlugins: not_inspected`만 보고한다.

### 7. provider `/models` 카탈로그 augmentation 차이 (DIVERGENT)

오라클 `catalog/provider-fetch.ts:247`의 Cursor RPC discovery, OAuth 무토큰 폴백 뉘앙스,
Vertex `{models}` 프로브 형태, registry/JawCode augmentation 일부가 go 쪽에서 확인되지 않았다.
일부는 다른 패키지에 있을 수 있어 교차 확인이 필요하다.

## OK로 확인된 축

Design B 주입(루프백 오버라이드, 외부 provider 보존, 컨텍스트 윈도 정리), 관리형 `[agents]`
기본값의 소유권 인식과 모호한 TOML fail-closed, 계정 라우팅(친화성·쿼터·일시정지·soft
avoid·429 결과), auth context 해석과 헤더 주입, 카탈로그 병합/쓰기/캐시 무효화, startup
health 파생, app-server 재시작, history provider 동기화.

## 검증하지 않은 것

`internal/codex` 밖 동작은 프로덕션 참조 확인 범위까지만. WS registry의 go 위치 미확인.
라이브 데몬 미검증.
