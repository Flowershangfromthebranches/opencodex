package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

func probeProvider(t *testing.T, api *API, name string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/test?name="+name, nil)
	response := httptest.NewRecorder()
	if !api.testProvider(response, request) {
		t.Fatal("probe route did not handle the request")
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("probe body is not JSON: %s", response.Body.String())
	}
	return response.Code, payload
}

// The dashboard's "test connection" button used to answer 501 in production:
// the server never injected a model fetcher, so a perfectly healthy provider
// reported "not implemented". The probe now runs directly.
func TestProviderProbeReachesTheUpstreamWithoutAnInjectedFetcher(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"one"},{"id":"two"}]}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: upstream.URL + "/v1", AuthMode: "key",
		APIKey: "k", AllowPrivateNetwork: true,
	}
	api := &API{config: &cfg}

	status, payload := probeProvider(t, api, "acme")
	if status != http.StatusOK {
		t.Fatalf("probe status = %d, body %#v", status, payload)
	}
	if payload["ok"] != true {
		t.Fatalf("a reachable provider probed as failed: %#v", payload)
	}
	if payload["models"] != float64(2) {
		t.Fatalf("models = %v, upstream listed 2", payload["models"])
	}
}

// A probe that could not reach the upstream is a successful report of a failed
// connection. Answering 502 made the dashboard show an API outage for an
// ordinary bad key; the oracle answers 200 with ok:false
// (src/server/management/provider-routes.ts:312-367).
func TestProviderProbeReportsUpstreamFailureAsOKFalse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: upstream.URL + "/v1", AuthMode: "key",
		APIKey: "bad", AllowPrivateNetwork: true,
	}
	api := &API{config: &cfg}

	status, payload := probeProvider(t, api, "acme")
	if status != http.StatusOK {
		t.Fatalf("probe status = %d, want 200 for a reported failure", status)
	}
	if payload["ok"] != false {
		t.Fatalf("a 401 upstream probed as ok: %#v", payload)
	}
	if payload["error"] == nil {
		t.Fatalf("no error text for the caller: %#v", payload)
	}
}

// A passthrough provider has no upstream /models to probe, and a static-catalog
// provider was never verified upstream. Both report honestly instead of
// pretending to have tested something.
func TestProviderProbeDescribesProvidersItCannotVerify(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["forwarded"] = config.ProviderConfig{Adapter: "openai-responses", BaseURL: "https://chatgpt.com/backend-api/codex", AuthMode: "forward"}
	static := false
	cfg.Providers["static"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://api.example.test/v1", AuthMode: "key", APIKey: "k", LiveModels: &static}
	api := &API{config: &cfg}

	if _, payload := probeProvider(t, api, "forwarded"); payload["ok"] != true || payload["message"] == nil {
		t.Fatalf("passthrough provider: %#v", payload)
	}
	if _, payload := probeProvider(t, api, "static"); payload["ok"] != false || payload["error"] == nil {
		t.Fatalf("static-catalog provider: %#v", payload)
	}
}

// An OAuth provider whose token this API cannot read says so, rather than
// probing anonymously and blaming the provider for the 401.
func TestProviderProbeReportsMissingOAuthLogin(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["oauthy"] = config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://api.example.test/v1", AuthMode: "oauth"}
	api := &API{config: &cfg}

	_, payload := probeProvider(t, api, "oauthy")
	if payload["ok"] != false {
		t.Fatalf("probe reported ok without a credential: %#v", payload)
	}
}
