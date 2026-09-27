import type { OcxParsedRequest } from "../../types";
import { ADVISOR_ADVICE_MARKER, ADVISOR_SUCCESS_TTL_MS, type AdvisorPreflightLedger } from "../state";
import { classifyTool, fingerprint, observedExit, type ClassifiedTool } from "./classify-tool";
import { evaluateAdaptiveTrigger, initialTriggerState, reduceTriggerEvent } from "./policy";
import type { AdaptiveTriggerState, TriggerDecision } from "./types";
const MAX_TASKS = 512, MAX_RESULTS = 2048, MAX_PENDING = 128;
interface Task { state: AdaptiveTriggerState; seen: Set<string>; pending: Map<string, ClassifiedTool>; at: number }
/** Observation only. Saturation disables escalation; no live state or ownership is evicted. */
export function createTriggerEngine() {
  const tasks = new Map<string, Task>();
  function get(key: string, now: number): Task | undefined {
    const current = tasks.get(key);
    if (current && now - current.at <= ADVISOR_SUCCESS_TTL_MS) return current;
    if (current) tasks.delete(key);
    if (tasks.size >= MAX_TASKS) {
      for (const [id, task] of tasks) if (now - task.at > ADVISOR_SUCCESS_TTL_MS) tasks.delete(id);
    }
    if (tasks.size >= MAX_TASKS) return undefined;
    const task = { state: initialTriggerState(key), seen: new Set<string>(), pending: new Map<string, ClassifiedTool>(), at: now };
    tasks.set(key, task);
    return task;
  }
  return {
    observe(key: string, parsed: OcxParsedRequest, now: number): TriggerDecision {
      const task = get(key, now);
      if (!task || task.seen.size >= MAX_RESULTS) return { action: "continue" };
      const messages = parsed.context.messages;
      let start = Math.max(0, messages.length - 128);
      for (let i = messages.length - 1; i >= start; i--) if (messages[i]?.role === "user") { start = i + 1; break; }
      const tools = new Map((parsed.context.tools ?? []).slice(0, 128).map(t => [`${t.namespace ?? ""}\0${t.name}`, t]));
      for (let i = start; i < messages.length && task.seen.size < MAX_RESULTS; i++) {
        const message = messages[i]!;
        if (message.role === "assistant") {
          for (const part of message.content.slice(0, MAX_PENDING)) {
            if (part.type !== "toolCall" || typeof part.id !== "string" || part.id.length > 1024) continue;
            const id = fingerprint(part.id);
            if (task.seen.has(id) || task.pending.has(id)) continue;
            task.pending.set(id, classifyTool(part, tools.get(`${part.namespace ?? ""}\0${part.name}`)));
            if (task.pending.size > MAX_PENDING) task.pending.delete(task.pending.keys().next().value!);
          }
        } else if (message.role === "toolResult" && typeof message.toolCallId === "string" && message.toolCallId.length <= 1024) {
          const id = fingerprint(message.toolCallId);
          if (task.seen.has(id)) continue;
          const call = task.pending.get(id);
          if (!call) continue;
          task.seen.add(id); task.pending.delete(id);
          const text = typeof message.content === "string" ? message.content : message.content[0]?.type === "text" ? message.content[0].text : "";
          if (message.toolName === "advisor" && text.slice(0, 1024).includes(ADVISOR_ADVICE_MARKER)) {
            task.state = reduceTriggerEvent(task.state, { type: "advisor_consulted" });
            continue;
          }
          const success = observedExit(message);
          task.state = reduceTriggerEvent(task.state, { type: "tool_completed", semanticClass: call.kind,
            fingerprint: call.fingerprint,
            success: call.kind === "mutation" && !call.requiresExit && !message.isError && success !== false ? true : success });
        }
      }
      return evaluateAdaptiveTrigger(task.state, { type: "worker_turn_completed" });
    },
    consulted(key: string, now: number, baseline = false) {
      const task = get(key, now);
      if (task) task.state = reduceTriggerEvent(task.state, { type: "advisor_consulted", baseline });
    },
    snapshot(key: string, now: number) { const task = get(key, now); return task ? { ...task.state } : undefined; },
    size() { return tasks.size; },
  };
}
export type TriggerEngine = ReturnType<typeof createTriggerEngine>;
const engines = new WeakMap<AdvisorPreflightLedger, TriggerEngine>();
export function triggerEngineFor(ledger: AdvisorPreflightLedger): TriggerEngine {
  let engine = engines.get(ledger);
  if (!engine) { engine = createTriggerEngine(); engines.set(ledger, engine); }
  return engine;
}
