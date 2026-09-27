import { describe, expect, test } from "bun:test";
import { evaluateAdvisorTrigger, initialTriggerState, reduceTriggerEvent } from "../../src/advisor/triggers/policy";
import type { AdvisorTriggerEvent } from "../../src/advisor/triggers/types";
const mutation = (target = "a"): AdvisorTriggerEvent => ({ type: "mutation_observed", observation: { target, tool: "edit_file" } });
const validation = (success: boolean, fingerprint = "test"): AdvisorTriggerEvent => ({ type: "validation_observed", observation: { kind: "test", success, fingerprint } });
const consult: AdvisorTriggerEvent = { type: "consultation_completed" };
function run(events: AdvisorTriggerEvent[]) {
  const state = events.reduce(reduceTriggerEvent, initialTriggerState("task"));
  return evaluateAdvisorTrigger(state, { type: "worker_turn_completed" });
}
describe("adaptive pure policy", () => {
  test("two failures escalate and a success resets", () => {
    expect(run([validation(false)])).toEqual({ action: "continue" });
    expect(run([validation(false), validation(false)])).toMatchObject({ reason: "repeated_validation_failure" });
    expect(run([validation(false), validation(true), validation(false)])).toEqual({ action: "continue" });
  });
  test("unvalidated mutations escalate, validated builds never do", () => {
    expect(run([mutation(), mutation("b")])).toMatchObject({ reason: "repeated_mutation_without_progress" });
    expect(run([mutation(), validation(true), mutation(), validation(true)])).toEqual({ action: "continue" });
  });
  test("failure reason wins for the edit/fail/edit/fail acceptance sequence", () => {
    expect(run([mutation(), validation(false), mutation(), validation(false)])).toMatchObject({ reason: "repeated_validation_failure" });
  });
  test("manual consultation resets evidence but does not permanently disable escalation", () => {
    expect(run([validation(false), consult, validation(false)])).toEqual({ action: "continue" });
    expect(run([consult, validation(false), validation(false)])).toMatchObject({ action: "consult" });
  });
  test("different failed diagnostics do not imply failed validation; new evidence clears mutations", () => {
    const diagnostic = (fingerprint: string, success: boolean): AdvisorTriggerEvent => ({ type: "diagnostic_observed", fingerprint, success });
    expect(run([diagnostic("a", false), diagnostic("b", false)])).toEqual({ action: "continue" });
    expect(run([mutation(), diagnostic("a", true), mutation()])).toEqual({ action: "continue" });
  });
  test("exact equivalent actions repeat; different actions do not", () => {
    const action = (fingerprint: string): AdvisorTriggerEvent => ({ type: "mutation_observed", observation: { tool: "shell", target: "path", fingerprint } });
    expect(run([action("a"), action("a")])).toMatchObject({ reason: "repeated_action" });
    const state = [action("a"), action("b")].reduce(reduceTriggerEvent, initialTriggerState("t"));
    expect(state.repeatedActionCount).toBe(1);
  });
  test("policy is deterministic and never mutates its inputs", () => {
    const state = Object.freeze(initialTriggerState("task"));
    const next = reduceTriggerEvent(state, mutation());
    expect(state.mutationCount).toBe(0);
    expect(reduceTriggerEvent(state, mutation())).toEqual(next);
    expect(evaluateAdvisorTrigger(next, { type: "worker_turn_completed" })).toEqual({ action: "continue" });
  });
});
