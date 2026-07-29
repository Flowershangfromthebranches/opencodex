package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

// Pool entries are keyed by a hash of the key itself, which is how the oracle
// recognises a resubmitted key as one it already has (apiKeyPoolEntryId,
// src/providers/api-keys.ts:89). Fixtures use the real id for that reason.
func poolAPI(t *testing.T, cfg *config.Config) *API {
	t.Helper()
	return &API{config: cfg, configPath: filepath.Join(t.TempDir(), "config.json")}
}

func postProvider(t *testing.T, api *API, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewReader(encoded))
	response := httptest.NewRecorder()
	if !api.handleProviders(response, request) {
		t.Fatal("provider route did not handle the request")
	}
	return response
}

// Editing a provider from the dashboard resubmits the form, which has no field
// for the fallback key pool. Replacing the stored entry wholesale therefore
// deletes every pooled key: the next rate limit has nothing left to rotate to,
// and the keys are gone from disk with no undo. The oracle carries the existing
// pool over and lets the submitted key join it
// (src/server/management/provider-routes.ts:120-129).
func TestProviderOverwriteKeepsTheAPIKeyPool(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://api.acme.test/v1", AuthMode: "key",
		APIKey: "key-one", DefaultModel: "m",
		APIKeyPool: []config.APIKeyEntry{{ID: config.APIKeyID("key-one"), Key: "key-one"}, {ID: config.APIKeyID("key-two"), Key: "key-two"}},
	}
	api := poolAPI(t, &cfg)

	response := postProvider(t, api, map[string]any{"name": "acme", "provider": map[string]any{
		"adapter": "openai-chat", "baseUrl": "https://api.acme.test/v1", "authMode": "key",
		"apiKey": "key-one", "defaultModel": "m",
	}})
	if response.Code != http.StatusOK {
		t.Fatalf("provider save status=%d body=%s", response.Code, response.Body.String())
	}

	if pool := cfg.Providers["acme"].APIKeyPool; len(pool) != 2 {
		t.Fatalf("the fallback key pool was destroyed by an edit: %#v", pool)
	}
}

// A new key submitted with the edit joins the pool as the active entry rather
// than replacing it.
func TestProviderOverwriteAddsTheSubmittedKeyToThePool(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://api.acme.test/v1", AuthMode: "key",
		APIKey: "key-one", DefaultModel: "m",
		APIKeyPool: []config.APIKeyEntry{{ID: config.APIKeyID("key-one"), Key: "key-one"}},
	}
	api := poolAPI(t, &cfg)

	postProvider(t, api, map[string]any{"name": "acme", "provider": map[string]any{
		"adapter": "openai-chat", "baseUrl": "https://api.acme.test/v1", "authMode": "key",
		"apiKey": "key-rotated", "defaultModel": "m",
	}})

	provider := cfg.Providers["acme"]
	if provider.APIKey != "key-rotated" {
		t.Fatalf("active key = %q", provider.APIKey)
	}
	var keys []string
	for _, entry := range provider.APIKeyPool {
		keys = append(keys, entry.Key)
	}
	if len(keys) != 2 {
		t.Fatalf("submitted key did not join the pool: %#v", keys)
	}
}

// An explicitly submitted pool still wins, so a user can genuinely replace it.
func TestProviderOverwriteHonorsAnExplicitPool(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "https://api.acme.test/v1", AuthMode: "key",
		APIKey: "key-one", DefaultModel: "m",
		APIKeyPool: []config.APIKeyEntry{{ID: config.APIKeyID("key-one"), Key: "key-one"}, {ID: config.APIKeyID("key-two"), Key: "key-two"}},
	}
	api := poolAPI(t, &cfg)

	postProvider(t, api, map[string]any{"name": "acme", "provider": map[string]any{
		"adapter": "openai-chat", "baseUrl": "https://api.acme.test/v1", "authMode": "key",
		"apiKey": "key-three", "defaultModel": "m",
		"apiKeyPool": []map[string]any{{"id": config.APIKeyID("key-three"), "key": "key-three"}},
	}})

	pool := cfg.Providers["acme"].APIKeyPool
	if len(pool) != 1 || pool[0].Key != "key-three" {
		t.Fatalf("an explicit pool replacement was overruled: %#v", pool)
	}
}
