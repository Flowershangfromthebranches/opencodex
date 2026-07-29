package management

import "strings"

// subagentDefaultSyncEffective reports whether the injection model is actually
// being mirrored into Codex's subagent defaults.
//
// The stored flag alone is not the answer: mirroring needs a model to mirror,
// so a config with the flag on and no injection model is not syncing anything.
// Reporting the raw flag would show the dashboard a toggle that claims to be
// doing something it cannot. The oracle computes it the same way
// (src/config.ts:1152-1156).
func subagentDefaultSyncEffective(settings AgentSettings) bool {
	return settings.SyncSubagentDefaults != nil && *settings.SyncSubagentDefaults &&
		strings.TrimSpace(settings.InjectionModel) != ""
}
