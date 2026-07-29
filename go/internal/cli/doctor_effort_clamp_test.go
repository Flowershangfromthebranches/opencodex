package cli

import (
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/codex"
)

// Reasoning efforts disappear during catalog sync when the selected Codex
// runtime cannot serve them. Without a diagnostic, a user who asks for xhigh
// and quietly gets high has no way to find out why -- the request simply
// behaves differently than they configured.
func TestDoctorReportsRemovedEfforts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENCODEX_HOME", home)
	options := codex.ResolveCodexRuntimeOptions{ConfigDir: home}
	if err := codex.PersistEffortClamp(&codex.EffortClampDiagnostic{
		RemovedEfforts: []string{"xhigh", "max"},
	}, options); err != nil {
		t.Fatal(err)
	}

	report := &doctorReport{}
	appendCodexRuntimeUpgradeChecks(report)

	var found *doctorCheck
	for index := range report.Checks {
		if report.Checks[index].Name == "Codex effort clamp" {
			found = &report.Checks[index]
		}
	}
	if found == nil {
		t.Fatalf("the clamp was never reported: %#v", report.Checks)
	}
	if !strings.Contains(found.Detail, "xhigh") || !strings.Contains(found.Detail, "max") {
		t.Fatalf("detail does not name the removed efforts: %q", found.Detail)
	}
	if found.Hint == "" {
		t.Fatal("no hint told the user how to recover the efforts")
	}
}

// Nothing removed means nothing to report: a clean machine should not carry a
// permanent warning row.
func TestDoctorStaysQuietWithoutAClamp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENCODEX_HOME", home)

	report := &doctorReport{}
	appendCodexRuntimeUpgradeChecks(report)

	for _, check := range report.Checks {
		if check.Name == "Codex effort clamp" {
			t.Fatalf("a clamp row appeared with no clamp recorded: %#v", check)
		}
	}
}
