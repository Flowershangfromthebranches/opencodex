# 050 — WP2: 파괴적 스토리지 경로

`030_synthesis.md`가 두 번째 사이클로 지목한 유닛. 사용자 데이터를 옮기는 코드라
잘못 배선하면 되돌릴 수 없다.

## P 재검증: 무엇이 있고 무엇이 없나

L9(`018_L9_storage_usage.md`)의 보고를 현재 트리에서 다시 쟀다.

**결정층은 있다.** `DecideArchivedCleanup`(`cleanup_decide.go:35`)이 오라클
`cleanup.ts:1733`의 admission 순서를 그대로 담는다 — 모드 검증 → digest 형식 → 후보 해석 →
digest 대조 → pending restore 겹침. 스테이징 원시요소도 있다(`cleanup_stage.go`의
`StageCandidates`/`RollbackStaged`/`PurgeStaged`/`RemoveStageIfEmpty`).

**삭제 방향 DB 조정층이 없다.** go에 DB 조정 자체가 없는 것은 아니다 —
복원 방향은 있다(`thread_reconcile.go:294-296`이 스레드 행을 되살리고,
`satellite_commit.go:141-147`이 복원 satellite 섹션을 커밋한다). 없는 것은 삭제 방향이다.
오라클 `reconcileDeletedThreads`(`cleanup.ts:1435-1530`)가 요구하는
원시요소를 go에서 이름과 의미로 찾은 결과:

```
BeginSatelliteWriteLocks       satellite_locks.go:64     있음
RollbackAllSatelliteLocks      satellite_locks.go:150    있음
WriteSatelliteBackup           satellite_backup.go:86    있음
ClearSatelliteBackup           satellite_backup.go:146   있음
SnapshotStateDependents        snapshot.go:57            있음
SnapshotSatelliteBackupInLocks ABSENT
DeleteAndCommitSatellites      ABSENT
RestoreSatelliteBackup         ABSENT
LoadMatchingThreads            ABSENT
FindReferencedHistory          ABSENT
DeleteThreadsAndDependents     ABSENT
```

없는 여섯 개가 전부 **삭제 방향**이라는 점이 중요하다. go의 satellite 코드는 restore
방향으로만 지어져 있다(`satellite_commit.go`의 `CommitSatelliteSections`는
`RestoreMovedState`를 받는다). 삭제 경로는 스냅샷 → 삭제 → 실패 시 복원이라는 반대
생명주기가 필요하고, 그 위에 상태 DB의 `BEGIN IMMEDIATE` 트랜잭션과 참조 이력 재확인이 얹힌다.

## 이 사이클의 범위 결정 (PHASE-SPLIT-01)

오라클 실행기는 `cleanup.ts:1733-1955` 220여 줄이고, 그것이 부르는 `reconcileDeletedThreads`가
100줄, 그 아래 satellite 삭제/복원이 더 있다. **한 사이클에 넣으면 검증이 흐려진다.**

그리고 순서상 더 중요한 사실이 있다: 실행기를 다 만들어도 **정책 저장은 여전히 고장난 채다.**
`Storage.tsx:813-818`이 PUT 응답에서 `json.policy`를 읽고 없으면 곧장 실패로 표시한다
(`:853-859`의 run-now 저장 경로도 같다). go는 정책을 최상위로 반환하므로 저장이 성공해도
사용자는 실패를 본다. 이것은 실행기와 무관한 **독립적 사용자 가치**다.
+
GET의 `job`은 성격이 다르다. 일반 로드에서는 `policyFieldsFromResponse`(`:683-686`)가 그 키를
버리므로 없어도 무해하고, 폴링(`:920-925`)도 `if (!job) continue`로 방어한다. 즉 `job`은
**오라클 계약이자 다음 사이클을 위한 준비**이지, 이 사이클이 Run Now를 고친다는 뜻이 아니다.
Run Now는 `POST /cleanup-policy/run`이 붙는 WP2b 전에는 동작할 수 없다 — 그 전까지 폴링은
`job.lastOutcome`을 못 받고 `:939-941`에서 타임아웃으로 실패 처리된다.

그래서 이 사이클은 **응답 계약**을 닫고, 실행기는 다음 사이클로 자른다.

### 이 사이클 (WP2a): 정책 응답 계약 + 실행 상태

#### 1. `GET /api/storage/cleanup-policy`에 `job` 추가

오라클 `logs-usage-routes.ts:446-451`:

```ts
const policy = normalizeStorageCleanupPolicy(config.storageCleanupPolicy);
return jsonResponse({ ...policy, job: getStorageCleanupPolicyJobState() });
```

go `storage_routes.go:32-36`은 정책만 반환한다.

#### 2. `PUT`이 `{ok, policy, job}`을 반환

오라클 `:454-465`는 `{ ok: true, policy: saved, job: ... }`. go `:90-125`는 정책 자체를
최상위로 반환한다. GUI가 `json.policy`를 읽으므로 **저장이 항상 실패로 보인다.**

오라클에는 하나 더 있다(`:460-462`): 클라이언트가 `enabled`를 생략하면 이전 값을 유지한다 —
암묵적 활성화 금지. go는 이미 보존하는 것으로 보인다(`policy_input.go:126-132`가 덮어쓰기 전에
`"enabled": prev.Enabled`를 시드하고, 라우트가 previous를 넘긴다 — `storage_routes.go:101-103`).
다만 기존 테스트(`policy_input_test.go:197-205`)는 **한쪽 방향만** 증명한다: 생략이 기본 false를
켜지 않는다는 것. 이전 값이 true일 때 생략이 그것을 유지하는지는 증명되지 않았다.
파괴적 정책 경계이므로 양방향을 명시적으로 단언한다(감사 블로커 3).

#### 3. 정책 실행 상태(`PolicyJobState`)를 go에 만든다

오라클 `policy-job.ts:35-41`의 `PolicyJobState` 전체 모양(감사 블로커 2 — 초판은 `finishedAt`과
outcome 필드를 빠뜨렸다):

```
PolicyJobState { status, reason?, startedAt?, finishedAt?, lastError?, lastOutcome? }
PolicyJobOutcome { ok, skipped?, deferred?, error?, mode?, freedBytes?, removed?, trashDir? }
```
(`policy-job.ts:24-33`)

NEW `go/internal/storage/policy_job_state.go`: **전체 모양을 지금 정의한다.** 그래야 WP2b가
실제 상태 전이를 붙일 때 필드가 늘어나는 것이 스코프 드리프트로 보이지 않는다.

이 사이클에서는 전이가 없으므로 읽기는 `{status:"idle"}`을 반환한다. 그것이 거짓이 아닌
이유는 실제로 돌고 있는 job이 없기 때문이고, 오라클도 실행 전에는 같은 값을 준다.
**테스트는 "지금 idle이고 모양이 호환된다"를 단언하지, "영원히 idle"을 제품 불변식으로
박지 않는다** — 그것이 WP2b를 회귀처럼 보이게 만드는 함정이다.

#### 4. `POST /api/storage/cleanup-policy/run`은 이 사이클에서 만들지 않는다

실행기가 없는데 라우트만 만들면 200을 받고 아무 일도 안 일어난다 — L4가 지적한
"조용히 잘린 응답"을 우리가 새로 만드는 셈이다. 라우트는 실행기와 같은 사이클에 붙인다.

### 다음 사이클 (WP2b, goalplan에 append)

`ExecuteArchivedCleanup` 코디네이터 + 삭제 방향 satellite 원시요소 6종 +
`POST /api/storage/cleanup` + `POST /api/storage/cleanup-policy/run`.

## 검증 계획

- `go build ./... && go vet ./...`
- NEW `go/internal/management/storage_policy_contract_test.go`: GET이 `job`을 담는지,
  PUT이 `{ok, policy, job}`을 담는지 — **GUI가 읽는 정확한 키 이름**으로 단언한다.
  키 이름이 계약이므로 구조체 필드명이 아니라 JSON 키를 본다.
- `enabled` 생략 양방향 테스트: 이전 false + 생략 → false 유지, 이전 **true** + 생략 → true 유지.
- job 상태 테스트는 "현재 idle이며 오라클 모양과 호환"을 단언한다(영구 idle을 박지 않는다).
- 라이브 확인: `:10100`에 실제 GET/PUT을 날려 응답 본문을 증거로 남긴다.
- 파괴적 IO는 이 사이클에 없으므로 임시 CODEX_HOME 실행도 필요 없다.

## 범위 밖

파괴적 파일 이동, 실제 cleanup 실행, 오라클 수정, 다른 세션이 작업 중인 파일.
