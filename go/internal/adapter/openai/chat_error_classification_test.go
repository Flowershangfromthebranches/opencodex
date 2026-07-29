package openai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

func streamErrorEvent(t *testing.T, frame string) types.AdapterEvent {
	t.Helper()
	adapter := &ChatAdapter{}
	body := io.NopCloser(strings.NewReader("data: " + frame + "\n\n"))
	for event := range adapter.ParseStream(context.Background(), body) {
		if event.Type == types.EventError {
			return event
		}
	}
	t.Fatal("no error event")
	return types.AdapterEvent{}
}

// An upstream error object carries the fields a client needs to decide what to
// do: `code` says which failure it was, `type` says which family, and `status`
// says whether retrying is even sensible. Reducing all three to a message
// string makes a 429 indistinguishable from a content-policy refusal. The
// oracle forwards each field when present (src/adapters/openai-chat.ts:712-722).
func TestChatStreamErrorPreservesCodeTypeAndStatus(t *testing.T) {
	event := streamErrorEvent(t, `{"error":{"message":"rate limited","code":"rate_limit_exceeded","type":"requests","status":429}}`)

	if event.Error != "rate limited" {
		t.Fatalf("message = %q", event.Error)
	}
	if event.Code != "rate_limit_exceeded" {
		t.Fatalf("code = %q, upstream sent rate_limit_exceeded", event.Code)
	}
	if event.ErrorType != "requests" {
		t.Fatalf("errorType = %q, upstream sent requests", event.ErrorType)
	}
	if event.StatusCode != 429 {
		t.Fatalf("statusCode = %d, upstream sent 429", event.StatusCode)
	}
}

// A cyber-policy refusal is a client error whatever status the provider
// attached, so the oracle pins it to 400 rather than letting it look retryable.
func TestChatStreamErrorPinsCyberPolicyToClientError(t *testing.T) {
	event := streamErrorEvent(t, `{"error":{"message":"refused","code":"cyber_policy"}}`)

	if event.StatusCode != http.StatusBadRequest {
		t.Fatalf("cyber_policy status = %d, want 400", event.StatusCode)
	}
}

// A bare error with no classification stays unclassified rather than gaining an
// invented status.
func TestChatStreamErrorLeavesUnclassifiedErrorsAlone(t *testing.T) {
	event := streamErrorEvent(t, `{"error":{"message":"something broke"}}`)

	if event.StatusCode != 0 || event.Code != "" || event.ErrorType != "" {
		t.Fatalf("unclassified error gained fields: status=%d code=%q type=%q", event.StatusCode, event.Code, event.ErrorType)
	}
}
