package openai

import (
	"encoding/json"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
	"github.com/lidge-jun/opencodex-go/internal/types"
)

// `service_tier` is an OpenAI Responses concept. The oracle's chat-completions
// body builder never emits it (src/adapters/openai-chat.ts:525-603) — the same
// reasoning that gates `prompt_cache_key` two lines above this in Go: strict
// backends like Groq and Cerebras reject unknown top-level fields, so a request
// that merely passed through a tier-aware client fails outright on a provider
// that never advertised the field.
func TestChatBodyOmitsServiceTier(t *testing.T) {
	body := map[string]any{}
	request := &types.NormalizedRequest{
		ModelID: "llama-3.3-70b",
		Options: types.RequestOptions{ServiceTier: "priority"},
	}
	applyProviderRequestOptions(body, request, config.ProviderConfig{})

	if _, present := body["service_tier"]; present {
		encoded, _ := json.Marshal(body)
		t.Fatalf("service_tier was forwarded to a chat-completions provider: %s", encoded)
	}
}
