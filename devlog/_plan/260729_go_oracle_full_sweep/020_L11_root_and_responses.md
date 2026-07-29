# 020 — L11 인벤토리: 루트 모듈 9개 + `src/responses/**`

15파일 / 8,009줄. 대응 go: `server/responses_*`, `server/responses_state.go`, `bridge/`,
`config/`, `service/`, `types/`, `protocol/`(저수준 SSE·retry·stall만 — A 감사가 정정한 매핑).
exported 277개 중 선언뿐 16개.

## 설정 키 차등이 이 레인의 실제 결함이다

선언뿐인 go export 16개는 전부 편의 생성자·프레임워크 오류 스켈레톤·아키텍처 인터페이스로,
오라클 뒷받침이 있는 UNWIRED가 하나도 없었다. 대신 **설정 스키마**에서 세 건이 나왔다.

| 키 | 오라클 | go | 등급 |
| --- | --- | --- | --- |
| `modelReasoningSummaryDelivery` (provider) | `types.ts:993`, `config.ts:840` | 타입 필드 없음, 프로덕션 사용 없음 | MISSING |
| `codexAccountNamespaces` (root) | `config.ts:681,705` (충돌 규칙 포함) | 없음 | MISSING |
| `syncCodexSubagentDefaults` (root) | `config.ts:677,988,1128` | CLI 파리티(`config_parity.go:238`)만 알고 런타임 `Config`에 필드 없음 | DIVERGENT |

세 번째가 특히 고약하다: 사용자가 설정을 쓰면 CLI가 받아주고 보존까지 하는데, 런타임은 그
값을 볼 수 없어 네이티브 기본값 동기화가 실제로는 일어나지 않는다.

`storageCleanupPolicy`는 `ExtraFields` 경유로 관리 라우트가 정규화·읽기·쓰기를 하므로
`OK-EQUIVALENT`. go 전용 `authToken`/`debug`/`log`는 `OUT-OF-SCOPE`.

## 알려진 정적 대조 함정 재확인

`bridge.go:581`이 `eventType := "response." + status`로 조립하므로
`response.completed`/`failed`/`incomplete`의 grep 부재는 **FALSE-POSITIVE**다. 이 레인이
그것을 직접 읽어 확인했다.

## 최근 커밋이 만든 동작 확인

- `f72aa112a`의 명시적 `end_turn` 보존(`false` 포함): `bridge.go:34,405,630`에 존재 — OK.
- `7baffd64e`의 빈 completed output 재조립: `relay_inspector.go:135`에 존재 — OK.

## OK로 확인된 축

assistant `phase` 보존, freeform custom tool의 `custom_tool_call` 왕복, `tool_search_call`,
네임스페이스 MCP 도구의 평탄화/복원, Responses Lite `additional_tools` 병합, agent 메시지의
user 턴 변환과 빈 메시지 placeholder, 암호화 payload 가드, reasoning envelope `ocxr1:`,
compaction envelope `ocx1:`과 replay, remote compact v1 보존, previous_response_id 재생과
provider 연속 상태, 상태 저장 바이트 캡·TTL·디스크 스냅샷·stale temp 정리,
`store:false` 의미, incomplete 저장 조건, stall timeout 기본 300초, 업스트림 stall의
`response.incomplete` + 취소, reasoning 사다리와 rank/sanitize/ultra→max 경계, 런타임 effort
cap, 라우터 네임스페이스 해석, provider 설정 기본/백필/검증, 환경변수 확장과 프록시 미러링,
live-config 저장의 `claudeCode` 수동 편집 보호, 서비스 설치/시작/상태 문법.

## 검증하지 않은 것

테스트/라이브 호출 없음. L1-L10/L12 패키지는 이 레인 심볼의 프로덕션 호출 지점 확인 범위까지만.
