package bridge

import (
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// terminalFailure pulls the error object out of the response.failed event the
// bridge emits for a failing turn.
func terminalFailure(t *testing.T, event types.AdapterEvent) map[string]any {
	t.Helper()
	_, response := Convert("model", []types.AdapterEvent{event})
	if response.Error == nil {
		t.Fatal("response carried no error object")
	}
	return response.Error
}

// An adapter that classified the upstream failure must have that
// classification reach the client. Dropping `code` left a rate limit and a
// policy refusal looking identical -- both arrived as a bare 502 -- so callers
// could not tell a retryable failure from a permanent one.
func TestErrorEventKeepsTheAdapterClassification(t *testing.T) {
	failure := terminalFailure(t, types.AdapterEvent{
		Type: types.EventError, Error: "rate limited",
		Code: "rate_limit_exceeded", ErrorType: "requests", StatusCode: 429,
	})

	if failure["code"] != "rate_limit_exceeded" {
		t.Fatalf("code = %v, adapter reported rate_limit_exceeded", failure["code"])
	}
	if failure["type"] != "requests" {
		t.Fatalf("type = %v, adapter reported requests", failure["type"])
	}
}

// A cyber-policy refusal is pinned to the client-error shape rather than left
// as a retryable upstream failure.
func TestErrorEventPinsCyberPolicy(t *testing.T) {
	failure := terminalFailure(t, types.AdapterEvent{
		Type: types.EventError, Error: "refused", Code: "cyber_policy", StatusCode: 502,
	})

	if failure["code"] != "cyber_policy" {
		t.Fatalf("code = %v", failure["code"])
	}
	if failure["type"] != "invalid_request_error" {
		t.Fatalf("type = %v, want invalid_request_error", failure["type"])
	}
}

// An unclassified error still gets the message-derived classification, so this
// change adds information without removing the old inference.
func TestUnclassifiedErrorStillInfersFromMessage(t *testing.T) {
	failure := terminalFailure(t, types.AdapterEvent{Type: types.EventError, Error: "429 too many requests"})

	if failure["type"] == nil || failure["type"] == "" {
		t.Fatalf("message inference stopped classifying: %#v", failure)
	}
}
