package management

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

// stubStartupHealth answers the one call this test cares about and leaves the
// rest of the backend inert.
type stubStartupHealth struct{}

func (stubStartupHealth) StartupHealth(context.Context) (any, error) {
	return map[string]any{"status": "native"}, nil
}
func (stubStartupHealth) CodexRuntimeReport(context.Context) any { return nil }
func (stubStartupHealth) RunStartupAction(context.Context, string) (string, error) {
	return "", nil
}
func (stubStartupHealth) WindowsTray(context.Context, string) (map[string]any, error) {
	return nil, nil
}
func (stubStartupHealth) SyncModels(context.Context) (map[string]any, error) { return nil, nil }
func (stubStartupHealth) CheckUpdate(context.Context, string) (map[string]any, error) {
	return nil, nil
}
func (stubStartupHealth) StartUpdate(context.Context, string, bool) (map[string]any, error) {
	return nil, nil
}
func (stubStartupHealth) UpdateStatus(context.Context, string) (map[string]any, bool, error) {
	return nil, false, nil
}

// The setting being saved is the one startup health reports on, so the save
// has to answer with the recomputed health. Omitting it leaves the dashboard
// rendering the pre-toggle badge until something else happens to refresh it --
// a stale UI with no error to explain it.
func TestSavingSettingsReturnsRecomputedStartupHealth(t *testing.T) {
	cfg := config.Default()
	api := newParityAPI(t, &cfg, func(options *Options) { options.RuntimeControl = stubStartupHealth{} })

	response := serveManagement(api, http.MethodPut, "/api/settings", `{"codexAutoStart":true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("payload: %v (%s)", err, response.Body.String())
	}
	if _, present := payload["startupHealth"]; !present {
		t.Fatalf("startupHealth missing from the save response: %v", payload)
	}
	if payload["ok"] != true || payload["codexAutoStart"] != true {
		t.Fatalf("existing keys changed: %v", payload)
	}
}
