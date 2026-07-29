package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runModelsForTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	err := runModels(args, IO{Out: out, Err: out})
	return out.String(), err
}

// The five runtime verbs existed in the oracle and not in Go, so `ocx models
// live` answered "unknown models subcommand" — the CLI told the user the
// command did not exist rather than that the proxy was not running.
func TestModelsRuntimeVerbsAreRegistered(t *testing.T) {
	for _, verb := range []string{"live", "edit", "selected", "context", "shadow"} {
		_, err := runModelsForTest(t, verb)
		if err == nil {
			continue // reached the runtime path and succeeded
		}
		if strings.Contains(err.Error(), "unknown models subcommand") {
			t.Fatalf("%q is still unregistered: %v", verb, err)
		}
	}
}

// Argument validation runs before any network call, so these report a usage
// problem rather than "proxy is not running" — which would send someone off to
// start a proxy for a typo.
func TestModelsRuntimeArgumentValidation(t *testing.T) {
	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{"edit without id", []string{"edit"}, "custom model id is required"},
		{"edit without changes", []string{"edit", "custom-1"}, "at least one field to change is required"},
		{"edit negative context", []string{"edit", "custom-1", "--context-window", "-5"}, "non-negative integer"},
		{"selected without provider", []string{"selected"}, "provider is required"},
		{"selected set and clear", []string{"selected", "acme", "--set", "a", "--clear"}, "cannot be combined"},
		{"context unknown action", []string{"context", "sideways"}, "unknown context action"},
		{"context value not a number", []string{"context", "value", "abc"}, "positive integer"},
		{"context value zero", []string{"context", "value", "0"}, "positive integer"},
		{"context provider without state", []string{"context", "provider", "acme"}, "on|off are required"},
		{"context all without state", []string{"context", "all"}, "requires on|off"},
		{"shadow unknown action", []string{"shadow", "sideways"}, "unknown shadow action"},
		{"shadow set without fields", []string{"shadow", "set"}, "model and/or --enabled is required"},
		{"shadow bad enabled", []string{"shadow", "set", "--enabled", "maybe"}, "must be on or off"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := runModelsForTest(t, testCase.args...)
			if err == nil {
				t.Fatalf("%v was accepted", testCase.args)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %q, want it to mention %q", err, testCase.want)
			}
		})
	}
}

// A comma list drops blanks rather than sending empty model ids upstream.
func TestSplitCSVIgnoresBlanks(t *testing.T) {
	got := splitCSV(" a , ,b,, c ")
	if strings.Join(got, "|") != "a|b|c" {
		t.Fatalf("splitCSV = %#v", got)
	}
}
