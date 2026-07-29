package kiro

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

func encryptedHistoryRequest(encrypted bool) *types.NormalizedRequest {
	return &types.NormalizedRequest{
		ModelID: "claude-sonnet-5",
		Context: types.RequestContext{
			Messages: []types.Message{
				{Role: "user", Content: json.RawMessage(`"do the thing"`)},
				{Role: "assistant", Content: json.RawMessage(`[{"type":"toolCall","id":"call-1","name":"shell","arguments":{}}]`)},
				{
					Role:                     "toolResult",
					ToolCallID:               "call-1",
					Content:                  json.RawMessage(`"carrier text"`),
					ContainsEncryptedContent: encrypted,
				},
			},
		},
	}
}

// Encrypted tool output cannot be translated to Kiro's wire. Forwarding the
// carrier text instead would send the model something that is not the result it
// asked for, and it would look like a successful turn -- so the request is
// refused, naming the call.
func TestEncryptedToolResultIsRefused(t *testing.T) {
	adapter := NewAdapter("https://kiro.example.test", "test-token")
	_, err := adapter.BuildRequest(context.Background(), encryptedHistoryRequest(true))
	if err == nil {
		t.Fatal("an encrypted tool result was translated anyway")
	}
	if !strings.Contains(err.Error(), "encrypted") || !strings.Contains(err.Error(), "call-1") {
		t.Fatalf("error does not identify the problem or the call: %v", err)
	}
}

// An ordinary tool result still translates, or the guard would break every
// tool-using conversation.
func TestOrdinaryToolResultStillTranslates(t *testing.T) {
	adapter := NewAdapter("https://kiro.example.test", "test-token")
	if _, err := adapter.BuildRequest(context.Background(), encryptedHistoryRequest(false)); err != nil {
		t.Fatalf("an ordinary tool result was refused: %v", err)
	}
}
