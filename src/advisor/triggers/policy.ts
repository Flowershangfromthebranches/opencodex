import type { AdaptiveTriggerState, AdvisorTriggerEvent, TriggerDecision, TriggerPolicy } from "./types";
// Three edits tolerate tools that split one logical patch into two file operations.
export const DEFAULT_TRIGGER_POLICY: Readonly<TriggerPolicy> = Object.freeze({ validationFailures: 2, mutations: 3, repetitions: 3 });
export function initialTriggerState(taskKey: string): AdaptiveTriggerState {
  return { taskKey, consecutiveValidationFailures: 0, mutationsSinceSuccessfulValidation: 0,
    repeatedActionCount: 0, consultationCount: 0, baselineOnly: false, eventsSinceConsultation: 0,
    mutationSinceConsultation: false, validationAfterMutation: false };
}
function clearSignals(state: AdaptiveTriggerState) {
  state.consecutiveValidationFailures = 0;
  state.mutationsSinceSuccessfulValidation = 0;
  state.repeatedActionCount = 0;
  delete state.lastActionFingerprint;
}
export function reduceTriggerEvent(previous: AdaptiveTriggerState, event: AdvisorTriggerEvent): AdaptiveTriggerState {
  const state = { ...previous };
  if (event.type === "advisor_consulted") {
    clearSignals(state);
    state.consultationCount++;
    state.baselineOnly = event.baseline === true;
    state.eventsSinceConsultation = 0;
    state.mutationSinceConsultation = false;
    state.validationAfterMutation = false;
  } else if (event.type === "tool_completed") {
    state.eventsSinceConsultation++;
    if (event.semanticClass === "mutation" && event.success === true) {
      state.mutationsSinceSuccessfulValidation++;
      state.mutationSinceConsultation = true;
    } else if (event.semanticClass === "validation") {
      // Missing status is not failure or progress. Break the sequence conservatively.
      if (event.success === undefined) { clearSignals(state); return state; }
      if (state.mutationSinceConsultation) state.validationAfterMutation = true;
      if (event.success) { clearSignals(state); return state; }
      state.consecutiveValidationFailures++;
      // Mutations between attempts do not erase equivalent-validation evidence.
      state.repeatedActionCount = event.fingerprint && event.fingerprint === state.lastActionFingerprint
        ? state.repeatedActionCount + 1 : event.fingerprint ? 1 : 0;
      state.lastActionFingerprint = event.fingerprint;
    }
  }
  return state;
}
/** Evaluate reduced observation state at a completed-turn boundary; no effects or authority. */
export function evaluateAdaptiveTrigger(state: AdaptiveTriggerState, event: AdvisorTriggerEvent,
  policy: Readonly<TriggerPolicy> = DEFAULT_TRIGGER_POLICY): TriggerDecision {
  if (event.type !== "worker_turn_completed") return { action: "continue" };
  if (state.consultationCount && !state.baselineOnly && (!state.mutationSinceConsultation || !state.validationAfterMutation)) return { action: "continue" };
  if (state.repeatedActionCount >= policy.repetitions) return { action: "consult", reason: "repeated_action",
    evidence: [`equivalent_failed_validations=${state.repeatedActionCount}`] };
  if (state.consecutiveValidationFailures >= policy.validationFailures) return { action: "consult", reason: "repeated_validation_failure",
    evidence: [`validationFailures=${state.consecutiveValidationFailures}`] };
  if (state.mutationsSinceSuccessfulValidation >= policy.mutations) return { action: "consult", reason: "repeated_mutation_without_validation",
    evidence: [`mutationsSinceSuccessfulValidation=${state.mutationsSinceSuccessfulValidation}`] };
  return { action: "continue" };
}
