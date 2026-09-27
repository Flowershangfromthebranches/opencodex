import { expect, test } from "bun:test";
import { initialTriggerState, reduceTriggerEvent, evaluateAdaptiveTrigger, DEFAULT_TRIGGER_POLICY } from "../../src/advisor/triggers/policy";
import type { AdvisorTriggerEvent } from "../../src/advisor/triggers/types";
const fail: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "validation", success: false, fingerprint: "test:a" };
const pass: AdvisorTriggerEvent = { ...fail, success: true };
const edit: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "mutation", success: true };
function state(events: AdvisorTriggerEvent[]) { return events.reduce(reduceTriggerEvent, initialTriggerState("task")); }
function decide(events: AdvisorTriggerEvent[]) { return evaluateAdaptiveTrigger(state(events), { type: "worker_turn_completed" }); }
test("two validation failures consult, successful or unknown validation breaks sequence", () => {
  expect(decide([fail]).action).toBe("continue");
  expect(decide([fail, edit, fail])).toMatchObject({ action: "consult", reason: "repeated_validation_failure" });
  expect(decide([fail, pass, fail]).action).toBe("continue");
  expect(decide([fail, { ...fail, success: undefined }, fail]).action).toBe("continue");
});
test("three mutations tolerate split patches and success resets them", () => {
  expect(decide([edit, edit]).action).toBe("continue");
  expect(decide([edit, edit, edit])).toMatchObject({ reason: "repeated_mutation_without_validation" });
  expect(decide([edit, edit, pass, edit]).action).toBe("continue");
  expect(decide([edit, pass, edit, pass, edit, pass]).action).toBe("continue");
});
test("repeated validations survive intervening mutations; distinct targets are not equivalent", () => {
  expect(decide([fail, edit, fail, edit, fail])).toMatchObject({ reason: "repeated_action" });
  expect(state([fail, edit, { ...fail, fingerprint: "test:b" }]).repeatedActionCount).toBe(1);
});
test("different negative diagnostics never count as failed validation", () => {
  expect(decide(["a", "b", "c"].map(fingerprint => ({ type: "tool_completed", semanticClass: "diagnostic", success: false, fingerprint })))).toEqual({ action: "continue" });
});
test("consultation requires new mutation then validation, and a fresh failure cycle", () => {
  const consulted: AdvisorTriggerEvent = { type: "advisor_consulted" };
  expect(decide([fail, fail, consulted, fail]).action).toBe("continue");
  expect(decide([consulted, fail, fail]).action).toBe("continue");
  expect(decide([consulted, edit, fail]).action).toBe("continue");
  expect(decide([consulted, edit, fail, edit, fail]).action).toBe("consult");
  expect(decide([consulted, fail, fail, edit]).action).toBe("continue");
});
test("policy is pure, configurable internally, and only evaluates completed turns", () => {
  const s = Object.freeze(state([fail, fail]));
  expect(evaluateAdaptiveTrigger(s, fail)).toEqual({ action: "continue" });
  expect(evaluateAdaptiveTrigger(s, { type: "worker_turn_completed" }, { ...DEFAULT_TRIGGER_POLICY, validationFailures: 3 }).action).toBe("continue");
  expect(evaluateAdaptiveTrigger(s, { type: "worker_turn_completed" })).toEqual(evaluateAdaptiveTrigger(s, { type: "worker_turn_completed" }));
});
