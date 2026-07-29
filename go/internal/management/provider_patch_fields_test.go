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

func patchProvider(t *testing.T, api *API, name string, patch map[string]any) (int, string) {
	t.Helper()
	encoded, _ := json.Marshal(patch)
	request := httptest.NewRequest(http.MethodPatch, "/api/providers?name="+name, bytes.NewReader(encoded))
	response := httptest.NewRecorder()
	if !api.handleProviders(response, request) {
		t.Fatal("provider route did not handle the request")
	}
	return response.Code, response.Body.String()
}

func patchAPI(t *testing.T) (*API, *config.Config) {
	t.Helper()
	cfg := config.Default()
	// apiKeyTransport is only meaningful on the anthropic wire, and config
	// validation enforces that, so the fixture uses an anthropic provider.
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "anthropic", BaseURL: "https://api.acme.test/v1", AuthMode: "key", APIKey: "k", DefaultModel: "m",
	}
	return &API{config: &cfg, configPath: filepath.Join(t.TempDir(), "config.json")}, &cfg
}

// The dashboard's provider editor sends these fields; Go rejected them as "not
// patchable", so the edits silently failed. The oracle accepts all three
// (src/server/management/provider-routes.ts:223-246).
func TestProviderPatchAcceptsTransportAndNote(t *testing.T) {
	api, cfg := patchAPI(t)

	if status, body := patchProvider(t, api, "acme", map[string]any{"apiKeyTransport": "x-api-key"}); status != http.StatusOK {
		t.Fatalf("apiKeyTransport patch status=%d body=%s", status, body)
	}
	if got := cfg.Providers["acme"].APIKeyTransport; got != "x-api-key" {
		t.Fatalf("apiKeyTransport = %q", got)
	}

	if status, body := patchProvider(t, api, "acme", map[string]any{"note": "billing account B"}); status != http.StatusOK {
		t.Fatalf("note patch status=%d body=%s", status, body)
	}
	if got := cfg.Providers["acme"].Note; got != "billing account B" {
		t.Fatalf("note = %q", got)
	}
}

// Empty string clears rather than storing an empty value, matching the oracle's
// delete-on-empty behaviour.
func TestProviderPatchClearsTransportAndNoteOnEmptyString(t *testing.T) {
	api, cfg := patchAPI(t)
	provider := cfg.Providers["acme"]
	provider.APIKeyTransport, provider.Note = "bearer", "old note"
	cfg.Providers["acme"] = provider

	patchProvider(t, api, "acme", map[string]any{"apiKeyTransport": ""})
	patchProvider(t, api, "acme", map[string]any{"note": "   "})

	if got := cfg.Providers["acme"].APIKeyTransport; got != "" {
		t.Fatalf("apiKeyTransport was not cleared: %q", got)
	}
	if got := cfg.Providers["acme"].Note; got != "" {
		t.Fatalf("note was not cleared: %q", got)
	}
}

// An unknown transport is still rejected, so accepting the field did not turn
// it into a free-text slot.
func TestProviderPatchRejectsUnknownTransport(t *testing.T) {
	api, _ := patchAPI(t)
	if status, _ := patchProvider(t, api, "acme", map[string]any{"apiKeyTransport": "carrier-pigeon"}); status != http.StatusBadRequest {
		t.Fatalf("unknown transport status = %d, want 400", status)
	}
}

// The DTO is the dashboard's only view of a provider. Omitting these fields
// meant the editor could not show what was configured -- it rendered defaults
// and then wrote them back on save.
func TestProviderListExposesTransportAndNote(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "anthropic", BaseURL: "https://api.acme.test/v1", AuthMode: "key", APIKey: "k",
		APIKeyTransport: "x-api-key", Note: "billing account B",
	}
	api := &API{config: &cfg}

	request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	response := httptest.NewRecorder()
	if !api.handleProviders(response, request) {
		t.Fatal("provider route did not handle the request")
	}
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatalf("provider list is not JSON: %s", response.Body.String())
	}
	var acme map[string]any
	for _, row := range rows {
		if row["name"] == "acme" {
			acme = row
		}
	}
	if acme == nil {
		t.Fatalf("acme missing from the provider list: %s", response.Body.String())
	}
	if acme["apiKeyTransport"] != "x-api-key" {
		t.Fatalf("apiKeyTransport missing from the DTO: %#v", acme)
	}
	if acme["note"] != "billing account B" {
		t.Fatalf("note missing from the DTO: %#v", acme)
	}
}
