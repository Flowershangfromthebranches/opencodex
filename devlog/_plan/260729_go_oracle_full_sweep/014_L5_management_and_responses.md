# 014 — L5 인벤토리: `src/server/management/**` + `src/server/responses/**`

23파일 / 8,512줄. 대응 go: `internal/management/**`, `internal/server/responses_*.go`.

## 라우트 차등 (양방향)

오라클 121쌍(메서드+경로) 대 go 117쌍.

### go에 등록되지 않은 오라클 라우트 6개

| 라우트 | 오라클 | go |
| --- | --- | --- |
| `POST /api/storage/cleanup` | `logs-usage-routes.ts:273` | ABSENT |
| `POST /api/storage/cleanup-policy/run` | `:468` | ABSENT |
| `POST /api/oauth/accounts/clear-cooldown` | `oauth-account-routes.ts:319` | ABSENT |
| `POST /api/system/restart` | `system-routes.ts:90` | 핸들러는 `management/system.go:15`에 있는데 `RegisteredRoutes()`에 없음 |
| `POST /api/codex-auth/accounts/clear-cooldown` | `auth-api.ts:785` | ABSENT |
| `PUT/PATCH /api/codex-auth/pool-strategy` | `auth-api.ts:841` | ABSENT |

`system/restart`가 특히 눈에 띈다. 핸들러가 있는데 라우트 목록에 없으니 프로덕션 mux가
등록하지 않는다 — 대시보드 메모리 카드의 재시작 버튼이 닿지 않는다.

### 오라클에 없는 go 전용 라우트 3개

`POST /api/combos/reset`, `GET/PUT /api/model-aliases`. 사용자 도달 가능한 확장이므로
기록해둔다(오라클 방향 역행은 아니지만 표면 차이).

## 응답 키 차등 (공유 라우트)

| 라우트 | 오라클이 담는 것 | go | 사용자 증상 |
| --- | --- | --- | --- |
| `GET /api/storage/cleanup-policy` | 정책 + `job` (`:446-451`) | 정책만 (`storage_routes.go:32-36`) | `Storage.tsx:920`이 `body.job`을 폴링 → 실행 완료를 볼 수 없음 |
| `PUT /api/storage/cleanup-policy` | `{ok, policy, job}` (`:454-465`) | 원본 정책만 (`:90-125`) | `Storage.tsx:813,853`이 `json.policy` 요구 → 저장이 실패로 보임 |
| `GET /api/injection-model` | `syncCodexSubagentDefaults` + `available[{provider,model,namespaced}]` (`agent-settings-routes.ts:192-199`) | 토글 없음, `available`이 `[]string` (`agents.go:63-69`) | 대시보드 토글이 항상 꺼짐, 드롭다운 라벨이 깨짐 |
| `PUT /api/injection-model` | `syncCodexSubagentDefaults` 반환 (`:280-285`) | 미지원 (`agents.go:72-134`) | "네이티브 Codex 서브에이전트 기본값으로 사용" 토글이 저장되지 않음 |

## 오래된 보고 정정

`/api/subagent-models`의 `available` 누락은 해소됐다(`agents.go:21-27`). 스토리지 라우트가
통째로 404라는 보고도 낡았다 — 읽기 클러스터는 랜딩했고, 위의 두 라우트와 응답 형태만 남았다.

## Responses 서버 계층

`core.ts:1060`, `compact.ts:120`, `encrypted-payload.ts:182`, `terminal-guard.ts:89`,
`collaboration.ts:102` 전부 go 대응이 있다(`responses_core_port.go:210`,
`responses_compact_port.go:39`, `encrypted_payload_port.go:144`, `terminal_guard.go:132`,
`collaboration_port.go:33`). 이 레인에서 라우트 수준 Responses 결함은 확인되지 않았다.

## 처분한 미참조 export

`RequestAttempt.MarkFirstOutput`(`logs.go:64`), `RequestLog.Hydrate`(`logs.go:172`) — 대응하는
오라클 프로덕션 호출자를 이 레인에서 찾지 못해 내부/레거시로 처분. (L9가 별도로 usage
`ReadRecent` 미배선을 확정했으므로 `Hydrate`는 050/090에서 재검토 가치가 있다.)

## 검증하지 않은 것

라이브 `:10100` 미호출 — 소스와 실행 중 데몬은 다를 수 있다. 테스트/빌드 미실행.
