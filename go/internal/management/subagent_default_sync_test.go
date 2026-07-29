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

func injectionAPI(t *testing.T) (*API, *config.Config) {
	t.Helper()
	cfg := config.FreshInstall()
	api := &API{config: &cfg, configPath: filepath.Join(t.TempDir(), "config.json")}
	api.agents = AgentSettings{GuidanceEnabled: cfg.MultiAgentGuidanceEnabled}
	return api, &cfg
}

func injectionModel(t *testing.T, api *API, method string, body any) map[string]any {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/injection-model", bytes.NewReader(encoded))
	response := httptest.NewRecorder()
	if !api.handleAgents(response, request) {
		t.Fatal("injection-model was not routed")
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not JSON: %s", response.Body.String())
	}
	return payload
}

// The dashboard renders this toggle from the GET response. Omitting the field
// entirely meant the control had nothing to read, and the PUT had nothing to
// write, so the setting could not be changed from the UI at all.
func TestInjectionModelReportsAndStoresSubagentSync(t *testing.T) {
	api, cfg := injectionAPI(t)

	if payload := injectionModel(t, api, http.MethodGet, nil); payload["syncCodexSubagentDefaults"] != false {
		t.Fatalf("fresh config should not report syncing: %#v", payload["syncCodexSubagentDefaults"])
	}

	payload := injectionModel(t, api, http.MethodPut, map[string]any{"model": "gpt-5.5", "syncCodexSubagentDefaults": true})
	if payload["syncCodexSubagentDefaults"] != true {
		t.Fatalf("PUT did not report syncing: %#v", payload)
	}
	if cfg.SyncCodexSubagentDefaults == nil || !*cfg.SyncCodexSubagentDefaults {
		t.Fatalf("the flag was not persisted: %v", cfg.SyncCodexSubagentDefaults)
	}
	if got := injectionModel(t, api, http.MethodGet, nil)["syncCodexSubagentDefaults"]; got != true {
		t.Fatalf("GET does not reflect the stored flag: %#v", got)
	}
}

// The flag alone is not the answer. Mirroring needs a model to mirror, so a
// config with the toggle on and no injection model is not syncing anything —
// reporting true there would show a control claiming to do something it cannot.
func TestSubagentSyncNeedsAnInjectionModel(t *testing.T) {
	api, _ := injectionAPI(t)

	payload := injectionModel(t, api, http.MethodPut, map[string]any{"syncCodexSubagentDefaults": true})
	if payload["syncCodexSubagentDefaults"] != false {
		t.Fatalf("sync reported active with no injection model: %#v", payload)
	}

	payload = injectionModel(t, api, http.MethodPut, map[string]any{"model": "gpt-5.5"})
	if payload["syncCodexSubagentDefaults"] != true {
		t.Fatalf("adding a model did not activate the stored flag: %#v", payload)
	}

	payload = injectionModel(t, api, http.MethodPut, map[string]any{"model": ""})
	if payload["syncCodexSubagentDefaults"] != false {
		t.Fatalf("clearing the model left sync reported as active: %#v", payload)
	}
}

// An omitted field leaves the stored value alone, so an unrelated edit does not
// silently turn the toggle off.
func TestSubagentSyncSurvivesAnUnrelatedEdit(t *testing.T) {
	api, cfg := injectionAPI(t)
	injectionModel(t, api, http.MethodPut, map[string]any{"model": "gpt-5.5", "syncCodexSubagentDefaults": true})

	injectionModel(t, api, http.MethodPut, map[string]any{"prompt": "be concise"})

	if cfg.SyncCodexSubagentDefaults == nil || !*cfg.SyncCodexSubagentDefaults {
		t.Fatalf("a prompt edit cleared the sync flag: %v", cfg.SyncCodexSubagentDefaults)
	}
}
