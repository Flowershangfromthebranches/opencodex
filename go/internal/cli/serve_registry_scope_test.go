package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lidge-jun/opencodex-go/internal/config"
	"github.com/lidge-jun/opencodex-go/internal/oauth"
)

// Moved down from cmd/ocx, where it used to point the canonical `openai`
// provider at a local httptest server and read the Authorization header off
// the wire. That endpoint is pinned now, so the assertion moves to the auth
// context the runtime actually resolves. The subject is unchanged: a legacy
// multi-account credential set must resolve through the canonical pool, and a
// thread must keep the account it was first given.
func TestLegacyOpenAIAccountsResolveThroughTheCanonicalPool(t *testing.T) {
	home := t.TempDir()
	store := oauth.NewCredentialStore(filepath.Join(home, "auth.json"))
	for index, token := range []string{"pool-token-one", "pool-token-two"} {
		credential := oauth.OAuthCredentials{
			Access: token, Refresh: "refresh", Expires: time.Now().Add(time.Hour).UnixMilli(),
			AccountID: fmt.Sprintf("pool-account-%d", index), Source: oauth.SourceOAuth,
		}
		if err := store.SaveNamedAccount(context.Background(), "openai", fmt.Sprintf("pool-%d", index), credential); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.FreshInstall()
	provider := cfg.Providers["openai"]
	provider.CodexAccountMode = "" // omitted mode defaults to pool for the canonical id
	cfg.Providers["openai"] = provider

	resolver, err := configuredAuthWithStore(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	first, err := resolver.ResolveAuth(context.Background(), "openai", "pool-thread-one")
	if err != nil {
		t.Fatalf("resolve pooled auth: %v", err)
	}
	if first.AccessToken == "" {
		t.Fatal("pooled selection returned no credential")
	}
	again, err := resolver.ResolveAuth(context.Background(), "openai", "pool-thread-one")
	if err != nil {
		t.Fatalf("resolve pooled auth again: %v", err)
	}
	if again.AccessToken != first.AccessToken {
		t.Fatalf("thread affinity broke: %q then %q", first.AccessToken, again.AccessToken)
	}
}

// The routing set is the set of providers the user configured. Seeding every
// built-in makes a provider nobody configured reachable: a bare `claude-*`
// selector finds the built-in Anthropic entry through the family fallback and
// leaves with whatever credential that entry resolves. The oracle filters to
// `activeProviderEntries` (src/router.ts:289) before any of this runs.
func TestConfiguredRegistryRoutesOnlyConfiguredProviders(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultProvider = "openrouter"
	cfg.Providers["openrouter"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://openrouter.ai/api/v1", AuthMode: "key",
		DefaultModel: "anthropic/claude-sonnet-5",
	}

	reg := configuredRegistry(cfg)

	if _, present := reg.Lookup("anthropic"); present {
		t.Fatal("built-in anthropic is routable although the user never configured it")
	}
	resolved, err := reg.ResolveModel("claude-sonnet-5")
	if err != nil {
		t.Fatalf("resolve bare claude selector: %v", err)
	}
	if resolved.Provider == "anthropic" {
		t.Fatalf("bare %q reached the unconfigured built-in anthropic provider", "claude-sonnet-5")
	}
	if resolved.Provider != "openrouter" {
		t.Fatalf("expected the configured default provider to answer, got %q", resolved.Provider)
	}
}

// A disabled provider is configured but must not route, exactly as the oracle's
// `provider.disabled !== true` filter decides (src/router.ts:289).
func TestConfiguredRegistryDropsDisabledProviders(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["openrouter"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "auto"}
	cfg.Providers["groq"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://api.groq.com/openai/v1", DefaultModel: "llama-3.3-70b", Disabled: true}

	reg := configuredRegistry(cfg)

	if _, present := reg.Lookup("groq"); present {
		t.Fatal("a disabled provider stayed in the routing set")
	}
	for _, entry := range reg.ListModels() {
		if entry.Provider == "groq" {
			t.Fatalf("disabled provider still advertises model %q", entry.ID)
		}
	}
}

// The legacy ids the migration deletes must never re-enter routing, matching
// the oracle's LEGACY_CHATGPT_PROVIDER_ID exclusion (src/router.ts:289).
func TestConfiguredRegistryDropsLegacyProviderIDs(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["chatgpt"] = config.ProviderConfig{Adapter: "openai-responses", BaseURL: "https://chatgpt.com/backend-api/codex", DefaultModel: "gpt-5.5"}
	cfg.Providers["openrouter"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "auto"}

	reg := configuredRegistry(cfg)

	if _, present := reg.Lookup("chatgpt"); present {
		t.Fatal("the legacy chatgpt id is routable")
	}
}

// A built-in's wire adapter and endpoint are canonical: a persisted config that
// names another endpoint must not redirect the request, and the adapter must
// stay the registry's. The oracle pins both (src/router.ts:231-248).
func TestConfiguredRegistryKeepsCanonicalAdapterAndEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["anthropic"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://evil.test/v1", AuthMode: "oauth", DefaultModel: "claude-sonnet-5",
	}

	entry, ok := configuredRegistry(cfg).Lookup("anthropic")
	if !ok {
		t.Fatal("configured anthropic vanished from the registry")
	}
	if entry.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("persisted baseURL redirected a pinned built-in: %q", entry.BaseURL)
	}
	if entry.Adapter != "anthropic" {
		t.Fatalf("persisted adapter overrode the canonical wire protocol: %q", entry.Adapter)
	}
}

// Entries that opt in — local runtimes and template URLs — keep the configured
// endpoint. This is the negative case that proves the pin is not blanket.
func TestConfiguredRegistryHonorsBaseURLOverrideOptIn(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["ollama"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "http://192.168.1.50:11434/v1", DefaultModel: "qwen3"}

	entry, ok := configuredRegistry(cfg).Lookup("ollama")
	if !ok {
		t.Fatal("configured ollama vanished from the registry")
	}
	if entry.BaseURL != "http://192.168.1.50:11434/v1" {
		t.Fatalf("an allowBaseUrlOverride provider lost its configured endpoint: %q", entry.BaseURL)
	}
}

// A provider the registry does not know keeps everything it was configured
// with; there is no canonical entry to prefer.
func TestConfiguredRegistryKeepsCustomProviderEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["my-gateway"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://gateway.internal/v1", DefaultModel: "house-model"}

	entry, ok := configuredRegistry(cfg).Lookup("my-gateway")
	if !ok {
		t.Fatal("a custom provider must stay routable")
	}
	if entry.BaseURL != "https://gateway.internal/v1" || entry.Adapter != "openai-chat" {
		t.Fatalf("custom provider was rewritten: adapter=%q baseURL=%q", entry.Adapter, entry.BaseURL)
	}
}

// Preset static headers are part of the wire contract, not a default the saved
// config replaces. opencode-free needs `x-opencode-client` on every request;
// a config without a headers map used to erase it.
func TestConfiguredRegistryMergesPresetStaticHeaders(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["opencode-free"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://opencode.ai/zen/v1", DefaultModel: "grok-code"}

	entry, ok := configuredRegistry(cfg).Lookup("opencode-free")
	if !ok {
		t.Fatal("configured opencode-free vanished from the registry")
	}
	if entry.StaticHeaders["x-opencode-client"] != "desktop" {
		t.Fatalf("preset static header was dropped: %#v", entry.StaticHeaders)
	}
}

// A configured header wins per key, so a user can still override one preset
// header without losing the rest.
func TestConfiguredRegistryLetsConfiguredHeadersWinPerKey(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["opencode-free"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://opencode.ai/zen/v1", DefaultModel: "grok-code",
		Headers: map[string]string{"x-opencode-client": "ocx", "x-trace": "on"},
	}

	entry, _ := configuredRegistry(cfg).Lookup("opencode-free")
	if entry.StaticHeaders["x-opencode-client"] != "ocx" {
		t.Fatalf("configured header did not win: %#v", entry.StaticHeaders)
	}
	if entry.StaticHeaders["x-trace"] != "on" {
		t.Fatalf("configured-only header was lost: %#v", entry.StaticHeaders)
	}
}
