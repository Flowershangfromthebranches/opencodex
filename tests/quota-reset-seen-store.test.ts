import { beforeEach, describe, expect, test } from "bun:test";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { getConfigDir } from "../src/config";
import type { QuotaResetEvent } from "../src/quota/reset-detector";
import {
  claimCountForTests,
  claimQuotaReset,
  hasSeenQuotaReset,
  listRecentQuotaResetEvents,
  recordQuotaResetEvent,
  resetQuotaResetStoreForTests,
} from "../src/quota/reset-seen-store";

const DAY = 24 * 60 * 60_000;
/**
 * Real wall clock, not a fixed constant.
 *
 * prune() reads Date.now() for age comparisons on purpose (a backdated claim must not change
 * unrelated keys' retention), so a hardcoded epoch would look decades stale and be pruned the
 * moment it was written.
 */
const NOW = Date.now();

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

  test("a claim is on disk the moment it is made, with no flush", () => {
    // No test-only flush: the claim path writes synchronously, because an unref'd 250 ms
    // debounce loses the claim when the process exits right after detecting — the exact case a
    // restart guarantee has to cover.
    expect(claimQuotaReset("persisted", NOW, NOW + DAY)).toBe(true);
    const raw = readFileSync(join(getConfigDir(), "quota-reset-state.json"), "utf8");
    expect(JSON.parse(raw).claims.persisted).toBeDefined();

    resetQuotaResetStoreForTests();
    expect(hasSeenQuotaReset("persisted")).toBe(true);
    expect(claimQuotaReset("persisted", NOW + 60_000)).toBe(false);
  });

  test("a claim survives a real second process", async () => {
    const script = join(getConfigDir(), "claim-probe.ts");
    const storeUrl = new URL("../src/quota/reset-seen-store.ts", import.meta.url).pathname;
    writeFileSync(script, [
      `const store = await import(${JSON.stringify(storeUrl)});`,
      `console.log(String(store.claimQuotaReset("cross-process", Date.now(), Date.now() + 86400000)));`,
    ].join("\n"));

    const run = async (): Promise<string> => {
      const proc = Bun.spawn(["bun", script], {
        env: { ...process.env, OPENCODEX_HOME: getConfigDir() },
        stdout: "pipe",
        stderr: "pipe",
      });
      const out = await new Response(proc.stdout).text();
      await proc.exited;
      return out.trim();
    };

    expect(await run()).toBe("true");
    // A second OS process must see the first one's claim. A debounced write failed this
    // silently: the timer is unref'd, so the first process exited before persisting.
    expect(await run()).toBe("false");
  });

  test("the hard ceiling bounds the map even when every claim is live", () => {
    const future = NOW + 365 * DAY;
    for (let index = 0; index < 2_000; index += 1) {
      claimQuotaReset(`live-${index}`, NOW, future + index);
    }
    // 512 is the soft budget, honoured by evicting settled claims. With none settled, the hard
    // ceiling at 1024 is what stops unbounded growth of the map and the JSON beside it.
    expect(claimCountForTests()).toBeLessThanOrEqual(1_024);
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
