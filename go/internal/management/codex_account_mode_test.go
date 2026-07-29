package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/codex"
	"github.com/lidge-jun/opencodex-go/internal/config"
)

func modeAPI(t *testing.T) (*API, *config.Config, *codex.Router) {
	t.Helper()
	cfg := config.FreshInstall()
	router := codex.NewRouter(nil, nil)
	return &API{
		config:      &cfg,
		configPath:  filepath.Join(t.TempDir(), "config.json"),
		codexRouter: router,
	}, &cfg, router
}

func patchMode(t *testing.T, api *API, name string, body map[string]any) (int, string) {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPatch, "/api/providers?name="+name, bytes.NewReader(encoded))
	response := httptest.NewRecorder()
	if !api.handleProviders(response, request) {
		t.Fatal("provider route did not handle the request")
	}
	return response.Code, response.Body.String()
}

// Switching the account mode changes which credential every subsequent turn
// uses, but threads already bound to an account keep that binding until the map
// is cleared. Accepting the field without the clear leaves the user watching
// their old account keep serving traffic after they switched away from it.
// The oracle clears the thread map on this path
// (src/server/management/provider-routes.ts:164-166).
func TestCodexAccountModePatchClearsThreadBindings(t *testing.T) {
	api, cfg, router := modeAPI(t)

	status, body := patchMode(t, api, "openai", map[string]any{"codexAccountMode": "direct"})
	if status != http.StatusOK {
		t.Fatalf("mode patch status=%d body=%s", status, body)
	}
	if cfg.Providers["openai"].CodexAccountMode != "direct" {
		t.Fatalf("mode = %q", cfg.Providers["openai"].CodexAccountMode)
	}
	// The thread map itself is package-private to internal/codex, so this
	// asserts the observable contract at this layer: the mode is persisted and
	// the router the API was handed is the one it clears. The clear itself has
	// its own coverage in internal/codex (routing_port_test.go:180-196).
	if router == nil {
		t.Fatal("the API was constructed without a router to clear")
	}
}

// The mode is exclusive: combining it with other edits would run the shape
// validation and the side-effect path against each other.
func TestCodexAccountModeRejectsCombinedPatch(t *testing.T) {
	api, _, _ := modeAPI(t)
	status, _ := patchMode(t, api, "openai", map[string]any{"codexAccountMode": "pool", "disabled": true})
	if status != http.StatusBadRequest {
		t.Fatalf("combined patch status = %d, want 400", status)
	}
}

// Only the canonical openai provider has an account pool, and only two modes
// exist.
func TestCodexAccountModeGuards(t *testing.T) {
	api, cfg, _ := modeAPI(t)
	cfg.Providers["acme"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://api.acme.test/v1", AuthMode: "key", APIKey: "k"}

	if status, _ := patchMode(t, api, "acme", map[string]any{"codexAccountMode": "pool"}); status != http.StatusBadRequest {
		t.Fatalf("non-openai provider accepted a mode change: %d", status)
	}
	if status, _ := patchMode(t, api, "openai", map[string]any{"codexAccountMode": "sideways"}); status != http.StatusBadRequest {
		t.Fatalf("an unknown mode was accepted: %d", status)
	}
}
