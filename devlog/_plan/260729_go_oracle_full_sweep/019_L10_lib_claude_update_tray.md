# 019 — L10 인벤토리: `src/lib/**` + `src/claude/**` + `src/update/**` + `src/tray/**`

49파일 / 10,170줄. 대응 go: `internal/lib`, `internal/claude`, `internal/update`,
`internal/tray`, `internal/platform`. exported 366개 중 선언뿐 26개.

## `DEAD_EXPORT_AUDIT.md` 재측정

그 문서(`e5ba7b7b` 기준)는 업데이트 알림 export가 통째로 미배선이라고 했다. **절반만 맞다.**
`MaybeShowUpdatePrompt`는 이제 `cli/serve.go:702`에서 호출되고, 포트 바인딩(`:166`) 전이라는
순서 요구도 지켜진다. 그런데 두 결함이 남았다.

## 확정 결함

### 1. 순수 go 사용자는 업데이트 알림을 아예 못 받는다 (UNWIRED, user-visible)

go `ocx start`는 기존 `version.json`을 읽기만 하고 갱신하지 않는다. `CacheStale`
(`update/notify.go:66`)은 선언뿐이고 `__refresh-version` 대응 경로도 없다. TS 런타임이 먼저
캐시를 써준 적이 없으면 프롬프트가 영원히 안 뜬다.

오라클: `notify.ts:151,162`, `cli/index.ts:931`.

### 2. 프롬프트의 "지금 업데이트"가 아무 일도 안 한다 (UNWIRED, user-visible)

`update/prompt.go:78-89`는 옵션 1을 처리하려면 `RunUpdate`와 `Exit`가 필요한데
`cli/serve.go:702-713`은 **둘 다 넘기지 않는다**. 캐시가 있어 프롬프트가 떠도 기본값 `1`을
누르면 그냥 반환한다. 덤으로 `DetectInstaller("")`는 항상 source로 분류해 명령 라벨도 틀린다.

이 두 건이 "배선됐다/안 됐다" 이분법의 한계를 보여준다 — 호출은 되지만 기능하지 않는다.

### 3. Windows Codex shim이 비-ASCII 프로필 경로를 그대로 박는다 (MISSING, user-visible)

오라클 `lib/win-paths.ts:31`, `codex/shim.ts:431`은 `%USERPROFILE%`/`%APPDATA%`/
`%LOCALAPPDATA%` 간접화로 `.cmd` 모지바케를 피한다. go `codex/shim.go:133`은 리터럴 UTF-8
경로를 쓴다. 한국어·중국어 Windows 사용자명에서 shim이 깨질 수 있다.

### 4. Windows `.cmd` 호출 인자 처리가 다르다 (DIVERGENT, user-visible)

go `platform/winexec.go:13`은 배치 파일을 `cmd.exe`로 보내지만 TS의 npm-local-bin 이중 이스케이프와
`windowsVerbatimArguments` 계약을 재현하지 않는다. `%`, 공백, 따옴표, CMD 메타문자가 든 인자가
갈릴 수 있다. 활성화 증거는 Windows 호스트가 필요하다.

## OUT-OF-SCOPE로 분류한 것

`lib/bun-runtime.ts`, `lib/bun-stream-caps.ts` — Bun 런타임 전용 관심사이므로 go 포트가
가질 이유가 없다. 등급 계약에 이 칸을 넣은 이유가 이것이다.

## OK / OK-EQUIVALENT로 확인된 축

리댁션(go가 Copilot/AWS/경로 마스킹 추가), 계정 id 마스킹, Windows ACL 하드닝(필수/선택 모드
+ 서비스 호출자), WinSW 서비스 백엔드, Windows 트레이 설치/시작/중지/상태/제거,
Claude Desktop 3P 설정·프로필·모델 별칭, Claude inbound 모델/별칭 변환, Claude 모델 정보,
Claude outbound 스트림 변환, AWS EventStream 디코드(Kiro/protocol 경로), abort/deadline
원시요소(생성자 export는 미사용이나 동작은 다른 경로로 구현).

## 검증하지 않은 것

Windows 서비스/트레이/권한상승/ACL 경로는 macOS에서 정적 대조만. 업데이터·서비스·트레이
명령 미실행. 전체 go 테스트 미실행.
