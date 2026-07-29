# 030 — 종합: 12 레인이 찾아낸 것과 그것을 닫는 순서

기준 커밋 `2fe6d2100`(레인 조사 시점), 종합 시점 HEAD는 그보다 앞설 수 있다.
`src/` 352파일 전부가 정확히 한 번씩 조사됐다(중복 0, 누락 0 — `000_plan.md` §5 재현 스크립트가
`COVERAGE_OK`를 출력).

## 이 조사가 실제로 무엇을 바꿨나

"go가 오라클을 다 따라잡았는가"는 이번이 네 번째 질문이고, 앞의 세 번과 답이 또 다르다.
앞선 유닛들은 렌즈를 하나씩 들고 봤다 — 라우트 표면(`260728_go_port_parity`), 관리 API
사각지대(`260729_go_port_blindspot_sweep`), 남은 4개 라우트와 외부 구현 대조
(`260729_go_parity_chase`). 이번에는 렌즈 없이 전면을 훑었고, 그래서 **앞선 조사들이 구조적으로
볼 수 없었던 층**이 드러났다.

가장 큰 것: 어댑터 내부의 실패 처리 의미론. Kiro 하나에서만 MISSING 5건 + DIVERGENT 12건이
나왔는데, 이것들은 라우트도 정상이고 심볼도 존재하며 파리티 테스트도 통과한다. 다른 것은
**빈 스트림을 502로 볼지 재시도 가능한 incomplete로 볼지**, **출력을 이미 내보낸 뒤의 실패를
재시도 가능으로 표시할지**같은 판정이다. 사용자에게는 "가끔 턴이 깨진다"로 나타나고, 정적
대조로는 절대 잡히지 않는다.

두 번째로 큰 것: 양방향 스윕이 강제한 결과. A-phase 감사가 요구한 "go 쪽 exported 심볼을 전부
세고 참조 없는 것을 오라클과 대조하라"가 없었다면, 레인들은 TS→go 방향만 보고 "다 있다"고
보고했을 것이다. 실제로 이 규칙이 잡아낸 것들: Cursor 네이티브 exec가 wire에서 안 닿음,
`apply_patch` 변형 차단 미배선, 스토리지 cleanup 실행기 전체 미배선, 업데이트 프롬프트의
"지금 업데이트"가 no-op, Codex effort-clamp 진단 3종 미배선.

## 정정된 오탐 (기록해 둔다)

| 항목 | 이전 기록 | 재측정 결과 |
| --- | --- | --- |
| `FilterCursorConfiguredModelsByLiveDiscovery` | `260729_go_parity_chase`가 후보로 남김 | 배선됨 (`internal/cli/cursor_discovery.go:44`) |
| `ResolveAndPersistCodexRuntime` | `DEAD_EXPORT_AUDIT.md`가 미배선 의심 | 배선됨 (`internal/cli/startup_health.go:190`) |
| `LoadLastEffortClamp` | 같음 | 배선됨 (`startup_health.go:191`) |
| `ClassifyCodexRouting` | 같음 | 배선됨 (`internal/codex/inject.go:533`) |
| `/api/subagent-models`의 `available` | blindspot sweep이 누락 보고 | 이미 수정됨 (`agents.go:21`) |
| `response.completed/failed/incomplete` | 문자열 grep이 부재 보고 | 동적 조립(`"response." + status`), 정상 방출 |
| `MaybeShowUpdatePrompt` | 통째로 미배선 보고 | 호출은 됨. 단 `RunUpdate`/`Exit`가 nil이라 반쪽 |

마지막 항목이 이 조사 방식의 가치를 잘 보여준다. "배선됐다/안 됐다"의 이분법으로는
`MaybeShowUpdatePrompt`가 해소로 보이지만, 실제로는 프롬프트가 뜨고 사용자가 `1`을 눌러도
아무 일도 일어나지 않는다.

## 확정 결함 집계

| 레인 | MISSING | UNWIRED | DIVERGENT | 최대 위험 항목 |
| --- | ---: | ---: | ---: | --- |
| L1 어댑터 코어 | 0 | 0 | 3 | Azure가 Responses 대신 Chat Completions로 감쌈 |
| L2 Cursor | 0 | 3 | 2 | 네이티브 exec wire 미도달, `apply_patch` 차단 미배선 |
| L3 Kiro | 5 | 1 | 12 | 도구 턴 fallback 재시도 의미론 |
| L4 server 코어 | 0 | 1 | 0 | `/v1/opencodex/artifacts/{id}` 라우트 부재 |
| L5 management | 5 | 1 | 4 | 스토리지 정책 저장/실행 UI가 동작 안 함 |
| L6 codex | 2 | 5 | 2 | sync가 프로젝트 설정 우회 경고를 안 냄 |
| L7 CLI | 4 | 8 | 3 | `models`/`provider` 런타임 서브커맨드 전면 부재 |
| L8 oauth/providers | 3 | 8 | 5 | **Kiro/Copilot 리프레시 미배선, 터미널 실패가 needsReauth를 안 남김** |
| L9 storage/usage | 2 | 4 | 2 | **cleanup 실행기 전체 미배선(파괴적 경로)** |
| L10 lib/claude/update/tray | 1 | 2 | 2 | 업데이트 알림이 순수 go 사용자에게 안 뜸 |
| L11 루트/responses | 2 | 0 | 1 | `codexAccountNamespaces` 설정 키 부재 |
| L12 사이드카 | 2 | 0 | 1 | 비디오 브리지 통째로 미이식 |
| **합계** | **26** | **33** | **37** | |

96건. 이 중 상당수는 서로 같은 뿌리를 공유하므로 구현 work-phase는 96개가 아니다.

## 구현 순서 (의존성 기준, PHASE-SPLIT-01)

효과 크기가 아니라 **위험도와 의존 관계**로 잘랐다. 앞 단계가 뒤 단계의 전제를 만든다.

```
040 보안/자격증명 ──── OAuth 리프레시·needsReauth·풀 가드. 다른 모든 것의 아래층.
        │
050 파괴적 IO ──────── storage cleanup 실행기 + 정책 실행. 위험도 최고.
        │
060 전송 의미론 ────── Kiro 실패 처리, Cursor exec wire + apply_patch 차단.
        │
070 관리 표면 ──────── 누락 라우트 6개 + 응답 형태 4건 + artifacts 라우트.
        │
080 CLI 표면 ───────── models/provider 런타임 서브커맨드, debug, doctor 플래그.
        │
090 진단·설정 ──────── codex 경고/클램프 배선, 설정 키 3종, 업데이트 프롬프트.
        │
100 사이드카 ───────── 비디오 브리지. 독립적이고 순수 신규 기능이라 마지막.
```

각 decade가 하나의 work-phase = 하나의 완전한 PABCD 사이클이다.

### 040 — 자격증명·인증 경계 (최우선)

가장 먼저인 이유: 여기 결함은 조용히 실패하고, 그 실패가 위 계층 전부에 오진을 만든다.
Kiro 토큰이 갱신되지 않으면 L3의 재시도 의미론을 고쳐도 사용자는 여전히 실패를 본다.

| # | 항목 | 근거 |
| --- | --- | --- |
| 1 | Kiro·GitHub Copilot 요청시 리프레시 배선 | `oauth_guardian.go:20-41`에 두 provider 없음. 구현은 `oauth/kiro.go:165`, `github_copilot.go:75`에 존재 |
| 2 | 터미널 리프레시 실패 → `needsReauth` 기록 | `store_refresh.go:112`가 오류만 반환. 오라클은 `index.ts:297-311`에서 세대 지정 마킹 |
| 3 | Anthropic 풀의 local-cli 채택 가드 | 오라클 `anthropic-routing.ts:138,500`; go `anthropic_pool.go:284-377`에 없음 |
| 4 | API 키 429 회전이 transport를 재구성하지 않음 | 오라클 `key-failover.ts:62`; go는 키만 반환 |
| 5 | Kiro 강제 로그인 롤백 | 오라클 `index.ts:702`; go는 import 전용 |
| 6 | pre-multiauth 백업, Copilot base URL 허용목록 | `store.ts:168`, `:203` |

### 050 — 파괴적 스토리지 경로

두 번째인 이유: 사용자 데이터를 옮기는 코드다. 잘못 배선하면 되돌릴 수 없다.
로직은 이미 대량으로 있고 **실행기 조립만 없다** — 그래서 조립 순서가 정확해야 한다.

| # | 항목 | 근거 |
| --- | --- | --- |
| 1 | `ExecuteArchivedCleanup` 조립 (probe→refs→stage→manifest→DB 조정→rollback/purge) | 오라클 `cleanup.ts:1733`; go 원시요소는 `cleanup_decide.go`, `cleanup_stage.go`에 전부 있음 |
| 2 | `POST /api/storage/cleanup` 라우트 | 오라클 `logs-usage-routes.ts:273` |
| 3 | 정책 실행 job + `POST /api/storage/cleanup-policy/run` | 오라클 `:468`, `policy-job.ts:327` |
| 4 | 정책 GET/PUT 응답에 `job`/`policy` 포함 | GUI `Storage.tsx:813,920`가 그 키를 읽음 |

### 060 — 전송 계층 실패 의미론

| # | 항목 | 근거 |
| --- | --- | --- |
| 1 | Kiro fallback 상태기계 정합 (빈 assistant 미첨부, `priorEmittedOutput` 게이트, usage 병합) | `kiro.ts:1290,1336,1400` vs `kiro.go:839,874,885` |
| 2 | Kiro 빈 스트림 → retryable incomplete | `kiro.ts:1235` vs `kiro.go:792` |
| 3 | Kiro 스트림 catch 재시도 판정 | `kiro.ts:1257` vs `kiro.go:599` |
| 4 | Kiro 도구 결과: 암호화 거부, 빈 결과 sentinel, carrier 문구 | `kiro.ts:467` vs `kiro.go:247` |
| 5 | Kiro system prompt 예산이 호출자 지시를 자르지 않게 | `kiro.ts:355` vs `kiro.go:270` |
| 6 | Kiro identity 중립화 + tool catalog nudge | `kiro.ts:407`, go ABSENT |
| 7 | Kiro throttle 쿨다운 | `kiro-retry.ts:109`, go ABSENT |
| 8 | Cursor 네이티브 exec oneof 인코딩/디코딩 | `exec_wire.go:12`가 fs/shell/fetch를 안 봄 |
| 9 | Cursor `apply_patch` 변형 차단 배선 | `tool_guidance.go:69` 호출자 없음 |
| 10 | Cursor `codex-sandbox` 정책 의미 | `exec-policy.ts:28` vs `policy.go:49` |
| 11 | Cursor pre-commit 재시도 배선 | `retry.go:14` 호출자 없음 |
| 12 | Azure 어댑터 wire 경로 | `azure.ts:5` vs `azure.go:27` |
| 13 | identity 중립화가 새 Codex 문구를 놓침 | `identity.ts:27-40` vs `identity.go:6-13` |

### 070 — 관리 API 표면

| # | 항목 |
| --- | --- |
| 1 | `GET /v1/opencodex/artifacts/{id}` 등록 (`images.ResolveArtifactPath`는 이미 있음) |
| 2 | `POST /api/system/restart`를 `RegisteredRoutes()`에 추가 (핸들러는 존재) |
| 3 | `POST /api/oauth/accounts/clear-cooldown` |
| 4 | `POST /api/codex-auth/accounts/clear-cooldown`, `PUT/PATCH /api/codex-auth/pool-strategy` |
| 5 | `/api/injection-model`의 `syncCodexSubagentDefaults` + `available` 객체 형태 |

### 080 — CLI 표면

`models live/edit/enable/disable/provider/selected/context/shadow`,
`provider edit/quota/presets/account-mode/selected`, `debug` 런타임 스코프,
`doctor --fix-codex-runtime`, 그리고 그에 맞춘 help 텍스트. 백엔드는 대부분 이미 있으므로
대부분 배선 작업이다.

### 090 — 진단과 설정

`ocx sync`의 프로젝트 설정 경고, effort-clamp 진단 3종, journal injected-state,
계정 로그 라벨, Windows 플러그인 마켓플레이스 진단, 설정 키
`modelReasoningSummaryDelivery`/`codexAccountNamespaces`/`syncCodexSubagentDefaults`,
업데이트 프롬프트의 `RunUpdate`/`Exit`/캐시 갱신, Windows shim 경로 env 간접화.

### 100 — 비디오 브리지

순수 신규 기능이고 다른 것과 얽히지 않는다. `PlanVideoBridge`, `BuildVideoTool`,
xAI 비디오 submit/poll 클라이언트, 아티팩트 다운로드, Responses 경로 배선.

## 이 문서가 주장하지 않는 것

- 96건이 전부 즉시 고쳐야 할 버그라는 주장이 아니다. `OUT-OF-SCOPE`와
  `TS-DEAD-OR-GENERATED`는 이미 제외했지만, 남은 것 중에도 구현 중 반박될 수 있는 항목이 있다.
  각 work-phase의 P가 자기 decade 문서를 현재 트리에 재검증한 뒤 실행한다.
- Windows 전용 항목(shim 경로, `.cmd` 인용)은 macOS에서 정적 대조만 했다. 활성화 증거는
  Windows 호스트가 필요하며, 그 사실을 해당 work-phase가 명시해야 한다.
- 레인들이 "검증하지 않은 것"으로 남긴 항목(예: Kiro region resolver, WS registry의 go 위치)은
  다음 사이클의 조사 대상이지 결함 확정이 아니다.
