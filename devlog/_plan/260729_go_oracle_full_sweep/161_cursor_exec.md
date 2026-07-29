# 161 — WP3c: Cursor exec 표면 재측정

`060`이 Kiro 유닛에 함께 적었던 Cursor 항목들. Kiro와 코드를 공유하지 않으므로 `wp3b`에서
분리해 여기서 잰다. **1건을 닫았고 나머지는 이미 있거나 결함 미확인이다.**

| 항목 | 재측정 | 처리 |
| --- | --- | --- |
| apply_patch 가드 | 부재 (상수와 프롬프트 문구만 있음) | 닫음 `8fed82073` |
| 샌드박스 정책 | `policy.go`에 완비(`ExecCodexSandbox` 포함) | — |
| pre-commit 재시도 | 동등한 게이트 존재 | — |
| native exec wire | 이미 동작 | — |

## apply_patch 가드 (`8fed82073`)

가장 실질적인 결함이었다. Cursor의 네이티브 실행기가 **apply_patch가 광고된 요청에서도**
파일을 직접 쓰고 지웠다. 그러면 Codex가 파일 편집에 대해 하는 일이 전부 우회된다 — 승인,
샌드박스 정책, 사용자가 보는 diff, rollout 기록. 편집은 Codex 뒤에서 일어나고 턴은 정상으로
보인다.

go에 `ApplyPatchTool` 상수가 있고 프롬프트 문구도 `apply_patch`를 쓰라고 말하지만,
**강제하는 코드가 없었다.**

### 세 조건이 모두 필요하다

오라클(`tool-definitions.ts:135-141`)과 동일하게:

- **top-level**: 네임스페이스가 붙은 동명 도구는 다른 도구다.
- **freeform**: Codex의 apply_patch는 custom tool이다. 같은 이름의 평범한 function은 다른 것.
- **tool_choice가 허용**: 모델이 고를 수 없는 apply_patch는 대안이 아니다.

셋 중 하나라도 빠지면 대체할 것이 없으므로 네이티브 변경을 막으면 안 된다 — 그때 막으면
기능을 그냥 없애는 것이다.

### 플래그는 요청 단위다

실행기 하나가 여러 턴을 처리한다. apply_patch를 광고한 턴이 **다음 턴의 네이티브 쓰기까지
꺼버리면** 안 되므로 `ExecRequest`에 실었다. 테스트로 고정했다.

### 거부 문구가 대안을 지목한다

권한 실패로 보고하면 모델이 "파일시스템을 못 쓴다"고 결론내고 멈춘다. apply_patch로
경로를 바꾸라고 말해야 한다.

## 이미 있던 것들

- **샌드박스 정책**: `policy.go`가 `NativeExecMode`(`off`/`on`/`codex-sandbox`)와 경로·URL
  검사를 갖고 있다. `allowLocal`이 명시적 opt-in을 요구한다.
- **pre-commit 재시도**: 오라클은 "아무것도 emit되지 않았고 + 요청이 커밋되지 않았고 +
  일시적 실패"일 때만 재시도한다. go의 `prepareContinuityRecovery`(`adapter.go:251-255`)가
  같은 조건을 건다 — `emittedOutput`과 `replayUnsafe`를 모두 확인하고, 후자는 exec가
  실행되는 순간 세워진다(`adapter.go:375`). 형태는 다르지만 **보호하는 불변식은 같다.**

목록에 있다는 이유로 이미 있는 것을 다시 만들지 않는다 — 이 스윕에서 여섯 번째다.
