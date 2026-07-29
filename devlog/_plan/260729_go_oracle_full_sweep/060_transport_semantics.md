# 060 — WP3: 전송 계층 실패 의미론 (Kiro 스트림 종결)

`030_synthesis.md`의 060 표는 13항목인데, 그 안에 Kiro 스트림 종결·Cursor exec wire·Azure
wire 경로가 섞여 있다. 세 개는 서로 다른 어댑터의 서로 다른 층이라 한 사이클에 넣으면
검증이 흐려진다. 이 사이클은 **Kiro 스트림 종결 의미론**만 닫는다.

고르는 기준은 사용자 도달 거리다. Kiro 스트림 종결은 매 턴 지나가는 경로이고, 잘못된
판정이 곧바로 "턴이 깨졌다"로 보인다.

**A 감사 FAIL 반영 개정본.** 초판은 빈 스트림 + catch 재시도 분류 두 건을 잡았는데, 감사가
둘째의 전제를 무너뜨렸다: 재시도 판정의 핵심인 "이미 출력이 나갔는가" 게이트는 go의 버퍼링
모델에서 `sawText || sawReasoning`로 계산할 수 없고, fallback 2차 시도의 `priorEmittedOutput`
까지 필요하다(오라클 `kiro.ts:1264-1273`, `:1336`, `:1381`). 즉 **재시도 분류는 fallback
상태기계와 분리 불가능**하다. 그것을 분리한 채로 넣으면 게이트가 헐거워져 이미 출력이 나간
뒤에 replay-safe를 광고하고 턴이 중복될 수 있다 — 지금보다 나쁘다.

그래서 이 사이클은 **빈 스트림 한 건**으로 좁힌다. 재시도 분류는 fallback 상태기계와 같은
사이클(`wp3b`)로 옮긴다. 감사의 표현대로 "empty-stream만 남기면 방어 가능한 좁은 phase이고,
fallback을 미룬 채 catch 분류를 넣는 것은 아니다".

## P 재검증

### 빈 성공 스트림을 502 오류로 만든다 (DIVERGENT, user-visible)

오라클 `src/adapters/kiro.ts:1235-1245`는 `retryableKiroIncomplete`(`:633-648`)로
`type:"incomplete"`, `reason:"empty_kiro_stream"`, `retryable:true`, `endTurn:false`를 낸다.

go `internal/adapter/kiro/kiro.go:792-794`(감사 블로커 5가 정정한 정확한 위치 — 초판이 적은
`:604`는 디코드 오류 분기다):

```go
	if !result.sawText && !result.sawReasoning && !sawRealTool {
		fail("Kiro returned a successful but empty response stream", true)
		return result
	}
```

`fail`(`:544-546`)은 `EventError` + `StatusCode: 502` + `terminalError = true`를 낸다.
메시지는 같은데 **이벤트 종류가 다르다.** 오류는 클라이언트에게 업스트림 실패로 보이고,
incomplete는 이어갈 수 있는 미완결로 보인다.

MODIFY `go/internal/adapter/kiro/kiro.go:792-794`:

```go
// after
	if !result.sawText && !result.sawReasoning && !sawRealTool {
		// An empty-but-successful stream is not an error: nothing went wrong on
		// the wire, the model simply said nothing. Reporting 502 makes a
		// resumable turn look like an upstream failure
		// (oracle: src/adapters/kiro.ts:1235).
		result.events = append(result.events, types.AdapterEvent{
			Type: types.EventIncomplete, Reason: "empty_kiro_stream",
			Message: "Kiro returned a successful but empty response stream",
			Retryable: true, EndTurn: false, Usage: result.usage,
		})
		return result
	}
```

**`terminalError`는 세우지 않는다 (감사 블로커 4).** 초판은 "그 플래그가 fallback을
좌우한다"고 썼는데 거짓이었다. fallback 분기는 `first.needsFallback`만 본다(`:862-865`),
그리고 `needsFallback`은 required 모드 + 텍스트/reasoning + 미완료일 때만 세워진다(`:788-790`).
빈 스트림은 그 조건에 해당하지 않으므로 어느 쪽이든 fallback은 돌지 않는다. 오라클도 빈
스트림에서 fallback 없이 terminal incomplete를 낸다(`kiro.ts:1318-1320`). 세울 이유가 없는
플래그는 세우지 않는다.

## 이 사이클이 다루지 않는 것 (`wp3b-transport`로 이월)

- **스트림 catch 재시도 분류 + Kiro fallback 상태기계** — 한 묶음이다. 감사가 보인 대로
  emitted-output 게이트는 `priorEmittedOutput`, `assistantText`, `open`, `completionCalls`,
  `completionAnswer`, 그리고 이미 `result.events`에 쌓인 가시 이벤트까지 세야 하고, 2차
  시도에는 1차의 출력 여부가 전달돼야 한다. Smithy 오류 분류표도 함께 확정한다:
  `read prelude: EOF`는 정상 종료, `read prelude: unexpected EOF`와 `truncated frame:
  unexpected EOF`는 출력 0일 때만 재시도, 길이/CRC/헤더 오류는 비재시도. 오라클의
  `/eventstream:\s*truncated/`를 그대로 옮기면 go에서는 헤더 손상까지 삼키므로 위험하다.
- 도구 결과 carrier/암호화, system prompt 예산, identity 중립화, throttle 쿨다운,
  Cursor exec wire + apply_patch 차단 + sandbox 정책 + pre-commit 재시도, Azure wire 경로.

## 검증 계획

- `go build ./... && go vet ./...`
- 빈 스트림: 텍스트도 reasoning도 도구도 없는 성공 스트림을 먹여 `EventIncomplete` +
  `empty_kiro_stream` + `Retryable:true` + `EndTurn:false`가 나오는지. **502 오류가 아님**을
  명시적으로 단언한다.
- 회귀 방어: 텍스트가 있는 정상 스트림은 여전히 `EventDone`으로 끝나는지(이 분기는
  텍스트·reasoning·도구가 모두 없을 때만 타므로 일반 턴에 영향이 없어야 한다).
- 어블레이션으로 그 분기가 실제로 발화하는지 증명한다.
- 라이브 Kiro 호출은 하지 않는다.

## 범위 밖

오라클 수정, 라이브 provider 호출, 다른 세션이 작업 중인 파일.
