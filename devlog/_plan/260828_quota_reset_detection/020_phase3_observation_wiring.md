# wp3 — Observation wiring

Feed the wp2 detector real snapshots from both quota subsystems, and add the opt-in poller
that makes idle detection possible at all.

## Why a poller is required, not optional

`fetchProviderQuotaReports` has exactly one caller — the `/api/provider-quotas` route at
`src/server/management/provider-routes.ts:421`. With no dashboard open and no CLI call,
that function never runs, so no second snapshot ever exists. Passive observation alone
cannot detect a reset that happens overnight, which is precisely the case the user cares
about. The poller is default-OFF; when off, this phase degrades to passive-only.

## NEW `src/quota/reset-observer.ts`

The one place that turns a (prev, next) pair into a delivered notification. Core-owned
entry point so the seams below stay one-line calls.

```ts
/**
 * Observe one committed snapshot transition. Never throws: a detection or delivery
 * failure must not break the quota write that triggered it.
 */
export function observeQuotaSnapshot(input: {
  readonly scope: string;
  readonly accountKey: string;
  readonly previous: ReadonlyArray<QuotaWindowObservation>;
  readonly next: ReadonlyArray<QuotaWindowObservation>;
  readonly now?: number;
}): void;
```

Body: bail immediately when the feature is off (`isQuotaResetNotificationEnabled()` from
wp4, which reads config and is cheap); otherwise pair windows by identity, call
`detectQuotaReset` per window, drop events whose key `hasSeenQuotaReset`, then
`markQuotaResetSeen` + `recordQuotaResetEvent` + hand to the sink. Everything wrapped in
`try/catch` at the top level, matching the fail-safe posture of
`src/server/memory-watchdog.ts:127`.

Order matters: the key is CLAIMED before dispatch, via the single synchronous
`claimQuotaReset` from wp2. A sink that fails must not cause a re-notification storm on the
next poll; delivery failure is recorded on the event (wp4) rather than retried by
re-detecting. Claiming atomically also closes the concurrency window where a poller tick and
a live pooled response observe the same transition in the same second.

## MODIFY `src/codex/quota.ts`

Two commit points, both already holding `existing` and `next`.

Before (`:335`):
```ts
  accountQuota.set(accountId, next);
  schedulePersistAccountQuotas();
}
```
After:
```ts
  accountQuota.set(accountId, next);
  schedulePersistAccountQuotas();
  notifyCodexQuotaSnapshot(accountId, existing, next);
}
```

The credits-only commit at `:289` gets NO call. Trap 1: that branch copies every window
field from `existing` verbatim and changes only `resetCredits`, so there is by construction
no window transition to detect. Skipping it is cheaper and more honest than detecting
nothing.

NEW local helper in the same file, keeping the module's import surface unchanged at the top
level by using a lazy import — `src/codex/quota.ts` is reachable from
`src/server/responses/core.ts`, so a static import of the observer would pull the whole
notification subsystem onto the core request path and defeat the default-OFF guarantee:

```ts
/**
 * Hand a committed transition to the optional reset observer.
 *
 * Lazy import on purpose: this file sits on the hot pooled-response path via
 * applyAccountQuotaFromUpstreamHeaders, and a static import would load the observer,
 * its config read, and its sink registry into every install — the same mistake
 * tests/core-lab-boundary.test.ts exists to prevent for src/lab/.
 */
function notifyCodexQuotaSnapshot(
  accountId: string,
  previous: StoredAccountQuota | undefined,
  next: StoredAccountQuota,
): void {
  if (previous === undefined) return;
  void import("../quota/reset-observer")
    .then(m => m.observeQuotaSnapshot({
      scope: "codex",
      accountKey: accountId,
      previous: codexWindowObservations(previous),
      next: codexWindowObservations(next),
    }))
    .catch(() => {
      // Detection is best-effort; a quota write must never fail because of it.
    });
}
```

`codexWindowObservations(quota)` maps `StoredAccountQuota` to the neutral shape:
`shortPercent/shortResetAt` -> `"5h"`, `weeklyPercent/weeklyResetAt` -> `"weekly"`,
`monthlyPercent/monthlyResetAt` -> `"monthly"`, each `customWindows[]` entry ->
`"custom:<label>"`. Windows absent from the snapshot are omitted, not zero-filled.

## MODIFY `src/providers/quota.ts`

One seam: the sole site where a newer report displaces an older one.

Before (`:2337`):
```ts
      const reports = response.reports.filter(item => mayCommitProviderQuotaKey(item.provider, writerGeneration));
      cache = { key, ts: Date.now(), response: { ...response, reports } };
```
After:
```ts
      const reports = response.reports.filter(item => mayCommitProviderQuotaKey(item.provider, writerGeneration));
      const superseded = previous;
      cache = { key, ts: Date.now(), response: { ...response, reports } };
      notifyProviderQuotaSnapshot(superseded, reports);
```

Committing first, notifying second: the cache write is the source of truth and must not be
delayed by observation. `previous` is already bound at `:2290`.

`notifyProviderQuotaSnapshot` mirrors the codex helper (lazy import, per-provider pairing,
`accountKey` = the provider's active-account key so a switch cannot inherit history).
Providers present in `previous` but absent from `reports` are skipped entirely — trap 3 and
the terminal-failure deletion at `:2330` both remove rows for reasons that are not resets.

## NEW `src/quota/reset-poller.ts`

Follows the repo's canonical opt-in job shape: `src/storage/policy-scheduler.ts:13` for the
timer, `src/storage/policy-job.ts:445` for gating in the callee rather than the timer.

```ts
export function startQuotaResetPoller(intervalMs?: number): void;
export function stopQuotaResetPoller(): void;
```

- Idempotent: `if (timer) return;`
- `timer.unref?.()` so it never keeps the process alive — the rule
  `src/storage/policy-scheduler.ts:3` states and every loop in this repo follows
- Each tick re-reads config and returns immediately when disabled, so toggling `enabled`
  takes effect without a restart (the rationale spelled out at
  `src/oauth/token-guardian.ts:276`)
- Default interval 15 minutes, floored at 60 s. Above the 5-minute provider TTL and the
  10-minute account TTL, so a tick costs at most one upstream probe per window.
- Tick calls `fetchProviderQuotaReports(loadConfig(), true)`; the forced probe is what
  produces a second snapshot. Failures are swallowed.
- Registers teardown via `registerOptionalShutdownHook("quota-reset-poller", stop)`
  (`src/lib/optional-shutdown-hooks.ts:32`) at start, so a process that never enables it
  registers nothing.

Started from `src/server/background-lifecycle.ts:54` alongside
`startStorageCleanupScheduler()`, inside the existing try/rollback block. That file is not
one of the three core-boundary-protected entrypoints.

## NEW `tests/quota-reset-observation.test.ts`

- a codex weekly rollover driven through `setAccountQuotaFromParsed` twice fires exactly
  one event, with the fake sink capturing `kind === "scheduled"`
- a surprise drop driven the same way fires `kind === "surprise"`
- the credits-only path (`{ resetCredits: 3 }` only) fires nothing despite a new
  `updatedAt` — trap 1 regression
- a first-ever snapshot fires nothing — trap 2 regression
- `clearAccountQuota` then a fresh low-percent write fires nothing — trap 3 regression
- a provider report whose provider disappears from the new report fires nothing
- with the feature disabled, the sink is never invoked and no timer exists
- poller: `startQuotaResetPoller` twice creates one timer; `stopQuotaResetPoller` clears it

## Accept criteria

`bun test tests/quota-reset-observation.test.ts` exits 0; the wp2 suites stay green;
`bun test tests/core-lab-boundary.test.ts` exits 0; `bun x tsc --noEmit` exits 0.
Activation evidence: the captured-sink assertions naming `scheduled` and `surprise` through
the REAL writer, not the detector in isolation.
