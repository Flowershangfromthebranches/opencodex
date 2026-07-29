package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func runProviderForTest(t *testing.T, args ...string) error {
	t.Helper()
	out := &bytes.Buffer{}
	return runProvider(args, IO{Out: out, Err: out})
}

// These verbs existed in the oracle and nowhere in Go, so scripting a quota
// read or a mode switch against the Go runtime meant curl.
func TestProviderRuntimeVerbsAreRegistered(t *testing.T) {
	for _, args := range [][]string{
		{"quota"},
		{"presets"},
		{"account-mode", "pool"},
		{"selected", "acme"},
	} {
		err := runProviderForTest(t, args...)
		if err != nil && strings.Contains(err.Error(), "unknown provider subcommand") {
			t.Fatalf("%v is still unregistered: %v", args, err)
		}
	}
}

// Validation runs before the network call, so a bad mode reports the usage
// problem rather than sending someone to start a proxy they do not need.
func TestProviderAccountModeValidation(t *testing.T) {
	for _, args := range [][]string{
		{"account-mode"},
		{"account-mode", "sideways"},
		{"account-mode", "pool", "direct"},
	} {
		err := runProviderForTest(t, args...)
		if err == nil || !strings.Contains(err.Error(), "pool or direct") {
			t.Fatalf("%v error = %v, want a mode usage error", args, err)
		}
	}
}

func TestProviderQuotaRejectsStrayArguments(t *testing.T) {
	err := runProviderForTest(t, "quota", "extra")
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("error = %v", err)
	}
}

// `ocx doctor --fix-codex-runtime` reports what it did rather than editing
// config silently. With no newer runtime available it says so and names the
// current selection, which is the common case on a healthy machine.
func TestDoctorFixCodexRuntimeReportsItsOutcome(t *testing.T) {
	out := &bytes.Buffer{}
	if err := runDoctor(context.Background(), []string{"--fix-codex-runtime"}, IO{Out: out, Err: out}); err != nil {
		t.Fatalf("doctor --fix-codex-runtime: %v", err)
	}
	printed := out.String()
	if !strings.Contains(printed, "Codex runtime") && !strings.Contains(printed, "Selected:") {
		t.Fatalf("no runtime outcome was reported:\n%s", printed)
	}
}

func TestDoctorRejectsUnknownFlags(t *testing.T) {
	out := &bytes.Buffer{}
	err := runDoctor(context.Background(), []string{"--nope"}, IO{Out: out, Err: out})
	if err == nil || !strings.Contains(err.Error(), "--fix-codex-runtime") {
		t.Fatalf("usage error = %v, want it to advertise the new flag", err)
	}
}
