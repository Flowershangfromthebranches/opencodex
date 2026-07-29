package cli

import (
	"testing"
)

// "-" clears a field, which is different from omitting the flag. Losing that
// distinction would make a field impossible to unset from the CLI. The route
// type-checks these as strings, so clearing sends "" rather than null.
func TestProviderEditPatchClearsWithADash(t *testing.T) {
	patch, err := providerEditPatch([]string{"--default-model", "-", "--auth-mode", "-"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"defaultModel", "authMode"} {
		value, present := patch[key]
		if !present {
			t.Fatalf("%s missing from the patch: %v", key, patch)
		}
		if value != "" {
			t.Fatalf("%s = %v, want an empty string so the field is cleared", key, value)
		}
	}
}

// Every flag this command offers must be a field the route actually accepts;
// otherwise the CLI advertises an edit that always fails with 400.
func TestProviderEditPatchOnlyUsesPatchableKeys(t *testing.T) {
	patch, err := providerEditPatch([]string{
		"--adapter", "openai-chat", "--base-url", "https://example.invalid/v1",
		"--default-model", "m", "--auth-mode", "key",
		"--enabled", "true", "--live-models", "true", "--allow-private-network", "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	patchable := map[string]bool{
		"adapter": true, "baseUrl": true, "defaultModel": true,
		"authMode": true, "disabled": true, "liveModels": true, "allowPrivateNetwork": true,
	}
	for key := range patch {
		if !patchable[key] {
			t.Fatalf("%q is not accepted by PATCH /api/providers", key)
		}
	}
}

// An omitted flag must not appear at all, or the patch would overwrite fields
// the user never mentioned.
func TestProviderEditPatchOmitsUntouchedFields(t *testing.T) {
	patch, err := providerEditPatch([]string{"--adapter", "openai-chat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(patch) != 1 || patch["adapter"] != "openai-chat" {
		t.Fatalf("patch = %v, want only the adapter", patch)
	}
}

// --enabled is the user-facing verb; the wire field is its inverse.
func TestProviderEditPatchInvertsEnabledIntoDisabled(t *testing.T) {
	enabled, err := providerEditPatch([]string{"--enabled", "false"})
	if err != nil {
		t.Fatal(err)
	}
	if enabled["disabled"] != true {
		t.Fatalf("--enabled false -> %v, want disabled true", enabled)
	}

	reenabled, err := providerEditPatch([]string{"--enabled", "true"})
	if err != nil {
		t.Fatal(err)
	}
	if reenabled["disabled"] != false {
		t.Fatalf("--enabled true -> %v, want disabled false", reenabled)
	}
}

func TestProviderEditPatchRejectsBadInput(t *testing.T) {
	if _, err := providerEditPatch([]string{"--enabled", "maybe"}); err == nil {
		t.Fatal("accepted a non-boolean --enabled")
	}
	if _, err := providerEditPatch([]string{"--adapter"}); err == nil {
		t.Fatal("accepted --adapter without a value")
	}
	if _, err := providerEditPatch([]string{"--unknown-flag"}); err == nil {
		t.Fatal("accepted an unknown flag")
	}
}
