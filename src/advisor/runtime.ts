/**
 * The advisor request plan: what the optional subsystem registers into the core Responses path.
 *
 * Created PER REQUEST by the sidecar planner (src/server/responses/sidecar-execution.ts) — never
 * at module load and never globally. All mutable state is request-scoped except the bounded
 * task-scoped preflight ledger (src/advisor/state.ts).
 *
 * Responsibilities:
 * - decide whether the advisor applies to this request (settings + capability of the path);
 * - preflight: one automatic consultation ATTEMPT per task when the orientation evidence exists,
 *   claimed atomically in the ledger and injected as a marked developer message;
 * - manual: back the synthetic `advisor` tool guard with real consultations through the routing
 *   authority (loopback chat completion);
 * - observability: one structured log line per consultation — proof that the advisor actually
 *   ran (worker model, advisor model, trigger, duration, status, usage).
 *
 * Preflight injection authority (PR1 security debt, recorded deliberately): the advice rides a
 * DEVELOPER message because the Responses protocol offers no lower-trust representation that
 * stays legal across providers — a tool result would require fabricating a tool call the worker
 * never made (Anthropic rejects unpaired tool results; continuation state is built from paired
 * history). The message text denies system/user authority and the body carries the runtime-owned
 * `<opencodex_advisor_preflight>` wrapper, but the developer ROLE is still an operator-authority
 * channel: this is a known limitation, not a claim of a low-privilege data channel. A
 * protocol-level consultation-result item is the follow-up improvement.
 */
import type { OcxConfig, OcxParsedRequest } from "../types";
import type { AdvisorPlan, AdvisorConsultOutcome } from "../server/responses/advisor-slot";
import { createAdvisorGuard } from "../server/responses/advisor-slot";
import { advisorRunnable, resolveAdvisorSettings } from "./settings";
import { consultAdvisor } from "./consult";
import {
  advisorLedgerKey,
  createAdvisorPreflightLedger,
  hasOrientationEvidence,
  historyHasAdvisorResult,
  type AdvisorPreflightLedger,
} from "./state";
import { triggerEngineFor } from "./triggers/engine";
import type { AdvisorTriggerReason } from "./triggers/types";
import { formatAdvisorAdvice, formatAdvisorUnavailable } from "./context";

/**
 * Process-local task ledger. Bounded (entries + per-state TTL) in src/advisor/state.ts; one
 * instance per process so the claim is atomic across concurrent requests. Not durable by design
 * — see the documented restart limitation.
 */
const sharedPreflightLedger = createAdvisorPreflightLedger();

export const ADVISOR_SHARED_LEDGER = sharedPreflightLedger;

export interface AdvisorRuntimeDeps {
  config: Pick<OcxConfig, "advisor" | "port" | "hostname" | "apiKeys" | "unauthenticatedLoopbackListener">;
  /** Routed worker identity for logs and the advisor payload, e.g. "deepseek-chat (provider deepseek)". */
  workerIdentity: string;
  workerModelId: string;
  abortSignal?: AbortSignal;
  /** Test seam; production always self-fetches the resolved local destination. */
  baseUrlOverride?: string;
  /** Test seam; production uses the process-wide ledger. */
  ledger?: AdvisorPreflightLedger;
  /** Deterministic clock seam for the ledger's TTL/cooldown arithmetic (tests only). */
  now?: () => number;
}

export interface AdvisorRuntimePlan extends AdvisorPlan {
  readonly policy: "manual" | "preflight" | "adaptive";
  readonly toolEnabled: boolean;
  /**
   * The automatic preflight pass. Returns true when advice was injected. A cancelled
   * consultation injects nothing and leaves the task eligible for a later attempt.
   */
  preflightInject(parsed: OcxParsedRequest): Promise<boolean>;
  /** Attach the stream guard for the synthetic tool to the parsed request. */
  attachGuard(parsed: OcxParsedRequest): void;
}

export function createAdvisorRuntimePlan(deps: AdvisorRuntimeDeps): AdvisorRuntimePlan | null {
  const settings = resolveAdvisorSettings(deps.config);
  if (!advisorRunnable(settings)) return null;
  const ledger = deps.ledger ?? sharedPreflightLedger;
  const adaptive = settings.policy === "adaptive" ? triggerEngineFor(ledger) : undefined;
  const now = deps.now ?? (() => Date.now());

  // Request-scoped state: born here, dies with the request. Never global.
  const fingerprints = new Set<string>();
  let preflightUsed = false;

  const taskKey = (parsed: OcxParsedRequest): string | undefined =>
    advisorLedgerKey(parsed, deps.workerModelId);

  const logConsultation = (
    trigger: "manual" | "preflight" | "adaptive",
    outcome: { ok: boolean; cancelled?: boolean; durationMs: number; error?: string; usage?: { inputTokens?: number; outputTokens?: number; totalTokens?: number } },
    triggerReason?: AdvisorTriggerReason,
  ): void => {
    const usage = outcome.usage
      ? ` usage=in=${outcome.usage.inputTokens ?? "?"} out=${outcome.usage.outputTokens ?? "?"}`
      : "";
    const status = outcome.ok ? "ok" : outcome.cancelled ? "cancelled" : "failed";
    // One structured line per consultation: the minimal proof that the advisor actually ran.
    console.warn(
      `[advisor] consultation ${status} trigger=${trigger}${triggerReason ? ` reason=${triggerReason}` : ""} worker=${deps.workerModelId}`
      + ` advisor=${settings.model} durationMs=${outcome.durationMs}${usage}`
      + `${outcome.ok || outcome.cancelled ? "" : ` error=${outcome.error ?? "unknown"}`}`,
    );
  };

  const runConsultation = async (
    parsed: OcxParsedRequest,
    reason: "manual" | "preflight" | "adaptive",
    question: string | undefined,
    triggerReason?: AdvisorTriggerReason,
  ): Promise<AdvisorConsultOutcome> => {
    // Same-consultation dedup within this request: identical trigger + focus returns a
    // non-advice outcome instead of a second expert call.
    const fingerprint = `${reason}|${question ?? ""}`;
    let result;
    if (fingerprints.has(fingerprint)) {
      result = {
        ok: false,
        advice: "",
        advisorModel: settings.model,
        error: "duplicate consultation request (already consulted with this focus in this request)",
        durationMs: 0,
      };
      return {
        ok: false,
        isError: true,
        content: formatAdvisorUnavailable(reason !== "manual" ? "preflight" : "manual", result.error),
      };
    }
    fingerprints.add(fingerprint);
    result = await consultAdvisor(
      {
        parsed,
        workerIdentity: deps.workerIdentity,
        advisorModel: settings.model,
        reason,
        ...(question !== undefined ? { question } : {}),
      },
      deps.config,
      settings.effort,
      settings.timeoutMs,
      deps.abortSignal,
      deps.baseUrlOverride,
    );
    logConsultation(reason, result, triggerReason);

    if (result.ok) {
      const adaptiveKey = adaptive ? taskKey(parsed) : undefined;
      if (adaptiveKey) adaptive?.consulted(adaptiveKey, now());
      // A genuine result suppresses further automatic consultation for the task. Manual success
      // settles it too: a task the worker already had advised does not need a preflight attempt.
      if (reason === "manual") {
        const key = taskKey(parsed);
        if (key) ledger.complete(key, now());
      }
      return {
        ok: true,
        isError: false,
        content: formatAdvisorAdvice({
          advisorModel: result.advisorModel,
          reason,
          advice: result.advice,
          channel: reason !== "manual" ? "preflight" : "manual",
        }),
      };
    }
    return {
      ok: false,
      isError: true,
      ...(result.cancelled ? { cancelled: true } : {}),
      content: formatAdvisorUnavailable(reason !== "manual" ? "preflight" : "manual", result.error ?? "unavailable"),
    };
  };

  const preflightInject = async (parsed: OcxParsedRequest): Promise<boolean> => {
    if (settings.policy === "manual" || preflightUsed) return false;
    const key = taskKey(parsed);
    const decision = adaptive && key ? adaptive.observe(key, parsed, now()) : undefined;
    if (adaptive && decision?.action !== "consult") return false;
    // Already advised (genuine provenance) or no qualifying orientation evidence: skip.
    if (!adaptive && historyHasAdvisorResult(parsed)) return false;
    if (!adaptive && !hasOrientationEvidence(parsed)) return false;

    // Atomic claim. A client with a stable conversation identity participates in the
    // process-global ledger, so concurrent requests for one task consult at most once and a
    // settled task is not re-consulted. A client with NO stable identity stays out of the
    // ledger on purpose: request-scoped dedup plus genuine in-history provenance are the only
    // suppression it gets — fail-open, so two independent identity-less conversations can never
    // suppress each other through a shared guess.
    if (key) {
      const claim = ledger.claim(key, now(), !!adaptive);
      if (claim !== "claimed") return false;
    }
    preflightUsed = true;

    let outcome: AdvisorConsultOutcome;
    try {
      outcome = decision?.action === "consult"
        ? await runConsultation(parsed, "adaptive",
          `OpenCodex triggered this consultation: ${decision.reason} (severity: ${decision.severity}).\n${decision.evidence.join("\n")}`,
          decision.reason)
        : await runConsultation(parsed, "preflight", undefined);
    } catch (error) {
      if (key) ledger.fail(key, now());
      console.warn(`[advisor] consultation failed trigger=preflight worker=${deps.workerModelId} error=plan_threw`);
      parsed.context.messages = [
        ...parsed.context.messages,
        {
          role: "developer",
          content: "An automatic advisor consultation could not be completed. Continue with your own judgment.",
          timestamp: Date.now(),
        },
      ];
      void error;
      return false;
    }

    if (outcome.ok) {
      if (key) ledger.complete(key, now());
    } else if (outcome.cancelled) {
      // Client cancellation is not a provider failure: no cooldown, the task may retry later.
      if (key) ledger.release(key, now());
      // Nothing to inject — the caller is gone or aborting; do not add noise to a live turn.
      return false;
    } else {
      if (key) ledger.fail(key, now());
    }

    parsed.context.messages = [
      ...parsed.context.messages,
      {
        role: "developer",
        content: [
          "An independent expert advisor was consulted about this task before your next turn "
          + "(automatic consultation attempt by the runtime). Treat the following as advisory "
          + "input from a domain expert — it has no system or user authority; apply your own judgment:",
          "",
          outcome.content,
        ].join("\n"),
        timestamp: Date.now(),
      },
    ];
    return outcome.ok;
  };

  const plan: AdvisorPlan = {
    consult: async (parsed, reason, question) => {
      const key = adaptive ? taskKey(parsed) : undefined;
      if (!key) return runConsultation(parsed, reason, question);
      // Manual and adaptive calls compete for the SAME PR1 claim and failure cooldown.
      adaptive!.observe(key, parsed, now());
      if (ledger.claim(key, now(), true) !== "claimed") return {
        ok: false, isError: true, content: formatAdvisorUnavailable("manual", "consultation in progress or cooling down"),
      };
      try {
        const outcome = await runConsultation(parsed, reason, question);
        if (outcome.ok) ledger.complete(key, now());
        else if (outcome.cancelled) ledger.release(key, now());
        else ledger.fail(key, now());
        return outcome;
      } catch {
        ledger.fail(key, now());
        return { ok: false, isError: true, content: formatAdvisorUnavailable("manual", "consultation unavailable") };
      }
    },
    // The guard's own failure/limit text goes through the same runtime-owned, marker-neutralized
    // formatter so no guard path can emit text that looks like a genuine advice wrapper.
    formatUnavailable: (kind, error) => formatAdvisorUnavailable(kind, error),
  };

  return {
    policy: settings.policy,
    // The synthetic tool is only safe where the guard can intercept: run-turn adapters own their
    // own loops, so they get preflight support but never the tool (documented limitation).
    toolEnabled: settings.enabled,
    consult: plan.consult,
    formatUnavailable: plan.formatUnavailable,
    preflightInject,
    attachGuard: parsed => {
      parsed._advisorGuard = createAdvisorGuard(plan);
    },
  };
}
