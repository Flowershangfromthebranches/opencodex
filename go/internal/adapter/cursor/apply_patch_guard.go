package cursor

import (
	"fmt"
	"strings"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// NativeMutationRefusal is what the model is told when it tries to write or
// delete a file directly while apply_patch is on the table.
//
// It names the alternative rather than reporting a permission failure: the
// model should reroute, not conclude the filesystem is unavailable and give up.
func NativeMutationRefusal(operation string) string {
	return fmt.Sprintf("Cursor-native %s is disabled for this Codex request because apply_patch is available. "+
		"Use the apply_patch tool for file edits so Codex can approve the change, enforce sandbox policy, "+
		"show diffs, and record rollout. No file was changed.", operation)
}

// RequestAdvertisesApplyPatch reports whether this request offers apply_patch as
// a callable tool.
//
// All three conditions matter. It must be the top-level tool (a namespaced
// lookalike is a different tool), it must be freeform (Codex's apply_patch is a
// custom tool; a plain function of the same name is something else), and
// tool_choice must actually permit it — an apply_patch the model cannot select
// is not an alternative to anything.
func RequestAdvertisesApplyPatch(tools []types.Tool, choice ToolChoice) bool {
	for _, tool := range tools {
		if tool.Namespace != "" || tool.Name != ApplyPatchTool || !tool.Freeform {
			continue
		}
		if toolAllowedByChoice(tool.Name, choice) {
			return true
		}
	}
	return false
}

func toolAllowedByChoice(name string, choice ToolChoice) bool {
	switch choice.Mode {
	case "none":
		return false
	case "required", "function", "tool":
		// A forced single tool excludes everything else, including this one.
		return choice.Name == "" || choice.Name == name
	case "allowed":
		if len(choice.AllowedTools) == 0 {
			return true
		}
		for _, allowed := range choice.AllowedTools {
			if allowed == name || strings.HasSuffix(allowed, "_"+name) {
				return true
			}
		}
		return false
	default:
		return true
	}
}
