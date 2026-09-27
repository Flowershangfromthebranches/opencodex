export type ToolSemanticClass = "read" | "search" | "diagnostic" | "validation" | "mutation" | "execution" | "unknown";
export type AdvisorTriggerReason = "repair_failed";
export type TriggerPhase = "normal" | "failure_observed" | "repair_attempted";
export type AdvisorTriggerEvent =
  | { type: "tool_completed"; semanticClass: ToolSemanticClass; fingerprint?: string; success?: boolean }
  | { type: "worker_turn_completed" }
  | { type: "advisor_consulted"; baseline?: boolean };
/** Same-validation evidence. `unknown` means a fingerprint was missing, not that the commands matched. */
export type SameValidationEvidence = "true" | "unknown";
export interface RepairFailedObservation {
  sameValidation: SameValidationEvidence;
  repairMutations: number;
}
export interface AdaptiveTriggerState {
  taskKey: string;
  phase: TriggerPhase;
  lastFailedValidationFingerprint?: string;
  mutationsAfterFailure: number;
  consultationCount: number;
  baselineOnly: boolean;
  /** Set only when a follow-up validation failure closed a repair. Cleared on reset. */
  repairFailed?: RepairFailedObservation;
}
export type TriggerDecision = { action: "continue" } | {
  action: "consult";
  reason: AdvisorTriggerReason;
  evidence: string[];
};
