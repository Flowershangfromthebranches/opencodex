package config

import "testing"

func namespaceConfig(namespaces map[string]string, accounts ...CodexAccount) Config {
	cfg := Default()
	cfg.Providers["acme"] = ProviderConfig{Adapter: "openai-chat", BaseURL: "https://api.acme.test/v1", AuthMode: "key", APIKey: "k"}
	cfg.CodexAccountNamespaces = namespaces
	cfg.CodexAccounts = accounts
	return cfg
}

// A selector shares its space with provider ids, combo aliases, account ids and
// other selectors' targets, because `alpha/model` has to resolve to exactly one
// thing. Each collision is refused at config load rather than becoming
// order-dependent routing at request time.
func TestNamespaceCollisionsAreRefused(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		namespaces map[string]string
		accounts   []CodexAccount
	}{
		{"provider name", map[string]string{"acme": "acct-1"}, nil},
		{"combo namespace", map[string]string{"combo": "acct-1"}, nil},
		{"canonical openai", map[string]string{"openai": "acct-1"}, nil},
		{"configured account id", map[string]string{"acct-1": "acct-2"}, []CodexAccount{{ID: "acct-1", Email: "a@example.test"}}},
		{"another selector target", map[string]string{"alpha": "acct-1", "acct-1": "acct-2"}, nil},
		{"reserved object key", map[string]string{"__proto__": "acct-1"}, nil},
		{"invalid selector shape", map[string]string{"has space": "acct-1"}, nil},
		{"invalid target", map[string]string{"alpha": "not a valid id"}, nil},
		{"internal desktop id as target", map[string]string{"alpha": "__main__"}, nil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := namespaceConfig(testCase.namespaces, testCase.accounts...)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("%#v was accepted", testCase.namespaces)
			}
		})
	}
}

// The shapes that must keep working: the Desktop sentinel and an ordinary pool
// selector.
func TestValidNamespacesAreAccepted(t *testing.T) {
	cfg := namespaceConfig(map[string]string{
		"main":  "@main",
		"alpha": "acct-1",
	}, CodexAccount{ID: "acct-1", Email: "a@example.test"})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a valid namespace map was rejected: %v", err)
	}
}

// An absent map is the common case and must not be treated as an empty one that
// fails some emptiness rule.
func TestNoNamespacesIsValid(t *testing.T) {
	cfg := namespaceConfig(nil)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a config without namespaces was rejected: %v", err)
	}
}
