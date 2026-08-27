/**
 * Provider- and Codex-specific quota shapes mapped to the detector's neutral window list.
 *
 * Separate from the detector so the detector stays free of any dependency on either quota
 * subsystem's types, and separate from the observer so the seams can build observations
 * without loading the sink registry.
 */

import type { QuotaWindowObservation } from "./reset-detector";

type CodexLikeQuota = {
  shortPercent?: number;
  shortResetAt?: number;
  weeklyPercent?: number;
  weeklyResetAt?: number;
  monthlyPercent?: number;
  monthlyResetAt?: number;
  customWindows?: ReadonlyArray<{ label: string; percent: number; resetAt?: number }>;
};

type ProviderLikeQuota = {
  fiveHourPercent?: number;
  fiveHourResetAt?: number;
  weeklyPercent?: number;
  weeklyResetAt?: number;
  monthlyPercent?: number;
  monthlyResetAt?: number;
  customWindows?: ReadonlyArray<{ label: string; percent: number; resetAt?: number }>;
};

function window(
  label: string,
  percent: number | undefined,
  resetAt: number | undefined,
): QuotaWindowObservation | null {
  // A window with neither a percent nor a clock carries no information. Emitting it would
  // only create a baseline that can never produce a transition.
  if (percent === undefined && resetAt === undefined) return null;
  return {
    window: label,
    ...(percent !== undefined ? { percent } : {}),
    ...(resetAt !== undefined ? { resetAt } : {}),
  };
}

function customWindows(
  entries: ReadonlyArray<{ label: string; percent: number; resetAt?: number }> | undefined,
): QuotaWindowObservation[] {
  if (!entries) return [];
  const out: QuotaWindowObservation[] = [];
  for (const entry of entries) {
    if (typeof entry?.label !== "string") continue;
    const mapped = window(`custom:${entry.label}`, entry.percent, entry.resetAt);
    if (mapped) out.push(mapped);
  }
  return out;
}

/** Windows absent from the snapshot are omitted, never zero-filled: absence is not 0%. */
export function codexWindowObservations(quota: CodexLikeQuota): QuotaWindowObservation[] {
  const out: QuotaWindowObservation[] = [];
  for (const mapped of [
    window("5h", quota.shortPercent, quota.shortResetAt),
    window("weekly", quota.weeklyPercent, quota.weeklyResetAt),
    window("monthly", quota.monthlyPercent, quota.monthlyResetAt),
  ]) {
    if (mapped) out.push(mapped);
  }
  out.push(...customWindows(quota.customWindows));
  return out;
}

export function providerWindowObservations(quota: ProviderLikeQuota): QuotaWindowObservation[] {
  const out: QuotaWindowObservation[] = [];
  for (const mapped of [
    window("5h", quota.fiveHourPercent, quota.fiveHourResetAt),
    window("weekly", quota.weeklyPercent, quota.weeklyResetAt),
    window("monthly", quota.monthlyPercent, quota.monthlyResetAt),
  ]) {
    if (mapped) out.push(mapped);
  }
  out.push(...customWindows(quota.customWindows));
  return out;
}
