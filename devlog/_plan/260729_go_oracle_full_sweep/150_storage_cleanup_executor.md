# 150 — WP2b: 저장소 정리 실행기 (읽기 방향 먼저)

`050_storage_execute.md`가 `wp2b`로 이월한 파괴적 실행 경로. 재측정 결과 **이 유닛은 한
사이클에 들어가지 않는다.** 안전 게이트에 해당하는 읽기 방향 2건을 먼저 닫고, 삭제 방향은
열어둔다.

## 재측정: go에 이미 있는 것

`internal/storage`는 비어 있지 않다. 스테이징·롤백·퍼지·위성 백업이 이미 있다.

| 오라클 | go | 상태 |
| --- | --- | --- |
| `stageCandidates` / `rollbackStaged` / `purgeStaged` | `cleanup_stage.go:31,67,92` | 있음 |
| 위성 백업 쓰기/삭제 | `satellite_backup.go:86,146` | 있음 |
| 격리 디렉터리·no-replace rename | `quarantine.go:78,115` | 있음 |
| preview·digest·선택 | `cleanup.go:238,380,405` | 있음 |
| `POST /api/storage/cleanup/preview` | `storage_routes.go:27` | 있음 |

없는 것은 **DB 쪽 6개 프리미티브와 그것을 엮는 실행기**, 그리고 `POST /api/storage/cleanup`
라우트다.

## 이번 사이클에서 닫은 것 (`5f637de17`)

읽기 방향 2건 + 경로 정규화. **이것들이 안전 게이트다** — 나중에 무엇이 파일을 지우든,
어느 행이 삭제 대상인지 정하는 코드보다 안전할 수는 없다.

### `NormalizeArchivedRolloutPath`

정리가 `archived_sessions` 밖으로 나가지 못하게 막는다. 활성 세션·다른 디렉터리·홈 밖을
가리키는 rollout은 `""`가 되어 절대 매칭되지 않는다. 이것이 없으면 퍼센트 정리가 **살아 있는
대화에 닿을 수 있다.**

Windows 드라이브 절대경로를 모든 호스트에서 인식한다 — Windows에서 쓴 스토어를 다른 데서 열 수
있기 때문이다. 파일명 뒤쪽의 콜론은 드라이브 문자가 아니다(Codex rollout 파일명에 ISO
타임스탬프가 들어간다).

### `LoadMatchingThreads`

`archived` 컬럼이 있으면 `archived=1`만 삭제 대상이다. rollout이 우연히 `archived_sessions`에
있는 **활성 스레드는 정리 대상이 아니다.** 컬럼이 없는 구 스키마는 경로 매칭만으로 판단한다
(오라클과 동일).

`threads` 테이블이 없으면 "매칭 0건"이 아니라 **거부**다. 이해하지 못하는 스토어를 상대로
정리를 진행시키면 안 된다.

### `FindReferencedHistory`

삭제가 다른 것을 고아로 만드는 세 경우 중 하나라도 걸리면 거부한다: paginated history,
삭제 경계를 넘는 spawn edge, 살아남는 스레드가 우리 것을 fork 원본으로 지목하는 경우.

**실제 DB 오류는 전파한다.** busy/손상을 "참조 없음"으로 읽으면 실패한 질문을 근거로 데이터를
지우게 된다 — 가능한 결과 중 최악이다.

동시에 삭제 집합 **안에서 닫힌** edge는 허용한다. 전부 거부하는 규칙은 기능을 무용지물로
만든다.

## 남은 것 (`wp2c`)

- `DeleteThreadsAndDependents` — 의존 행까지 지우는 트랜잭션
- `SnapshotSatelliteBackupInLocks` / `DeleteAndCommitSatellites` / `RestoreSatelliteBackup`
  — 위성 DB의 락 안 스냅샷·커밋·롤백
- `ExecuteArchivedCleanup` — 위 전부를 오라클 순서대로 엮는 조정자
  (`cleanup.ts:1733-1900`, 약 200줄)
- `POST /api/storage/cleanup`, `POST /api/storage/cleanup-policy/run`

이것들을 한 커밋에 넣지 않는 이유: **되돌릴 수 없는 삭제**이고, 롤백 경로가 실패했을 때의
동작이 정확해야 한다. 임시 `CODEX_HOME`에서 실제 실행 + 롤백 증거를 요구하는 검증이 필요하고,
그것은 자기 사이클에 값한다.
