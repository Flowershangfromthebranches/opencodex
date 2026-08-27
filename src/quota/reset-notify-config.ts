/**
 * Read-time resolution of the optional quotaResetNotify config section.
 *
 * Cached against the config generation. loadConfig is a readFileSync plus a full
 * configSchema.safeParse with no memoization, and the enable check runs once per pooled
 * response, so calling it there directly would put a config parse on the hot path for a
 * feature nobody enabled. Keying the cache on captureConfigGeneration keeps a config edit
 * effective without a restart.
 */

import { loadConfig } from "../config";
import { captureConfigGeneration } from "../lib/state-store-sweeper";
import type { QuotaResetKind } from "./reset-detector";

export type ResolvedQuotaResetNotify = {
  readonly enabled: boolean;
  readonly kinds: ReadonlySet<QuotaResetKind>;
  /** 0 means passive-only: observe live refreshes, never poll. */
  readonly pollMs: number;
  readonly webhookUrl?: string;
  readonly allowPrivateNetwork: boolean;
  readonly timeoutMs: number;
  readonly command?: readonly string[];
};

const ALL_KINDS: ReadonlySet<QuotaResetKind> = new Set(["scheduled", "surprise"]);
const DEFAULT_POLL_SECONDS = 900;
const MIN_POLL_SECONDS = 60;
const DEFAULT_TIMEOUT_MS = 5_000;
const MAX_TIMEOUT_MS = 30_000;

const DISABLED: ResolvedQuotaResetNotify = Object.freeze({
  enabled: false,
  kinds: ALL_KINDS,
  pollMs: 0,
  allowPrivateNetwork: false,
  timeoutMs: DEFAULT_TIMEOUT_MS,
});

type RawNotify = {
  enabled?: unknown;
  kinds?: unknown;
  pollSeconds?: unknown;
  webhookUrl?: unknown;
  allowPrivateNetwork?: unknown;
  timeoutMs?: unknown;
  command?: unknown;
};

function positiveInt(value: unknown, fallback: number, min: number, max: number): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, Math.floor(value)));
}

function nonBlankStrings(value: unknown): readonly string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const out = value.filter((item): item is string => typeof item === "string" && item.trim() !== "");
  return out.length > 0 ? out : undefined;
}

export function resolveQuotaResetNotify(raw: unknown): ResolvedQuotaResetNotify {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return DISABLED;
  const value = raw as RawNotify;

  const webhookUrl = typeof value.webhookUrl === "string" && value.webhookUrl.trim() !== ""
    ? value.webhookUrl.trim()
    : undefined;
  const command = nonBlankStrings(value.command);

  // An "enabled" subsystem with nowhere to send is a misconfiguration. Reporting it as off
  // keeps the default-OFF guarantee honest: no sink means no timer and no observation.
  const enabled = value.enabled === true && (webhookUrl !== undefined || command !== undefined);
  if (!enabled) return DISABLED;

  const requestedKinds = Array.isArray(value.kinds)
    ? value.kinds.filter((kind): kind is QuotaResetKind => kind === "scheduled" || kind === "surprise")
    : [];
  const kinds: ReadonlySet<QuotaResetKind> = requestedKinds.length > 0
    ? new Set(requestedKinds)
    : ALL_KINDS;

  const pollSeconds = value.pollSeconds === 0
    ? 0
    : positiveInt(value.pollSeconds, DEFAULT_POLL_SECONDS, MIN_POLL_SECONDS, 24 * 60 * 60);

  return Object.freeze({
    enabled: true,
    kinds,
    pollMs: pollSeconds * 1_000,
    ...(webhookUrl !== undefined ? { webhookUrl } : {}),
    allowPrivateNetwork: value.allowPrivateNetwork === true,
    timeoutMs: positiveInt(value.timeoutMs, DEFAULT_TIMEOUT_MS, 100, MAX_TIMEOUT_MS),
    ...(command !== undefined ? { command } : {}),
  });
}

let cached: { generation: number; resolved: ResolvedQuotaResetNotify } | null = null;

export function currentQuotaResetNotify(): ResolvedQuotaResetNotify {
  const generation = captureConfigGeneration();
  if (cached && cached.generation === generation) return cached.resolved;
  let resolved = DISABLED;
  try {
    resolved = resolveQuotaResetNotify(
      (loadConfig() as { quotaResetNotify?: unknown }).quotaResetNotify,
    );
  } catch {
    // An unreadable config must not enable a notifier, and must not throw on a quota write.
  }
  cached = { generation, resolved };
  return resolved;
}

export function isQuotaResetNotificationEnabled(): boolean {
  return currentQuotaResetNotify().enabled;
}

export function resolveQuotaResetPollMs(): number {
  return currentQuotaResetNotify().pollMs;
}

export function resetQuotaResetNotifyCacheForTests(): void {
  cached = null;
}
