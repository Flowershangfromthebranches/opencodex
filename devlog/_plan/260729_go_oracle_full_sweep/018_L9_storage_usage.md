# 018 — L9 인벤토리: `src/storage/**` + `src/usage/**`

16파일 / 6,420줄(그중 `cleanup.ts`만 3,014줄). 대응 go: `internal/storage`(35파일),
`internal/usage`. exported 200개 스윕.

## 파괴적 경로 차등 — 이 유닛에서 가장 위험한 발견

**선택과 미리보기는 이식됐다.** archived 후보 나열, `.jsonl`+`.jsonl.zst` 묶기, 안전하지 않은
이름 거부, 오래된 것부터 정렬, size/mtime/물리 경로를 digest에 바인딩, pending restore 보호,
정확 후보 해석과 digest 거부 계약 — 전부 go에 있다
(`cleanup.go:133,204,237,301`, `cleanup_resolve.go:13`, `cleanup_decide.go:35`).

**실행기는 없다.** 오라클 `cleanup.ts:1733`의 순서(DB 프로브 → 참조 로드 → 스테이징 →
매니페스트 → DB 조정 → 롤백/삭제)를 조립하는 go 진입점이 없다. 원시요소는 다 있는데
호출자가 없다:

```
DecideArchivedCleanup   internal/storage/cleanup_decide.go:35   비테스트 참조 = 선언뿐
StageCandidates         internal/storage/cleanup_stage.go:31    같음
RollbackStaged          internal/storage/cleanup_stage.go:67    같음
PurgeStaged             internal/storage/cleanup_stage.go:92    같음
RemoveStageIfEmpty      internal/storage/cleanup_stage.go:111   같음
CreateExclusiveStageDir internal/storage/quarantine.go:78       같음
WriteSatelliteBackup    internal/storage/satellite_backup.go:86 같음
SnapshotStateDependents internal/storage/snapshot.go:57         같음
```

그리고 `POST /api/storage/cleanup` 라우트도 없다(`storage_routes.go:25`).
즉 Storage 페이지는 후보를 미리 볼 수 있지만 **실행 버튼이 닿는 곳이 없다**.

restore는 반대로 잘 배선돼 있다(`restore_entry.go:14` + `storage_routes.go:39,181`),
뮤테이션 슬롯 잠금도 동등(`mutation.go:190`).

정책 실행도 미배선이다. 평가 헬퍼는 `policy_run.go:119`에 있으나 비공개이고, 기본값
로드/저장 래퍼도 `POST /api/storage/cleanup-policy/run` 라우트도 없다.

## usage 쪽 결함

| # | 항목 | 등급 | 근거 |
| --- | --- | --- | --- |
| 1 | 가격 해석: jawcode 정확 행 + vendor 모델 수준 폴백 | DIVERGENT | 오라클 `cost.ts:139,190,221`; go `prices.go:101`은 하드코딩 오버레이만 |
| 2 | 캐시 read/write 분리에서 명시적 0 구분 불가 | DIVERGENT | `cost.ts:106` vs `cost.go:53`. go는 `cacheReadInputTokens: 0`과 부재를 구분 못 해 legacy `cachedInputTokens`로 폴백 |
| 3 | 최근 usage로 요청 로그 시딩 | UNWIRED | 오라클 `request-log.ts:197`, `log.ts:509`; go `usage/log.go:168` `ReadRecent` 호출자 없음 |

가격 결함은 조용하다 — 숫자가 틀려도 오류가 아니라서 아무도 신고하지 않는다.

## OK로 확인된 축

스토리지 스캔 버킷과 `.trash` 제외, 퍼센트 선택과 목표 개수, 미리보기 digest 계약,
pending restore 예외 처리, restore 라우트/코디네이터/오류 매핑, 뮤테이션 슬롯,
정책 정규화/GET/PUT, usage 로그 append/read/snapshot, usage 요약 범위와 표면 집계,
OpenAI priority tier 배수, usage 디버그 리댁션과 롤링 캡.

## 검증하지 않은 것

cleanup·restore·정책 실행을 **한 번도 돌리지 않았다**(임시 CODEX_HOME에서도). 소스/grep
인벤토리만. jawcode 메타데이터 행 단위 비교는 하지 않았고, 가격 결함은 코드 경로 능력 기준이다.
