# 060 — WP7: docs and locale parity

No owner score: this is release hygiene the repository's own review rules already
require ("user-facing behavior changes should update `docs-site/`, and keep
translated locales from contradicting the English source"). Three landed features
currently violate it.

Locales are exactly seven: `fr`, `ko`, `zh-cn`, `zh-tw`, `ru`, `ja`, `tr`
(`docs-site/astro.config.mjs:61-71`). English lives at the docs root, so eight
files per surface.

## A. Kiro: docs claim one login slot; the code rotates a pool

Commit `d82b3049d` landed multi-account Kiro quota rotation. Every
`reference/cli/providers-accounts.md` still says Kiro has a single login slot that
re-login replaces:

| File | Line |
|---|---:|
| `reference/cli/providers-accounts.md` | 149-150 |
| `fr/…` | 151-152 |
| `ko/…` | 116 |
| `zh-cn/…` | 129-130 |
| `zh-tw/…` | 97 |
| `ru/…` | 141-142 |
| `ja/…` | 116 |
| `tr/…` | 165-167 |

True behavior, verified in code: with two or more eligible stored accounts a 429
rotates automatically and prefers the account with the most known remaining
allowance; `ocx account login kiro` appends one account at a time.
Authority: `src/oauth/generic-account-failover.ts:164-175` (default/quorum),
`:193-234` (429 rotation), `src/oauth/account-quota-rank.ts:55-75` (ranking),
`src/oauth/store.ts:485-527` (append/upsert).

**Blocking companion defect.** The CLI itself still prints the stale
single-slot note (`src/cli/account.ts:28-29,218-220`) and
`tests/cli-account.test.ts:592-598` **asserts that it prints**. Fixing only the
docs would leave the shipped CLI contradicting them. So this phase either removes
the note and inverts that test, or splits it as a prerequisite — it may not be
quietly ignored.

## B. Combo strategies: English has five, translations have two

`OcxComboStrategy` is a five-value union (`src/types/config.ts:665`), branches at
`src/combos/resolve.ts:156-193`. English is already correct
(`guides/combos.md:173-190,345-347`;
`reference/configuration/routing.md:76-77,186-188`). All seven translations still
document only `failover` and `round-robin`.

| Locale | `guides/combos.md` anchor; stale rows | `routing.md` stale rows; combo sentence |
|---|---|---|
| fr | after 172; 317-319 | 49-50; 130 |
| ko | after 102; 220-222 | 59-60; 100 |
| zh-cn | after 128; 251-253 | 64-65; 110 |
| zh-tw | after 143; 256-258 | 47-48; 128 |
| ru | after 129; 271-273 | 74-75; 123 |
| ja | after 102; 221-223 | 60-61; 102 |
| tr | after 196; 349-351 | 95-96; 219-221 |

Semantics to preserve when translating (do not paraphrase these away):

- `random` — independent per-request draw over eligible targets, odds
  proportional to `weight`; `stickyLimit` does not apply.
- `least-used` — fewest process-recorded successes; counters reset on restart;
  ties break by configuration order; weight and stickiness ignored.
- `reset-window` — earliest cached quota reset (five-hour/weekly/monthly/custom);
  absent or stale quota and ties preserve configuration order; weight and
  stickiness ignored.

Also correct the three per-locale rows so weight reads as applying only to
`round-robin` and `random`, and `stickyLimit` only to `round-robin`.

## C. Two landed config keys with zero documentation

`xaiResponsesXSearch` (`e308f13da`) — add one row to English plus all seven
`reference/configuration/providers.md`, after the existing xAI Responses opt-in
row (en 106, fr 102, ko 90, zh-cn 90, zh-tw 71, ru 103, ja 90, tr 109):

```md
| \`xaiResponsesXSearch?\` | \`boolean\` | Disabled by default. On an xAI Responses destination, append the provider-hosted \`x_search\` declaration only when a live \`web_search\` survives final normalization. Existing declarations are not duplicated, caller \`tool_choice\`/\`allowed_tools\` selectors are not widened, and this is separate from the web-search sidecar's \`search.xSearch\`. |
```

Authority: `src/types/provider.ts:438-442`; `src/adapters/xai-web-search.ts:209-243`.

`blockedModelRedirects` (`db7606f30`) — add one row to English plus all seven
`reference/configuration/routing.md`, plus an example
`{ "gpt-5.6-terra": "gpt-5.6-luna" }` under "Model resolution order":

```md
| \`blockedModelRedirects?\` | \`Record<string, string>\` | unset | Exact resolved model-id replacements. A match keeps the already-selected provider/account route, replaces its upstream model id, and records route reason \`blocked-model-redirect\`; omission leaves routing unchanged. |
```

Authority: `src/types/config.ts:451-457`; `src/router.ts:509-528`.

## D. Three further gaps found while mapping the above

- `x-opencodex-request-id` (`b9cb23656`) — all eight
  `reference/proxy-formats.md`: admitted HTTP Responses get a proxy-generated
  `ocx-<32 hex>`; caller/upstream values are overwritten; the header is
  CORS-exposed; rejected auth/origin responses omit it.
  Code: `src/server/index.ts:447-478`.
- `cacheHitRate` (`7071b2d47`) — all eight `reference/management-api.md` under
  `GET /api/usage`: present in `models[]`, `providers[]`, `days[].models[]`;
  clamped `cacheReadInputTokens / inputTokens` in `[0,1]`, or `null` when
  telemetry or input is unavailable. Code: `src/usage/summary.ts:57-107,772,949`.
- `vercelGatewayRouting` (`aa5f711d7`) — English-only. Port
  `reference/configuration/providers.md:96-97` and the section at `:523-556` into
  all seven translations (rows after each `modelOpenRouterRouting?`: fr 93, ko 81,
  zh-cn 81, zh-tw 62, ru 94, ja 81, tr 100; section before each static-model
  heading: fr 373, ko 300, zh-cn 302, zh-tw 270, ru 374, ja 299, tr 411).

## Verification

No full suite. `git grep` for every stale phrase and every new literal, then
`cd docs-site && bun install --frozen-lockfile && bun run build` on the SSH host.
If the CLI note is fixed here, `tests/cli-account.test.ts` runs focused.

## Done

English and all seven locales state the real Kiro behavior and all five combo
strategies, and both config keys are documented (criterion c-7).
