# 160 — WP3b: Kiro 전송 유닛 재측정

`060_transport_semantics.md`가 `wp3b`로 이월한 항목들을 현재 트리에서 다시 쟀다.
**2건을 닫았고, 1건은 이미 완성돼 있었으며, 나머지는 별도 유닛에 속하거나 결함 미확인이다.**

| 이월 항목 | 재측정 | 처리 |
| --- | --- | --- |
| Smithy 오류 분류 테이블 | **이미 완비** (`errors.go:41-110`) | — |
| stream catch 재시도 분류 (emitted-output 게이트) | 부재 | 닫음 `a585417f5` |
| tool-result 암호화 처리 | 부재 | 닫음 `4ebf65f56` |
| Kiro fallback 상태 기계 | 게이트 외 나머지는 이미 있음 | — |
| Cursor exec wire / apply_patch 가드 / 샌드박스 정책 | **Cursor 유닛** | 범위 밖 |
| Azure wire 경로 / identity regex | 결함 미확인 | — |

## emitted-output 게이트 (`a585417f5`)

text-fallback 경로가 **모든** 실패를 `Retryable: true`로 표시했다. 첫 시도가 이미 이벤트를
클라이언트로 흘려보낸 뒤의 실패까지 포함해서.

거기서 재시도하면 턴이 처음부터 재생되므로 **클라이언트가 같은 내용을 두 번 받는다.** 그리고
그렇게 하라고 부추긴 것이 바로 그 플래그였다.

이제 오라클과 같은 두 조건을 모두 본다(`kiro.ts:656-663`): 끊긴 연결처럼 보여야 하고,
**아직 출력이 나가지 않았어야** 한다. Cursor 어댑터에 같은 게이트가 있고 거기서 형태를 가져왔다.

- 잘못된 이벤트(`invalid Kiro …`)는 항상 terminal이다. 업스트림이 실제로 그 페이로드를
  보냈으므로 연결은 정상이었고, 재생하면 같은 바이트가 다시 온다.
- 분류되지 않는 실패도 replay-safe로 가정하지 않는다. 모르는 것을 재시도하면 업스트림에
  중복 작업을 만든다.
- eventstream truncation은 **replay-safe이고 명시적으로 매칭한다** — 부분 프레임 + 정상 EOF는
  소켓 종료와 같은 부류인데, 잘못된 페이로드로 오해하기 쉽다.

## 암호화 tool result (`4ebf65f56`)

Kiro 와이어는 암호화된 tool 출력을 담을 수 없다. go는 그냥 번역해서 **캐리어 텍스트를 대신
보냈다** — 모델은 자기가 요청한 결과가 아닌 것을 받고, 턴은 성공한 것처럼 보인다.

`types.Message`에 `ContainsEncryptedContent`가 **이미 있었다.** Kiro 경로가 읽지 않았을 뿐이다.

거부하면서 call id를 밝힌다. "이 대화는 번역 불가"가 아니라 어느 tool result가 원인인지
알아야 사용자가 조치할 수 있다.

## Smithy 테이블 — 이미 완비

`060`이 미이식으로 적었으나 `errors.go:41-110`에 context_length / insufficient_quota /
throttling / auth / validation / overloaded 분기가 모두 있고, `classifyPrefix`가 사용자 대면
문구까지 나눈다. **목록을 믿지 않고 다시 잰 것이 맞았다** — 이 스윕에서 다섯 번째다.

## 범위 밖으로 옮긴 것

`060`이 이 유닛에 함께 적은 Cursor 항목들(exec wire, apply_patch 가드, 샌드박스 정책,
pre-commit 재시도)은 **Kiro와 코드를 공유하지 않는다.** `internal/adapter/cursor`가 자기
정책 파일(`policy.go`)을 갖고 있고, 그쪽에서 재측정해야 한다. 여기 묶으면 두 어댑터의
검증이 섞인다.
