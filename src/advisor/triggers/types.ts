export type ToolSemanticClass = "read" | "search" | "diagnostic" | "validation" | "mutation" | "execution" | "unknown";
export type AdvisorTriggerReason = "repeated_validation_failure" | "repeated_mutation_without_progress" | "repeated_action";
export interface ValidationObservation { kind: string; success: boolean; fingerprint?: string }
export interface MutationObservation { target: string; tool: string; fingerprint?: string }
export type AdvisorTriggerEvent =
  | { type: "worker_turn_started" | "worker_turn_completed" }
  | { type: "mutation_observed"; observation: MutationObservation }
  | { type: "validation_observed"; observation: ValidationObservation }
  | { type: "diagnostic_observed"; fingerprint: string; success: boolean }
  | { type: "consultation_completed" };
export interface AdvisorTriggerState {
  readonly taskKey: string;
  sequence: number;
  mutationCount: number;
  consecutiveValidationFailures: number;
  mutationsSinceProgress: number;
  repeatedActionCount: number;
  lastActionFingerprint?: string;
  lastDiagnosticFingerprint?: string;
  consultationCount: number;
  meaningfulEventsSinceConsultation: number;
  lastMutationSequence?: number;
}
export interface AdvisorTriggerPolicy {
  validationFailures: number;
  mutationsWithoutProgress: number;
  repeatedActions: number;
  cooldownEvents: number;
}
export type TriggerDecision = { action: "continue" } | {
  action: "consult";
  reason: AdvisorTriggerReason;
  severity: "normal" | "high";
  evidence: string[];
};
