# 011 — L2 인벤토리: `src/adapters/cursor/**` (손으로 쓴 부분)

레인 범위 31파일 / 6,293줄. 대응 go 패키지 `go/internal/adapter/cursor`.
조사 시점 HEAD `2fe6d2100`. 정적 대조가 아니라 양방향 스윕으로 판정했다.

## 먼저 오탐 하나가 정정됐다

`260729_go_parity_chase`가 후보로 남겨둔 `FilterCursorConfiguredModelsByLiveDiscovery`는
미배선이 아니다.

```
internal/adapter/cursor/discovery.go:89:func FilterCursorConfiguredModelsByLiveDiscovery(...)
internal/cli/cursor_discovery.go:44:	return cursoradapter.FilterCursorConfiguredModelsByLiveDiscovery(
```

참조 2개, 프로덕션 호출자 있음. 그 문서의 후보 표시를 해소로 정정한다.

## 확정 결함

### 1. Cursor 네이티브 exec가 구현돼 있는데 wire에서 닿지 않는다 (UNWIRED, user-visible)

오라클: `src/adapters/cursor/native-exec.ts:147`, `native-exec-fs.ts:57`,
`native-exec-shell.ts:65`, `native-exec-network.ts:20`.
go: 실행기는 `exec_fs.go:92`, `exec_shell.go:63`, `exec_network.go:31`에 **있다**.
그런데 `go/internal/adapter/cursor/exec_wire.go:12`의 `UnmarshalExecServerMessage`는
request-context / MCP / resources / desktop 필드만 디코드한다. read·write·delete·ls·grep·
shell·shellStream·backgroundShellSpawn·writeShellStdin·fetch는 **디코드도 마샬도 되지 않는다.**

사용자가 겪는 것: Cursor가 네이티브 파일/셸/fetch를 요청하면 go는 빈 `ExecRequest`를 받고
의미 있는 응답을 보내지 못한다. 오류가 아니라 정지/열화로 나타난다.

최소 수정: `UnmarshalExecServerMessage`/`MarshalExecClientMessage`에 TS가 지원하는 oneof
케이스를 추가하고, 성공과 정책 거부 결과 양쪽을 마샬한다.

### 2. `apply_patch` 변형 차단이 배선되지 않았다 (UNWIRED, user-visible, 보안 성격)

오라클은 `apply_patch`가 광고되면 `rejectNativeFileMutations`를 켜서 Cursor 네이티브
write/delete를 거부하고 모델에게 `apply_patch`를 쓰라고 답한다
(`src/adapters/cursor/live-transport.ts:504`, `native-exec-fs.ts:87`).
go에는 `CursorRequestAdvertisesApplyPatch`(`tool_guidance.go:69`)가 있으나 **프로덕션
호출자가 없어** `RequestPolicy.DenyMutations`가 설정되지 않는다.

사용자가 겪는 것: `nativeLocalExec:on`에서 Cursor 네이티브 write/delete가 오라클의 Codex
편집 정책을 우회한다.

최소 수정: 턴별 네이티브 exec 컨텍스트를 만들 때 그 함수를 호출해 요청 스코프
`DenyMutations`를 세운다.

### 3. `nativeLocalExec: codex-sandbox` 의미가 다르다 (DIVERGENT, user-visible)

오라클은 요청이 `danger-full-access`를 선언한 경우에만 `codex-sandbox`를 허용한다
(`src/adapters/cursor/exec-policy.ts:17,28,41`).
go는 `codex-sandbox`를 사실상 거부와 같게 다루고(`policy.go:49,59`), 요청 선언 기반 허용
경로가 없다. 결과적으로 오라클에서 되던 것이 go에서는 `nativeLocalExec:on`을 켜야 된다.

### 4. pre-commit 재시도가 배선되지 않았다 (UNWIRED, user-visible)

오라클은 라이브 턴을 `runCursorTurnWithRetry`로 감싼다(`src/adapters/cursor.ts:116`,
`transport-retry.ts:66`). go의 `DoPreCommitRetry`(`retry.go:14`)는 선언/테스트뿐이다.
커밋 이전 일시적 전송 실패가 재시도되지 않는다.

### 5. 라이브 discovery effort 접미사 집합 불일치 (DIVERGENT, user-visible)

TS 정규 집합은 `none`을 포함(`effort-map.ts:51`), go는 `minimal`을 포함하고 `none`이 없다
(`discovery.go:18`). Cursor가 `model-none`을 반환하면 go만 그 base를 걸러낸다.

## 스윕에서 처분한 나머지 미참조 export

| 심볼 | 위치 | 처분 |
| --- | --- | --- |
| `BuildCursorToolDefinitions` | `tool_defs.go:111` | 테스트 seam. 프로덕션은 `tools.go:19` `budgetTools` 경로 |
| `ClearAll` | `context_usage.go:103` | 테스트 헬퍼 |
| `ClearCursorThreadContinuity` | `continuity.go:143` | 테스트 리셋 헬퍼 |
| `DecodeCursorArgsMap` | `arg_codec.go:31` | 테스트 seam, 동작은 배선됨 |
| `HandleKV` | `exec.go:228` | 패키지 내부, 프로덕션은 `proto.go:173` `marshalKVReply` |
| `NewLiveTransport` | `transport.go:46` | 대체/테스트 transport seam |

## OK로 확인된 주요 축

계정 스코프 스레드 연속성(`adapter.go:97`), 정적 카탈로그와 라이브 discovery, auto 라우터
파라미터, 툴 카탈로그 예산과 shell/apply_patch 고정, 최상위 `mcp_tools`, 클라이언트 툴콜을
로컬 MCP로 실행하지 않는 안전 경계, Connect 프레이밍, 로컬 MCP 서버, 데스크톱 브리지.

## 검증하지 않은 것

- 테스트/빌드 미실행, 라이브 Cursor 호출 없음.
- 생성 protobuf(`gen/agent_pb.ts`)는 L3 담당이라 제외.
