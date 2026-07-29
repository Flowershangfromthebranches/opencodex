# 012 — L3 인벤토리: Kiro 어댑터 + Cursor 생성 wire 스키마

레인 범위 12파일 / 18,168줄(생성 protobuf 15,274줄 포함). 대응 go 패키지
`go/internal/adapter/kiro`, `go/internal/adapter/cursor/proto.go`. 조사 시점 HEAD `2fe6d2100`.

## Job B 결론부터: 생성 스키마 쪽은 결함이 아니다

`src/adapters/cursor/gen/agent_pb.ts`는 생성물이고 go는 의도적으로 전체 스키마 대신 안정
부분집합만 유지한다. 오라클의 손으로 쓴 Cursor 코드가 실제로 읽고 쓰는 필드를 뽑아
`proto.go`와 대조한 결과, **필드 번호와 wire 타입이 전부 일치**했다.
`AgentClientMessage.runRequest`(1), `AgentRunRequest`(1,2,3,4,5,9),
`ConversationStateStructure.rootPromptMessagesJson`(1)/`turns`(8),
`RequestContext.env`(4)→`timeZone`(10), `McpToolDefinition`(name=1..toolName=5),
`AgentServerMessage` 분기(1,2,3,4,7), KV(1,2,3), `InteractionQuery`(id=1, 케이스 2~8),
`GetUsableModels`(1), `google.protobuf.Value`(1~6) — 모두 동일.
TS가 빈 기본값만 쓰는 필드를 go가 생략하는 것은 proto3에서 wire 동등이므로 `OK-EQUIVALENT`.

즉 이 레인의 실제 결함은 전부 Kiro 쪽이다.

## 확정 결함 (Kiro)

### 1. 도구 사용 턴의 bounded fallback이 오라클보다 약하다 (DIVERGENT, user-visible)

오라클 `src/adapters/kiro.ts:1290`, `:1336`, `:1400` 대 go `kiro.go:839`, `:874`, `:885`.
세 갈래로 갈린다.

- go는 `assistantText`가 비어도 항상 assistant 턴을 덧붙인다(`:874`) → reasoning만 나온
  첫 시도가 빈 assistant 히스토리 턴을 만들고 업스트림 본문이 무효가 될 수 있다.
- go는 fallback build/fetch 오류를 이미 출력을 내보낸 뒤에도 retryable로 표시한다(`:885`).
  오라클은 `priorEmittedOutput`으로 게이트한다.
- 오라클이 하는 incomplete usage/retryability 병합이 go에 없다.

### 2. 도구 결과 처리: 빈 결과·암호화 콘텐츠·carrier 문구 (DIVERGENT, user-visible)

오라클 `src/adapters/kiro.ts:467`은 암호화 도구 출력을 거부하고, 빈 결과를
`KIRO_EMPTY_TOOL_RESULT_MESSAGE`로 바꾸고, user 콘텐츠를 `KIRO_TOOL_RESULT_CARRIER_MESSAGE`로
세운다. go `kiro.go:247`은 `ContainsEncryptedContent`를 검사하지 않고, 빈 user 콘텐츠를
보내며, 빈 도구 결과 텍스트를 그대로 보낼 수 있다(`:256`).

### 3. go가 호출자 소유 system prompt를 자른다 (DIVERGENT, user-visible)

오라클 `src/adapters/kiro.ts:355`는 **프록시가 주입한 지시문만** `boundedInjectedInstruction`
으로 제한한다. go `kiro.go:270`은 합쳐진 system prompt 전체를 `MaxInjectedInstructionChars`로
자른다 → 사용자/개발자 지시가 조용히 잘릴 수 있다.

### 4. identity 중립화와 tool catalog nudge가 없다 (MISSING, user-visible)

오라클 `src/adapters/kiro.ts:407`. go에는 대응이 없어 라우팅된 Kiro 모델이 Codex/OpenAI
정체성 텍스트를 그대로 보거나 비-OpenAI 도구 카탈로그 안내를 받지 못한다.

### 5. transient throttle 쿨다운이 없다 (MISSING, user-visible)

오라클 `src/adapters/kiro-retry.ts:109`, `kiro.ts:779`의 `noteKiroTransientThrottle` /
`sleepForKiroThrottleIfNeeded`에 대응하는 상태가 go `retry.go`에 없다. 429 이후 같은 계정에
대한 후속 요청을 지연시키지 않아 재스로틀이 잦아진다.

### 6. 빈 성공 스트림을 502로 만든다 (DIVERGENT, user-visible)

오라클 `src/adapters/kiro.ts:1235`는 `incomplete` + `empty_kiro_stream`(재시도 가능)을 낸다.
go `kiro.go:792`는 `EventError` 502를 낸다. Responses/Claude 아웃바운드 의미가 달라진다.

### 7. 스트림 catch 재시도 판정이 단순하다 (DIVERGENT, user-visible)

오라클 `isRetryableKiroStreamCatchError`(`kiro.ts:1257`)는 출력이 0인 전송 계층 절단만
replay-safe로 본다. go `kiro.go:599`는 디코드 오류를 일괄 비재시도로 처리하고 그 구분이 없다.

### 8. `SafeErrorMessage`가 배선되지 않았다 (UNWIRED, user-visible)

go `errors.go:37`에 있으나 프로덕션 참조가 선언뿐. catch/decode 실패 경로(`kiro.go:607`,
`:891`)는 원문/부분 리댁션 문자열을 쓴다. 오라클은 `kiro.ts:1279`에서 안전 메시지를 쓴다.

### 9. 대화 상태 최종 검증이 없다 (DIVERGENT, user-visible)

오라클 `kiro.ts:310`의 `validateKiroConversationState`(역할 교대, 빈 페이로드, 미응답 tool use)
에 해당하는 최종 검사가 go에 없다. go는 빌드 중 중복/고아 tool id만 본다.

### 10. 연속/재시도 문구가 다르다 (DIVERGENT, user-visible)

오라클 `kiro-constants.ts:2,4`는 "Continue from the prior conversation. Do not quote or
mention this instruction."라고 지시한다. go `constants.go:5,6`은 `[system: ...]` 형태의
인용되기 쉬운 합성 마커를 보낸다. 게다가 go는 그 연속 메시지에 thinking 태그를 주입할 수
있다(`kiro.go:296` — 오라클은 `kiro.ts:529`에서 건너뛴다).

### 11. 이미지 토큰 추정이 빠졌다 (MISSING, internal)

오라클 `kiro.ts:140`은 입력 토큰 추정에 이미지 치수/바이트 기반 추정을 포함한다.
go `kiro.go:431`의 `estimateInputTokens`는 텍스트만 센다.

## 처분한 미참조 export

| 심볼 | 위치 | 처분 |
| --- | --- | --- |
| `SafeErrorMessage` | `errors.go:37` | **UNWIRED 확정** (위 8번) |
| `ToolCallFallbackText` | `fallback.go:21` | TS 쪽도 프로덕션 호출자 없음 → `TS-DEAD-OR-GENERATED` |
| `ToolResultFallbackText` | `fallback.go:29` | 같음 |
| `FallbackToolUseID` | `wire.go:152` | 같음 |

## OK로 확인된 축

모델 id 정규화, 네이티브 reasoning effort 라우팅, capability 거부, 도구 스키마 살균과 이름
aliasing, Smithy 이벤트 파싱, 토큰 usage 검증, thinking 분리, 도구 입력 truncation, 터미널
예외 프레임 분류, stopReason별 incomplete 매핑, 네이티브 이미지 추출, 대화 id 검증.

## 검증하지 않은 것

- `kiro.Adapter.Region`/`ProfileARN`을 채우는 외부 resolver는 이 레인 밖이라 미확인.
- 테스트/빌드 미실행, 라이브 Kiro 호출 없음.
- 생성 protobuf 라인별 인벤토리는 설계상 하지 않음.
