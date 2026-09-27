/**
 * Conversation-scoped advisor state.
 *
 * Two scopes, deliberately separate:
 *
 * 1. REQUEST-scoped state lives in the per-request plan closure (see runtime.ts) — consultation
 *    count, preflight flag, consultation fingerprints. It is born and dies with one request and
 *    is never shared.
 *
 * 2. TASK-scoped preflight dedup (this file): a bounded, process-local ledger keyed by a stable
 *    conversation fingerprint so `policy: "preflight"` consults AT MOST ONCE per task across the
 *    many stateless full-history requests a worker sends. Bounded by entry count and TTL; entries
 *    are plain strings — no secrets, no message bodies.
 *
 * Known limitation (documented in the PR and public docs): after a proxy restart the ledger is
 * empty, so a task in progress may get one more preflight consultation. That is fail-open for
 * correctness and only costs one extra expert call.
 */

const MAX_ENTRIES = 512;
const TTL_MS = 24 * 60 * 60 * 1000;

export interface PreflightLedgerEntry {
  markedAt: number;
  reason: "preflight" | "manual";
}

export interface AdvisorPreflightLedger {
  /** True when this conversation fingerprint already had its guaranteed consultation. */
  has(key: string, now?: number): boolean;
  /** Record a consultation for a conversation fingerprint. Evicts expired/oldest entries. */
  mark(key: string, reason: "preflight" | "manual", now?: number): void;
  /** Test/observability seam: current entry count. */
  size(): number;
}

export function createAdvisorPreflightLedger(): AdvisorPreflightLedger {
  const entries = new Map<string, PreflightLedgerEntry>();
  return {
    has(key, now = Date.now()) {
      const entry = entries.get(key);
      if (!entry) return false;
      if (now - entry.markedAt > TTL_MS) {
        entries.delete(key);
        return false;
      }
      return true;
    },
    mark(key, reason, now = Date.now()) {
      if (entries.has(key)) entries.delete(key);
      entries.set(key, { markedAt: now, reason });
      while (entries.size > MAX_ENTRIES) {
        // Map iteration is insertion-ordered; the oldest entry goes first.
        const oldest = entries.keys().next();
        if (oldest.done) break;
        entries.delete(oldest.value);
      }
    },
    size() {
      return entries.size;
    },
  };
}

/**
 * Stable conversation fingerprint for preflight dedup.
 *
 * `threadId` is the primary identity: Codex sends per-thread headers, so two independent
 * conversations never share a ledger entry. The fallback (a stateless client that sends no
 * thread identity) hashes the first user message — stable within one task for full-history
 * clients, but two threadless conversations that open with the same prompt share a key; that
 * residual limitation is documented. The worker model id is always part of the key.
 */
export function conversationPreflightKey(
  firstUserText: string,
  workerModelId: string,
  threadId?: string,
): string {
  return threadId
    ? `tid:${threadId}:${djb2(workerModelId)}`
    : `djb2:${djb2(firstUserText)}:${djb2(workerModelId)}`;
}

/**
 * The best stable conversation identity available on a parsed request: the client's own thread
 * id when the surface provides one, then the shared parent thread id, then the Cursor
 * conversation id. Undefined for clients that send no identity header.
 */
export function conversationThreadId(parsed: {
  _codexOwnThreadId?: string;
  _clientThreadId?: string;
  _cursorConversationId?: string;
}): string | undefined {
  return parsed._codexOwnThreadId ?? parsed._clientThreadId ?? parsed._cursorConversationId;
}

export function firstUserText(parsed: { context: { messages: readonly { role: string; content: unknown }[] } }): string {
  for (const message of parsed.context.messages) {
    if (message.role !== "user") continue;
    const content = message.content;
    if (typeof content === "string") return content;
    if (Array.isArray(content)) {
      const text = content
        .filter((part): part is { type: "text"; text: string } =>
          !!part && typeof part === "object" && (part as { type?: unknown }).type === "text"
          && typeof (part as { text?: unknown }).text === "string")
        .map(part => part.text)
        .join("");
      if (text.trim() !== "") return text;
    }
  }
  return "";
}

function djb2(value: string): string {
  let hash = 5381;
  for (let i = 0; i < value.length; i += 1) {
    hash = ((hash << 5) + hash + value.charCodeAt(i)) | 0;
  }
  // Keep it positive and printable.
  return (hash >>> 0).toString(36);
}

/**
 * Detect an ALREADY-PRESENT advisor result in the conversation history. The advice wrapper
 * (<opencodex_advisor>) is the single marker both reinjection paths write, so a task that
 * already carried advice (manual or preflight, this process or a previous_response_id replay)
 * never triggers a second guaranteed consultation on top of it.
 */
export function historyHasAdvisorResult(parsed: { context: { messages: readonly { role: string; content: unknown }[] } }): boolean {
  for (let i = parsed.context.messages.length - 1; i >= 0; i -= 1) {
    const message = parsed.context.messages[i];
    if (message.role !== "toolResult" && message.role !== "developer") continue;
    const content = message.content;
    const text = typeof content === "string"
      ? content
      : Array.isArray(content)
        ? content
          .filter((part): part is { type: "text"; text: string } =>
            !!part && typeof part === "object" && (part as { type?: unknown }).type === "text"
            && typeof (part as { text?: unknown }).text === "string")
          .map(part => part.text)
          .join("")
        : "";
    if (text.includes("<opencodex_advisor>")) return true;
  }
  return false;
}

/**
 * Deterministic preflight trigger: has this conversation already produced at least one valid
 * orientation/tool-result continuation since the latest user message? This is the documented
 * approximation for "before the first substantive implementation" — the protocol layer offers no
 * safe pre-mutation checkpoint, so OpenCodex fires the guaranteed consultation on the first
 * worker reasoning turn that arrives WITH tool evidence of orientation. Only text/toolResult
 * content is inspected; never reasoning, never encrypted items.
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
