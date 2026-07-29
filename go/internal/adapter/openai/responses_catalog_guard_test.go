package openai

import (
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

func withCatalogLookup(t *testing.T, lookup func(string) (bool, bool)) {
	t.Helper()
	previous := CatalogModelSupportsReasoningSummaries
	CatalogModelSupportsReasoningSummaries = lookup
	t.Cleanup(func() { CatalogModelSupportsReasoningSummaries = previous })
}

// A client that started before a catalog refresh keeps sending
// reasoning_summary_delivery for a model that no longer supports it, and the
// upstream rejects the whole request. The oracle re-reads the active catalog on
// every request for exactly that reason (src/adapters/openai-responses.ts:61).
func TestReasoningSummaryStrippedWhenTheCatalogSaysUnsupported(t *testing.T) {
	withCatalogLookup(t, func(string) (bool, bool) { return false, true })

	body := map[string]any{"stream_options": map[string]any{"reasoning_summary_delivery": "auto", "include_usage": true}}
	stripDisabledResponsesReasoningSummaries(body, config.ProviderConfig{}, "stale-model")

	stream, _ := body["stream_options"].(map[string]any)
	if stream == nil {
		t.Fatal("stream_options was removed although include_usage remained")
	}
	if _, present := stream["reasoning_summary_delivery"]; present {
		t.Fatal("the catalog said unsupported but the field was still sent")
	}
	if stream["include_usage"] != true {
		t.Fatal("an unrelated stream option was dropped")
	}
}

// A supported model keeps the field, and a model the catalog does not know is
// left alone rather than guessed at.
func TestReasoningSummaryKeptWhenSupportedOrUnknown(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		supported, known bool
	}{
		{"supported", true, true},
		{"unknown", false, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			withCatalogLookup(t, func(string) (bool, bool) { return testCase.supported, testCase.known })

			body := map[string]any{"stream_options": map[string]any{"reasoning_summary_delivery": "auto"}}
			stripDisabledResponsesReasoningSummaries(body, config.ProviderConfig{}, "model")

			stream, _ := body["stream_options"].(map[string]any)
			if stream == nil || stream["reasoning_summary_delivery"] != "auto" {
				t.Fatalf("field was stripped for a %s model: %#v", testCase.name, body)
			}
		})
	}
}

// Explicit provider config still wins over the catalog, in both directions.
func TestProviderConfigOverridesTheCatalog(t *testing.T) {
	withCatalogLookup(t, func(string) (bool, bool) { return false, true })
	body := map[string]any{"stream_options": map[string]any{"reasoning_summary_delivery": "auto"}}
	stripDisabledResponsesReasoningSummaries(body, config.ProviderConfig{
		ModelSupportsReasoningSummaries: map[string]bool{"model": true},
	}, "model")
	if stream, _ := body["stream_options"].(map[string]any); stream["reasoning_summary_delivery"] != "auto" {
		t.Fatal("an explicit provider opt-in was overruled by the catalog")
	}
}

// With no lookup installed the sanitizer behaves exactly as it did before the
// guard existed, so CLI paths that never serve requests are unaffected.
func TestNoCatalogLookupLeavesPreviousBehaviour(t *testing.T) {
	withCatalogLookup(t, nil)
	body := map[string]any{"stream_options": map[string]any{"reasoning_summary_delivery": "auto"}}
	stripDisabledResponsesReasoningSummaries(body, config.ProviderConfig{}, "model")
	if stream, _ := body["stream_options"].(map[string]any); stream["reasoning_summary_delivery"] != "auto" {
		t.Fatal("behaviour changed when no catalog lookup is installed")
	}
}
