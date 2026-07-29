# 132 — 설정 소유권 매니페스트: 결함이 아니라 **의도적 분기**

`130`이 "config 소유권 개념 없음"으로 적은 항목을 재측정한 결과다. 결론부터: **go에 넣지
않는다.** 이유는 아래.

## 오라클이 하는 것

`src/lib/config-ownership.ts`가 `.opencodex-owner.json` + `.opencodex-uninstall.json`에
**opencodex가 쓴 경로만** 기록한다(`recordOwnedConfigPath`, 8곳에서 호출). `ocx uninstall`이
그 매니페스트를 읽어 소유한 것만 지운다(`removeOwnedConfigState`, `cli/index.ts:619`).

매니페스트가 없거나 손상됐으면 **거부한다**(`status: "refused"`). 즉 이 코드의 목적은
"지우는 것"이 아니라 **"모르는 것은 지우지 않는 것"** 이다.

## go의 현재 상태

`runUninstall`(`lifecycle_extended.go:49`)은 서비스·shim 통합만 제거하고 이렇게 말한다:

```
OpenCodex integrations removed; configuration and credentials were preserved.
```

**go는 config를 아예 지우지 않는다.** 매니페스트가 없는 것이 아니라, 매니페스트가 보호할
삭제 동작 자체가 없다.

## 그래서 이것은 결함이 아니다

`130`의 표는 "`config-ownership.ts` → ABSENT"만 보고 미이식으로 분류했다. 그 판정은 **호출
측을 보지 않은 것**이다. 소유권 추적은 uninstall의 삭제 범위를 좁히는 안전장치이고, go에는
좁힐 삭제가 없다.

지금 이식하면 순서가 거꾸로다: **안전장치를 먼저 만들고 그것이 보호할 위험한 동작을 나중에
추가하는 꼴**이다. 매니페스트 자체는 아무 사용자 문제도 해결하지 않는다 — 오히려 uninstall이
config를 지우게 만드는 변경이 뒤따라야 의미가 생기고, 그것은 별개의 제품 결정이다.

## 파리티 관점에서 남는 것

go와 오라클의 `uninstall`은 **의도적으로 다르게 동작한다**:

| | 오라클 | go |
| --- | --- | --- |
| 서비스/shim | 제거 | 제거 |
| config·credentials | 소유한 것만 제거 | **보존** |

go 쪽이 보수적이다. 이 차이를 없애려면 "go uninstall이 config를 지워야 하는가"를 먼저 정해야
하고, 그것은 코드가 답할 수 없다. 답이 "그렇다"가 되는 날 소유권 매니페스트가 **선결 조건**으로
따라온다 — 그때 이 문서가 근거가 된다.

## 판정

`NOOP` — 현재 트리에서 고칠 결함이 없다. `130`의 해당 행을 "미이식"에서 "의도적 분기"로
정정한다.
