import { afterEach, expect, test } from "bun:test";
import { createAdvisorRuntimePlan } from "../../src/advisor/runtime";
import { createAdvisorPreflightLedger, ADVISOR_FAILURE_COOLDOWN_MS, advisorLedgerKey } from "../../src/advisor/state";
import { createTriggerEngine, triggerEngineFor } from "../../src/advisor/triggers/engine";
import { parseRequest } from "../../src/responses/parser";
import type { OcxConfig } from "../../src/types";
const originalFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = originalFetch; });
type Step = { name: string; args?: object; exit?: number; output?: string };
const read: Step = { name: "read_file", args: { path: "a.ts" }, output: "orientation" };
const edit: Step = { name: "write_file", args: { path: "src/a.ts", content: "private source" }, output: "written" };
const fail: Step = { name: "shell", args: { cmd: "bun test" }, exit: 1 };
const pass: Step = { ...fail, exit: 0 };
function parsed(steps: Step[], thread = "adaptive-test") {
  const input: unknown[] = [{ role: "user", content: "Fix the bug" }];
  steps.forEach((s, i) => input.push(
    { type: "function_call", call_id: `c${i}`, name: s.name, arguments: JSON.stringify(s.args ?? {}) },
    { type: "function_call_output", call_id: `c${i}`, output: s.output ?? JSON.stringify({ exit_code: s.exit }) },
  ));
  const request = parseRequest({ model: "worker", stream: false, input });
  request._codexOwnThreadId = thread;
  return request;
}
function setup(policy: "adaptive" | "preflight" | "manual" = "adaptive") {
  let clock = 1000, failing = false;
  const calls: Record<string, unknown>[] = [];
  globalThis.fetch = (async (_: unknown, init?: RequestInit) => {
    calls.push(JSON.parse(String(init?.body)));
    return failing ? new Response("unavailable", { status: 503 }) : Response.json({ choices: [{ message: { content: "Run a discriminating experiment." } }] });
  }) as typeof fetch;
  const ledger = createAdvisorPreflightLedger();
  const plan = (abortSignal?: AbortSignal) => createAdvisorRuntimePlan({
    config: { advisor: { enabled: true, model: "expert/model", policy } } as OcxConfig,
    workerIdentity: "worker", workerModelId: "worker", ledger, now: () => clock, baseUrlOverride: "http://advisor.test", abortSignal,
  })!;
  return { calls, ledger, plan, baseline: () => plan().preflightInject(parsed([read])), advance: () => { clock += ADVISOR_FAILURE_COOLDOWN_MS + 1; }, fail: (value: boolean) => { failing = value; } };
}
test("adaptive includes exactly one preflight baseline then escalates without worker advisor calls", async () => {
  const s = setup();
  expect(await s.plan().preflightInject(parsed([]))).toBe(false);
  expect(await s.baseline()).toBe(true);
  expect(await s.plan().preflightInject(parsed([read, edit, fail]))).toBe(false);
  const next = parsed([read, edit, fail, edit, fail]);
  expect(await s.plan().preflightInject(next)).toBe(true);
  expect(s.calls).toHaveLength(2);
  expect(s.calls[1]?.model).toBe("expert/model");
  expect(JSON.stringify(s.calls[1])).toContain("repair_failed");
  expect(String(next.context.messages.at(-1)?.content)).toContain("Run a discriminating experiment.");
  expect(next.modelId).toBe("worker");
  expect(await s.plan().preflightInject(parsed([read, edit, fail, edit, fail]))).toBe(false);
  expect(s.calls).toHaveLength(2);
});
test("normal coding and different diagnostics cause zero extra escalations", async () => {
  for (const steps of [[edit, pass, edit, pass, edit, pass], [fail, edit, pass],
    ["git diff a", "git diff b", "git show HEAD:c"].map(cmd => ({ name: "shell", args: { cmd }, exit: 1 }))]) {
    const s = setup(); await s.baseline();
    expect(await s.plan().preflightInject(parsed([read, ...steps]))).toBe(false);
    expect(s.calls).toHaveLength(1);
  }
});
test("manual success resets observations without permanently disabling adaptive", async () => {
  const s = setup();
  expect((await s.plan().consult(parsed([read]), "manual", "review")).ok).toBe(true);
  expect(await s.plan().preflightInject(parsed([read, fail]))).toBe(false);
  expect(await s.plan().preflightInject(parsed([read, fail, edit, fail]))).toBe(true);
  expect(s.calls).toHaveLength(2);
});
test("provider 503 uses PR1 cooldown and retry retains adaptive channel", async () => {
  const s = setup(); await s.baseline(); s.fail(true);
  const steps = [read, edit, fail, edit, fail];
  const request = parsed(steps);
  expect(await s.plan().preflightInject(request)).toBe(false);
  expect(String(request.context.messages.at(-1)?.content)).toContain("opencodex_advisor_unavailable");
  expect(await s.plan().preflightInject(parsed(steps))).toBe(false);
  expect(s.calls).toHaveLength(2);
  s.advance(); s.fail(false);
  expect(await s.plan().preflightInject(parsed(steps))).toBe(true);
  expect(JSON.stringify(s.calls[2])).toContain("repair_failed");
});
test("concurrent adaptive and manual requests use one PR1 in-flight claim", async () => {
  for (const manual of [false, true]) {
    const s = setup(); await s.baseline();
    const steps = [read, edit, fail, edit, fail];
    await Promise.all([s.plan().preflightInject(parsed(steps)), manual
      ? s.plan().consult(parsed(steps), "manual", "review") : s.plan().preflightInject(parsed(steps))]);
    expect(s.calls).toHaveLength(2);
  }
});
test("manual and preflight policies retain their previous behavior", async () => {
  const manual = setup("manual");
  expect(await manual.plan().preflightInject(parsed([fail, fail]))).toBe(false);
  expect(manual.calls).toHaveLength(0);
  const preflight = setup("preflight");
  expect(await preflight.baseline()).toBe(true);
  expect(await preflight.plan().preflightInject(parsed([read, edit, fail, edit, fail]))).toBe(false);
  expect(preflight.calls).toHaveLength(1);
});
test("identity-less clients retain baseline but never accumulate adaptive observations", async () => {
  const s = setup(); const request = parsed([fail, fail]); delete request._codexOwnThreadId;
  expect(await s.plan().preflightInject(request)).toBe(true);
  expect(s.ledger.size()).toBe(0);
  expect(triggerEngineFor(s.ledger).size()).toBe(0);
});
test("observation state saturates without eviction, expires and stores no content", () => {
  const engine = createTriggerEngine();
  for (let i = 0; i < 512; i++) engine.observe(String(i), parsed([edit]), 0);
  expect(engine.observe("overflow", parsed([fail, fail]), 0).action).toBe("continue");
  expect(engine.snapshot("overflow", 0)).toBeUndefined();
  expect(engine.size()).toBe(512);
  expect(JSON.stringify(engine.snapshot("0", 0))).not.toContain("private source");
  expect(engine.snapshot("0", 24 * 60 * 60 * 1000 + 1)?.phase).toBe("normal");
});
test("stable task key isolates new user tasks and full history replay deduplicates", () => {
  const engine = createTriggerEngine(); const request = parsed([fail]);
  const key = advisorLedgerKey(request, "worker")!;
  engine.observe(key, request, 0); engine.observe(key, request, 1);
  expect(engine.snapshot(key, 1)?.phase).toBe("failure_observed");
  request.context.messages.push({ role: "user", content: "new task", timestamp: 0 });
  const nextKey = advisorLedgerKey(request, "worker")!;
  expect(nextKey).not.toBe(key);
  expect(engine.observe(nextKey, request, 1).action).toBe("continue");
});

test("edits without a prior failure do not escalate after the baseline", async () => {
  const s = setup(); await s.baseline();
  const writes = [1, 2, 3, 4].map(i => ({ name: "write_file", args: { path: `src/f${i}.ts`, content: "x" }, output: "written" }));
  expect(await s.plan().preflightInject(parsed([read, ...writes]))).toBe(false);
  expect(s.calls).toHaveLength(1);
});
test("baseline does not block the first repair failure", async () => {
  const s = setup(); await s.baseline();
  expect(await s.plan().preflightInject(parsed([read, fail, edit]))).toBe(false);
  expect(await s.plan().preflightInject(parsed([read, fail, edit, fail]))).toBe(true);
  expect(JSON.stringify(s.calls[1])).toContain("repair_failed");
  expect(JSON.stringify(s.calls[1])).toContain("same_validation=true");
  expect(await s.plan().preflightInject(parsed([read, fail, edit, fail, edit, fail]))).toBe(false);
});
function promptOf(call: Record<string, unknown> | undefined): string {
  return JSON.stringify(call);
}
test("repair evidence before any baseline success is one adaptive consultation", async () => {
  const s = setup();
  const logs: string[] = [];
  const warn = console.warn;
  console.warn = (...args: unknown[]) => { logs.push(args.map(String).join(" ")); warn(...args); };
  try {
    const request = parsed([read, fail, edit, fail]);
    expect(await s.plan().preflightInject(request)).toBe(true);
  } finally {
    console.warn = warn;
  }
  expect(s.calls).toHaveLength(1);
  const prompt = promptOf(s.calls[0]);
  expect(prompt).toContain("observable non-convergence");
  expect(prompt).toContain("repair_failed");
  expect(prompt).not.toContain("before the worker's first substantive turn");
  expect(logs.some(line => line.includes("trigger=adaptive") && line.includes("reason=repair_failed"))).toBe(true);
  expect(logs.some(line => line.includes("trigger=preflight"))).toBe(false);
});
test("orientation without a repair failure stays a preflight consultation", async () => {
  const s = setup();
  const logs: string[] = [];
  const warn = console.warn;
  console.warn = (...args: unknown[]) => { logs.push(args.map(String).join(" ")); warn(...args); };
  try {
    expect(await s.baseline()).toBe(true);
  } finally {
    console.warn = warn;
  }
  expect(s.calls).toHaveLength(1);
  expect(promptOf(s.calls[0])).toContain("before the worker's first substantive turn");
  expect(promptOf(s.calls[0])).not.toContain("repair_failed");
  expect(logs.some(line => line.includes("trigger=preflight"))).toBe(true);
});
test("a failed preflight retries the next eligible repair as adaptive", async () => {
  const s = setup();
  s.fail(true);
  expect(await s.baseline()).toBe(false);
  expect(s.calls).toHaveLength(1);
  expect(promptOf(s.calls[0])).toContain("before the worker's first substantive turn");
  s.advance();
  s.fail(false);
  expect(await s.plan().preflightInject(parsed([read, fail, edit, fail]))).toBe(true);
  expect(s.calls).toHaveLength(2);
  expect(promptOf(s.calls[1])).toContain("repair_failed");
  expect(promptOf(s.calls[1])).toContain("observable non-convergence");
  expect(promptOf(s.calls[1])).not.toContain("before the worker's first substantive turn");
});
test("a different follow-up validation does not escalate", async () => {
  const s = setup(); await s.baseline();
  const build = { name: "shell", args: { cmd: "npm run build" }, exit: 1 };
  expect(await s.plan().preflightInject(parsed([read, fail, edit, build]))).toBe(false);
  expect(s.calls).toHaveLength(1);
});
test("adaptive cancellation releases the original claim and retries without failure cooldown", async () => {
  const s = setup(); await s.baseline();
  const steps = [read, edit, fail, edit, fail];
  const controller = new AbortController(); controller.abort();
  const request = parsed(steps); const count = request.context.messages.length;
  expect(await s.plan(controller.signal).preflightInject(request)).toBe(false);
  expect(request.context.messages).toHaveLength(count);
  expect(await s.plan().preflightInject(parsed(steps))).toBe(true);
  expect(s.calls).toHaveLength(2);
});
test("dedup saturation disables escalation instead of recounting old results", () => {
  const engine = createTriggerEngine();
  for (let i = 0; i < 2048; i++) {
    const request = parsed([read]);
    for (const message of request.context.messages) {
      if (message.role === "assistant") for (const part of message.content) if (part.type === "toolCall") part.id = `id-${i}`;
      if (message.role === "toolResult") message.toolCallId = `id-${i}`;
    }
    engine.observe("same", request, 0);
  }
  expect(engine.observe("same", parsed([fail, fail]), 0).action).toBe("continue");
});
