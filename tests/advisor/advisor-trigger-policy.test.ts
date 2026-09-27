import { expect, test } from "bun:test";
import { initialTriggerState, reduceTriggerEvent, evaluateAdaptiveTrigger } from "../../src/advisor/triggers/policy";
import type { AdvisorTriggerEvent } from "../../src/advisor/triggers/types";
const fail = (fingerprint = "test:a"): AdvisorTriggerEvent => ({ type: "tool_completed", semanticClass: "validation", success: false, fingerprint });
const pass: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "validation", success: true, fingerprint: "test:a" };
const edit: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "mutation", success: true };
const read: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "read", success: true };
const diagnostic: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "diagnostic", success: false, fingerprint: "git" };
function state(events: AdvisorTriggerEvent[]) { return events.reduce(reduceTriggerEvent, initialTriggerState("task")); }
function decide(events: AdvisorTriggerEvent[]) { return evaluateAdaptiveTrigger(state(events), { type: "worker_turn_completed" }); }
test("a failure, or a failure followed only by an edit, does not consult", () => {
  expect(decide([fail()])).toEqual({ action: "continue" });
  expect(state([fail()]).phase).toBe("failure_observed");
  expect(decide([fail(), edit])).toEqual({ action: "continue" });
  expect(state([fail(), edit]).phase).toBe("repair_attempted");
});
test("failure, repair, then the same validation failure consults", () => {
  expect(decide([fail(), edit, fail()])).toMatchObject({
    action: "consult",
    reason: "repair_failed",
    evidence: ["previous_validation_failed=true", "repair_mutations=1", "followup_validation_failed=true", "same_validation=true"],
  });
  const twice = decide([fail(), edit, edit, fail()]);
  expect(twice.action).toBe("consult");
  if (twice.action === "consult") expect(twice.evidence).toContain("repair_mutations=2");
});
test("a passing validation after a repair resets and does not consult", () => {
  expect(decide([fail(), edit, pass])).toEqual({ action: "continue" });
  expect(state([fail(), edit, pass]).phase).toBe("normal");
  expect(decide([fail(), pass, fail()]).action).toBe("continue");
});
test("failures with no repair mutation do not consult", () => {
  expect(decide([fail(), read, diagnostic, fail()])).toEqual({ action: "continue" });
  expect(state([fail(), fail()]).phase).toBe("failure_observed");
  expect(state([fail(), fail()]).mutationsAfterFailure).toBe(0);
});
test("edits without a prior failure never consult", () => {
  expect(decide([edit, edit, edit, edit])).toEqual({ action: "continue" });
  expect(state([edit, edit, edit, edit]).phase).toBe("normal");
});
test("a different follow-up validation does not consult", () => {
  expect(decide([fail("test:a"), edit, fail("build:b")])).toEqual({ action: "continue" });
  expect(state([fail("test:a"), edit, fail("build:b")])).toMatchObject({ phase: "failure_observed", lastFailedValidationFingerprint: "build:b", mutationsAfterFailure: 0 });
});
test("a missing fingerprint consults without claiming the validations were the same", () => {
  const bare: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "validation", success: false };
  for (const events of [[bare, edit, bare], [fail(), edit, bare]] as const) {
    const decision = decide([...events]);
    expect(decision.action).toBe("consult");
    if (decision.action === "consult") expect(decision.evidence).toContain("same_validation=unknown");
  }
});
test("an unreadable validation result is neither failure nor progress", () => {
  const unread: AdvisorTriggerEvent = { type: "tool_completed", semanticClass: "validation", success: undefined, fingerprint: "test:a" };
  expect(state([fail(), unread]).phase).toBe("failure_observed");
  expect(decide([fail(), edit, unread])).toEqual({ action: "continue" });
});
test("different negative diagnostics never count as failed validation", () => {
  expect(decide(["a", "b", "c"].map(fingerprint => ({ type: "tool_completed", semanticClass: "diagnostic", success: false, fingerprint })))).toEqual({ action: "continue" });
});
test("advice resets the cycle; a single later failure does not consult", () => {
  const consulted: AdvisorTriggerEvent = { type: "advisor_consulted" };
  expect(decide([fail(), edit, fail(), consulted, fail()])).toEqual({ action: "continue" });
  expect(decide([consulted, fail(), edit, fail()])).toMatchObject({ action: "consult", reason: "repair_failed" });
  expect(state([fail(), edit, fail(), consulted]).phase).toBe("normal");
});
test("policy is pure and only evaluates completed turns", () => {
  const ready = Object.freeze(state([fail(), edit, fail()]));
  expect(evaluateAdaptiveTrigger(ready, fail())).toEqual({ action: "continue" });
  expect(evaluateAdaptiveTrigger(ready, { type: "worker_turn_completed" })).toEqual(evaluateAdaptiveTrigger(ready, { type: "worker_turn_completed" }));
});
