package management

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

// stubOAuthBackend satisfies the interface so the /api/oauth/ handler reaches
// the route under test. The 501 gate in front of it is correct behaviour —
// clear-cooldown genuinely needs OAuth management configured — so the fixture
// supplies one rather than the route being loosened.
type stubOAuthBackend struct{}

func (stubOAuthBackend) Providers() []string { return []string{"anthropic"} }
func (stubOAuthBackend) Start(context.Context, string, OAuthLoginOptions) (OAuthLoginStart, error) {
	return OAuthLoginStart{}, nil
}
func (stubOAuthBackend) Cancel(string) (bool, error)                             { return false, nil }
func (stubOAuthBackend) SubmitCode(context.Context, string, string) error        { return nil }
func (stubOAuthBackend) Status(string) OAuthStatus                               { return OAuthStatus{} }
func (stubOAuthBackend) Logout(context.Context, string) error                    { return nil }
func (stubOAuthBackend) Accounts(string) (OAuthAccountSet, error)                { return OAuthAccountSet{}, nil }
func (stubOAuthBackend) SetActive(context.Context, string, string) (bool, error) { return false, nil }
func (stubOAuthBackend) SetAlias(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (stubOAuthBackend) RemoveAccount(context.Context, string, string) (bool, error) {
	return false, nil
}

func routeAPI(t *testing.T) (*API, *config.Config) {
	t.Helper()
	cfg := config.FreshInstall()
	return &API{
		config:     &cfg,
		configPath: filepath.Join(t.TempDir(), "config.json"),
		now:        time.Now,
		oauth:      stubOAuthBackend{},
	}, &cfg
}

func callRoute(t *testing.T, api *API, method, path string, body any) (int, map[string]any) {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	response := httptest.NewRecorder()
	handled := api.handleCodexAuth(response, request)
	if !handled {
		handled = api.handleOAuth(response, request)
	}
	if !handled {
		t.Fatalf("%s %s was not routed — the dashboard would get a 404", method, path)
	}
	var payload map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	return response.Code, payload
}

// A cooldown outlives the condition that caused it, so the dashboard offers a
// way to lift one. Go never registered the route, so that button 404'd and the
// account stayed idle for the full Retry-After the provider named.
func TestCodexAuthClearCooldownIsRouted(t *testing.T) {
	api, _ := routeAPI(t)
	status, payload := callRoute(t, api, http.MethodPost, "/api/codex-auth/accounts/clear-cooldown", map[string]any{"id": "account-a"})
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%#v", status, payload)
	}
	if payload["ok"] != true || payload["id"] != "account-a" {
		t.Fatalf("payload = %#v", payload)
	}
	// Nothing was cooling, so the honest answer is that nothing was cleared.
	if payload["cleared"] != false {
		t.Fatalf("cleared = %v with no active cooldown", payload["cleared"])
	}
}

func TestCodexAuthClearCooldownRejectsAnEmptyID(t *testing.T) {
	api, _ := routeAPI(t)
	if status, _ := callRoute(t, api, http.MethodPost, "/api/codex-auth/accounts/clear-cooldown", map[string]any{"id": "  "}); status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// Only Anthropic has a pooled cooldown. Accepting another provider would report
// success for an operation that did nothing.
func TestOAuthClearCooldownIsAnthropicOnly(t *testing.T) {
	api, _ := routeAPI(t)
	if status, _ := callRoute(t, api, http.MethodPost, "/api/oauth/accounts/clear-cooldown", map[string]any{"provider": "xai", "accountId": "a"}); status != http.StatusBadRequest {
		t.Fatalf("non-anthropic provider status = %d, want 400", status)
	}
	if status, _ := callRoute(t, api, http.MethodPost, "/api/oauth/accounts/clear-cooldown", map[string]any{"provider": "anthropic"}); status != http.StatusBadRequest {
		t.Fatalf("missing accountId status = %d, want 400", status)
	}
	status, payload := callRoute(t, api, http.MethodPost, "/api/oauth/accounts/clear-cooldown", map[string]any{"provider": "Anthropic", "accountId": "acct-1"})
	if status != http.StatusOK || payload["ok"] != true {
		t.Fatalf("status=%d payload=%#v", status, payload)
	}
}

// The GET side already reported the pool strategy; without the writer the
// dashboard could show the setting but never change it.
func TestPoolStrategyWriteRoute(t *testing.T) {
	api, cfg := routeAPI(t)

	status, payload := callRoute(t, api, http.MethodPut, "/api/codex-auth/pool-strategy", map[string]any{"strategy": "round-robin", "stickyLimit": 7})
	if status != http.StatusOK {
		t.Fatalf("status = %d payload=%#v", status, payload)
	}
	if cfg.AccountPoolStrategy != "round-robin" {
		t.Fatalf("strategy = %q", cfg.AccountPoolStrategy)
	}
	if cfg.AccountPoolStickyLimit == nil || *cfg.AccountPoolStickyLimit != 7 {
		t.Fatalf("stickyLimit = %v", cfg.AccountPoolStickyLimit)
	}
	if payload["accountPoolStrategy"] != "round-robin" || payload["accountPoolStickyLimit"] != float64(7) {
		t.Fatalf("response did not echo the effective settings: %#v", payload)
	}
}

// PATCH is accepted too, and a partial body leaves the other field alone.
func TestPoolStrategyPatchLeavesTheOtherFieldAlone(t *testing.T) {
	api, cfg := routeAPI(t)
	callRoute(t, api, http.MethodPut, "/api/codex-auth/pool-strategy", map[string]any{"strategy": "fill-first", "stickyLimit": 5})

	if status, _ := callRoute(t, api, http.MethodPatch, "/api/codex-auth/pool-strategy", map[string]any{"stickyLimit": 9}); status != http.StatusOK {
		t.Fatalf("patch status = %d", status)
	}
	if cfg.AccountPoolStrategy != "fill-first" {
		t.Fatalf("a stickyLimit-only patch changed the strategy to %q", cfg.AccountPoolStrategy)
	}
	if *cfg.AccountPoolStickyLimit != 9 {
		t.Fatalf("stickyLimit = %d", *cfg.AccountPoolStickyLimit)
	}
}

func TestPoolStrategyRejectsInvalidValues(t *testing.T) {
	api, _ := routeAPI(t)
	for _, body := range []map[string]any{
		{},
		{"strategy": "sideways"},
		{"stickyLimit": 0},
		{"stickyLimit": 101},
	} {
		if status, _ := callRoute(t, api, http.MethodPut, "/api/codex-auth/pool-strategy", body); status != http.StatusBadRequest {
			t.Fatalf("body %#v produced status %d, want 400", body, status)
		}
	}
}
