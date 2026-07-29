package codex

import "testing"

// A selector and a provider id share one space, because `alpha/model` has to
// resolve to exactly one thing. Admitting the collision would make routing
// depend on lookup order.
func TestProviderNameCollidesWithANamespace(t *testing.T) {
	namespaces := map[string]string{"alpha": "acct-1"}
	if CodexAccountNamespaceProviderCollisionError(namespaces, "alpha") == "" {
		t.Fatal("a provider named after a namespace was allowed")
	}
	// Case-insensitive: a difference in case would still collide at routing.
	if CodexAccountNamespaceProviderCollisionError(namespaces, "ALPHA") == "" {
		t.Fatal("case-different collision was allowed")
	}
	if CodexAccountNamespaceProviderCollisionError(namespaces, "beta") != "" {
		t.Fatal("an unrelated provider was refused")
	}
}

func TestAccountIDCollidesWithANamespace(t *testing.T) {
	namespaces := map[string]string{"alpha": "acct-1"}
	if CodexAccountIDNamespaceCollisionError(namespaces, "alpha") == "" {
		t.Fatal("an account id equal to a selector was allowed")
	}
	if CodexAccountIDNamespaceCollisionError(namespaces, "acct-2") != "" {
		t.Fatal("an unrelated account id was refused")
	}
}

func TestNamespaceForModelMatchesOnlyConfiguredPrefixes(t *testing.T) {
	namespaces := map[string]string{"alpha": "acct-1"}
	if got := CodexAccountNamespaceForModel(namespaces, "alpha/gpt-5.5"); got != "alpha" {
		t.Fatalf("namespace = %q", got)
	}
	for _, model := range []string{"beta/gpt-5.5", "gpt-5.5", "/leading"} {
		if got := CodexAccountNamespaceForModel(namespaces, model); got != "" {
			t.Fatalf("%q matched %q", model, got)
		}
	}
}

// The Desktop account has two spellings; callers should never have to know
// both.
func TestNamespaceEntriesNormalizeTheDesktopTarget(t *testing.T) {
	entries := CodexAccountNamespaceEntries(map[string]string{
		"main": MainCodexAccountNamespaceTarget, "other": "acct-1",
	})
	if entries["main"] != MainCodexAccountID {
		t.Fatalf("main target = %q", entries["main"])
	}
	if entries["other"] != "acct-1" {
		t.Fatalf("pool target = %q", entries["other"])
	}
}

func TestAccountIDValidation(t *testing.T) {
	for _, valid := range []string{"acct-1", "a.b_c-9", "A1"} {
		if !IsValidCodexAccountID(valid) {
			t.Fatalf("%q was rejected", valid)
		}
	}
	// __main__ is the internal Desktop id and the three object keys have
	// special meaning in the JSON maps credentials live in.
	for _, invalid := range []string{"", "__main__", "__proto__", "PROTOTYPE", "constructor", "has spaces", "a/b"} {
		if IsValidCodexAccountID(invalid) {
			t.Fatalf("%q was accepted", invalid)
		}
	}
}

// Public selectors must never be derived from an account id or its id-derived
// fallback label: those are the PRIVATE identifiers, and exposing one as a
// routing prefix tells anyone reading a model string which account is which.
func TestDefaultNamespacesNeverExposePrivateIdentifiers(t *testing.T) {
	accounts := []CodexAccount{
		{ID: "secret-account-1"},
		{ID: "secret-account-2", LogLabel: "pab12cd"},
	}
	namespaces := DefaultCodexAccountNamespaces(NamespaceConfig{Accounts: accounts, ProviderIDs: []string{"anthropic"}})

	if len(namespaces) != 3 {
		t.Fatalf("expected main plus two accounts, got %#v", namespaces)
	}
	for namespace := range namespaces {
		for _, account := range accounts {
			if namespace == account.ID || namespace == FallbackCodexAccountLogLabel(account.ID) {
				t.Fatalf("selector %q exposes a private identifier", namespace)
			}
		}
	}
	// A persisted random log label IS a public selector and is reused as-is.
	if namespaces["pab12cd"] != "secret-account-2" {
		t.Fatalf("a persisted log label was not reused: %#v", namespaces)
	}
}

// A provider or combo alias already owns its prefix, so a generated selector
// steps aside rather than shadowing it.
func TestDefaultNamespacesAvoidOccupiedPrefixes(t *testing.T) {
	namespaces := DefaultCodexAccountNamespaces(NamespaceConfig{
		Accounts:     []CodexAccount{{ID: "acct-1"}},
		ProviderIDs:  []string{"main"},
		ComboAliases: []string{"main-2/model"},
	})
	if _, taken := namespaces["main"]; taken {
		t.Fatalf("a generated selector shadowed the provider named main: %#v", namespaces)
	}
	if _, taken := namespaces["main-2"]; taken {
		t.Fatalf("a generated selector shadowed the combo alias prefix: %#v", namespaces)
	}
	found := false
	for _, target := range namespaces {
		if target == MainCodexAccountNamespaceTarget {
			found = true
		}
	}
	if !found {
		t.Fatalf("the Desktop account lost its selector entirely: %#v", namespaces)
	}
}

// Appending must not rename or replace what a user already set.
func TestAppendLeavesExistingEntriesAlone(t *testing.T) {
	namespaces := map[string]string{"chosen": "acct-1"}
	added := AppendDefaultCodexAccountNamespace(namespaces, NamespaceConfig{
		Accounts: []CodexAccount{{ID: "acct-1"}, {ID: "acct-2"}},
	}, CodexAccount{ID: "acct-2"})
	if !added {
		t.Fatal("a new account was not added")
	}
	if namespaces["chosen"] != "acct-1" {
		t.Fatalf("an explicit entry was rewritten: %#v", namespaces)
	}
	if len(namespaces) != 2 {
		t.Fatalf("expected exactly one new entry: %#v", namespaces)
	}
}

func TestAppendRefusesDuplicatesAndInvalidAccounts(t *testing.T) {
	namespaces := map[string]string{"chosen": "acct-1"}
	config := NamespaceConfig{Accounts: []CodexAccount{{ID: "acct-1"}}}

	if AppendDefaultCodexAccountNamespace(namespaces, config, CodexAccount{ID: "acct-1"}) {
		t.Fatal("an account that already has a selector was added again")
	}
	if AppendDefaultCodexAccountNamespace(namespaces, config, CodexAccount{ID: "chosen"}) {
		t.Fatal("an account id colliding with a selector was added")
	}
	if AppendDefaultCodexAccountNamespace(namespaces, config, CodexAccount{ID: "acct-3", IsMain: true}) {
		t.Fatal("the Desktop account was given a pool selector")
	}
	if AppendDefaultCodexAccountNamespace(map[string]string{}, config, CodexAccount{ID: "acct-3"}) {
		t.Fatal("appending to an ungenerated map was allowed")
	}
}
