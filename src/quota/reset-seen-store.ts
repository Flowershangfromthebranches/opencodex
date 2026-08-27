/**
 * Durable "already notified" ledger plus a bounded ring of recent reset events.
 *
 * Exactly-once has to hold across a restart, because the whole point of a surprise reset is
 * that it happens while nobody is watching. An in-memory set would re-notify every reset
 * whose window is still open the next time the proxy starts.
 *
 * Deliberately NOT stored in config.json: this is high-frequency job state, and
 * mutatePersistedConfig fails closed when the config did not come from a file.
 */

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { atomicWriteFile, getConfigDir } from "../config";
import type { QuotaResetEvent } from "./reset-detector";

const STATE_FILENAME = "quota-reset-state.json";
const PERSIST_DEBOUNCE_MS = 250;

/**
 * Age floor for pruning a claimed key.
 *
 * 90 days, not 30: a monthly window's key is legitimately older than a month while still
 * current, and pruning it would let the same reset notify twice.
 */
const CLAIM_MAX_AGE_MS = 90 * 24 * 60 * 60_000;
const MAX_CLAIMS = 512;
const MAX_RING_EVENTS = 100;

type ClaimRecord = {
  /** When the claim was made. */
  at: number;
  /** The window deadline this claim belongs to; a future value means the claim is live. */
  resetAt?: number;
};

type StateFile = {
  version: 1;
  claims: Record<string, ClaimRecord>;
  events: QuotaResetEvent[];
};

const claims = new Map<string, ClaimRecord>();
let ring: QuotaResetEvent[] = [];
let hydrated = false;
let persistTimer: ReturnType<typeof setTimeout> | null = null;

function statePath(): string {
  return join(getConfigDir(), STATE_FILENAME);
}

function hydrate(): void {
  if (hydrated) return;
  hydrated = true;
  try {
    const path = statePath();
    if (!existsSync(path)) return;
    const parsed = JSON.parse(readFileSync(path, "utf8")) as StateFile;
    if (!parsed || parsed.version !== 1) return;
    if (parsed.claims && typeof parsed.claims === "object") {
      for (const [key, record] of Object.entries(parsed.claims)) {
        if (!record || typeof record.at !== "number") continue;
        claims.set(key, {
          at: record.at,
          ...(typeof record.resetAt === "number" ? { resetAt: record.resetAt } : {}),
        });
      }
    }
    if (Array.isArray(parsed.events)) ring = parsed.events.slice(-MAX_RING_EVENTS);
  } catch {
    // A corrupt or partially written cache must never break a quota refresh. Starting empty
    // risks one duplicate notification; throwing here would break the write that triggered us.
    claims.clear();
    ring = [];
  }
}

function schedulePersist(): void {
  if (persistTimer) clearTimeout(persistTimer);
  persistTimer = setTimeout(() => {
    persistTimer = null;
    try {
      const body: StateFile = {
        version: 1,
        claims: Object.fromEntries(claims),
        events: ring,
      };
      atomicWriteFile(statePath(), `${JSON.stringify(body)}\n`);
    } catch {
      // Best-effort persistence, matching the codex quota cache.
    }
  }, PERSIST_DEBOUNCE_MS);
  persistTimer.unref?.();
}

/** Drop claims that are both old and settled. A live deadline is never pruned. */
function prune(now: number): void {
  for (const [key, record] of claims) {
    if (record.resetAt !== undefined && record.resetAt > now) continue;
    if (now - record.at <= CLAIM_MAX_AGE_MS) continue;
    claims.delete(key);
  }
  if (claims.size <= MAX_CLAIMS) return;
  // Over budget: evict the oldest SETTLED claims only. Evicting a live one would trade a
  // memory bound for a duplicate notification, which is the bug this store prevents.
  const settled = [...claims.entries()]
    .filter(([, record]) => record.resetAt === undefined || record.resetAt <= now)
    .sort((left, right) => left[1].at - right[1].at);
  for (const [key] of settled) {
    if (claims.size <= MAX_CLAIMS) break;
    claims.delete(key);
  }
}

/**
 * Atomically claim a reset key. Returns true for the FIRST caller only.
 *
 * One synchronous check-and-set rather than a separate has/mark pair: a poller tick and a
 * live pooled response can observe the same transition, and two callers that both read
 * "unseen" would both notify. There is no await inside, so with Bun's single-threaded turn
 * semantics this is indivisible with respect to other observers.
 */
export function claimQuotaReset(key: string, at: number, resetAt?: number): boolean {
  hydrate();
  if (claims.has(key)) return false;
  claims.set(key, { at, ...(resetAt !== undefined ? { resetAt } : {}) });
  prune(at);
  schedulePersist();
  return true;
}

/** Read-only probe. Never used to gate a notification — see claimQuotaReset. */
export function hasSeenQuotaReset(key: string): boolean {
  hydrate();
  return claims.has(key);
}

export function recordQuotaResetEvent(event: QuotaResetEvent): void {
  hydrate();
  ring.push(event);
  if (ring.length > MAX_RING_EVENTS) ring = ring.slice(-MAX_RING_EVENTS);
  schedulePersist();
}

/** Recent events, newest first. */
export function listRecentQuotaResetEvents(limit = MAX_RING_EVENTS): QuotaResetEvent[] {
  hydrate();
  const bounded = Math.max(1, Math.min(limit, MAX_RING_EVENTS));
  return [...ring].reverse().slice(0, bounded);
}

/** Test-only: forget in-memory state so the next call re-reads OPENCODEX_HOME. */
export function resetQuotaResetStoreForTests(): void {
  claims.clear();
  ring = [];
  hydrated = false;
  if (persistTimer) {
    clearTimeout(persistTimer);
    persistTimer = null;
  }
}

/** Test-only: flush the debounced write immediately. */
export function flushQuotaResetStoreForTests(): void {
  if (!persistTimer) return;
  clearTimeout(persistTimer);
  persistTimer = null;
  try {
    const body: StateFile = { version: 1, claims: Object.fromEntries(claims), events: ring };
    atomicWriteFile(statePath(), `${JSON.stringify(body)}\n`);
  } catch {
    // Same best-effort contract as the debounced path.
  }
}
