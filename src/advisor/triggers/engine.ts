import type { OcxParsedRequest, OcxTool } from "../../types";
import { ADVISOR_SUCCESS_TTL_MS, type AdvisorPreflightLedger } from "../state";
import { classifyTool, fingerprint, observedExit, type ClassifiedTool } from "./classify-tool";
import { evaluateAdvisorTrigger, initialTriggerState, reduceTriggerEvent } from "./policy";
import type { AdvisorTriggerEvent, AdvisorTriggerState, TriggerDecision } from "./types";

const MAX_TASKS = 512;
const MAX_SEEN_RESULTS = 2048;
const MAX_PENDING = 128;
const MAX_MESSAGES = 128;
interface Task {
  state: AdvisorTriggerState;
  seen: Set<string>;
  pending: Map<string, ClassifiedTool>;
  at: number;
}
/** Shares PR1 identity and ledger ownership; contains metadata only and starts no timer. */
export function createTriggerEngine() {
  const tasks = new Map<string, Task>();
  function get(key: string, now: number): Task {
    const oldest = tasks.entries().next().value;
    if (oldest && now - oldest[1].at > ADVISOR_SUCCESS_TTL_MS) tasks.delete(oldest[0]);
    let task = tasks.get(key);
    if (task && now - task.at > ADVISOR_SUCCESS_TTL_MS) { tasks.delete(key); task = undefined; }
    if (!task) {
      task = { state: initialTriggerState(key), seen: new Set(), pending: new Map(), at: now };
      tasks.set(key, task);
      if (tasks.size > MAX_TASKS) tasks.delete(tasks.keys().next().value!);
    }
    return task;
  }
  function record(task: Task, event: AdvisorTriggerEvent) { task.state = reduceTriggerEvent(task.state, event); }
  return {
    observe(key: string, parsed: OcxParsedRequest, now: number): TriggerDecision {
      const task = get(key, now);
      // Saturate rather than evict dedup keys and count old replay as new activity.
      if (task.seen.size >= MAX_SEEN_RESULTS) return { action: "continue" };
      const messages = parsed.context.messages;
      let start = Math.max(0, messages.length - MAX_MESSAGES);
      for (let i = messages.length - 1; i >= start; i--) {
        if (messages[i]?.role === "user") { start = i + 1; break; }
      }
      const tools = new Map<string, OcxTool>();
      for (const tool of (parsed.context.tools ?? []).slice(0, 128)) tools.set(`${tool.namespace ?? ""}\0${tool.name}`, tool);
      for (let i = start; i < messages.length && task.seen.size < MAX_SEEN_RESULTS; i++) {
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
          task.seen.add(id);
          task.pending.delete(id);
          const success = observedExit(message);
          if (call.kind === "validation" && success !== undefined) {
            record(task, { type: "validation_observed", observation: { kind: "command", success, fingerprint: call.fingerprint } });
          } else if (call.kind === "mutation" && !message.isError && success !== false) {
            record(task, { type: "mutation_observed", observation: { target: call.target ?? "command", tool: call.tool, fingerprint: call.fingerprint } });
          } else if (call.kind === "diagnostic" && call.fingerprint && success !== undefined) {
            record(task, { type: "diagnostic_observed", fingerprint: call.fingerprint, success });
          }
        }
      }
      return evaluateAdvisorTrigger(task.state, { type: "worker_turn_completed" });
    },
    consulted(key: string, now: number) { record(get(key, now), { type: "consultation_completed" }); },
    snapshot(key: string, now: number) { return { ...get(key, now).state }; },
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
