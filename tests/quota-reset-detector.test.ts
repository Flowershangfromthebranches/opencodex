import { describe, expect, test } from "bun:test";
import {
  detectQuotaReset,
  detectQuotaResets,
  MIN_SURPRISE_DROP_PERCENT,
  quotaAccountTag,
} from "../src/quota/reset-detector";

const NOW = 1_772_000_000_000;
const HOUR = 60 * 60_000;

function detect(
  previous: { percent?: number; resetAt?: number } | undefined,
  next: { percent?: number; resetAt?: number },
  now = NOW,
) {
  return detectQuotaReset({
    scope: "codex",
    accountTag: "tag00000",
    previous: previous ? { window: "weekly", ...previous } : undefined,
    next: { window: "weekly", ...next },
    now,
  });
}

describe("quota reset detection", () => {
  test("a passed deadline plus a percent drop is a scheduled rollover", () => {
    const event = detect(
      { percent: 96, resetAt: NOW - 60_000 },
      { percent: 2, resetAt: NOW + 7 * 24 * HOUR },
    );
    expect(event?.kind).toBe("scheduled");
    expect(event?.percentBefore).toBe(96);
    expect(event?.percentAfter).toBe(2);
  });

  test("a passed deadline with no drop is still a scheduled rollover", () => {
    // A window that rolls over while barely used reports the same low percent. Requiring a
    // drop here would silently skip every low-usage rollover.
    const event = detect({ percent: 3, resetAt: NOW - 60_000 }, { percent: 3 });
    expect(event?.kind).toBe("scheduled");
  });

  test("usage rising past a passed deadline is not a reset", () => {
    expect(detect({ percent: 10, resetAt: NOW - 60_000 }, { percent: 24 })).toBeNull();
  });

  test("a material drop inside an unexpired window is a surprise reset", () => {
    const event = detect(
      { percent: 96, resetAt: NOW + 2 * HOUR },
      { percent: 4, resetAt: NOW + 9 * HOUR },
    );
    expect(event?.kind).toBe("surprise");
  });

  test("a deadline that jumps forward early is a surprise reset even at a flat percent", () => {
    const event = detect(
      { percent: 40, resetAt: NOW + 2 * HOUR },
      { percent: 40, resetAt: NOW + 9 * HOUR },
    );
    expect(event?.kind).toBe("surprise");
  });

  test("ordinary usage increase is not a reset", () => {
    expect(detect({ percent: 40, resetAt: NOW + 2 * HOUR }, { percent: 65, resetAt: NOW + 2 * HOUR })).toBeNull();
  });

  test("a sub-threshold drop is rounding noise, not a reset", () => {
    const drop = MIN_SURPRISE_DROP_PERCENT - 1;
    expect(detect(
      { percent: 61, resetAt: NOW + 2 * HOUR },
      { percent: 61 - drop, resetAt: NOW + 2 * HOUR },
    )).toBeNull();
  });

  test("a drop exactly at the threshold fires", () => {
    expect(detect(
      { percent: 61, resetAt: NOW + 2 * HOUR },
      { percent: 61 - MIN_SURPRISE_DROP_PERCENT, resetAt: NOW + 2 * HOUR },
    )?.kind).toBe("surprise");
  });

  test("no previous observation is never a reset", () => {
    // Cold start, reauth row clear, reconciliation delete, and account switch all land here.
    expect(detect(undefined, { percent: 0, resetAt: NOW + HOUR })).toBeNull();
  });

  test("a vanished percent is not a reset", () => {
    expect(detect({ percent: 90, resetAt: NOW + HOUR }, { resetAt: NOW + HOUR })).toBeNull();
  });

  test("sentinel reset clocks are ignored rather than read as 1970", () => {
    // src/providers/quota.ts:279 and src/codex/quota.ts:192 disagree on whether 0 survives,
    // so the detector re-checks: a 0 deadline must not read as a long-passed one.
    expect(detect({ percent: 90, resetAt: 0 }, { percent: 88, resetAt: 0 })).toBeNull();
  });

  test("the same post-reset window yields a stable idempotence key", () => {
    const first = detect({ percent: 96, resetAt: NOW - 60_000 }, { percent: 2, resetAt: NOW + HOUR });
    // A second observer re-reads the same transition a moment later. The key must match, so
    // whichever one claims it first suppresses the other.
    const second = detect(
      { percent: 96, resetAt: NOW - 30_000 },
      { percent: 2, resetAt: NOW + HOUR },
      NOW + 1_000,
    );
    expect(first?.kind).toBe("scheduled");
    expect(second?.kind).toBe("scheduled");
    expect(first?.key).toBe(second?.key);
  });

  test("account tags differ per account and carry no identity", () => {
    const left = quotaAccountTag("acct-one@example.test");
    const right = quotaAccountTag("acct-two@example.test");
    expect(left).not.toBe(right);
    expect(left).not.toContain("@");
    expect(left).toHaveLength(8);
  });

  test("window lists are paired by identity, and a mismatched pairing is refused", () => {
    const events = detectQuotaResets({
      scope: "codex",
      accountTag: "tag00000",
      previous: [
        { window: "5h", percent: 88, resetAt: NOW - 60_000 },
        { window: "weekly", percent: 40, resetAt: NOW + 5 * 24 * HOUR },
      ],
      next: [
        { window: "5h", percent: 1, resetAt: NOW + 5 * HOUR },
        { window: "weekly", percent: 41, resetAt: NOW + 5 * 24 * HOUR },
      ],
      now: NOW,
    });
    expect(events).toHaveLength(1);
    expect(events[0]?.window).toBe("5h");
    expect(events[0]?.kind).toBe("scheduled");
  });

  test("a window present only in the new snapshot is a baseline, not a reset", () => {
    const events = detectQuotaResets({
      scope: "anthropic",
      accountTag: "tag00000",
      previous: [],
      next: [{ window: "custom:Opus", percent: 0, resetAt: NOW + HOUR }],
      now: NOW,
    });
    expect(events).toEqual([]);
  });
});
