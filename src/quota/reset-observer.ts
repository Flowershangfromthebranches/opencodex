/**
 * The one place a quota snapshot becomes a delivered reset notification.
 *
 * Kept behind a lazy import from both quota subsystems: src/codex/quota.ts and
 * src/providers/quota.ts are both statically reachable from
 * src/server/responses/core.ts, so a static edge here would load this module, its config
 * resolution, and its sink registry into every install — the failure AGENTS.md documents for
 * Compatibility Lab, where a six-hop chain pulled ~69 modules onto the core path.
 *
 * Never throws. A detection or delivery failure must not break the quota write that
 * triggered it.
 */

import {
  detectQuotaResets,
  quotaAccountTag,
  type QuotaResetEvent,
  type QuotaWindowObservation,
} from "./reset-detector";
import {
  claimQuotaReset,
  quotaResetAccountSalt,
  recordQuotaResetEvent,
  swapLastObservedWindows,
} from "./reset-seen-store";

export type QuotaResetSink = (event: QuotaResetEvent) => void;

let sink: QuotaResetSink | null = null;

/**
 * Install the delivery sink. Core owns the slot; wp4 registers into it at activation.
 *
 * Until something registers, detection still runs its bookkeeping but delivers nowhere,
 * which is what keeps the subsystem inert on a default install.
 */
export function setQuotaResetSink(next: QuotaResetSink | null): void {
  sink = next;
}

/** True when a sink is installed. Used by the seams to skip work entirely. */
export function hasQuotaResetSink(): boolean {
  return sink !== null;
}

/**
 * Observe one committed snapshot for a (scope, account) pair.
 *
 * The baseline comes from the detector's own persisted last-seen map, not from the caller:
 * see swapLastObservedWindows for why neither upstream cache can supply one.
 */
export function observeQuotaSnapshot(input: {
  readonly scope: string;
  readonly accountKey: string;
  readonly windows: ReadonlyArray<QuotaWindowObservation>;
  readonly now?: number;
}): QuotaResetEvent[] {
  try {
    if (!sink) return [];
    if (input.windows.length === 0) return [];
    const now = input.now ?? Date.now();
    const accountTag = quotaAccountTag(input.accountKey, quotaResetAccountSalt());
    const previous = swapLastObservedWindows(input.scope, accountTag, input.windows);
    if (!previous) return [];

    const detected = detectQuotaResets({
      scope: input.scope,
      accountTag,
      previous,
      next: input.windows,
      now,
    });

    const delivered: QuotaResetEvent[] = [];
    for (const event of detected) {
      // Claim BEFORE dispatch. A sink that fails must not cause a re-notification storm on
      // the next poll, and two observers racing one transition must produce one notification.
      if (!claimQuotaReset(event.key, now, event.resetAt)) continue;
      recordQuotaResetEvent(event);
      delivered.push(event);
      try {
        sink(event);
      } catch {
        // A sink is best-effort by contract; its failure is recorded by the sink layer.
      }
    }
    return delivered;
  } catch {
    return [];
  }
}
