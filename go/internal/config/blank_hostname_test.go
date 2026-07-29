package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A hand-edited `"hostname": ""` must not cost the user their configuration.
// Go failed validation, then failed the repair pass on the same value, and
// returned an error -- so the proxy would not start at all, over a field the
// runtime already has a default for.
//
// The oracle degrades a blank hostname to undefined on READ for exactly this
// reason, documented at src/config.ts:669-675: the repair path merges in
// defaults, so a value that fails twice would reset providers and apiKeys,
// which is strictly worse than the bind bug the validation exists for.
func TestLoadDegradesBlankHostnameInsteadOfDiscardingTheConfig(t *testing.T) {
	path := writeConfigFile(t, `{
		"hostname": "",
		"port": 10999,
		"defaultProvider": "acme",
		"providers": {"acme": {"adapter": "openai-chat", "baseUrl": "https://api.acme.test/v1", "authMode": "key", "apiKey": "k"}}
	}`)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("a blank hostname made the whole config unloadable: %v", err)
	}
	if loaded.Host != DefaultHost {
		t.Fatalf("hostname = %q, want the loopback default", loaded.Host)
	}
	if loaded.Port != 10999 {
		t.Fatalf("port = %d — the user's settings were reset", loaded.Port)
	}
	if _, present := loaded.Providers["acme"]; !present {
		t.Fatalf("the user's providers were discarded: %#v", loaded.Providers)
	}
	if loaded.DefaultProvider != "acme" {
		t.Fatalf("defaultProvider = %q", loaded.DefaultProvider)
	}
}

// Degrading is a READ-time allowance. A live caller writing a blank hostname is
// told, because silently rewriting it to loopback would look like the bind
// succeeded on the address they asked for (oracle: src/config.ts:1176-1182).
func TestValidateStillRejectsABlankHostname(t *testing.T) {
	cfg := Default()
	cfg.Host = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("a blank hostname was accepted on the write path")
	}
}

// A whitespace-only hostname is the same case as an empty one.
func TestLoadDegradesWhitespaceHostname(t *testing.T) {
	path := writeConfigFile(t, `{"hostname": "   ", "defaultProvider": "acme", "providers": {"acme": {"adapter": "openai-chat", "baseUrl": "https://api.acme.test/v1", "authMode": "key", "apiKey": "k"}}}`)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("whitespace hostname failed to load: %v", err)
	}
	if loaded.Host != DefaultHost {
		t.Fatalf("hostname = %q", loaded.Host)
	}
}
