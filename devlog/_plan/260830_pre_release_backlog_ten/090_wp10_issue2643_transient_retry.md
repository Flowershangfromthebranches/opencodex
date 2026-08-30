# 090 — WP10: issue #2643 transient-5xx retry (PR #2655)

Owner score **70** — the highest-scored item *in this unit*; #1107 (71) and #695
(69) score higher in the backlog and are deferred with recorded reasons
(`001_audit_response.md`). Author `TooSpace`. Runs last because it is a
re-implementation on the most volatile file in the repository, not because it
matters least.

## Accepted scope

Provider option `transientRetryOn5xx?: { enabled?: boolean; attempts?: number }`,
**disabled by default**. `{}` opts in with defaults; `enabled: false` and
omission both disable. Key-auth `openai-chat` only — omitted `authMode` counts as
the documented key-auth default; OAuth, forward, local, and unknown modes are
rejected. Retry pre-stream `500/502/503/504/520/521/522` across all three sends:
initial Responses, terminal-guard continuation, and native
`/v1/chat/completions`. Reuse `fetchWithTransientRetry` with its fixed 400 ms
backoff base, 5 s cap, `Retry-After`, and cancellation handling. Preserve the
legacy direct-Google exception.

Excluded: `baseDelayMs`/`maxDelayMs` config, other adapters or auth modes,
mid-stream SSE replay, any 429 change (`retryOn429` stays separate), 507/529, and
dashboard/PATCH editing — v1 is config-file only, so
`src/server/management/provider-routes.ts` stays unchanged.

## PR #2655 as it stands

Head `bc3c4daeb`, 7 files, +375/−13, draft, `CHANGES_REQUESTED`, ~67 commits
behind. It introduces exactly `transientRetryOn5xx`, `enabled`, `attempts`.

Two defects in the diff itself:

1. `transientRetryPolicyFor` never checks `adapter === "openai-chat"`, so any
   generic key-auth adapter can opt in — wider than the accepted scope.
2. `transientRetryOn5xxPolicyConfigError` is never called by
   `providerManagementConfigError`, so its claimed management validation is dead
   code.

## The defect that matters most: a multiplicative retry budget

`fetchWithTransientRetry` forwards the **same** `attempts` into every nested
`fetchWithResetRetry`. So `attempts: 3` can emit **9** upstream sends and
`attempts: 10` can emit **100**. Shipping a retry feature that multiplies load
against an already-failing provider is worse than shipping no retry at all.

### Why it is latent today and why this PR activates it

`fetchWithTransientRetry` forwards the whole `opts` object — `attempts`
included — into every `fetchWithResetRetry` call. The existing doc comment on the
function states the hazard outright:

> note `opts.attempts` is shared with the inner reset layer (no caller passes it
> today).

That parenthetical is the entire safety margin. **PR #2655 is the first caller to
pass `attempts`**, so it converts a documented latent hazard into live behavior:
the outer loop runs up to `attempts` transient rounds and each round's
`fetchWithResetRetry` independently retries up to `attempts` resets.

### Before (current shape, abridged)

```ts
const attempts = Math.max(1, opts.attempts ?? TRANSIENT_RETRY_MAX_ATTEMPTS);
let res = await fetchWithResetRetry(doFetch, opts);          // inner budget = attempts
for (let attempt = 0; attempt < attempts - 1; attempt++) {
  if (res.ok || !isTransientUpstreamStatus(res.status)) return res;
  if (opts.abortSignal?.aborted) return res;
  if (Date.now() - attemptStart > slowAttemptMs) return res;
  cancelResponseBodyBestEffort(res);
  await sleepWithAbort(delay, opts.abortSignal);
  transientStatuses.push(res.status);
  try {
    res = await fetchWithResetRetry(doFetch, opts, "transient-5xx"); // again = attempts
  } catch (err) {
    throw new UpstreamRetryEvidenceError(transientStatuses, err);
  }
}
return res;
```

Worst case sends = `attempts × attempts`: 3 → 9, 10 → 100.

### After (total-send budget)

```ts
const budget = Math.max(1, opts.attempts ?? TRANSIENT_RETRY_MAX_ATTEMPTS);
let sent = 0;                                   // shared across BOTH layers

// One wrapper counts every real upstream send, wherever it originates.
const countedFetch: ReplayableFetch = (recovery) => {
  sent += 1;                                    // increment BEFORE awaiting, so a
  return doFetch(recovery);                      // rejection still consumes budget
};

const remaining = () => Math.max(1, budget - sent);

let attemptStart = Date.now();
let res = await fetchWithResetRetry(countedFetch, { ...opts, attempts: remaining() });

while (sent < budget) {
  if (res.ok || !isTransientUpstreamStatus(res.status)) return res;
  if (opts.abortSignal?.aborted) return res;   // MUST precede the cancel below,
                                              // so we never return a cancelled body
  if (Date.now() - attemptStart > slowAttemptMs) return res;
  // budget check happens in the while condition, so the body never over-sends
  cancelResponseBodyBestEffort(res);
  await sleepWithAbort(delay, opts.abortSignal);  // THROWS on mid-sleep abort;
                                                 // rejection propagates (see note 6)
  attemptStart = Date.now();
  transientStatuses.push(res.status);
  try {
    res = await fetchWithResetRetry(
      countedFetch,
      { ...opts, attempts: remaining() },
      "transient-5xx",
    );
  } catch (err) {
    throw new UpstreamRetryEvidenceError(transientStatuses, err);
  }
}
return res;                                     // exhausted: body intact
```

Contract points that must hold, each with its reason:

1. **`sent` increments before the await**, so a rejected send still consumes
   budget. Counting only successes would let a reset storm loop forever.
2. **The inner layer receives `remaining()`, never the original `attempts`.**
   This is the actual fix; everything else is bookkeeping.
3. **`Math.max(1, …)`** keeps the inner call legal when budget is exhausted — the
   `while` condition, not a zero-attempt inner call, is what stops the loop.
4. **The terminal response body is never cancelled.** `cancelResponseBodyBestEffort`
   runs only on a response we are about to replace. On exhaustion the last `res`
   returns with its body intact, matching current ok/non-transient/aborted/slow
   behavior.
5. **Evidence wrapping is unchanged**: a rejection after any transient status still
   throws `UpstreamRetryEvidenceError` so the failure stays account-attributed
   rather than being downgraded to the pre-connection class.
6. **Cancellation has two distinct paths, and conflating them is a real bug.**
   Verified on `origin/dev`: `sleepWithAbort` (`src/lib/upstream-retry.ts:53-72`)
   **throws** `abortError(signal)` — at `:55` if already aborted, at `:64` if the
   signal fires mid-backoff. It does not resolve early. So:
   - **Abort observed before the retry decision** (`opts.abortSignal?.aborted`
     check): return the current `res` with its body intact. No budget consumed.
   - **Abort fires during the backoff sleep**: `sleepWithAbort` rejects and the
     rejection propagates out of `fetchWithTransientRetry`. The body of the
     response we were about to replace was **already cancelled** by the preceding
     `cancelResponseBodyBestEffort`, which is correct — that response is being
     discarded, and the caller receives an abort rejection, not a response.

   This is existing `dev` behavior and the budget change must not alter it. The
   earlier draft of this document claimed an abort "returns the current response"
   in both cases; that was wrong for the mid-sleep path and would have described a
   response whose body had already been cancelled.

   One ordering consequence worth stating explicitly: the abort check must stay
   **before** `cancelResponseBodyBestEffort`, so a pre-abort return never hands
   back a cancelled body.

Worked sequences the tests must pin:

| Sequence with `attempts: 3` | Total sends | Result |
|---|---:|---|
| 503, 503, 200 | 3 | 200 |
| ECONNRESET, 503, 200 | 3 | 200 |
| 503, ECONNRESET, ECONNRESET | 3 | `UpstreamRetryEvidenceError` |
| 503 × 4 | **3, not 9** | last 503 returned, body intact |
| 400 | 1 | returned immediately, no retry |

## Rebase vs re-implement — per file

A three-way `git merge-tree` produced no conflict markers, which is exactly the
trap: textual cleanliness is not semantic safety on this surface.

| File | Dev drift since merge base | Call |
|---|---|---|
| `src/config.ts` | +63/−3, 4 commits | **Re-implement** — provider schema surroundings moved |
| `src/server/responses/core.ts` | +309/−31, 7 commits | **Re-implement** — OAuth/account recovery and tool authorization changed around this loop |
| `src/providers/key-failover.ts` | unchanged | replay + add the adapter gate |
| `src/server/chat-native.ts` | unchanged | replay after the budget fix |
| `src/types/provider.ts` | +19/−0 | replay; fix `attempts` docs |
| `src/types.ts` | +2/−0 | replay |
| `tests/transient-retry-policy.test.ts` | absent | adds cleanly; split coverage into subsystem files |

Credit `TooSpace` in the commit message: this is a re-implementation of their
design, not independent work.

## Diff shape

1. `src/lib/upstream-retry.ts` — the counted total-send budget above.
2. `src/types/provider.ts:50,558`, `src/types.ts:89-95` — `TransientRetryPolicy`;
   document `{}` as opt-in, default 3, range 1-10, **total sends including the
   initial request**.
3. `src/config.ts:439,490-520,1349,1418,1835,2218` — strict schema beside
   `retryOn429PolicySchema`; declare it in `providerConfigSchema`; add a redacting
   load sanitizer called from both load paths. Invalid `enabled` drops the whole
   policy; unknown fields are removed; a non-empty policy with no valid fields is
   deleted; an intentional `{}` survives. Mandatory because
   `providerConfigSchema` ends in `.passthrough()`.
4. `src/providers/key-failover.ts:12,29-37,98` — `DEFAULT_TRANSIENT_RETRY` and
   `transientRetryPolicyFor`, taking `adapter` and returning `null` unless
   `openai-chat` with key auth.
5. `src/server/chat-native.ts:21-36,205-227` — select the helper from the resolver
   only; pass the total-send budget.
6. `src/server/responses/core.ts:226-230,5559-5578,6078-6101` — extend the
   existing `key-failover` import (no new module edge); apply to initial and
   continuation sends; parenthesize the policy-or-Google selection explicitly.
7. `src/server/auth-cors.ts:9,599` — wire the config-error function into
   `providerManagementConfigError` **unconditionally**. POST/reload can carry this
   field, so conditional wording would leave the validator dead exactly as it is
   dead in PR #2655 today. Do **not** touch `applyProviderPatchFields`
   (`src/server/management/provider-routes.ts:112`) — PATCH editing stays out of
   v1. This is a management boundary and needs explicit security review.
8. Docs row after `retryOn429` at
   `docs-site/src/content/docs/reference/configuration/providers.md:120`, plus the
   locales that already document `retryOn429`.

## Core/Lab boundary

Touches `src/server/responses/core.ts` — one of the three protected files. It adds
**no** Lab import: `core.ts` already imports `key-failover`. Does not touch
`src/router.ts` or `src/server/lifecycle.ts`.
`tests/core-lab-boundary.test.ts` is the oracle.

## Tests

`tests/upstream-transient-retry.test.ts` (helper + **mixed `ECONNRESET` and 503
asserting the hard total-send count** — this is the regression for the
multiplicative bug), `tests/transient-retry-policy.test.ts` (resolver + initial
Responses), `tests/chat-completions-endpoint.test.ts` (native Chat 503 and
exhaustion), `tests/terminal-guard-server.test.ts` (continuation),
`tests/config-user-edits.test.ts` (sanitizer, `{}`, malformed, redaction),
`tests/server-combo-failover-e2e.test.ts` (same-target exhaustion precedes combo
advancement), `tests/core-lab-boundary.test.ts`.

## Overlap with WP6

Both add a provider config key in `src/config.ts`. Re-read WP6's landed schema
shape before rebasing this.

## Done

Retry lands limited to key-auth `openai-chat` with `enabled`/`attempts` and
fixed delays, green, with the total-send budget proven (criterion c-10).
