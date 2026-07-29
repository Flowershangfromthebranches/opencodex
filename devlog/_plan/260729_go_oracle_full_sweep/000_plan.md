# 000 — 오라클 전수 대조: go 런타임이 아직 삼키지 못한 로직을 전부 찾는다

브랜치 `dev2-go`, 기준 커밋 `9014787d3`, 작성 2026-07-29 (A-phase 감사 반영 개정).
세션 goalplan: `.codexclaw/goalplans/exhaustively-audit-the-opencodex-typescript-orac/`.

## 이 유닛이 앞의 세 유닛과 다른 점

`260728_go_port_parity`는 CLI/스토리지/OAuth 축을 세웠고, `260729_go_port_blindspot_sweep`은
"파리티 테스트가 보지 않는 관리 API 경로에 결함이 산다"를 실측했고, `260729_go_parity_chase`는
그 시점에 남아 있던 4개 라우트와 cli-proxy-api 대조 3건을 닫았다. 셋 다 **표적 조사**였다.
각각 라우트 표면, 관리 API, 외부 구현 대조라는 렌즈를 하나씩 들고 그 렌즈에 잡히는 것만 봤다.

이번 유닛의 요구는 다르다 — "오라클의 어떤 로직도 빠지면 안 된다". 렌즈를 고르지 않고
`src/` 전면을 훑는다. 규모는 이렇다.

```
오라클 src/:      112,856줄 / TS 파일 352개 (cursor/gen/agent_pb.ts 15,274줄 생성물 포함)
go/internal/:     104,355줄 (non-test, `9014787d3` 시점)
```

go 줄 수는 다른 세션이 같은 브랜치에 커밋을 얹는 동안 계속 움직인다. 이 숫자는 스냅샷이며
판정 근거가 아니다 — 판정 근거는 언제나 그 시점의 `path:line`이다.

줄 수가 비슷하다는 사실은 아무것도 증명하지 않는다. 앞선 세 유닛이 반복해서 보여준 실패
모양은 "구현이 틀렸다"가 아니라 **"구현이 끝까지 연결되지 않았다"**였고, 그런 코드는 줄 수에
정상적으로 계상된다. 그래서 이 유닛의 판정 단위는 파일이나 줄이 아니라 **동작**이다.

## 방법: 10 레인 병렬 조사, 정적 대조는 증거가 아니라 후보 생성기

`src/` 전면을 서로 겹치지 않는 10개 도메인 레인으로 자르고, 레인마다 gpt-5.5 explorer
서브에이전트를 하나씩 붙인다. 각 레인은 자기 담당 오라클 파일에서 **동작 단위**를 뽑고,
대응하는 go 코드를 찾아 다섯 등급 중 하나로 판정한다.

| 등급 | 의미 | 판정에 필요한 증거 |
| --- | --- | --- |
| `MISSING` | go에 대응 구현이 없다 | 오라클 `path:line` + go 트리 전체 검색 결과가 빈 것 |
| `UNWIRED` | go에 구현은 있으나 프로덕션 호출자가 없다 | go 선언 `path:line` + 비테스트 참조가 자기 선언뿐임 + 오라클의 호출 지점 |
| `DIVERGENT` | 양쪽 다 있으나 동작이 다르다 | 양쪽 `path:line` 한 쌍 + 무엇이 어떻게 다른지 |
| `OK` | 충실히 이식됨 | go `path:line` |
| `OK-EQUIVALENT` | 구현 방식은 다르나 관측 동작이 같다 | 양쪽 `path:line` + 왜 동등한지 (예: go가 생성 스키마 전체 대신 안정 부분집합만 유지) |
| `OUT-OF-SCOPE` | go 포트가 가질 이유가 없다 | 오라클 `path:line` + 왜 Bun/TS 전용인지 (예: `src/lib/bun-runtime.ts`) |
| `TS-DEAD-OR-GENERATED` | 오라클 쪽이 죽었거나 생성물이다 | 오라클 `path:line` + TS 프로덕션 참조가 없음을 보인 결과 |
| `FALSE-POSITIVE` | 정적 대조로는 빠져 보이나 실제로는 있다 | 왜 정적 비교가 속았는지 |

뒤의 세 등급을 `OK`나 `FALSE-POSITIVE`에 뭉뚱그리면 종합 단계에서 "동등 이식"과 "go의
관심사가 아님"과 "오라클 쪽 사문화"를 구별할 수 없다. 그러면 다음 사이클이 존재하지 않는
작업을 계획하거나 실재하는 결함을 면제한다.

`FALSE-POSITIVE` 칸이 장식이 아닌 이유는 `260729_go_port_blindspot_sweep` §4.3에 이미
기록돼 있다. `go/internal/bridge/bridge.go:569`가 `eventType := "response." + status`로
이벤트 이름을 조립하기 때문에 문자열 grep은 `response.completed`/`failed`/`incomplete`가
전부 없다고 보고한다. 셋 다 정상 방출된다. 같은 유닛의 `260729_go_parity_chase`도
`/api/system/restart`와 `/api/oauth/accounts/clear-cooldown`을 미이식으로 잡았다가 오탐으로
정정했다. **정적 대조 결과는 후보 목록이지 결함 목록이 아니다.**

### 레인 지도 (서로소, `src/` 전체를 덮는다)

**집행 규칙 (STRICT):** 레인 실행은 `001_lane_assignment.txt`를 축자적으로 따른다. 아래 표는
사람이 읽는 요약이고, 파일 소속과 규모의 유일한 근거는 그 데이터 파일이다. A 감사가 초판
산문의 규모 수치와 실제 배정 사이 불일치를 잡아냈기 때문에 이 우선순위를 못박는다.
`001_lane_assignment.txt`는 phase 문서가 아니라 **생성된 데이터 부록**이다.

감사가 지적한 과적 레인 둘을 쪼개 12 레인이 됐다. 슬라이스를 결정하는 제약은 병렬 파견 수가
아니라 "한 레인이 한 서브에이전트의 컨텍스트에 들어가야 한다"이다.

| 레인 | 오라클 범위 | files/lines | 대응 go 패키지 | 산출 문서 |
| --- | --- | --- | --- | --- |
| L1 | `src/adapters/` 최상위 (cursor·kiro 제외) | 22 / 5,703 | `adapter/{anthropic,google,openai}`, `adapter/preflight.go` | `010` |
| L2 | `src/adapters/cursor/**` (생성물 제외) + `cursor.ts` | 31 / 6,293 | `adapter/cursor` | `011` |
| L3 | `src/adapters/cursor/gen/agent_pb.ts` + `kiro*.ts` | 12 / 18,168 | `adapter/kiro`, `adapter/cursor/proto.go` | `012` |
| L4 | `src/server/` 최상위 (management·responses 제외) | 28 / 8,364 | `server/` (라우팅·릴레이·라이브·로그·이미지) | `013` |
| L5 | `src/server/management/**` + `src/server/responses/**` | 23 / 8,512 | `management/`, `server/responses_*_port.go` | `014` |
| L6 | `src/codex/**` | 47 / 14,773 | `codex/` | `015` |
| L7 | `src/cli/**` | 33 / 8,268 | `cli/` | `016` |
| L8 | `src/oauth/**` + `src/providers/**` | 44 / 10,964 | `oauth/`, `providers/`, `registry/` | `017` |
| L9 | `src/storage/**` + `src/usage/**` | 16 / 6,420 | `storage/`, `usage/` | `018` |
| L10 | `src/lib/**` + `src/claude/**` + `src/update/**` + `src/tray/**` | 49 / 10,170 | `lib/`, `claude/`, `update/`, `tray/`, `platform/` | `019` |
| L11 | 루트 모듈 9개 + `src/responses/**` | 15 / 8,009 | `server/responses_*`, `server/responses_state.go`, `bridge/`, `config/`, `service/`, `types/`, `protocol/`(저수준 SSE·retry·stall만) | `020` |
| L12 | `chat/` `combos/` `grok/` `images/` `vision/` `web-search/` `generated/` | 32 / 7,212 | `chat/`, `combos/`, `grok/`, `images/`, `vision/`, `search/` | `021` |

L3이 12파일에 18,168줄인 것은 생성 protobuf 15,274줄 때문이다. 그 파일은 동작 인벤토리가
아니라 **wire 계약 대조** 대상이다 — go는 `adapter/cursor/proto.go`에서 생성 스키마를 통째로
들지 않고 안정 부분집합만 유지하므로, 판정은 "필드가 없다"가 아니라 "쓰는 필드가 맞는가"다.
L11의 `src/responses/**`는 `protocol/`이 아니라 `server/responses_*`와 `bridge/`가 주
대응이다(감사 블로커 3).

합계가 `src/` 전체와 일치하는지는 §5의 재현 스크립트로 확인한다. 현재 결과: 12 레인 합
352파일, 중복 0, 누락 0.

### 각 레인이 반드시 하는 것

1. 담당 오라클 파일에서 **export되는 동작**과 **프로덕션 호출 지점**을 뽑는다. 순수 내부
   헬퍼는 그 자체로 항목이 아니고, 그것을 쓰는 상위 동작이 항목이다.
2. go 쪽 대응을 찾는다. 이름이 다를 수 있으므로 이름 grep으로 끝내지 않고, 해당 go 패키지의
   심볼 목록을 훑어 의미로 대조한다.
3. **양방향 스윕 (STRICT).** TS→go 방향만으로는 이 포트의 지배적 결함 모양인 "이식됐지만
   아무도 부르지 않는 모듈"을 구조적으로 놓친다 — `260729_go_port_blindspot_sweep` §4가
   실측한 대로 exported 933개 중 비테스트 참조가 자기 선언뿐인 것이 112개였다. 그래서 각
   레인은 담당 go 대응 패키지의 **exported 심볼을 전부 열거하고 비테스트 프로덕션 참조를
   센 뒤**, 참조 없는 심볼마다 오라클에 프로덕션 호출자가 있는지 대조한다. 레인 보고서는 그
   스윕에 쓴 명령과 출력을 포함해야 하고, 참조 없는 심볼 각각을 `테스트 전용` /
   `패키지 내부 전용` / `범위 밖` / `UNWIRED 확정` 중 하나로 처분해야 한다.
   **스윕 출력이 없는 레인 보고서는 무효이며, 특히 "전부 OK"는 스윕 출력 없이 받지 않는다.**
4. 등급 표로 반환한다. 각 행에 `src/...:line`과 `go/internal/...:line`(없으면 `ABSENT`).
5. 사용자 표면에 닿는 정도를 `user-visible` / `internal` 로 표시한다.

참조 카운트 예시(레인이 자기 패키지에 맞춰 변형):

```bash
cd go
grep -nE '^func [A-Z]|^func \([^)]+\) [A-Z]|^type [A-Z]' internal/<pkg>/*.go | grep -v _test.go
# 후보 심볼마다
grep -rn "\b<Symbol>\b" internal cmd --include='*.go' | grep -v _test.go
```

### 각 레인이 하지 않는 것

- 코드 수정. 이 사이클은 docs-only다.
- 오라클 재작성 제안. go가 오라클을 따라간다.
- 결함 판정을 문서 인용으로 대체하는 것. `DEAD_EXPORT_AUDIT.md`는 `e5ba7b7b` 기준이라
  지금 트리보다 오래됐다 — 인용하려면 재측정이 선행이다.

## 무엇이 이미 닫혔는지 (재조사 대상에서 빼지는 않되, 중복 보고는 표시한다)

오늘 하루에 랜딩한 것들. 레인이 이것들을 `OK`로 재확인하면 그것도 유효한 결과다.

| 항목 | 커밋 |
| --- | --- |
| `/api/subagent-models`·`/api/injection-model`의 `available` | `c2bfd6ec2` |
| 업데이트 알림 CLI 배선 | `758cd4af3` |
| 완료 응답 output 재조립 | `7baffd64e` |
| cleanup 결정 계약 / 스테이징 + 롤백 | `5a655121c`, `86be6732b` |
| 비활성 모델 관용 매칭 | `9ec3bb4a8` |
| restore 진입점 + 라우트 | `a604d46b4`, `bb5aa976e` |

## 산출물

- `001_lane_assignment.txt`: 생성된 레인 배정 데이터 부록(phase 문서 아님).
- `010`~`021`: 레인별 인벤토리 문서(L1→`010` … L12→`021`).
- `030_synthesis.md`: 12개 레인을 합쳐 중복 제거하고 의존성 순서로 정렬한 종합 표.
- `040` 이후: 구현 work-phase마다 decade 문서 하나. 각 문서는 정확한 경로,
  NEW/MODIFY/DELETE, before/after diff를 담는 실행 가능한 PRD여야 한다
  (DIFFLEVEL-ROADMAP-01). 개수는 종합 결과가 결정하므로 이 문서에서 미리 못 박지 않는다 —
  대신 종합 직후 goalplan `workPhases[]`에 1:1로 append한다.

## loop-spec (C4)

- **archetype**: spec-satisfaction repair. 오라클이 검증자다.
- **trigger**: 사용자 요구 "오라클의 어떤 로직도 빠지면 안 된다".
- **goal**: go 런타임이 오라클의 사용자 도달 동작을 빠짐없이 수행한다.
- **non-goals**: 오라클 수정, 릴리스, 푸시, cursor 생성 protobuf 재생성.
- **verifier**: `cd go && go build ./... && go vet ./... && go test ./...` + 변경된 표면의
  라이브 호출(:10100).
- **stop condition**: 모든 레인 인벤토리가 처분 완료이고, `MISSING`/`UNWIRED`/`DIVERGENT`
  항목이 각각 go 수정+테스트로 닫히거나 증거를 동반한 반박으로 기록됨.
- **memory artifact**: 이 유닛 폴더 + goalplan + ledger.
- **write scope**: `go/**`, 이 유닛 폴더, `.codexclaw/goalplans/**`.
- **dirty-tree 보호**: `dev2-go`는 공유 브랜치이고 다른 세션이 같은 트리에 커밋을 얹는다.
  초판이 보호 대상으로 적은 5개 파일은 그 세션이 `f72aa112a`(end_turn/message phase)와
  `9014787d3`(system env opt-out)으로 커밋해 이미 깨끗해졌다. 그러므로 고정 파일 목록이
  아니라 **규칙**이 보호 장치다: 모든 write work-phase는 시작 직전 `git status --short`를
  다시 읽고, 그 시점에 더러운 파일은 건드리지 않으며, 겹치면 먼저 보고한다.
- **escalation**: 오라클 동작 자체가 모호해 이식 방향을 정할 수 없을 때 → `NEEDS_HUMAN`.
- **terminal outcomes**: `DONE` / `BLOCKED` / `NEEDS_HUMAN` / `BUDGET_EXHAUSTED`.
  남은 도메인 목록은 종료 사유가 아니라 다음 work-phase다(LOOP-UNIT-CHAIN-01).

## 5. 재현: 레인 커버리지 검증

```bash
cd /Users/jun/Developer/new/700_projects/opencodex
# 레인이 덮는 파일 집합이 src/ 전체와 같은지
find src -name '*.ts' | sort > /tmp/all.txt
wc -l /tmp/all.txt   # 352
# 레인 배정 데이터가 이 집합과 정확히 일치하는지
grep -v '^##' devlog/_plan/260729_go_oracle_full_sweep/001_lane_assignment.txt | grep . | sort > /tmp/lanes.txt
diff <(sort /tmp/all.txt) /tmp/lanes.txt && echo COVERAGE_OK
```

각 레인 문서의 §담당 파일 목록을 합집합했을 때 이 352개와 일치해야 한다. 불일치는
종합 문서(`030`)에서 명시적으로 처분한다.
