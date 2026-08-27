import { beforeEach, describe, expect, test } from "bun:test";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { getConfigDir } from "../src/config";
import type { QuotaResetEvent } from "../src/quota/reset-detector";
import {
  claimQuotaReset,
  flushQuotaResetStoreForTests,
  hasSeenQuotaReset,
  listRecentQuotaResetEvents,
  recordQuotaResetEvent,
  resetQuotaResetStoreForTests,
} from "../src/quota/reset-seen-store";

const NOW = 1_772_000_000_000;
const DAY = 24 * 60 * 60_000;

function event(key: string): QuotaResetEvent {
  return {
    kind: "surprise",
    scope: "codex",
    accountTag: "tag00000",
    window: "weekly",
    detectedAt: NOW,
    key,
  };
}

beforeEach(() => {
  resetQuotaResetStoreForTests();
});

describe("quota reset claim store", () => {
  test("a key can be claimed exactly once", () => {
    expect(claimQuotaReset("k1", NOW)).toBe(true);
    expect(claimQuotaReset("k1", NOW)).toBe(false);
    expect(hasSeenQuotaReset("k1")).toBe(true);
    expect(hasSeenQuotaReset("k2")).toBe(false);
  });

  test("concurrent observers of one key produce exactly one winner", () => {
    // No await between the two calls: this is the poller-versus-live-response race.
    const results = [claimQuotaReset("race", NOW), claimQuotaReset("race", NOW)];
    expect(results.filter(Boolean)).toHaveLength(1);
  });

  test("a claim survives a process restart", () => {
    expect(claimQuotaReset("persisted", NOW, NOW + DAY)).toBe(true);
    flushQuotaResetStoreForTests();
    // Forget in-memory state; the next call must re-read the same OPENCODEX_HOME.
    resetQuotaResetStoreForTests();
    expect(hasSeenQuotaReset("persisted")).toBe(true);
    expect(claimQuotaReset("persisted", NOW + 60_000)).toBe(false);
  });

  test("a corrupt state file hydrates to empty without throwing", () => {
    writeFileSync(join(getConfigDir(), "quota-reset-state.json"), "{not json");
    resetQuotaResetStoreForTests();
    expect(() => hasSeenQuotaReset("anything")).not.toThrow();
    expect(hasSeenQuotaReset("anything")).toBe(false);
  });

  test("an old settled claim is pruned", () => {
    claimQuotaReset("stale", NOW - 100 * DAY, NOW - 99 * DAY);
    claimQuotaReset("fresh", NOW);
    expect(hasSeenQuotaReset("stale")).toBe(false);
    expect(hasSeenQuotaReset("fresh")).toBe(true);
  });

  test("an old claim whose window is still open is KEPT", () => {
    // A monthly key is legitimately older than the age floor while remaining current;
    // pruning it would let the same reset notify twice.
    claimQuotaReset("live-monthly", NOW - 100 * DAY, NOW + DAY);
    claimQuotaReset("fresh", NOW);
    expect(hasSeenQuotaReset("live-monthly")).toBe(true);
  });

  test("the event ring is bounded and newest-first", () => {
    for (let index = 0; index < 120; index += 1) recordQuotaResetEvent(event(`k${index}`));
    const recent = listRecentQuotaResetEvents();
    expect(recent).toHaveLength(100);
    expect(recent[0]?.key).toBe("k119");
    expect(listRecentQuotaResetEvents(5)).toHaveLength(5);
  });
});
