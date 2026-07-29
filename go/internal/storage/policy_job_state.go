package storage

import "sync"

// PolicyJobOutcome is what a finished cleanup-policy run produced
// (oracle: src/storage/policy-job.ts:24-33). Every field past `ok` is optional
// because a run can end by doing nothing, by deferring, or by failing, and each
// of those carries a different subset.
type PolicyJobOutcome struct {
	OK         bool   `json:"ok"`
	Skipped    string `json:"skipped,omitempty"`
	Deferred   string `json:"deferred,omitempty"`
	Error      string `json:"error,omitempty"`
	Mode       string `json:"mode,omitempty"`
	FreedBytes *int64 `json:"freedBytes,omitempty"`
	Removed    *int   `json:"removed,omitempty"`
	TrashDir   string `json:"trashDir,omitempty"`
}

// PolicyJobState is what the dashboard polls to learn whether a run is still
// going and how the last one ended (oracle: src/storage/policy-job.ts:35-41).
//
// The whole shape is declared now, before anything can move it, so that wiring
// the executor later adds behavior rather than fields. A response that grows new
// keys is indistinguishable from a contract that was wrong the first time.
type PolicyJobState struct {
	Status      string            `json:"status"`
	Reason      string            `json:"reason,omitempty"`
	StartedAt   *float64          `json:"startedAt,omitempty"`
	FinishedAt  *float64          `json:"finishedAt,omitempty"`
	LastError   string            `json:"lastError,omitempty"`
	LastOutcome *PolicyJobOutcome `json:"lastOutcome,omitempty"`
}

// PolicyJobIdle is the state before anything has run.
const PolicyJobIdle = "idle"

var policyJobMu sync.RWMutex
var policyJobState = PolicyJobState{Status: PolicyJobIdle}

// StorageCleanupPolicyJobState reports the current run state. Until the cleanup
// executor is wired there is genuinely nothing running, so this answers idle --
// which is the same answer the oracle gives before its first run.
func StorageCleanupPolicyJobState() PolicyJobState {
	policyJobMu.RLock()
	defer policyJobMu.RUnlock()
	state := policyJobState
	if state.LastOutcome != nil {
		outcome := *state.LastOutcome
		state.LastOutcome = &outcome
	}
	return state
}

// SetStorageCleanupPolicyJobState replaces the run state. The executor work-phase
// drives it; it exists now so the read path above has a real owner instead of a
// constant.
func SetStorageCleanupPolicyJobState(state PolicyJobState) {
	if state.Status == "" {
		state.Status = PolicyJobIdle
	}
	policyJobMu.Lock()
	defer policyJobMu.Unlock()
	policyJobState = state
}
