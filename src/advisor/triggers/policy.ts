import type { AdvisorTriggerEvent, AdvisorTriggerPolicy, AdvisorTriggerState, TriggerDecision } from "./types";
export const DEFAULT_TRIGGER_POLICY: Readonly<AdvisorTriggerPolicy> = Object.freeze({
  validationFailures: 2, mutationsWithoutProgress: 2, repeatedActions: 2, cooldownEvents: 2,
});
export function initialTriggerState(taskKey: string): AdvisorTriggerState {
  return { taskKey, sequence: 0, mutationCount: 0, consecutiveValidationFailures: 0,
    mutationsSinceProgress: 0, repeatedActionCount: 0, consultationCount: 0,
    meaningfulEventsSinceConsultation: 0 };
}
function clearSignals(state: AdvisorTriggerState): void {
  state.consecutiveValidationFailures = 0;
  state.mutationsSinceProgress = 0;
  state.repeatedActionCount = 0;
  delete state.lastActionFingerprint;
}
/** Pure reducer. Only objective result metadata enters the state. */
export function reduceTriggerEvent(previous: AdvisorTriggerState, event: AdvisorTriggerEvent): AdvisorTriggerState {
  const state = { ...previous, sequence: previous.sequence + 1 };
  if (event.type === "consultation_completed") {
    clearSignals(state);
    state.consultationCount++;
    state.meaningfulEventsSinceConsultation = 0;
    return state;
  }
  if (event.type === "worker_turn_started" || event.type === "worker_turn_completed") return state;
  state.meaningfulEventsSinceConsultation++;
  let fingerprint: string | undefined;
  if (event.type === "mutation_observed") {
    state.mutationCount++;
    state.mutationsSinceProgress++;
    state.lastMutationSequence = state.sequence;
    fingerprint = event.observation.fingerprint;
  } else if (event.type === "validation_observed") {
    if (event.observation.success) { clearSignals(state); return state; }
    state.consecutiveValidationFailures++;
    fingerprint = event.observation.fingerprint;
  } else if (event.type === "diagnostic_observed") {
    // A new successful diagnostic is an observable proxy, not a claim about root cause.
    if (event.success && event.fingerprint !== state.lastDiagnosticFingerprint) clearSignals(state);
    state.lastDiagnosticFingerprint = event.fingerprint;
    return state; // failed diagnostic experiments are never counted as failed validation
  }
  state.repeatedActionCount = fingerprint && fingerprint === state.lastActionFingerprint
    ? state.repeatedActionCount + 1 : fingerprint ? 1 : 0;
  state.lastActionFingerprint = fingerprint;
  return state;
}
/** Evaluate the state AFTER this event. No I/O, clock, global state, or mutations. */
export function evaluateAdvisorTrigger(
  state: AdvisorTriggerState, event: AdvisorTriggerEvent,
  policy: Readonly<AdvisorTriggerPolicy> = DEFAULT_TRIGGER_POLICY,
): TriggerDecision {
  if (event.type !== "worker_turn_completed") return { action: "continue" };
  if (state.consultationCount > 0 && state.meaningfulEventsSinceConsultation < policy.cooldownEvents) return { action: "continue" };
  if (state.consecutiveValidationFailures >= policy.validationFailures) return {
    action: "consult", reason: "repeated_validation_failure", severity: "high",
    evidence: [`consecutive_validation_failures:${state.consecutiveValidationFailures}`],
  };
  if (state.repeatedActionCount >= policy.repeatedActions) return {
    action: "consult", reason: "repeated_action", severity: "normal",
    evidence: [`equivalent_actions_without_progress:${state.repeatedActionCount}`],
  };
  if (state.mutationsSinceProgress >= policy.mutationsWithoutProgress) return {
    action: "consult", reason: "repeated_mutation_without_progress", severity: "normal",
    evidence: [`mutations_without_progress:${state.mutationsSinceProgress}`],
  };
  return { action: "continue" };
}
