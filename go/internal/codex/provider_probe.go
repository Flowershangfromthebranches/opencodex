package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lidge-jun/opencodex-go/internal/config"
	"github.com/lidge-jun/opencodex-go/internal/destination"
)

// ProviderProbeResult is what the dashboard's "test connection" button reports.
type ProviderProbeResult struct {
	OK        bool
	Models    int
	LatencyMS int64
	Message   string
	Error     string
}

// ProbeProviderModels performs the provider's live /models fetch DIRECTLY and
// reports only real upstream evidence.
//
// It deliberately does NOT go through FetchProviderModels: that path degrades to
// cached, stale, or configured models on failure, which is right for catalog
// assembly and wrong here — a provider with a revoked key would "pass" on the
// strength of a static catalog. The oracle keeps the same separation and says so
// (src/server/management/provider-routes.ts:312-317).
func ProbeProviderModels(ctx context.Context, client *http.Client, name string, provider config.ProviderConfig, accessToken string) ProviderProbeResult {
	if provider.Disabled {
		return ProviderProbeResult{Error: "Provider is disabled"}
	}
	if provider.AuthMode == "forward" {
		return ProviderProbeResult{OK: true, Message: "Passthrough provider is configured (forwards your Codex login; no upstream /models)."}
	}
	if provider.LiveModels != nil && !*provider.LiveModels {
		return ProviderProbeResult{Error: "static catalog only — upstream not verified"}
	}
	token := strings.TrimSpace(accessToken)
	if token == "" {
		token = strings.TrimSpace(provider.APIKey)
	}
	if provider.AuthMode == "oauth" && token == "" {
		return ProviderProbeResult{Error: "static catalog only — upstream not verified (not logged in)"}
	}
	modelsURL, err := providerModelsURL(provider.BaseURL)
	if err != nil {
		return ProviderProbeResult{Error: "provider base URL is invalid"}
	}
	if !provider.AllowPrivateNetwork {
		if err := rejectPrivateDestination(ctx, modelsURL); err != nil {
			return ProviderProbeResult{Error: err.Error()}
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	// Same pinning as discovery: the probe fetches a URL the user typed, and a
	// pre-check the dialer does not honour is not a check.
	client = destination.PinnedClient(client, provider.AllowPrivateNetwork)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL.String(), nil)
	if err != nil {
		return ProviderProbeResult{Error: "provider models request could not be built"}
	}
	request.Header.Set("Accept", "application/json")
	for header, value := range provider.Headers {
		request.Header.Set(header, value)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	started := time.Now()
	response, err := client.Do(request)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return ProviderProbeResult{LatencyMS: latency, Error: "provider models request failed"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ProviderProbeResult{LatencyMS: latency, Error: fmt.Sprintf("upstream /models returned %d", response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderModelsResponseBytes+1))
	if err != nil || len(body) > maxProviderModelsResponseBytes {
		return ProviderProbeResult{LatencyMS: latency, Error: "upstream /models returned an unexpected shape"}
	}
	count, err := probeModelCount(body)
	if err != nil {
		return ProviderProbeResult{LatencyMS: latency, Error: err.Error()}
	}
	return ProviderProbeResult{OK: true, Models: count, LatencyMS: latency}
}

// probeModelCount accepts the three shapes providers actually return: OpenAI's
// {data:[...]}, Google's {models:[...]}, and Together-style top-level arrays.
func probeModelCount(body []byte) (int, error) {
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		return 0, errors.New("upstream /models returned an unexpected shape")
	}
	switch typed := document.(type) {
	case []any:
		return len(typed), nil
	case map[string]any:
		for _, key := range []string{"data", "models"} {
			if list, ok := typed[key].([]any); ok {
				return len(list), nil
			}
		}
	}
	return 0, errors.New("upstream /models returned an unexpected shape")
}
