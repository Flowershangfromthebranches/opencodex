import { afterEach, expect, test } from "bun:test";
import { createAdvisorRuntimePlan } from "../../src/advisor/runtime";
import { createAdvisorPreflightLedger, ADVISOR_FAILURE_COOLDOWN_MS, advisorLedgerKey } from "../../src/advisor/state";
import { createTriggerEngine, triggerEngineFor } from "../../src/advisor/triggers/engine";
import { parseRequest } from "../../src/responses/parser";
import type { OcxConfig } from "../../src/types";
const originalFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = originalFetch; });
type Step = { name: string; args?: object; exit?: number; output?: string };
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
  let clock = 1000;
  let failing = false;
  const calls: Record<string, unknown>[] = [];
  globalThis.fetch = (async (_: unknown, init?: RequestInit) => {
    calls.push(JSON.parse(String(init?.body)));
    return failing ? new Response("unavailable", { status: 503 }) : new Response(JSON.stringify({ choices: [{ message: { content: "Run a discriminating experiment." } }] }), { headers: { "Content-Type": "application/json" } });
  }) as typeof fetch;
  const ledger = createAdvisorPreflightLedger();
  const plan = () => createAdvisorRuntimePlan({
    config: { advisor: { enabled: true, model: "expert/model", policy } } as OcxConfig,
    workerIdentity: "worker", workerModelId: "worker", ledger, now: () => clock, baseUrlOverride: "http://advisor.test",
  })!;
  return { calls, ledger, plan, advance: () => { clock += ADVISOR_FAILURE_COOLDOWN_MS + 1; }, fail: (value: boolean) => { failing = value; } };
}
test("worker never calls advisor: edit/fail/edit/fail consults configured expert and reinjects advice", async () => {
  const s = setup();
  expect(await s.plan().preflightInject(parsed([edit, fail]))).toBe(false);
  const next = parsed([edit, fail, edit, fail]);
  expect(await s.plan().preflightInject(next)).toBe(true);
  expect(s.calls).toHaveLength(1);
  expect(s.calls[0]?.model).toBe("expert/model");
  expect(JSON.stringify(s.calls[0])).toContain("repeated_validation_failure");
  expect(String(next.context.messages.at(-1)?.content)).toContain("Run a discriminating experiment.");
  expect(next.modelId).toBe("worker");
  // A full-history retry must never recount the same result IDs.
  expect(await s.plan().preflightInject(parsed([edit, fail, edit, fail]))).toBe(false);
  expect(s.calls).toHaveLength(1);
});
test("successful validation keeps normal edit/build/edit/build entirely quiet", async () => {
  const s = setup();
  expect(await s.plan().preflightInject(parsed([edit, pass]))).toBe(false);
  expect(await s.plan().preflightInject(parsed([edit, pass, edit, pass]))).toBe(false);
  expect(s.calls).toHaveLength(0);
});
test("manual advice resets signals; a new unresolved cycle can consult again", async () => {
  const s = setup();
  expect((await s.plan().consult(parsed([edit]), "manual", "review")).ok).toBe(true);
  expect(await s.plan().preflightInject(parsed([edit, fail]))).toBe(false);
  expect(await s.plan().preflightInject(parsed([edit, fail, edit, fail]))).toBe(true);
  expect(s.calls).toHaveLength(2);
});
test("provider failure is not advice and PR1 failure cooldown prevents retry storms", async () => {
  const s = setup(); s.fail(true);
  const request = parsed([fail, fail]);
  expect(await s.plan().preflightInject(request)).toBe(false);
  expect(String(request.context.messages.at(-1)?.content)).toContain("opencodex_advisor_unavailable");
  expect(await s.plan().preflightInject(parsed([fail, fail]))).toBe(false);
  expect(s.calls).toHaveLength(1);
  const state = triggerEngineFor(s.ledger).snapshot(advisorLedgerKey(request, "worker")!, 1000);
  expect(state.consultationCount).toBe(0);
  s.advance(); s.fail(false);
  expect(await s.plan().preflightInject(parsed([fail, fail]))).toBe(true);
  expect(s.calls).toHaveLength(2);
});
test("concurrent automatic requests share one PR1 claim", async () => {
  const s = setup();
  const outcomes = await Promise.all([s.plan().preflightInject(parsed([fail, fail])), s.plan().preflightInject(parsed([fail, fail]))]);
  expect(outcomes.filter(Boolean)).toHaveLength(1);
  expect(s.calls).toHaveLength(1);
});
test("manual and adaptive requests also share one in-flight claim", async () => {
  const s = setup();
  await Promise.all([s.plan().consult(parsed([fail, fail]), "manual", "review"), s.plan().preflightInject(parsed([fail, fail]))]);
  expect(s.calls).toHaveLength(1);
});
test("manual stays inactive and preflight retains its once-per-task behavior", async () => {
  const manual = setup("manual");
  expect(await manual.plan().preflightInject(parsed([fail, fail]))).toBe(false);
  expect(manual.calls).toHaveLength(0);
  const preflight = setup("preflight");
  expect(await preflight.plan().preflightInject(parsed([edit]))).toBe(true);
  expect(await preflight.plan().preflightInject(parsed([edit, fail, fail]))).toBe(false);
  expect(preflight.calls).toHaveLength(1);
});
test("different failed diagnostics never trigger by count", async () => {
  const s = setup();
  const a = { name: "shell", args: { cmd: "git diff src/a.ts" }, exit: 1 };
  const b = { name: "shell", args: { cmd: "git show HEAD:src/b.ts" }, exit: 0 };
  expect(await s.plan().preflightInject(parsed([a, b]))).toBe(false);
  expect(s.calls).toHaveLength(0);
});
test("identity-less traffic stays out of shared adaptive state", async () => {
  const s = setup(); const request = parsed([fail, fail]); delete request._codexOwnThreadId;
  expect(await s.plan().preflightInject(request)).toBe(false);
  expect(s.ledger.size()).toBe(0);
  expect(triggerEngineFor(s.ledger).size()).toBe(0);
});
test("state is bounded, expires, and contains no tool bodies", () => {
  const engine = createTriggerEngine();
  for (let i = 0; i < 600; i++) engine.observe(String(i), parsed([edit]), 0);
  expect(engine.size()).toBe(512);
  expect(JSON.stringify(engine.snapshot("599", 0))).not.toContain("private source");
  expect(engine.snapshot("599", 24 * 60 * 60 * 1000 + 1).mutationCount).toBe(0);
});
