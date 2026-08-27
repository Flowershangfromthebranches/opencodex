import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { clearAccountQuota, setAccountQuotaFromParsed } from "../src/codex/quota";
import type { QuotaResetEvent } from "../src/quota/reset-detector";
import {
  hasQuotaResetSink,
  observeQuotaSnapshot,
  setQuotaResetSink,
} from "../src/quota/reset-observer";
import { resetQuotaResetStoreForTests } from "../src/quota/reset-seen-store";
import {
  isQuotaResetPollerRunning,
  resetQuotaResetPollerForTests,
  startQuotaResetPoller,
  stopQuotaResetPoller,
} from "../src/quota/reset-poller";
import { resetQuotaResetNotifyCacheForTests } from "../src/quota/reset-notify-config";

const ACCOUNT = "acct_reset_observation";
const HOUR = 60 * 60_000;

let captured: QuotaResetEvent[] = [];

/** Let the seams' lazy import() chains settle. */
async function settle(): Promise<void> {
  for (let index = 0; index < 6; index += 1) await Promise.resolve();
  await new Promise(resolve => setTimeout(resolve, 5));
}

beforeEach(() => {
  captured = [];
  resetQuotaResetStoreForTests();
  resetQuotaResetNotifyCacheForTests();
  resetQuotaResetPollerForTests();
  clearAccountQuota();
  setQuotaResetSink(event => {
    captured.push(event);
  });
});

afterEach(() => {
  setQuotaResetSink(null);
  resetQuotaResetPollerForTests();
  clearAccountQuota();
});

describe("codex quota seam", () => {
  test("a weekly rollover through the real writer fires exactly one scheduled event", async () => {
    const expired = Date.now() - 60_000;
    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 96, weeklyResetAt: expired });
    await settle();
    expect(captured).toEqual([]); // first write is a baseline

    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 2, weeklyResetAt: Date.now() + 7 * 24 * HOUR });
    await settle();
    expect(captured).toHaveLength(1);
    expect(captured[0]?.kind).toBe("scheduled");
    expect(captured[0]?.window).toBe("weekly");
    expect(captured[0]?.scope).toBe("codex");

    // Observing the same post-reset state again must not notify twice.
    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 3, weeklyResetAt: Date.now() + 7 * 24 * HOUR });
    await settle();
    expect(captured).toHaveLength(1);
  });

  test("a surprise reset through the real writer fires a surprise event", async () => {
    const future = Date.now() + 2 * HOUR;
    setAccountQuotaFromParsed(ACCOUNT, { shortPercent: 96, shortResetAt: future, shortWindowSeconds: 18_000 });
    await settle();
    captured = [];

    setAccountQuotaFromParsed(ACCOUNT, { shortPercent: 4, shortResetAt: future, shortWindowSeconds: 18_000 });
    await settle();
    expect(captured).toHaveLength(1);
    expect(captured[0]?.kind).toBe("surprise");
    expect(captured[0]?.window).toBe("5h");
  });

  test("a credits-only write fires nothing despite a fresh updatedAt", async () => {
    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 40, weeklyResetAt: Date.now() + 3 * 24 * HOUR });
    await settle();
    captured = [];

    // src/codex/quota.ts:276 copies every window field verbatim here.
    setAccountQuotaFromParsed(ACCOUNT, { resetCredits: 3 });
    await settle();
    expect(captured).toEqual([]);
  });

  test("a cleared row followed by a fresh low percent fires nothing", async () => {
    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 91, weeklyResetAt: Date.now() + 3 * 24 * HOUR });
    await settle();
    captured = [];

    // Reauth and account purge both do this deliberately.
    clearAccountQuota(ACCOUNT);
    resetQuotaResetStoreForTests();
    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 0, weeklyResetAt: Date.now() + 7 * 24 * HOUR });
    await settle();
    expect(captured).toEqual([]);
  });

  test("no sink installed means no observation at all", async () => {
    setQuotaResetSink(null);
    expect(hasQuotaResetSink()).toBe(false);
    const expired = Date.now() - 60_000;
    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 96, weeklyResetAt: expired });
    await settle();
    setAccountQuotaFromParsed(ACCOUNT, { weeklyPercent: 1, weeklyResetAt: Date.now() + HOUR });
    await settle();
    expect(captured).toEqual([]);
  });
});

describe("observer contract", () => {
  test("the baseline comes from the persisted map, not the caller", () => {
    const expired = Date.now() - 60_000;
    expect(observeQuotaSnapshot({
      scope: "anthropic",
      accountKey: "anthropic\u0000acct-1",
      windows: [{ window: "weekly", percent: 88, resetAt: expired }],
    })).toEqual([]);

    const delivered = observeQuotaSnapshot({
      scope: "anthropic",
      accountKey: "anthropic\u0000acct-1",
      windows: [{ window: "weekly", percent: 2, resetAt: Date.now() + 7 * 24 * HOUR }],
    });
    expect(delivered).toHaveLength(1);
    expect(delivered[0]?.kind).toBe("scheduled");
  });

  test("two accounts of one provider do not inherit each other's history", () => {
    const expired = Date.now() - 60_000;
    observeQuotaSnapshot({
      scope: "anthropic",
      accountKey: "anthropic\u0000acct-A",
      windows: [{ window: "weekly", percent: 96, resetAt: expired }],
    });
    // A switch to a different account is an identity change, not a reset.
    const delivered = observeQuotaSnapshot({
      scope: "anthropic",
      accountKey: "anthropic\u0000acct-B",
      windows: [{ window: "weekly", percent: 1, resetAt: Date.now() + HOUR }],
    });
    expect(delivered).toEqual([]);
  });

  test("a throwing sink does not propagate to the caller", () => {
    setQuotaResetSink(() => {
      throw new Error("sink exploded");
    });
    const expired = Date.now() - 60_000;
    observeQuotaSnapshot({
      scope: "codex",
      accountKey: "throwing",
      windows: [{ window: "weekly", percent: 96, resetAt: expired }],
    });
    expect(() => observeQuotaSnapshot({
      scope: "codex",
      accountKey: "throwing",
      windows: [{ window: "weekly", percent: 1, resetAt: Date.now() + HOUR }],
    })).not.toThrow();
  });
});

describe("idle poller", () => {
  test("starting twice creates one timer and stop clears it", () => {
    expect(isQuotaResetPollerRunning()).toBe(false);
    startQuotaResetPoller(60_000);
    startQuotaResetPoller(60_000);
    expect(isQuotaResetPollerRunning()).toBe(true);
    stopQuotaResetPoller();
    expect(isQuotaResetPollerRunning()).toBe(false);
  });

  test("a disabled config makes a tick a no-op", async () => {
    const { runQuotaResetPollerTickForTests, quotaResetPollerTickCountForTests } = await import(
      "../src/quota/reset-poller"
    );
    await runQuotaResetPollerTickForTests();
    expect(quotaResetPollerTickCountForTests()).toBe(0);
  });
});
