package config

import (
	"testing"
)

// `$NAME` means "read this from the environment" for the few values that are
// meant to be credentials -- a provider's apiKey and the proxy URL. The oracle
// resolves exactly those, at the point of use (src/router.ts:189,
// src/config.ts:1562, src/providers/quota.ts:553).
//
// Go instead round-tripped the whole config through JSON and substituted EVERY
// string, so any literal value that happens to start with `$` is replaced by
// the empty string of an undefined variable. A header used for routing or
// billing silently disappears, and the request goes out without it.
func TestResolveEnvironmentLeavesNonCredentialStringsAlone(t *testing.T) {
	t.Setenv("OCX_TEST_KEY_SCOPE", "resolved-secret")
	cfg := Default()
	cfg.Providers["custom"] = ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://api.example.test/v1", AuthMode: "key",
		APIKey:       "$OCX_TEST_KEY_SCOPE",
		DefaultModel: "$special-model",
		Models:       []string{"$special-model"},
		Headers:      map[string]string{"X-Billing-Tag": "$team-alpha"},
	}

	resolved, err := ResolveEnvironment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	provider := resolved.Providers["custom"]

	if provider.APIKey != "resolved-secret" {
		t.Fatalf("apiKey was not resolved from the environment: %q", provider.APIKey)
	}
	if provider.Headers["X-Billing-Tag"] != "$team-alpha" {
		t.Fatalf("a literal header value was eaten by env substitution: %q", provider.Headers["X-Billing-Tag"])
	}
	if provider.DefaultModel != "$special-model" {
		t.Fatalf("a literal model id was eaten by env substitution: %q", provider.DefaultModel)
	}
	if len(provider.Models) != 1 || provider.Models[0] != "$special-model" {
		t.Fatalf("a literal model list entry was eaten by env substitution: %#v", provider.Models)
	}
}

// The proxy is the other value the oracle resolves.
func TestResolveEnvironmentResolvesProxy(t *testing.T) {
	t.Setenv("OCX_TEST_PROXY_SCOPE", "http://proxy.example.test:8080")
	cfg := Default()
	cfg.Proxy = "$OCX_TEST_PROXY_SCOPE"

	resolved, err := ResolveEnvironment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Proxy != "http://proxy.example.test:8080" {
		t.Fatalf("proxy = %q", resolved.Proxy)
	}
}

// Pooled keys are credentials too, so they resolve like the primary one.
func TestResolveEnvironmentResolvesPooledKeys(t *testing.T) {
	t.Setenv("OCX_TEST_POOL_SCOPE", "pooled-secret")
	cfg := Default()
	cfg.Providers["custom"] = ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://api.example.test/v1", AuthMode: "key",
		APIKey:     "plain",
		APIKeyPool: []APIKeyEntry{{ID: "one", Key: "$OCX_TEST_POOL_SCOPE"}},
	}

	resolved, err := ResolveEnvironment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Providers["custom"].APIKeyPool[0].Key; got != "pooled-secret" {
		t.Fatalf("pooled key = %q", got)
	}
}
