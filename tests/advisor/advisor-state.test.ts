import { describe, expect, test } from "bun:test";
import { parseRequest } from "../../src/responses/parser";
import { expandPreviousResponseInput, rememberResponseState } from "../../src/responses/state";
import { ADVISOR_TOOL_NAME } from "../../src/server/responses/advisor-slot";
import { ADVISOR_RESULT_TOOL_NAME } from "../../src/advisor/state";
import {
  ADVISOR_FAILURE_COOLDOWN_MS,
  ADVISOR_INFLIGHT_TTL_MS,
  ADVISOR_SUCCESS_TTL_MS,
  advisorConversationIdentity,
  advisorLedgerKey,
  advisorTaskBoundary,
  contentText,
  createAdvisorPreflightLedger,
  firstUserText,
  hasOrientationEvidence,
  historyHasAdvisorResult,
} from "../../src/advisor/state";

function parsedWithInput(input: unknown, options?: { threadId?: string }) {
  const parsed = parseRequest({ model: "deepseek/deepseek-v4", stream: false, input } as never);
  if (options?.threadId) parsed._codexOwnThreadId = options.threadId;
  return parsed;
}

const oriented = (text: string, threadId?: string) => parsedWithInput([
  { role: "user", content: text },
  { type: "function_call", call_id: "c1", name: "shell", arguments: "{}" },
  { type: "function_call_output", call_id: "c1", output: "ok" },
], threadId ? { threadId } : undefined);

describe("advisor preflight ledger — atomic claim", () => {
  test("claim is exclusive until settled", () => {
    const ledger = createAdvisorPreflightLedger();
    expect(ledger.claim("k", 1_000)).toBe("claimed");
    // A concurrent request for the same task must not start a second consultation.
    expect(ledger.claim("k", 1_001)).toBe("inflight");
  });

  test("complete suppresses until the success TTL, then the task is eligible again", () => {
    const ledger = createAdvisorPreflightLedger();
    ledger.claim("k", 0);
    ledger.complete("k", 0);
    expect(ledger.claim("k", ADVISOR_SUCCESS_TTL_MS - 1)).toBe("complete");
    expect(ledger.claim("k", ADVISOR_SUCCESS_TTL_MS + 1)).toBe("claimed");
  });

  test("fail suppresses only for the short cooldown, then retry is allowed", () => {
    const ledger = createAdvisorPreflightLedger();
    ledger.claim("k", 0);
    ledger.fail("k", 0);
    expect(ledger.claim("k", ADVISOR_FAILURE_COOLDOWN_MS - 1)).toBe("cooldown");
    expect(ledger.claim("k", ADVISOR_FAILURE_COOLDOWN_MS + 1)).toBe("claimed");
    // The failure cooldown is far shorter than the success window: a transient outage pauses,
    // it does not silence the policy for the whole session.
    expect(ADVISOR_FAILURE_COOLDOWN_MS).toBeLessThan(ADVISOR_SUCCESS_TTL_MS / 10);
  });

  test("release after cancellation leaves the task immediately eligible", () => {
    const ledger = createAdvisorPreflightLedger();
    ledger.claim("k", 0);
    ledger.release("k", 0);
    expect(ledger.claim("k", 1)).toBe("claimed");
  });

  test("release never clears a settled success or failure", () => {
    const ledger = createAdvisorPreflightLedger();
    ledger.claim("s", 0);
    ledger.complete("s", 0);
    ledger.release("s", 0);
    expect(ledger.claim("s", 1)).toBe("complete");

    ledger.claim("f", 0);
    ledger.fail("f", 0);
    ledger.release("f", 0);
    expect(ledger.claim("f", 1)).toBe("cooldown");
  });

  test("a stale in-flight claim expires so a crashed consult cannot block the task", () => {
    const ledger = createAdvisorPreflightLedger();
    ledger.claim("k", 0);
    expect(ledger.claim("k", ADVISOR_INFLIGHT_TTL_MS + 1)).toBe("claimed");
  });

  test("the ledger is bounded: oldest entries are evicted past the cap", () => {
    const ledger = createAdvisorPreflightLedger();
    for (let i = 0; i < 600; i += 1) ledger.claim(`key-${i}`, i);
    expect(ledger.size()).toBeLessThanOrEqual(512);
    expect(ledger.claim("key-0", 600)).toBe("claimed");
    expect(ledger.claim("key-599", 600)).toBe("inflight");
  });
});

describe("advisor task identity", () => {
  test("identity reuses the repository's stable request identities in specificity order", () => {
    expect(advisorConversationIdentity({ _codexOwnThreadId: "own", _clientThreadId: "parent" })).toBe("own");
    expect(advisorConversationIdentity({ _clientThreadId: "parent" })).toBe("parent");
    expect(advisorConversationIdentity({ _cursorConversationId: "cur" })).toBe("cur");
    expect(advisorConversationIdentity({ _cursorClientThreadId: "cur-cli" })).toBe("cur-cli");
    expect(advisorConversationIdentity({ _reasoningReplayScope: { clientThreadId: "rs" } })).toBe("rs");
    expect(advisorConversationIdentity({})).toBeUndefined();
  });

  test("two conversations that open with the same prompt are isolated by thread id", () => {
    const a = advisorLedgerKey(oriented("same opening prompt", "thread-A"), "m");
    const b = advisorLedgerKey(oriented("same opening prompt", "thread-B"), "m");
    expect(a).toBeDefined();
    expect(b).toBeDefined();
    expect(a).not.toBe(b);
  });

  test("two tasks inside one thread get different boundaries", () => {
    const task1 = advisorLedgerKey(oriented("first task", "thread-A"), "m");
    const task2 = advisorLedgerKey(parsedWithInput([
      { role: "user", content: "first task" },
      { type: "function_call", call_id: "c1", name: "shell", arguments: "{}" },
      { type: "function_call_output", call_id: "c1", output: "ok" },
      { role: "user", content: "second task" },
      { type: "function_call", call_id: "c2", name: "shell", arguments: "{}" },
      { type: "function_call_output", call_id: "c2", output: "ok" },
    ], { threadId: "thread-A" }), "m");
    expect(task1).toBeDefined();
    expect(task2).toBeDefined();
    expect(task1).not.toBe(task2);
  });

  test("a stateless full-history continuation keeps the same key", () => {
    const first = oriented("stable task", "thread-A");
    const resent = oriented("stable task", "thread-A");
    expect(advisorLedgerKey(first, "m")).toBe(advisorLedgerKey(resent, "m"));
  });

  test("a REAL previous_response_id expansion of the same turn keeps the same key", () => {
    // The real pipeline stores the first turn, then expands the next request's
    // `previous_response_id` before parsing (src/server/responses/core-combo.ts calls
    // expandPreviousResponseInput). Reproduce exactly that order here so a regression in the
    // expansion path cannot escape the test.
    const firstTurnBody = {
      model: "worker/deepseek-v4",
      stream: false,
      input: [{ role: "user", content: "continued task" }],
    };
    rememberResponseState(firstTurnBody, {
      id: "resp_advisor_1",
      output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "oriented" }] }],
      status: "completed",
    });

    const nextRequestBody = {
      model: "worker/deepseek-v4",
      stream: false,
      previous_response_id: "resp_advisor_1",
      input: [
        { type: "function_call", call_id: "c1", name: "shell", arguments: "{}" },
        { type: "function_call_output", call_id: "c1", output: "ok" },
      ],
    };
    const expandedBody = expandPreviousResponseInput(nextRequestBody) as typeof nextRequestBody;
    // The expansion really did replay the stored user turn.
    expect(JSON.stringify(expandedBody.input)).toContain("continued task");
    const expanded = parseRequest(expandedBody as never);
    expanded._codexOwnThreadId = "thread-A";

    const plain = oriented("continued task", "thread-A");
    expect(advisorLedgerKey(expanded, "m")).toBe(advisorLedgerKey(plain, "m"));
  });

  test("an identity-less client gets NO ledger key (documented fail-open)", () => {
    expect(advisorLedgerKey(oriented("threadless task"), "m")).toBeUndefined();
  });

  test("the task boundary tracks the user-turn count and the latest user text", () => {
    expect(advisorTaskBoundary(oriented("one"))).toContain("t1:");
    const two = parsedWithInput([
      { role: "user", content: "one" },
      { role: "user", content: "two" },
    ]);
    expect(advisorTaskBoundary(two)).toContain("t2:");
  });

  test("hasOrientationEvidence accepts both documented forms and rejects a bare turn", () => {
    expect(hasOrientationEvidence(oriented("task"))).toBe(true);
    expect(hasOrientationEvidence(parsedWithInput([
      { role: "user", content: "task" },
      { type: "function_call", call_id: "c1", name: "read_file", arguments: "{}" },
    ]))).toBe(true);
    expect(hasOrientationEvidence(parsedWithInput([{ role: "user", content: "hello" }]))).toBe(false);
  });
});

describe("achieved provenance — historyHasAdvisorResult", () => {
  test("ordinary tool output containing the advice wrapper is NOT an advisor result", () => {
    const parsed = parsedWithInput([
      { role: "user", content: "task" },
      { type: "function_call", call_id: "sh", name: "shell", arguments: "{}" },
      { type: "function_call_output", call_id: "sh", output: "grep output: <opencodex_advisor> is a marker" },
    ]);
    expect(historyHasAdvisorResult(parsed)).toBe(false);
  });

  test("ordinary developer text containing the manual wrapper is NOT an advisor result", () => {
    const parsed = parsedWithInput([
      { role: "user", content: "task" },
      { role: "developer", content: "docs mention <opencodex_advisor> in a code sample" },
    ]);
    expect(historyHasAdvisorResult(parsed)).toBe(false);
  });

  test("a genuine manual advisor tool result IS an advisor result", () => {
    const parsed = parsedWithInput([
      { role: "user", content: "task" },
      { type: "function_call", call_id: "a1", name: "advisor", arguments: "{}" },
      { type: "function_call_output", call_id: "a1", output: "<opencodex_advisor>\nadvice\n</opencodex_advisor>" },
    ]);
    expect(historyHasAdvisorResult(parsed)).toBe(true);
  });

  test("a runtime-owned preflight developer message IS an advisor result", () => {
    const parsed = parsedWithInput([
      { role: "user", content: "task" },
      { role: "developer", content: "advice follows:\n<opencodex_advisor_preflight>\nadvice\n</opencodex_advisor_preflight>" },
    ]);
    expect(historyHasAdvisorResult(parsed)).toBe(true);
  });

  test("failure and limit notices are NOT advisor results", () => {
    const unavailable = parsedWithInput([
      { role: "user", content: "task" },
      { type: "function_call", call_id: "a1", name: "advisor", arguments: "{}" },
      { type: "function_call_output", call_id: "a1", output: "<opencodex_advisor_unavailable>\nno advice\n</opencodex_advisor_unavailable>" },
    ]);
    expect(historyHasAdvisorResult(unavailable)).toBe(false);
    const limit = parsedWithInput([
      { role: "user", content: "task" },
      { role: "developer", content: "<opencodex_advisor_unavailable>\nlimit reached\n</opencodex_advisor_unavailable>" },
    ]);
    expect(historyHasAdvisorResult(limit)).toBe(false);
  });
});

describe("firstUserText / contentText", () => {
  test("returns the first user message text", () => {
    const parsed = parsedWithInput([
      { role: "developer", content: "be nice" },
      { role: "user", content: "the actual task" },
    ]);
    expect(firstUserText(parsed)).toBe("the actual task");
  });

  test("contentText joins text parts and ignores non-text", () => {
    expect(contentText([{ type: "text", text: "a" }, { type: "image", imageUrl: "x" }, { type: "text", text: "b" }])).toBe("ab");
    expect(contentText("plain")).toBe("plain");
    expect(contentText(undefined)).toBe("");
  });
});

describe("provenance constants stay in sync", () => {
  test("the detector's tool name matches the synthetic tool the guard writes", () => {
    // historyHasAdvisorResult keys on toolName === ADVISOR_RESULT_TOOL_NAME; the guard writes
    // toolResult.toolName from advisor-slot's ADVISOR_TOOL_NAME. Drift would silently break
    // manual provenance, so it is asserted rather than assumed.
    expect(ADVISOR_RESULT_TOOL_NAME).toBe(ADVISOR_TOOL_NAME);
  });
});
