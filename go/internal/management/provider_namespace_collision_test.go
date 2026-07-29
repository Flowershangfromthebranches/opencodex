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

// A provider and an account selector occupy one routing space: `alpha/model`
// has to mean exactly one thing. Admitting a provider named after a configured
// selector would make routing depend on lookup order, so the write is refused
// with 409 -- the request is well formed, it conflicts with existing state.
func TestAddingAProviderThatShadowsANamespaceIsRefused(t *testing.T) {
	cfg := config.Default()
	cfg.CodexAccountNamespaces = map[string]string{"alpha": "acct-1"}
	api := &API{config: &cfg, configPath: filepath.Join(t.TempDir(), "config.json")}

	body, _ := json.Marshal(map[string]any{"name": "alpha", "provider": map[string]any{
		"adapter": "openai-chat", "baseUrl": "https://api.alpha.test/v1", "authMode": "key", "apiKey": "k",
	}})
	request := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewReader(body))
	response := httptest.NewRecorder()
	if !api.handleProviders(response, request) {
		t.Fatal("provider route did not handle the request")
	}
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", response.Code, response.Body.String())
	}
	if _, added := cfg.Providers["alpha"]; added {
		t.Fatal("the colliding provider was stored anyway")
	}
}

// An unrelated name is unaffected, or the guard would block ordinary setup.
func TestAddingAnUnrelatedProviderStillWorks(t *testing.T) {
	cfg := config.Default()
	cfg.CodexAccountNamespaces = map[string]string{"alpha": "acct-1"}
	api := &API{config: &cfg, configPath: filepath.Join(t.TempDir(), "config.json")}

	body, _ := json.Marshal(map[string]any{"name": "beta", "provider": map[string]any{
		"adapter": "openai-chat", "baseUrl": "https://api.beta.test/v1", "authMode": "key", "apiKey": "k",
	}})
	request := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewReader(body))
	response := httptest.NewRecorder()
	api.handleProviders(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}
