package config

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

var bracedEnvironmentReference = regexp.MustCompile(`^\$\{(\w+)\}$`)

// ResolveEnvValue resolves the same whole-value references accepted by the
// TypeScript runtime. Missing variables resolve to the empty string.
func ResolveEnvValue(value string) string {
	if value == "" {
		return ""
	}
	if match := bracedEnvironmentReference.FindStringSubmatch(value); match != nil {
		return os.Getenv(match[1])
	}
	if strings.HasPrefix(value, "$") {
		return os.Getenv(value[1:])
	}
	return value
}

// ResolveEnvironment returns a runtime-only copy with environment references
// expanded. Callers should retain and persist the original Config so resolved
// credentials are never written back to disk.
//
// Only credential-bearing values are expanded: each provider's apiKey, its
// pooled keys, and the proxy URL. The oracle resolves exactly these, at the
// point of use (src/router.ts:189, src/config.ts:1562,
// src/providers/quota.ts:553).
//
// Substituting every string in the document instead — which is what the JSON
// round-trip below used to do — silently destroys any literal value that
// happens to begin with `$`: an undefined variable resolves to "", so a header
// like `X-Billing-Tag: $team-alpha` or a model id starting with `$` simply
// vanishes from the request.
func ResolveEnvironment(cfg Config) (Config, error) {
	// Deep-copied through JSON so the caller's Config — the one that gets
	// persisted — never sees a resolved credential.
	data, err := json.Marshal(cfg)
	if err != nil {
		return Config{}, err
	}
	var resolved Config
	if err := json.Unmarshal(data, &resolved); err != nil {
		return Config{}, err
	}
	resolved.Proxy = ResolveEnvValue(resolved.Proxy)
	// authToken has no oracle counterpart (the TS proxy reads its admission
	// token from the environment directly), but it is a credential by the same
	// definition, and Load deliberately keeps the reference unexpanded on disk.
	resolved.AuthToken = ResolveEnvValue(resolved.AuthToken)
	for index := range resolved.APIKeys {
		resolved.APIKeys[index].Key = ResolveEnvValue(resolved.APIKeys[index].Key)
	}
	for name, provider := range resolved.Providers {
		provider.APIKey = ResolveEnvValue(provider.APIKey)
		for index := range provider.APIKeyPool {
			provider.APIKeyPool[index].Key = ResolveEnvValue(provider.APIKeyPool[index].Key)
		}
		resolved.Providers[name] = provider
	}
	return resolved, nil
}

// ApplyProxyEnv mirrors config.proxy into the conventional proxy variables.
// Existing user values win and loopback destinations always bypass the proxy.
func ApplyProxyEnv(cfg Config) {
	proxy := ResolveEnvValue(cfg.Proxy)
	if proxy == "" {
		return
	}
	if strings.TrimSpace(os.Getenv("HTTP_PROXY")) == "" && strings.TrimSpace(os.Getenv("http_proxy")) == "" {
		_ = os.Setenv("HTTP_PROXY", proxy)
	}
	if strings.TrimSpace(os.Getenv("HTTPS_PROXY")) == "" && strings.TrimSpace(os.Getenv("https_proxy")) == "" {
		_ = os.Setenv("HTTPS_PROXY", proxy)
	}
	existing := os.Getenv("NO_PROXY")
	if existing == "" {
		existing = os.Getenv("no_proxy")
	}
	entries := make([]string, 0)
	seen := make(map[string]bool)
	for _, entry := range strings.Split(existing, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		entries = append(entries, entry)
		seen[strings.ToLower(entry)] = true
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "[::1]"} {
		if !seen[strings.ToLower(host)] {
			entries = append(entries, host)
			seen[strings.ToLower(host)] = true
		}
	}
	_ = os.Setenv("NO_PROXY", strings.Join(entries, ","))
}
