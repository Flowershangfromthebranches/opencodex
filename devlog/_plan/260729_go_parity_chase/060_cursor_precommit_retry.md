# 060 — Cursor pre-commit 재시도가 배선되지 않음

상태: 확인됨, 미구현. 설계 결정이 필요해 별도 유닛으로 남긴다.

## 무엇이 빠졌나

`go/internal/adapter/cursor/retry.go:14`의 `DoPreCommitRetry`는 완성된 구현이다 —
지수 백오프 + 지터, 3회 시도, **커밋 전에만** 재시도(`state.RequestCommitted()`가
true면 즉시 포기). `RequestCommitted()`를 제공하는 `LiveTransport`도 있다
(`transport.go:77`).

그런데 **호출자가 테스트뿐이다**(`cursor_test.go:345`). 852개 export를 전수 조사해
나온 유일한 진짜 미배선 항목이다.

오라클은 쓴다. `src/adapters/cursor.ts:17`이 `runCursorTurnWithRetry`를 import하고
`:116`에서 턴 전체를 감싼다.

## 사용자에게 무엇이 다른가

Cursor 업스트림이 요청을 **커밋하기 전에** 끊기면(연결 실패, 502 같은 전송 계층
오류) 오라클은 조용히 재시도한다. Go는 그대로 실패를 올린다. 커밋 전이라 재시도가
안전한데도 사용자는 오류를 본다.

커밋 **후** 실패는 양쪽 다 재시도하지 않는다 — 그게 `RequestCommitted()` 가드의
존재 이유고, 중복 실행을 막는다.

## 왜 이번 사이클에 안 고쳤나

구조가 다르다. 오라클은 transport 팩토리를 넘겨받아 턴을 통째로 재실행할 수 있는
모양(`makeTransport`)이지만, Go 어댑터는 `BuildRequest`/`ParseStream`으로 쪼개져
있고 재시도 헬퍼가 감쌀 transport 객체를 어댑터가 들고 있지 않다.

즉 배선이 아니라 **어댑터 실행 경로를 재시도 가능한 단위로 다시 여는 작업**이다.
한 줄짜리 호출 추가가 아니므로, 전송 계층을 급히 바꾸기보다 자체 사이클에서
다루는 편이 안전하다.

## 다음 사이클에서 확인할 것

1. Go 서버 계층에 이미 재시도가 있어 이 갭을 덮고 있는지 (현재까지 확인된 바로는 없음)
2. `BuildRequest`가 idempotent한지 — 재실행 시 conversation ID나 usage 집계가
   중복되지 않는지
3. 재시도 발화 증거: 커밋 전 502를 주입해 3회 시도가 실제로 도는 것을 읽어서 확인
