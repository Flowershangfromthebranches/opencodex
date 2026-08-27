/**
 * Opt-in idle refresh so a reset is noticed while nobody is looking.
 *
 * This is load-bearing, not a convenience. fetchProviderQuotaReports has exactly one caller
 * — the GET /api/provider-quotas route — so with no dashboard open and no CLI invocation it
 * never runs, no second snapshot exists, and an overnight reset passes unobserved. That
 * overnight case is the whole reason the subsystem exists.
 *
 * Shape follows src/storage/policy-scheduler.ts: a module-singleton unref'd interval whose
 * config gate lives in the callee, so toggling `enabled` takes effect on the next tick
 * without a restart (the rationale spelled out at src/oauth/token-guardian.ts:276).
 */

const DEFAULT_INTERVAL_MS = 15 * 60_000;
/**
 * Above the 5-minute provider cache TTL and the 10-minute per-account TTL, so one tick costs
 * at most one upstream probe per window rather than hammering a quota endpoint that
 * rate-limits (src/providers/quota.ts:1425 records an observed 429 under repeated probing).
 */
export const MIN_INTERVAL_MS = 60_000;

let timer: ReturnType<typeof setInterval> | null = null;
let detachShutdownHook: (() => void) | null = null;

/** Number of ticks that have run. Test-only observability; carries no quota data. */
let tickCount = 0;

async function tick(): Promise<void> {
  try {
    const { isQuotaResetNotificationEnabled, resolveQuotaResetPollMs } = await import(
      "./reset-notify-config"
    );
    // Re-sync the sink on every tick, before the enable check bails out. This is what makes
    // enabling or disabling notifications take effect without a restart: an install that starts
    // with the section absent has no sink, and the seams therefore skip all work, so something
    // has to notice the operator turned it on. Cheap — the resolver is mtime-cached.
    const { syncQuotaResetActivation } = await import("./reset-activation");
    await syncQuotaResetActivation();
    if (!isQuotaResetNotificationEnabled()) return;
    if (resolveQuotaResetPollMs() === 0) return;
    tickCount += 1;
    const [{ loadConfig }, { fetchProviderQuotaReports }] = await Promise.all([
      import("../config"),
      import("../providers/quota"),
    ]);
    // Forced: an unforced call would be served from the 5-minute cache and produce no new
    // observation at all.
    await fetchProviderQuotaReports(loadConfig(), true);
  } catch {
    // A failed probe is not an error worth surfacing: the next tick tries again.
  }
}

/** Idempotent. A second call while running is a no-op, matching startStorageCleanupScheduler. */
export function startQuotaResetPoller(intervalMs = DEFAULT_INTERVAL_MS): void {
  if (timer) return;
  const bounded = Math.max(MIN_INTERVAL_MS, Math.floor(intervalMs));
  timer = setInterval(() => void tick(), bounded);
  // Never keep the process alive for a quota probe.
  timer.unref?.();
  void import("../lib/optional-shutdown-hooks")
    .then(hooks => {
      detachShutdownHook = hooks.registerOptionalShutdownHook(
        "quota-reset-poller",
        stopQuotaResetPoller,
      );
    })
    .catch(() => {
      // Without the hook the unref'd timer still cannot delay exit.
    });
}

export function stopQuotaResetPoller(): void {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
  detachShutdownHook?.();
  detachShutdownHook = null;
}

export function isQuotaResetPollerRunning(): boolean {
  return timer !== null;
}

/** Test-only: run one tick synchronously rather than waiting out the interval. */
export async function runQuotaResetPollerTickForTests(): Promise<void> {
  await tick();
}

export function quotaResetPollerTickCountForTests(): number {
  return tickCount;
}

export function resetQuotaResetPollerForTests(): void {
  stopQuotaResetPoller();
  tickCount = 0;
}
