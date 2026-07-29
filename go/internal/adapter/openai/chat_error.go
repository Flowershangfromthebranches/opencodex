package openai

import (
	ocxlib "github.com/lidge-jun/opencodex-go/internal/lib"
	"github.com/lidge-jun/opencodex-go/internal/types"
)

// chatStreamErrorEvent carries the upstream classification, not just its prose.
//
// `code` names the specific failure, `type` its family, and `status` says
// whether a retry is even sensible. Collapsing all three into a message string
// made a 429 indistinguishable from a content-policy refusal by the time the
// bridge classified it. The oracle forwards each field when it is present
// (src/adapters/openai-chat.ts:712-722).
func chatStreamErrorEvent(chunk map[string]any) types.AdapterEvent {
	event := types.AdapterEvent{Type: types.EventError, Error: responsesErrorMessage(chunk)}
	failure, _ := chunk["error"].(map[string]any)
	if failure == nil {
		return event
	}
	event.Code = stringValue(failure["code"])
	event.ErrorType = stringValue(failure["type"])
	switch {
	case ocxlib.IsCyberPolicyCode(event.Code):
		// A policy refusal is a client error whatever status the provider
		// attached; leaving it retryable-looking makes callers retry a request
		// that can never succeed.
		event.StatusCode = 400
	case failure["status"] != nil:
		event.StatusCode = intValue(failure["status"])
	}
	return event
}
