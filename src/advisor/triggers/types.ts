export type ToolSemanticClass = "read" | "search" | "diagnostic" | "validation" | "mutation" | "execution" | "unknown";
export type AdvisorTriggerReason = "repeated_validation_failure" | "repeated_mutation_without_validation" | "repeated_action";
export type AdvisorTriggerEvent =
  | { type: "tool_completed"; semanticClass: ToolSemanticClass; fingerprint?: string; success?: boolean }
  | { type: "worker_turn_completed" }
  | { type: "advisor_consulted"; baseline?: boolean };
export interface AdaptiveTriggerState {
  taskKey: string;
  consecutiveValidationFailures: number;
  mutationsSinceSuccessfulValidation: number;
  repeatedActionCount: number;
  lastActionFingerprint?: string;
  consultationCount: number;
  baselineOnly: boolean;
  eventsSinceConsultation: number;
  mutationSinceConsultation: boolean;
  validationAfterMutation: boolean;
}
export interface TriggerPolicy { validationFailures: number; mutations: number; repetitions: number }
export type TriggerDecision = { action: "continue" } | {
  action: "consult";
  reason: AdvisorTriggerReason;
  evidence: string[];
};
