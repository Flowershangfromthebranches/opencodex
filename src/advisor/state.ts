/**
 * Conversation- and task-scoped advisor state.
 *
 * Three scopes, deliberately separate:
 *
 * 1. REQUEST-scoped state lives in the per-request plan closure (see runtime.ts) — consultation
 *    count, dedup fingerprints, the preflight flag. Born and dies with one request.
 *
 * 2. TASK-scoped preflight ledger (this file): a bounded, process-local claim table that makes
 *    "at most one automatic consultation per task" atomic across concurrent requests. A claim
 *    requires a STABLE conversation identity plus the current task boundary; a client that sends
 *    no identity never enters this ledger (see `advisorLedgerKey`) and therefore fails open —
 *    it may be consulted once per request rather than risk two independent tasks suppressing
 *    each other through a shared guess.
 *
 * 3. PROVENANCE: "this task was already advised" is never inferred from a bare string. Manual
 *    advice counts only as a `toolResult` whose `toolName` is the synthetic advisor tool, and
 *    preflight advice only through the runtime-owned `<opencodex_advisor_preflight>` wrapper.
 *    Ordinary tool output, developer text, user text, and failure notices cannot forge it.
 *
 * Ledger entries are plain state records — no message bodies, no credentials. Every state is
 * bounded by entry count and its own TTL.
 */

/** Long-lived success suppression: the task lifetime approximation (also the ledger TTL cap). */
export const ADVISOR_SUCCESS_TTL_MS = 24 * 60 * 60 * 1000;
/**
 * Failure cooldown. One minute is the repository's standing minute-scale unit (tray polling,
 * subagent availability polling); it turns a transient 503 into a short pause instead of
 * silencing the policy for the rest of the coding session.
 */
export const ADVISOR_FAILURE_COOLDOWN_MS = 60 * 1000;
/**
 * In-flight claim expiry. Longer than the largest configurable consultation timeout (600s
 * upper bound on `advisor.timeoutMs`, default 120s) so a slow-but-alive consultation is never
 * mistaken for a wedged one, while a crashed claim cannot block a task forever.
 */
export const ADVISOR_INFLIGHT_TTL_MS = 10 * 60 * 1000;

const MAX_ENTRIES = 512;

export type AdvisorClaimState =
  /** The caller now owns the consultation for this task. */
  | "claimed"
  /** Another request for the same task is consulting right now. */
  | "inflight"
  /** This task already received advice; suppression holds until the success TTL expires. */
  | "complete"
  /** A recent consultation failed; suppression holds for the short failure cooldown. */
  | "cooldown";

interface LedgerEntry {
  state: "inflight" | "success" | "failed";
  at: number;
}

export interface AdvisorPreflightLedger {
  /**
   * Atomically try to own the consultation for a task key. Returns `claimed` exactly once per
   * task until `complete`/`fail`/`release` settles it, so two concurrent requests cannot both
   * consult.
   */
  claim(key: string, now?: number, repeatAfterSuccess?: boolean): AdvisorClaimState;
  /** The consultation succeeded: suppress further automatic consultations until the TTL. */
  complete(key: string, now?: number): void;
  /** The consultation failed: short cooldown, then the task may retry. */
  fail(key: string, now?: number): void;
  /** The consultation was cancelled (client abort): no cooldown, the task may retry at once. */
  release(key: string, now?: number): void;
  /** Test/observability seam: current entry count. */
  size(): number;
}

export function createAdvisorPreflightLedger(): AdvisorPreflightLedger {
  const entries = new Map<string, LedgerEntry>();

  const evict = (): void => {
    while (entries.size > MAX_ENTRIES) {
      // Map iteration is insertion-ordered; the oldest entry goes first.
      const oldest = entries.keys().next();
      if (oldest.done) break;
      entries.delete(oldest.value);
    }
  };

  const liveEntry = (key: string, now: number): LedgerEntry | undefined => {
    const entry = entries.get(key);
    if (!entry) return undefined;
    const ttl = entry.state === "success"
      ? ADVISOR_SUCCESS_TTL_MS
      : entry.state === "failed"
        ? ADVISOR_FAILURE_COOLDOWN_MS
        : ADVISOR_INFLIGHT_TTL_MS;
    if (now - entry.at > ttl) {
      entries.delete(key);
      return undefined;
    }
    return entry;
  };

  const set = (key: string, state: LedgerEntry["state"], now: number): void => {
    if (entries.has(key)) entries.delete(key);
    entries.set(key, { state, at: now });
    evict();
  };

  return {
    claim(key, now = Date.now(), repeatAfterSuccess = false) {
      const entry = liveEntry(key, now);
      if (!entry || (repeatAfterSuccess && entry.state === "success")) {
        set(key, "inflight", now);
        return "claimed";
      }
      if (entry.state === "success") return "complete";
      if (entry.state === "failed") return "cooldown";
      return "inflight";
    },
    complete(key, now = Date.now()) {
      set(key, "success", now);
    },
    fail(key, now = Date.now()) {
      set(key, "failed", now);
    },
    release(key, now = Date.now()) {
      const entry = entries.get(key);
      // Only an in-flight claim this caller owns is released; a settled success/failure stays.
      if (entry?.state === "inflight" && now - entry.at <= ADVISOR_INFLIGHT_TTL_MS) entries.delete(key);
    },
    size() {
      return entries.size;
    },
  };
}

/**
 * Stable conversation identity for the ledger, reusing the repository's existing request
 * identities in specificity order: the client's own thread, the shared parent thread, the
 * Cursor conversation, the Cursor client thread, then the reasoning-replay scope's thread.
 * Returns undefined for a client that sends no identity at all — such a caller stays out of the
 * process-global ledger on purpose (fail-open, see the module header).
 */
export function advisorConversationIdentity(parsed: {
  _codexOwnThreadId?: string;
  _clientThreadId?: string;
  _cursorConversationId?: string;
  _cursorClientThreadId?: string;
  _reasoningReplayScope?: { clientThreadId?: string };
}): string | undefined {
  return parsed._codexOwnThreadId
    ?? parsed._clientThreadId
    ?? parsed._cursorConversationId
    ?? parsed._cursorClientThreadId
    ?? parsed._reasoningReplayScope?.clientThreadId
    ?? undefined;
}

/**
 * The current task boundary inside a conversation: how many user turns the history carries and
 * what the latest one says. A new user message moves the boundary (a new task gets its own
 * claim); the same turn re-sent by a stateless full-history client keeps the same boundary, and
 * a `previous_response_id` expansion replays the same user turns, so continuations dedup.
 */
export function advisorTaskBoundary(parsed: {
  context: { messages: readonly { role: string; content: unknown }[] };
}): string {
  let userTurns = 0;
  let lastUserText = "";
  for (const message of parsed.context.messages) {
    if (message.role !== "user") continue;
    userTurns += 1;
    lastUserText = contentText(message.content);
  }
  return `t${userTurns}:${djb2(lastUserText.slice(0, 200))}`;
}

/**
 * The ledger key: conversation identity + task boundary + worker model. Returns undefined when
 * the caller has no stable conversation identity — the caller then relies on request-scoped
 * dedup and genuine in-history provenance instead of a shared guess (documented fail-open).
 */
export function advisorLedgerKey(
  parsed: {
    context: { messages: readonly { role: string; content: unknown }[] };
    _codexOwnThreadId?: string;
    _clientThreadId?: string;
    _cursorConversationId?: string;
    _cursorClientThreadId?: string;
    _reasoningReplayScope?: { clientThreadId?: string };
  },
  workerModelId: string,
): string | undefined {
  const identity = advisorConversationIdentity(parsed);
  if (!identity) return undefined;
  return `cid:${djb2(identity)}:${advisorTaskBoundary(parsed)}:${djb2(workerModelId)}`;
}

/** Text projection for string-or-parts content; used only for hashing, never transmitted. */
export function contentText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((part): part is { type: "text"; text: string } =>
      !!part && typeof part === "object" && (part as { type?: unknown }).type === "text"
      && typeof (part as { text?: unknown }).text === "string")
    .map(part => part.text)
    .join("");
}

function djb2(value: string): string {
  let hash = 5381;
  for (let i = 0; i < value.length; i += 1) {
    hash = ((hash << 5) + hash + value.charCodeAt(i)) | 0;
  }
  // Keep it positive and printable.
  return (hash >>> 0).toString(36);
}

/** Runtime-owned preflight wrapper. Distinct from the manual advice wrapper on purpose. */
export const ADVISOR_PREFLIGHT_MARKER = "<opencodex_advisor_preflight>";

/** The synthetic advisor tool's wire name; kept in sync with the tool definition by test. */
export const ADVISOR_RESULT_TOOL_NAME = "advisor";

/** Advice wrapper written by the MANUAL reinjection path (a paired tool result). */
export const ADVISOR_ADVICE_MARKER = "<opencodex_advisor>";

/**
 * Detect an ALREADY-PRESENT advisor result in the conversation history — by PROVENANCE, never by
 * a bare string:
 *
 * - manual: a `toolResult` whose `toolName` is the synthetic advisor tool AND whose content
 *   carries the advice wrapper. A shell/file/log result that merely contains the wrapper text is
 *   NOT an advisor result.
 * - preflight: a developer message carrying the runtime-owned `<opencodex_advisor_preflight>`
 *   wrapper. Ordinary developer text that happens to contain `<opencodex_advisor>` is NOT an
 *   advisor result.
 *
 * Failure notices (`<opencodex_advisor_unavailable>`) match neither form and therefore never
 * suppress a later consultation.
 */
export function historyHasAdvisorResult(parsed: {
  context: { messages: readonly { role: string; content?: unknown; toolName?: string }[] };
}): boolean {
  for (let i = parsed.context.messages.length - 1; i >= 0; i -= 1) {
    const message = parsed.context.messages[i]!;
    if (message.role === "toolResult") {
      if (message.toolName !== ADVISOR_RESULT_TOOL_NAME) continue;
      if (contentText(message.content).includes(ADVISOR_ADVICE_MARKER)) return true;
      continue;
    }
    if (message.role === "developer") {
      if (contentText(message.content).includes(ADVISOR_PREFLIGHT_MARKER)) return true;
    }
  }
  return false;
}

/** Kept for callers that only need the first user text (payload building, tests). */
export function firstUserText(parsed: { context: { messages: readonly { role: string; content: unknown }[] } }): string {
  for (const message of parsed.context.messages) {
    if (message.role !== "user") continue;
    const text = contentText(message.content);
    if (text.trim() !== "") return text;
  }
  return "";
}

/**
 * Deterministic preflight trigger: has this conversation already produced orientation evidence
 * since the latest user message? The documented approximation for "before the first substantive
 * implementation" — the protocol layer offers no safe pre-mutation checkpoint, so OpenCodex fires
 * the automatic attempt on the first worker reasoning turn that arrives with that evidence.
 * Evidence is an assistant tool call OR a tool result after the latest user message (both forms
 * are genuinely accepted; the docs say so). Only text/toolResult content is inspected — never
 * reasoning, never encrypted items.
 */
export function hasOrientationEvidence(parsed: { context: { messages: readonly { role: string; content: unknown }[] } }): boolean {
  let latestUserIndex = -1;
  for (let i = parsed.context.messages.length - 1; i >= 0; i -= 1) {
    if (parsed.context.messages[i]?.role === "user") {
      latestUserIndex = i;
      break;
    }
  }
  if (latestUserIndex < 0) return false;
  for (let i = latestUserIndex + 1; i < parsed.context.messages.length; i += 1) {
    const message = parsed.context.messages[i];
    if (message.role === "toolResult") return true;
    if (message.role === "assistant" && Array.isArray(message.content)
      && message.content.some(part =>
        !!part && typeof part === "object" && (part as { type?: unknown }).type === "toolCall")) {
      return true;
    }
  }
  return false;
}
