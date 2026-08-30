# 080 — WP9: issue #1168 GLM Coding Plan quota for open.bigmodel.cn

Owner score **65**. Two concrete defects on `dev`, both narrow.

## Accepted scope

Probe only `GET https://open.bigmodel.cn/api/monitor/usage/quota/limit`. Send the
BigModel key **directly** in `Authorization` with no `Bearer ` prefix. Keep
redirects disabled and never send credentials to lookalike hosts. Parse
`data.limits[]`: `TOKENS_LIMIT` with `unit=3, number=5` is the five-hour model
window, `unit=6, number=1` the weekly one. Preserve numeric millisecond
`nextResetTime`. Missing windows stay missing. Ignore unknown rows rather than
guessing. Preserve `api.z.ai` behavior exactly.

Excluded: `open.bigmodel.cn/api/paas/v4` (pay-as-you-go) stays ineligible; no
inferring plan generation from returned windows; and — the important one —
`TIME_LIMIT + unit=5` is the **shared monthly MCP call allowance** for
`search-prime`, `web-reader`, and `zread`. It is not a model-token quota.

## What PR #2028 already landed

Merged `5533c1d5c`: canonical-host admission, region-selected monitor host,
`redirect: "error"`, the `data.limits[]` parser with legacy fallback, provider
aliases, tests, docs. The seam exists; #1168 is what it got wrong.

## Defect 1 — wrong auth scheme

`fetchZaiQuota()` (`src/providers/quota.ts:722-747`) sends
`Authorization: \`Bearer \${apiKey}\`` to **both** hosts. BigModel wants the raw
key. Insert after `monitorHost` and before `fetch`:

```ts
const authorization = monitorHost === ZAI_CN_BASE_URL
  ? apiKey
  : \`Bearer \${apiKey}\`;
```

then use `Authorization: authorization`. Direct-key auth stays confined to the
already-canonicalized BigModel origin (`isCanonicalZaiBaseUrl()`, `:302-310`), so
this cannot leak a raw key to an arbitrary host.

## Defect 2 — an MCP call allowance is reported as model quota

`parseZaiQuotaLimits()` (`:644-681`) maps `TIME_LIMIT` into the monthly **model**
quota at `:674-678`:

```ts
} else if (row.type === "TIME_LIMIT") {
  quota.monthlyPercent = percent;
  if (resetAt !== undefined) quota.monthlyResetAt = resetAt;
  windows += 1;
}
```

So a user's MCP search allowance is displayed as their monthly model budget. Gate
rows before percent/reset parsing:

```ts
if (row.type !== "TOKENS_LIMIT" && row.type !== "CREDIT_LIMIT") continue;
```

then delete the `TIME_LIMIT` branch entirely and count only recognized token
windows in `windows`. Consequence to accept deliberately: a payload carrying only
MCP rows now returns `null` — no quota is the honest answer. Keep
`CREDIT_LIMIT` as existing Z.AI compatibility and leave
`parseZaiQuotaLegacyFields()` untouched so the flattened Z.AI fallback does not
regress. Update the comment at `:633-642` to say why `TIME_LIMIT` is ignored.

## Absent-window semantics

`ProviderQuota` fields are optional (`src/providers/quota-types.ts:26-35`).
Absent means **omitted**, never synthesized `0`, so the dashboard cannot render a
fake full-capacity bar. A V1 Lite response yields exactly
`{ fiveHourPercent, fiveHourResetAt, updatedAt }` with no weekly/monthly keys.
`tests/quota-bars-rows.test.ts:23-28` already establishes five-hour-only
rendering.

## Tests

Extend `tests/provider-quota.test.ts`: keep `api.z.ai` Bearer coverage
(`:955-989`) and assert `TIME_LIMIT` creates no monthly quota; replace
`:991-1026` with the real V1 Lite fixture expecting the direct key, five-hour 1%,
the exact reset, and absent weekly/monthly; switch the `/api/v1` expectation at
`:1053-1086` to the direct key; expect no report when only unknown windows plus
`TIME_LIMIT` remain (`:1130-1148`); rewrite `:1161-1187` to assert MCP exclusion.
Preserve canonical-host, pay-as-you-go rejection, redirect, malformed, and legacy
tests. Real payloads are inline in that file today; if extracting, use
`tests/fixtures/` rather than a new parallel tree.

## Docs

`docs-site/src/content/docs/guides/providers.md:657-666` currently documents both
wrong behaviors. Correct it to distinguish Z.AI Bearer from BigModel direct-key
auth and state that MCP `TIME_LIMIT` rows are excluded from model-quota reporting.

## Privacy

No raw payload, credential, account identifier, or MCP breakdown may reach normal
logs or UI responses. `privacy:scan` must stay green.

## Done

BigModel uses raw `Authorization`, parses `limits[]`, and `TIME_LIMIT` no longer
maps to monthly model quota, with fixtures (criterion c-9).
