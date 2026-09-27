import type { AdaptiveTriggerState, AdvisorTriggerEvent, SameValidationEvidence, TriggerDecision } from "./types";

export function initialTriggerState(taskKey: string): AdaptiveTriggerState {
  return { taskKey, phase: "normal", mutationsAfterFailure: 0, consultationCount: 0, baselineOnly: false };
}

function clearFailureCycle(state: AdaptiveTriggerState) {
  state.phase = "normal";
  state.mutationsAfterFailure = 0;
  delete state.lastFailedValidationFingerprint;
  delete state.repairFailed;
}

function noteFailure(state: AdaptiveTriggerState, fingerprint: string | undefined) {
  state.phase = "failure_observed";
  state.mutationsAfterFailure = 0;
  delete state.repairFailed;
  if (fingerprint) state.lastFailedValidationFingerprint = fingerprint;
  else delete state.lastFailedValidationFingerprint;
}

function sameValidationEvidence(previous: string | undefined, next: string | undefined): SameValidationEvidence | "different" {
  if (typeof previous === "string" && typeof next === "string") {
    return previous === next ? "true" : "different";
  }
  return "unknown";
}

export function reduceTriggerEvent(previous: AdaptiveTriggerState, event: AdvisorTriggerEvent): AdaptiveTriggerState {
  const state = { ...previous };
  if (event.type === "advisor_consulted") {
    clearFailureCycle(state);
    state.consultationCount++;
    state.baselineOnly = event.baseline === true;
    return state;
  }
  if (event.type !== "tool_completed") return state;
  if (event.semanticClass === "mutation" && event.success === true) {
    if (state.phase === "failure_observed" || state.phase === "repair_attempted") {
      state.phase = "repair_attempted";
      state.mutationsAfterFailure++;
    }
    return state;
  }
  if (event.semanticClass !== "validation" || event.success === undefined) return state;
  if (event.success) {
    clearFailureCycle(state);
    return state;
  }
  if (state.phase === "repair_attempted" && state.mutationsAfterFailure > 0) {
    const relation = sameValidationEvidence(state.lastFailedValidationFingerprint, event.fingerprint);
    if (relation === "different") {
      noteFailure(state, event.fingerprint);
      return state;
    }
    state.repairFailed = { sameValidation: relation, repairMutations: state.mutationsAfterFailure };
    return state;
  }
  noteFailure(state, event.fingerprint);
  return state;
}

/** Evaluate reduced observation state at a completed-turn boundary; no effects or authority. */
export function evaluateAdaptiveTrigger(state: AdaptiveTriggerState, event: AdvisorTriggerEvent): TriggerDecision {
  if (event.type !== "worker_turn_completed" || !state.repairFailed) return { action: "continue" };
  const repair = state.repairFailed;
  return {
    action: "consult",
    reason: "repair_failed",
    evidence: [
      "previous_validation_failed=true",
      `repair_mutations=${repair.repairMutations}`,
      "followup_validation_failed=true",
      `same_validation=${repair.sameValidation}`,
    ],
  };
}
