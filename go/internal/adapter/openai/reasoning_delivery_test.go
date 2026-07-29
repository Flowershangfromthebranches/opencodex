package openai

import (
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

func deliveryProvider(model, delivery string) config.ProviderConfig {
	return config.ProviderConfig{ModelReasoningSummaryDelivery: map[string]string{model: delivery}}
}

// The setting exists for a model whose upstream mishandles the delivery mode
// the client chose, so the configured value replaces what was sent.
func TestReasoningDeliveryOverrideReplacesTheRequestedMode(t *testing.T) {
	body := map[string]any{"stream_options": map[string]any{"reasoning_summary_delivery": "sequential", "include_usage": true}}
	applyReasoningSummaryDeliveryOverride(body, deliveryProvider("gpt-5.5", "concurrent"), "gpt-5.5")

	stream, _ := body["stream_options"].(map[string]any)
	if stream["reasoning_summary_delivery"] != "concurrent" {
		t.Fatalf("delivery = %v", stream["reasoning_summary_delivery"])
	}
	if stream["include_usage"] != true {
		t.Fatalf("an unrelated stream option was disturbed: %#v", stream)
	}
}

// It never ADDS the field. A client that did not ask for summary delivery must
// not have a mode imposed on it, which would turn a per-model correction into a
// global behaviour change.
func TestReasoningDeliveryOverrideNeverAddsTheField(t *testing.T) {
	body := map[string]any{"stream_options": map[string]any{"include_usage": true}}
	applyReasoningSummaryDeliveryOverride(body, deliveryProvider("gpt-5.5", "concurrent"), "gpt-5.5")
	stream, _ := body["stream_options"].(map[string]any)
	if _, present := stream["reasoning_summary_delivery"]; present {
		t.Fatalf("the field was added to a request that never asked for it: %#v", stream)
	}

	noStream := map[string]any{"model": "gpt-5.5"}
	applyReasoningSummaryDeliveryOverride(noStream, deliveryProvider("gpt-5.5", "concurrent"), "gpt-5.5")
	if _, present := noStream["stream_options"]; present {
		t.Fatalf("stream_options was invented: %#v", noStream)
	}
}

// A model the config does not name is left alone.
func TestReasoningDeliveryOverrideOnlyTouchesNamedModels(t *testing.T) {
	body := map[string]any{"stream_options": map[string]any{"reasoning_summary_delivery": "sequential"}}
	applyReasoningSummaryDeliveryOverride(body, deliveryProvider("other-model", "concurrent"), "gpt-5.5")
	stream, _ := body["stream_options"].(map[string]any)
	if stream["reasoning_summary_delivery"] != "sequential" {
		t.Fatalf("an unnamed model was rewritten: %v", stream["reasoning_summary_delivery"])
	}
}

// An unknown delivery value is refused at config load. Forwarding it would make
// the upstream answer 400, so the user would see a failing turn rather than a
// bad setting.
func TestConfigRejectsAnUnknownDeliveryMode(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["acme"] = config.ProviderConfig{
		Adapter: "openai-responses", BaseURL: "https://api.acme.test/v1", AuthMode: "key", APIKey: "k",
		ModelReasoningSummaryDelivery: map[string]string{"gpt-5.5": "telepathy"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("an unknown delivery mode was accepted")
	}

	for _, valid := range config.ReasoningSummaryDeliveryValues {
		cfg.Providers["acme"] = config.ProviderConfig{
			Adapter: "openai-responses", BaseURL: "https://api.acme.test/v1", AuthMode: "key", APIKey: "k",
			ModelReasoningSummaryDelivery: map[string]string{"gpt-5.5": valid},
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%q was rejected: %v", valid, err)
		}
	}
}
