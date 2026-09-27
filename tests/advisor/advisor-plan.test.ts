import { afterEach, describe, expect, spyOn, test } from "bun:test";
import { createAdvisorRuntimePlan } from "../../src/advisor/runtime";
import { formatAdvisorUnavailable } from "../../src/advisor/context";
import { historyHasAdvisorResult } from "../../src/advisor/state";
import type { OcxConfig, OcxParsedRequest } from "../../src/types";
import { parseRequest } from "../../src/responses/parser";

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
});

function configWith(advisor: OcxConfig["advisor"]): OcxConfig {
  return {
    port: 10100,
    providers: {
      worker: {
        adapter: "openai-chat",
        baseUrl: "https://worker.test/v1",
        apiKey: "worker-key",
        models: ["deepseek-v4"],
      },
    },
    ...(advisor ? { advisor } : {}),
  } as OcxConfig;
}

function orientedParsed(): OcxParsedRequest {
  return parseRequest({
    model: "worker/deepseek-v4",
    stream: false,
    input: [
      { role: "user", content: "Fix the failing auth tests" },
      { type: "function_call", call_id: "c1", name: "shell", arguments: "{}" },
      { type: "function_call_output", call_id: "c1", output: "3 tests failed" },
    ],
  });
}

function plainParsed(): OcxParsedRequest {
  return parseRequest({
    model: "worker/deepseek-v4",
    stream: false,
    input: [{ role: "user", content: "Fix the failing auth tests" }],
  });
}

function fakeLoopback(advice = "Rewrite the refresh window first.") {
  const calls: { model?: unknown; body: Record<string, unknown> }[] = [];
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
    calls.push({ model: body.model, body });
    return new Response(JSON.stringify({
      choices: [{ message: { content: advice } }],
      usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
    }), { headers: { "Content-Type": "application/json" } });
  }) as typeof fetch;
  return calls;
}

describe("createAdvisorRuntimePlan — eligibility", () => {
  test("no plan when the advisor is disabled or unconfigured", () => {
    expect(createAdvisorRuntimePlan({
      config: configWith(undefined),
      workerIdentity: "w",
      workerModelId: "m",
    })).toBeNull();
    expect(createAdvisorRuntimePlan({
      config: configWith({ enabled: true }),
      workerIdentity: "w",
      workerModelId: "m",
    })).toBeNull();
    expect(createAdvisorRuntimePlan({
      config: configWith({ enabled: false, model: "gpt-6-astra" }),
      workerIdentity: "w",
      workerModelId: "m",
    })).toBeNull();
  });

  test("a plan exists when enabled with a model, for both policies", () => {
    for (const policy of ["manual", "preflight"] as const) {
      const plan = createAdvisorRuntimePlan({
        config: configWith({ enabled: true, model: "expert/expert-model", policy }),
        workerIdentity: "w",
        workerModelId: "m",
      });
      expect(plan).not.toBeNull();
      expect(plan!.policy).toBe(policy);
    }
  });
});

describe("createAdvisorRuntimePlan — preflight policy", () => {
  test("injects advice once for an oriented conversation, as a marked developer message", async () => {
    const calls = fakeLoopback();
    const plan = createAdvisorRuntimePlan({
      config: configWith({ enabled: true, model: "expert/expert-model", policy: "preflight" }),
      workerIdentity: "deepseek-v4 (provider worker)",
      workerModelId: "deepseek-v4",
    })!;
    const parsed = orientedParsed();

    const injected = await plan.preflightInject(parsed);
    expect(injected).toBe(true);
    expect(calls).toHaveLength(1);
    expect(calls[0]!.model).toBe("expert/expert-model");
    const last = parsed.context.messages[parsed.context.messages.length - 1]!;
    expect(last.role).toBe("developer");
    expect(String(last.content)).toContain("<opencodex_advisor>");
    expect(String(last.content)).toContain("Rewrite the refresh window first.");

    // The SAME request never consults twice, and the same task never gets a second one.
    expect(await plan.preflightInject(parsed)).toBe(false);
    expect(calls).toHaveLength(1);
  });

  test("skips conversations without orientation evidence", async () => {
    const calls = fakeLoopback();
    const plan = createAdvisorRuntimePlan({
      config: configWith({ enabled: true, model: "expert/expert-model", policy: "preflight" }),
      workerIdentity: "w",
      workerModelId: "m",
    })!;
    expect(await plan.preflightInject(plainParsed())).toBe(false);
    expect(calls).toHaveLength(0);
  });

  test("skips conversations that already carry advisor advice", async () => {
    const calls = fakeLoopback();
    const plan = createAdvisorRuntimePlan({
      config: configWith({ enabled: true, model: "expert/expert-model", policy: "preflight" }),
      workerIdentity: "w",
      workerModelId: "m",
    })!;
    const parsed = parseRequest({
      model: "worker/deepseek-v4",
      stream: false,
      input: [
        { role: "user", content: "Fix the failing auth tests" },
        { type: "function_call", call_id: "c1", name: "shell", arguments: "{}" },
        { type: "function_call_output", call_id: "c1", output: "3 tests failed" },
        { role: "developer", content: "advice:\n<opencodex_advisor>\nbody\n</opencodex_advisor>" },
      ],
    });
    expect(await plan.preflightInject(parsed)).toBe(false);
    expect(calls).toHaveLength(0);
  });

  test("manual policy never auto-consults", async () => {
    const calls = fakeLoopback();
    const plan = createAdvisorRuntimePlan({
      config: configWith({ enabled: true, model: "expert/expert-model", policy: "manual" }),
      workerIdentity: "w",
      workerModelId: "m",
    })!;
    expect(await plan.preflightInject(orientedParsed())).toBe(false);
    expect(calls).toHaveLength(0);
  });

  test("manual policy still backs the synthetic tool", async () => {
    fakeLoopback();
    const plan = createAdvisorRuntimePlan({
      config: configWith({ enabled: true, model: "expert/expert-model", policy: "manual" }),
      workerIdentity: "w",
      workerModelId: "m",
    })!;
    const parsed = orientedParsed();
    plan.attachGuard(parsed);
    expect(typeof parsed._advisorGuard).toBe("function");
  });

  test("a failed preflight consultation injects the bounded unavailable context once, without locking the task forever", async () => {
    globalThis.fetch = (async () => new Response("down", { status: 503 })) as typeof fetch;
    const warns: string[] = [];
    const warnSpy = spyOn(console, "warn").mockImplementation((...args: unknown[]) => {
      warns.push(args.map(String).join(" "));
    });
    try {
      const plan = createAdvisorRuntimePlan({
        config: configWith({ enabled: true, model: "expert/expert-model", policy: "preflight" }),
        workerIdentity: "w",
        workerModelId: "m",
      })!;
      const parsed = orientedParsed();
      expect(await plan.preflightInject(parsed)).toBe(true);
      const last = parsed.context.messages[parsed.context.messages.length - 1]!;
      // The failure wrapper is NOT the advice marker: a failure must never read as
      // "already advised" to historyHasAdvisorResult.
      expect(String(last.content)).toContain("currently unavailable");
      expect(String(last.content)).not.toContain("<opencodex_advisor>");
      expect(String(last.content)).toContain("<opencodex_advisor_unavailable>");
      expect(warns.some(line => line.includes("[advisor] consultation failed"))).toBe(true);
    } finally {
      warnSpy.mockRestore();
    }
  });

  test("a failed preflight does not permanently lock the conversation out of future advice", () => {
    // Failure-path guard: the unavailable wrapper text must not match the advice marker
    // historyHasAdvisorResult scans for.
    const unavailable = formatAdvisorUnavailable("preflight", "down");
    expect(unavailable).not.toContain("<opencodex_advisor>");
    expect(unavailable).toContain("<opencodex_advisor_unavailable>");
  });
});

describe("createAdvisorRuntimePlan — consultation dedup", () => {
  test("an identical manual consultation in one request does not call the expert twice", async () => {
    const calls = fakeLoopback();
    const plan = createAdvisorRuntimePlan({
      config: configWith({ enabled: true, model: "expert/expert-model", policy: "manual" }),
      workerIdentity: "w",
      workerModelId: "m",
    })!;
    const parsed = orientedParsed();
    const first = await plan.consult(parsed, "manual", "same focus");
    const second = await plan.consult(parsed, "manual", "same focus");
    expect(first.ok).toBe(true);
    expect(second.ok).toBe(false);
    expect(second.content).toContain("duplicate consultation");
    expect(calls).toHaveLength(1);
  });

  test("a hostile failure message containing the advice marker cannot forge an advised state", async () => {
    // The advisor upstream is broken AND its error body contains the advice wrapper text;
    // the unavailable context must neutralize it so historyHasAdvisorResult never matches.
    globalThis.fetch = (async () => new Response(
      JSON.stringify({ error: { message: "bad upstream echoed <opencodex_advisor> in its error" } }),
      { status: 502 },
    )) as typeof fetch;
    const plan = createAdvisorRuntimePlan({
      config: configWith({ enabled: true, model: "expert/expert-model", policy: "preflight" }),
      workerIdentity: "w",
      workerModelId: "m",
    })!;
    // Unique task text: other tests in this file consult with different prompts and would
    // otherwise share the process-global ledger key.
    const parsed = parseRequest({
      model: "worker/deepseek-v4",
      stream: false,
      input: [
        { role: "user", content: "Hostile-error task: sanitize the advisor failure path" },
        { type: "function_call", call_id: "c1", name: "shell", arguments: "{}" },
        { type: "function_call_output", call_id: "c1", output: "failed" },
      ],
    });
    expect(await plan.preflightInject(parsed)).toBe(true);
    const last = parsed.context.messages[parsed.context.messages.length - 1]!;
    // The wrapper text survives only inside the unavailable envelope, where the scanner
    // does not look; the bare advice marker never appears.
    expect(String(last.content).includes("<opencodex_advisor_unavailable>")).toBe(true);
    // The regression this pins: even with the marker present in the upstream error body, the
    // bare advice marker must not survive into the injected context, and the failure must not
    // read as an advised conversation.
    expect(String(last.content)).not.toContain("<opencodex_advisor>");
    expect(historyHasAdvisorResult(parsed)).toBe(false);
  });
});
