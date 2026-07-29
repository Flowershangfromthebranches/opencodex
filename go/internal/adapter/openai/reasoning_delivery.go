package openai

import (
	"github.com/lidge-jun/opencodex-go/internal/config"
)

// applyReasoningSummaryDeliveryOverride replaces the delivery mode for models
// the provider config names.
//
// Only rewrites an EXISTING field: adding one the client never asked for would
// turn a per-model correction into a global behaviour change. The oracle has
// the same guard (src/adapters/openai-responses.ts:232-241) — the setting
// exists for models whose upstream mishandles the client's chosen mode, not to
// impose one.
func applyReasoningSummaryDeliveryOverride(body map[string]any, provider config.ProviderConfig, modelID string) {
	delivery, configured := config.ModelRecordValue(provider.ModelReasoningSummaryDelivery, modelID)
	if !configured || delivery == "" {
		return
	}
	stream, ok := body["stream_options"].(map[string]any)
	if !ok {
		return
	}
	if _, present := stream["reasoning_summary_delivery"]; !present {
		return
	}
	stream["reasoning_summary_delivery"] = delivery
}
