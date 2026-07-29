# 080 — 외부 감사 19건 재측정 + dev 머지 영향

기준 커밋 `02b540822`(= `origin/dev` 머지). 감사는 그 이전 스냅샷에서 쓰였으므로 **문서를
믿지 않고 현재 트리에서 다시 쟀다.** gpt-5.5 서브에이전트 5개가 담당 구간을 나눠 병렬 검증했다.

## 결과 요약

| 처분 | 개수 |
| --- | ---: |
| CONFIRMED | 18 (4번을 4a/4b/4c로 분할하면 20행) |
| ALREADY-FIXED | 1 |
| FALSE-POSITIVE | 0 |
| SUPERSEDED-BY-MERGE | 0 |

감사는 거의 전부 지금도 유효하다. 유일하게 해소된 것은 11번(Gemini inlineData)인데, 그것도
이 세션이 `6dbe278a2`로 직접 고친 것이라 외부 요인이 아니다.

**그리고 감사가 놓친 층이 하나 더 드러났다.** 머지가 가져온 보안 하드닝 14건 중 **9건이
go에 대응물이 없다.** 감사는 머지 이전 스냅샷을 봤으니 당연히 다루지 않았고, 앞선
`010`~`021` 레인들도 그 코드가 존재하기 전에 조사됐다.

## 재측정 표 (19건)

| # | 결함 | 처분 | 현재 근거 |
| ---: | --- | --- | --- |
| 1 | 성공한 Responses를 패스스루하지 않고 재조립 | CONFIRMED | go는 `streamMode=="eager-relay"` + SSE content-type일 때만 무변환 릴레이(`responses_core_port.go:690,842`), 그 외엔 파싱·재구성(`:737,802,945,990,1013`). 파서는 `function_call`만 인식(`responses.go:746`) → `custom_tool_call`/`tool_search_call`/`web_search_call` 유실 |
| 2 | 미설정 built-in provider가 라우팅 후보로 남음 | CONFIRMED | go는 전 built-in을 시드 후 config를 덧씌우기만 함(`serve.go:334,340,359`), family 라우팅이 전 엔트리를 훑음(`registry.go:340`), `ListModels`도 전부 노출(`:397`). OpenRouter만 설정해도 bare `claude-*`가 built-in Anthropic에 도달 가능 |
| 3 | 영속 config가 canonical built-in의 adapter/endpoint를 덮어씀 | CONFIRMED | `serve.go:340,345,347`이 registry adapter/baseURL/headers를 무조건 덮어쓰고 transport가 그것을 씀(`transport.go:23,63,66`). 머지가 추가한 per-model wire pin(`serve.go:669`)은 이 경로를 막지 않음 |
| 4a | destination policy가 config load에 부재 | CONFIRMED | `config.go:765,769`가 URL 문법만 검사. 오라클은 `config.ts:773,781` |
| 4b | 관리 API에서 메타데이터 차단이 우회 가능 | CONFIRMED | `provider_destination.go:16-18`이 `AllowPrivateNetwork`/registry-local이면 **메타데이터 분류(`:62`, `:92`) 전에** 성공 반환 |
| 4c | 요청 시 transport에 destination 검사 없음 | CONFIRMED | `server/fetch.go:35,45`가 곧장 `client.Do`. discovery만 DNS 사전검사가 있고(`catalog_provider_fetch.go:75,250,260`) 그것도 실제 연결에 고정되지 않음 |
| 5 | OpenCode Free `x-opencode-client` 헤더 누락 | CONFIRMED (범위 정정) | **go에 레지스트리가 둘이다.** `providers/registry_metadata.go:107`은 헤더를 **갖고 있고** `providers/derive.go:49,58`이 config로 복사한다(CLI `provider add` 경로, `cli/provider.go:180-182`). 그러나 런타임이 쓰는 `registry/registry.go:111`에는 없고, 관리 API 프리셋도 그 오래된 쪽을 서빙한다(`management/providers.go:15-16`). 게다가 `serve.go:347`이 preset static headers를 `provider.Headers`로 덮어쓴다. → CLI로 추가한 provider는 헤더가 있고, 대시보드/영속 config 경로는 잃는다 |
| 6 | 중간 스트림 실패 꼬리가 SSE 프레이밍·오류 스키마 위반 | CONFIRMED | `relay.go:158-166`이 선행 빈 줄 없음, `server_error` + 숫자 502, `last_error` 누락. 오라클은 `relay.ts:67-78` |
| 7 | 직접 스트림을 failover처럼 preflight | CONFIRMED | `responses_core_port.go:737-774`가 모든 non-eager 스트림을 preflight, 실패 시 SSE 헤더 전에 502 JSON(`:764-772`). 감사가 "regardless"라 한 것은 과장 — disconnect류는 이미 면제(`:764-765`)되지만 provider 오류 첫 이벤트는 여전히 502 |
| 8 | image_gen 클라이언트 툴 네임스페이스 미정규화 | CONFIRMED | go는 `stripConflictingHostedTools`만(`responses.go:230-240,432-457`). 없는 것: 네임스페이스 평탄화, dotted 별칭, `tool_choice` 재작성, `additional_tools` 순회, replay call 인코딩, 별칭 중복 제거, "쓸 만한 별칭이 대체할 때만 hosted 제거" 규칙 |
| 9 | OpenAI Chat 오류가 code/type/status 유실 | CONFIRMED | 스트림·단항 모두 메시지 문자열로 축약(`chat.go:281-285,393-395`), bridge가 `event.Code`를 복사하지 않음(`bridge.go:421-437`) |
| 10 | 툴콜 조립이 잘못 병합, 손상 인자를 `{}`로 변환 | CONFIRMED | index/id 둘 다 없으면 **index 0**으로 폴백(`chat.go:336-339`, 오라클은 마지막 활성 콜). 손상 JSON을 `{}`로 대체: `chat.go:376-378,415-417`, `anthropic.go:568-570`, `responses.go:752-754` |
| 11 | Gemini inlineData 이미지 유실 | **ALREADY-FIXED** | 이 세션의 `6dbe278a2`. 스트림·단항 모두 처리(`google.go:707,730-734`), 예산 공유(`:548,595,679`), 상한이 오라클과 일치(50MiB/100MiB). 잔여: `ArtifactsHome` 미설정 시 오류 이벤트 |
| 12 | service_tier를 일반 Chat provider에 전달 | CONFIRMED | `chat.go:203-204`가 무조건 전달. 오라클 Chat 본문 구성에는 없음(`openai-chat.ts:525-603`) |
| 13 | `/api/providers/test`가 프로덕션에서 미배선 | CONFIRMED | `server.go:519`가 `FetchModels`를 넘기지 않아 항상 501(`providers.go:231-237`). 주입해도 실패가 502 — 오라클은 200 `{ok:false}` |
| 14 | provider 덮어쓰기가 API 키 풀을 삭제 | CONFIRMED | `providers.go:59-63`이 엔트리를 통째 교체. `config.AddAPIKey`(`api_keys.go:67-92`)가 있는데 POST가 부르지 않음 |
| 15 | provider DTO가 대시보드 필드 누락 | CONFIRMED | `publicProvider`에 `apiKeyTransport` 없음, `discovery`는 순서 목록에만 있고 채워지지 않음(`shared.go:116-125`, `providers.go:35-37`). safeConfig는 16개 필드 누락 |
| 16 | provider PATCH 필드·Codex 모드 부수효과 누락 | CONFIRMED | `apiKeyTransport`/`note`/`codexAccountMode` 모두 거부(`providers.go:252-304`). `ClearThreadAccountMap`(`routing.go:178`)·쿼터 프라이밍 호출 없음 |
| 17 | reasoning_summary_delivery 요청시 카탈로그 가드 부재 | CONFIRMED | go는 provider config의 명시적 false만 봄(`responses_config.go:56-73`), 카탈로그 접근 자체가 없음 |
| 18 | Gemini `thought:true` 텍스트를 숨은 reasoning으로 재분류 | CONFIRMED | `google.go:721-728`. 오라클은 `thought` 분기 자체가 없고 항상 `text_delta`(`google.ts:488-498,656-668`) |
| 19 | 환경변수 치환이 모든 config 문자열에 재귀 적용 | CONFIRMED | `environment.go:27-64`가 config 전체를 JSON 왕복하며 모든 문자열 치환. `$`로 시작하는 리터럴 헤더 값·모델명이 빈 문자열이 됨 |

## 머지가 드러낸 새 층: 보안 하드닝 14건 중 9건 미이식

`origin/dev`가 가져온 것들이다. 앞선 레인 조사는 이 코드가 존재하기 전이라 볼 수 없었다.

| 우선순위 | 오라클 | go | 결과 |
| ---: | --- | --- | --- |
| 1 | `src/server/management-auth.ts` | **ABSENT** | 관리/데이터 자격증명 분리 없음. 관리 API가 공용 admission 경로를 그대로 씀(`middleware.go:60-87`), `Authorize`는 nil(`server.go:519`) |
| 2 | `src/lib/admin-secrets.ts` | **ABSENT** | 관리 전용 admin 토큰 개념 자체가 없음 |
| 3 | `src/lib/pinned-http.ts` | **ABSENT** (부분 존재) | discovery에는 DNS 사전 거부가 있다(`catalog_provider_fetch.go:75,250,260`). 없는 것은 **해석 결과를 실제 연결까지 고정**하는 것 — 평범한 `DialContext`/`client.Do`를 쓴다(`fetch.go:28,45`). 즉 사전검사와 연결 사이 TOCTOU가 남는다 |
| 4 | `src/lib/provider-outbound.ts` | **ABSENT** | pinned peer, 수동 리다이렉트 차단, 프록시 경계 의미가 없음 |
| 5 | `src/update/npm-invocation.mjs` | **ABSENT** | go 업데이트가 맨 `npm`/`npm.cmd` 실행(`runtime_management.go:536-540`), Windows PATH 해석이 cwd를 건너뛰지 않음(`winexec.go:30-40`) |
| 6 | update cwd 명령 하이재킹 방지 | **ABSENT** | 위와 같은 뿌리 |
| 7 | `src/lib/config-ownership.ts` | **ABSENT** | `.opencodex-owner.json` 소유권 개념 없음 |
| 8 | manifest 소유 상태만 제거 | **ABSENT** | 같은 뿌리 — 제거 경계가 없음 |
| 9 | 빈 hostname에서 config 보존 | **ABSENT** | go는 빈 hostname을 거부(`config.go:641-642`), 오라클은 보존·강등 |
| 10 | `src/lib/proxy-env.ts` | 부분 | `environment.go:67-100`이 HTTP/HTTPS/NO_PROXY만, `ALL_PROXY` 등 미반영 |
| 11 | 로컬 응답 프레이밍 거부 | 부분 | go는 `X-Frame-Options: DENY`(`middleware.go:128-130`), CSP `frame-ancestors 'none'` 없음 |
| 12 | 빈 bind hostname 거부 | 있음 | `config.go:641-642` |
| 13 | launchctl 환경 정리 하드닝 | 있음 | `systemenv.go:72-92,199-207` |
| 14 | `src/lib/provider-url.ts` | 있음 | `shared.go:128-141` |

## 구현 순서 (의존성 기준)

보안 경계가 가장 아래층이고, 그 위에 라우팅 무결성, 그 위에 전송 의미론이 온다.

```
090 자격증명·목적지 경계 ── 감사 2,3,4 + 머지 1,2,3,4. 다른 모든 것의 아래층.
        │
100 전송 의미론 ────────── 감사 1,6,7,9,10,12. 매 턴 지나가는 경로.
        │
110 요청 정규화 ────────── 감사 5,8,17,18,19. 요청/응답 본문 변형.
        │
120 관리 표면 ──────────── 감사 13,14,15,16. 대시보드 계약.
        │
130 플랫폼 하드닝 ──────── 머지 5,6,7,8,9,10,11. Windows/업데이트/소유권.
```

각 decade가 하나의 work-phase = 하나의 완전한 PABCD 사이클이다. 090이 먼저인 이유는
2·3·4가 **자격증명이 잘못된 곳으로 갈 수 있는** 경로이기 때문이다 — 그 위에서 전송 의미론을
고쳐봐야 요청이 엉뚱한 vendor로 가면 의미가 없다.

## 이 문서가 주장하지 않는 것

- 18건이 전부 같은 무게라는 주장이 아니다. P0 4건과 머지 1~4번이 보안 경계이고 나머지는
  동작 파리티다.
- 각 항목의 수정 방향은 서브에이전트가 제안한 "가장 작은 변경"이며, 각 work-phase의 P가
  현재 트리에 재검증한 뒤 확정한다.
- 5번과 3번은 뿌리가 겹친다(둘 다 `serve.go:347`의 registry 덮어쓰기). 090이 3번을 닫으면
  5번의 절반이 함께 닫힐 수 있으므로, 110의 P가 그 시점에 남은 부분만 다시 잰다.
- 머지 표의 "있음" 3건도 바이트 단위 동등을 증명한 것은 아니다. 해당 work-phase가 다시 본다.
