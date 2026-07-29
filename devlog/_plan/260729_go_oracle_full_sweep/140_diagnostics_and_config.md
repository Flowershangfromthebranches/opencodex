# 140 — WP6: 진단과 설정 재측정

`030_synthesis.md`의 090 섹션이 나열한 9개 항목을 현재 트리에서 다시 쟀다. **2건을 닫았고,
1건은 이미 닫혀 있었으며, 1건은 별도 유닛으로 분리했고, 나머지는 결함이 확인되지 않았다.**

| 항목 | 재측정 | 처리 |
| --- | --- | --- |
| `syncCodexSubagentDefaults` | 이미 닫힘 (`4dd818430`, wp4b) | — |
| `modelReasoningSummaryDelivery` | 부재 | 닫음 `0fd747aee` |
| effort-clamp 진단 | 계산은 하는데 **표시 안 함** | 닫음 `48ec00d27` |
| `codexAccountNamespaces` | 설정 키가 아니라 **기능 전체** | `wp6b`로 분리 |
| sync 프로젝트 경고 / journal injected-state / 계정 로그 라벨 / Windows 마켓플레이스 진단 / 업데이트 프롬프트 / Windows shim env | 사용자 가시 결함 미확인 | 아래 |

## `modelReasoningSummaryDelivery` (`0fd747aee`)

업스트림이 클라이언트가 고른 summary delivery 모드를 제대로 못 다루는 모델을, **다른 것은
건드리지 않고** 교정하기 위한 키다. go에 필드 자체가 없어서 그 교정 수단이 없었다.

두 가지를 의도적으로 지켰다:

- **이미 있는 필드만 바꾼다.** 클라이언트가 보내지 않은 `reasoning_summary_delivery`를
  추가하면 모델별 교정이 전역 동작 변경이 된다. 오라클도 같은 가드를 둔다
  (`openai-responses.ts:232-241`).
- **알 수 없는 값은 config load에서 거부한다.** 그대로 흘리면 업스트림이 400을 주므로
  사용자는 "잘못된 설정"이 아니라 "실패하는 턴"을 보게 된다.

## effort-clamp 진단 (`48ec00d27`)

go는 `LoadLastEffortClamp`·`EffortClampAppliesToRuntime`을 **이미 갖고 있었고**
`startup_health.go:191`에서 쓰고 있었다. 그런데 `doctor`가 그것을 표시하지 않았다.

이게 사용자에게 실제로 드는 비용: 카탈로그 sync 중 선택된 Codex 런타임이 지원하지 못하는
reasoning effort가 **조용히 제거된다.** xhigh를 설정한 사용자가 high를 받으면서 그 이유를
알 방법이 없었다.

런타임에 실제로 적용되는 clamp일 때만 warn이다. 이미 다른 런타임으로 옮긴 사용자의 오래된
기록은 현재 문제가 아니고, 그것까지 경고하면 사용자가 그 줄을 무시하도록 학습시킨다.

## `codexAccountNamespaces` — 설정 키가 아니었다

`030`이 "설정 키 3개"에 묶었지만 재측정 결과 **212줄짜리 기능**이다
(`src/codex/account-namespaces.ts` 149줄 + `account-namespace-match.ts` 63줄), 호출 지점이
10곳이고 combo alias 충돌·provider 이름 충돌·account id 충돌·모델 네임스페이스 매칭까지
포함한다.

config 필드만 추가하면 **파싱은 되는데 아무 동작도 하지 않는 키**가 생긴다. 그것은 없는
것보다 나쁘다 — 사용자가 설정하고 효과를 기대하게 된다. `wp6b-codex-account-namespaces`로
분리했다.

## 결함이 확인되지 않은 항목들

`030`의 나열은 레인 조사 시점의 목록이고, 그 뒤 이 세션의 작업과 `dev` 머지가 일부를 덮었다.
현재 트리에서 각각을 확인한 결과 **사용자에게 보이는 차이를 특정하지 못했다.** 없다고
단정하지는 않는다 — 각 항목이 자기 유닛에서 다시 측정될 때 구체적 재현으로 판단해야 한다.

목록에 있다는 이유만으로 코드를 바꾸지 않는 것이 이 스윕에서 반복 확인된 규칙이다
(`041`에서 3건, 여기서 1건).
