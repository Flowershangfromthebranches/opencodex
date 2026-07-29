# 017 — L8 인벤토리: `src/oauth/**` + `src/providers/**`

44파일 / 10,964줄. 대응 go: `internal/oauth`, `internal/providers`, `internal/registry`.
exported 325개 스윕.

## 보안 경계 발견 (이 레인의 최우선)

### 1. 요청시 리프레시 배선에 Kiro와 GitHub Copilot이 빠졌다 (DIVERGENT)

오라클 `oauth/index.ts:140,273,448`은 두 provider를 lazy refresh에 넣는다.
go는 구현을 갖고 있으면서(`oauth/kiro.go:165`, `github_copilot.go:75`)
`cli/oauth_guardian.go:20-41`에 `openai/chatgpt`, `anthropic`, `xai`, `google-antigravity`,
`cursor`, `kimi`만 등록한다. 만료된 Kiro/Copilot 계정이 갱신 대신 실패한다.

### 2. 터미널 리프레시 실패가 `needsReauth`를 남기지 않는다 (DIVERGENT)

오라클 `index.ts:297-311`, `:400-439`는 터미널 오류를 분류해 **해당 세대**를 재인증 필요로
표시한다. go `store_refresh.go:112-115`는 오류를 그대로 반환하고
`authcontext.go:108-113`이 감싸기만 한다. 취소·회전된 refresh grant가 영구 실패 상태로
기록되지 않아 평범한 실패처럼 계속 재시도된다.

### 3. Anthropic 풀의 local-cli 채택 가드 부재 (DIVERGENT)

오라클 `anthropic-routing.ts:138-145`, `:500-528`은 배경 풀 슬롯이 전역 Claude CLI 자격증명을
채택하는 것을 슬롯이 활성일 때로 제한한다. go `anthropic_pool.go:284-377`은 `NeedsReauth`와
쿨다운만 본다.

### 4. OK로 확인된 보안 축

콜백 서버 루프백 바인딩과 state 검증(`callback.go:135-288`), 로그 리댁션(`log.go:15-75`,
go가 CR/LF까지 제거), 교차 프로세스 리프레시 락과 세대 비교(`store_refresh.go:13-128`),
PKCE S256(엔트로피 차이는 있으나 둘 다 RFC 유효).

## provider 레지스트리 차등

오라클 60개, `internal/providers` 60개, `internal/registry` 60개 — id는 일치.
문제는 **go에 레지스트리가 둘**이고 스키마 충실도가 다르다는 것이다.
`internal/registry/registry.go:24-45`는 `AllowKeyAuthOverride`, `GoogleMode`, reasoning/modality
맵, 가상 모델, 접미사 스트립 같은 필드를 갖지 않는다. 런타임이 그 축소된 레지스트리를 쓰므로
오라클 메타데이터의 상당 부분을 관측할 수 없다.

## 그 외 확정 결함

| # | 항목 | 등급 |
| --- | --- | --- |
| 1 | Kiro 강제 로그인 롤백/세션 복구 (`index.ts:702`, `kiro-credentials.ts:444`) | MISSING |
| 2 | pre-multiauth 다운그레이드 백업 (`store.ts:168`) | MISSING |
| 3 | Copilot API base URL 허용목록 검증 (`store.ts:203`) | MISSING |
| 4 | 자격증명 정규화가 refresh 없는 것도 수용 (`store.go:116` vs `store.ts:188`) | DIVERGENT |
| 5 | API 키 429 회전이 transport를 재구성/영속화하지 않음 (`key-failover.ts:62`) | DIVERGENT |
| 6 | xAI 로컬 Grok CLI 세대 채택 3함수 미배선 (`local_token_detect.go:43,47,54`) | UNWIRED |
| 7 | context cap 관리 setter 3개 미배선 (`context_cap.go:41,53,62`) | UNWIRED |
| 8 | `MatchBaseURLChoice`, `BaseProviderLabel`, `ResolveAntigravityEffortWireModel`, `ResolveOpenAICompactModel`, `DetectModelCapabilities`, `EnrichProviderFromRegistry`, `DeriveFeaturedProviderIDs`, `DeriveJawcodeAliases`, `ShouldCaseFoldMetadataModelID`, `DeriveInitProviders` | UNWIRED |

## OK로 확인된 축

OAuth 로그인 관리(start/status/cancel/code), reauth 강제 로그인, store 뮤테이션 락과 레거시
자격증명 마이그레이션, Claude Code 로컬 토큰 import, 토큰 가디언 선제 갱신, Anthropic 풀
선택/친화성/전략/쿨다운, 계정 풀, API 키 CRUD와 마스킹, xAI transport 요청 id, Kimi 프롬프트
캐시 키, OpenAI 가상 모델(Responses 라우팅), OpenRouter 라우팅, context cap 적용, 쿼터
fetch/parse/cache, Kiro 모델 정규화.

## 검증하지 않은 것

실제 로그인·리프레시·쿼터 fetch 등 네트워크 호출 없음. 자격증명 파일 미열람, 값 미출력.
라이브 `:10100` 미검증.
