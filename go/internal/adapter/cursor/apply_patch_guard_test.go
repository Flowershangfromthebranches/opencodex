package cursor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

func applyPatchTool() types.Tool {
	return types.Tool{Name: ApplyPatchTool, Freeform: true}
}

// When apply_patch is on the table, a native write bypasses everything Codex
// exists to do for a file edit: approval, sandbox policy, the diff the user
// sees, and the rollout record. So it is refused, and the refusal names the
// alternative rather than reporting a permission failure -- the model should
// reroute, not conclude the filesystem is unavailable.
func TestNativeWriteIsRefusedWhenApplyPatchIsAvailable(t *testing.T) {
	executor := &FilesystemExecutor{}
	_, err := executor.writeWithPolicy(WriteRequest{Path: "/tmp/x", Content: []byte("data")}, true)
	if err == nil {
		t.Fatal("a native write went through while apply_patch was available")
	}
	if !strings.Contains(err.Error(), "apply_patch") {
		t.Fatalf("refusal does not point at the alternative: %v", err)
	}

	if _, err := executor.deleteWithPolicy(DeleteRequest{Path: "/tmp/x"}, true); err == nil {
		t.Fatal("a native delete went through while apply_patch was available")
	}
}

// Without apply_patch there is nothing to route through, so native mutation
// stays available and only the ordinary policy applies.
func TestNativeMutationStillWorksWithoutApplyPatch(t *testing.T) {
	executor := &FilesystemExecutor{}
	_, err := executor.writeWithPolicy(WriteRequest{Path: "/tmp/x", Content: []byte("data")}, false)
	if err != nil && strings.Contains(err.Error(), "apply_patch") {
		t.Fatalf("the guard fired with no apply_patch advertised: %v", err)
	}
}

// All three conditions matter: top-level, freeform, and permitted by
// tool_choice. A namespaced lookalike or a plain function of the same name is a
// different tool, and one the model cannot select is no alternative at all.
func TestApplyPatchDetectionRequiresAllThreeConditions(t *testing.T) {
	auto := ToolChoice{Mode: "auto"}
	if !RequestAdvertisesApplyPatch([]types.Tool{applyPatchTool()}, auto) {
		t.Fatal("a plain advertised apply_patch was not detected")
	}
	for _, tool := range []types.Tool{
		{Name: ApplyPatchTool, Freeform: true, Namespace: "mcp"},
		{Name: ApplyPatchTool},
		{Name: "apply_patch_v2", Freeform: true},
	} {
		if RequestAdvertisesApplyPatch([]types.Tool{tool}, auto) {
			t.Fatalf("%#v was mistaken for Codex apply_patch", tool)
		}
	}
}

func TestToolChoiceGatesApplyPatchDetection(t *testing.T) {
	tools := []types.Tool{applyPatchTool()}
	for _, testCase := range []struct {
		name   string
		choice ToolChoice
		want   bool
	}{
		{"auto", ToolChoice{Mode: "auto"}, true},
		{"none", ToolChoice{Mode: "none"}, false},
		{"forced elsewhere", ToolChoice{Mode: "function", Name: "shell"}, false},
		{"forced here", ToolChoice{Mode: "function", Name: ApplyPatchTool}, true},
		{"allowed list includes it", ToolChoice{Mode: "allowed", AllowedTools: []string{ApplyPatchTool}}, true},
		{"allowed list excludes it", ToolChoice{Mode: "allowed", AllowedTools: []string{"shell"}}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := RequestAdvertisesApplyPatch(tools, testCase.choice); got != testCase.want {
				t.Fatalf("detected=%v want=%v", got, testCase.want)
			}
		})
	}
}

// The flag is request-local because one executor serves many turns: a turn that
// advertises apply_patch must not disarm native writes for the next one.
func TestRejectionIsDecidedPerRequest(t *testing.T) {
	withPatch := &types.NormalizedRequest{
		ModelID: "auto",
		Context: types.RequestContext{Tools: []types.Tool{applyPatchTool()}},
		Options: types.RequestOptions{ToolChoice: json.RawMessage(`"auto"`)},
	}
	without := &types.NormalizedRequest{ModelID: "auto"}

	if !RequestAdvertisesApplyPatch(withPatch.Context.Tools, ParseToolChoice(withPatch.Options.ToolChoice)) {
		t.Fatal("apply_patch turn did not arm the guard")
	}
	if RequestAdvertisesApplyPatch(without.Context.Tools, ParseToolChoice(without.Options.ToolChoice)) {
		t.Fatal("a turn without apply_patch inherited the guard")
	}
}
