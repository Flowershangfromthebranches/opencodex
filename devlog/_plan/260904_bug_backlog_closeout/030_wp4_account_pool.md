# wp4 — account-pool fixes (#3425, #3352)

Both issues live in the Codex account selection path. Diagnosed by an independent Sol lane.

## #3425 — exhausted account keeps being selected after 502s

Findings:
- `applyQuotaAutoSwitch` returns the active account unchanged when quota is unknown
  (`src/codex/routing.ts:1655`); `hasCodexQuotaHeadroom` likewise treats unknown usage as
  eligible (`src/codex/routing.ts:1215`). A legacy fallback can also restore a configured
  active account after normal selection finds nothing (`src/codex/routing.ts:2118`).
- Known 100% usage already switches accounts — proven by `tests/codex-routing.test.ts:325`.
  So the reported 118 failures imply routing never saw the dashboard's snapshot, or
  upstream outcomes were not committed to health state. A plausible split-brain edge is
  the generation-guarded quota commit (`src/codex/auth-api.ts:1258`, `src/codex/quota.ts:279`).
- A body-less 502 carries no 429/402 quota evidence, so it is classified transient, not
  exhaustion. Mid-stream resets become synthetic 502s (`src/server/relay.ts:1374`) and are
  deliberately not replayed (`src/server/relay.ts:251`) — that explains `sendCount=1` and
  empty `recoveryKinds`. But three consecutive transient failures should still rotate
  (`src/codex/routing.ts:2459`), so bodylessness alone does not explain 118 selections.

Fix plan:
1. `hasCodexQuotaHeadroom` / `applyQuotaAutoSwitch`: consult `isCodexQuotaExhausted`
   before the unknown-usage branch; treat explicit 100% in a relevant window as a hard
   exclusion even when reset metadata is missing.
2. configured-active fallback: never restore an explicitly exhausted active account while
   another configured account exists; keep the legacy fallback for non-quota failures.
3. assert body-less HTTP and synthetic stream 502s increment the same account's transient
   streak exactly once.
Regression file: `tests/codex-routing.test.ts` (exists).
Security class: routing/quota only — stays out of security review as long as
`auth-api.ts` generation and token fetch are untouched.

## #3352 — false 401 "account does not support this model"

Findings:
- The 401 is produced locally, before any upstream call: direct forwarding throws at
  `src/codex/auth-context.ts:408`, pool selection at `src/codex/auth-context.ts:580`, and
  `CodexPoolAuthenticationError` becomes HTTP 401 at
  `src/server/responses/codex-auth-error.ts:72`.
- The entitlement layer is tri-state but admission collapses it to boolean. A timeout,
  network error, or empty roster yields `unknown` (`src/codex/model-entitlements.ts:958`),
  while `isDirectCallerEntitledToCodexModel` returns true only for `granted`
  (`src/codex/model-entitlements.ts:988`); pool eligibility likewise admits only granted
  accounts (`:1012`). A transient discovery failure is therefore treated as an
  authoritative denial — exactly the reported symptom.
- No evidence opencodex picks a different account; forwarding overwrites bearer and
  `ChatGPT-Account-Id` from the selected pool context (`src/codex/auth-context.ts:782`).
  The roster probe does send fewer headers than native Codex
  (`src/codex/model-entitlements.ts:538`), but nothing proves an omitted header causes it.

Fix plan:
1. entitlement API returns `granted | denied | unknown` instead of a boolean.
2. admission rejects only confirmed `denied`; on `unknown`, let a caller-owned credential
   reach upstream (the upstream response becomes authoritative), and treat unknown pool
   accounts as tentative candidates ranked after confirmed grants.
3. `modelsForCredential`: do not let a transient unconfirmed refresh evict a still-usable
   confirmed cache entry; keep confirmed evidence for a bounded stale-on-error interval.
4. thread the real inbound Codex client version into discovery. Do NOT speculatively add
   native headers — that would create a new compatibility dependency without evidence.
Regression files: `tests/codex-model-entitlements.test.ts`, `tests/codex-auth-context.test.ts`.
Security class: YES — authentication admission, bearer/account-header handling, and
credential-scoped caching. Requires explicit security review, including proof that tokens
and account ids are never logged and never shared across account cache entries.

## Accept criteria
- a PR per issue against dev, template-complete, with `Closes #3425` / `Closes #3352`
- entitlement change proves unknown-admitted vs confirmed-denied in a focused test
- no credential or token value is added to any log line (privacy:scan stays green in CI)

