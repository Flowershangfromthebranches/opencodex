package openai

import (
	"strconv"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

func collectToolCalls(t *testing.T, frames ...any) []*types.ToolCall {
	t.Helper()
	calls := map[string]*pendingChatCall{}
	order := []string{}
	for _, frame := range frames {
		accumulateChatCalls(frame, calls, &order)
	}
	var out []*types.ToolCall
	flushChatCalls(func(event types.AdapterEvent) bool {
		if event.Type == types.EventToolCall {
			out = append(out, event.ToolCall)
		}
		return true
	}, calls, order)
	return out
}

// Frames arrive from encoding/json, where every number is a float64. Building
// them with Go ints would make `index` unreadable and silently collapse every
// call onto one key -- which is how this test first "failed" against correct
// code.
func toolCallFrame(fields map[string]any) []any { return []any{fields} }

// A provider that omits both `index` and `id` on a continuation chunk is
// continuing the call it was last writing. Keying that chunk to index 0
// silently appends its arguments to the FIRST call, which corrupts two calls at
// once: one gains a fragment it never had, the other loses its tail. The oracle
// falls back to the last pending call (src/adapters/openai-chat.ts:756).
func TestChatToolCallContinuationFallsBackToTheLastCall(t *testing.T) {
	calls := collectToolCalls(t,
		toolCallFrame(map[string]any{"index": float64(0), "id": "call_a", "function": map[string]any{"name": "first", "arguments": `{"a":1}`}}),
		toolCallFrame(map[string]any{"index": float64(1), "id": "call_b", "function": map[string]any{"name": "second", "arguments": `{"b":`}}),
		// No index, no id: this continues `second`, not `first`.
		toolCallFrame(map[string]any{"function": map[string]any{"arguments": `2}`}}),
	)

	if len(calls) != 2 {
		t.Fatalf("expected two calls, got %d", len(calls))
	}
	if got := string(calls[0].Arguments); got != `{"a":1}` {
		t.Fatalf("first call absorbed a continuation meant for the second: %s", got)
	}
	if got := string(calls[1].Arguments); got != `{"b":2}` {
		t.Fatalf("second call lost its continuation: %s", got)
	}
}

// Arguments that never became valid JSON are a real upstream failure. Replacing
// them with `{}` hands the client a well-formed call the model never made --
// the tool then runs with no arguments instead of reporting the failure. The
// oracle forwards whatever arrived (openai-chat.ts:666) and lets the caller
// decide.
func TestChatToolCallPreservesCorruptArguments(t *testing.T) {
	calls := collectToolCalls(t,
		toolCallFrame(map[string]any{"index": float64(0), "id": "call_a", "function": map[string]any{"name": "run", "arguments": `{"path":"/tmp`}}),
	)

	if len(calls) != 1 {
		t.Fatalf("expected one call, got %d", len(calls))
	}
	if got := string(calls[0].Arguments); got != `{"path":"/tmp` {
		t.Fatalf("truncated arguments were rewritten to %s, hiding the upstream failure", got)
	}
}

// The mixed-keying rescue must survive: a call opened under an index key still
// absorbs an id-only continuation instead of splitting in two.
func TestChatToolCallAbsorbsIDOnlyContinuation(t *testing.T) {
	calls := collectToolCalls(t,
		toolCallFrame(map[string]any{"index": float64(0), "id": "call_a", "function": map[string]any{"name": "run", "arguments": `{"a":`}}),
		toolCallFrame(map[string]any{"id": "call_a", "function": map[string]any{"arguments": `1}`}}),
	)

	if len(calls) != 1 {
		t.Fatalf("an id-only continuation split into %d calls", len(calls))
	}
	if got := string(calls[0].Arguments); got != `{"a":1}` {
		t.Fatalf("arguments = %s", got)
	}
}

// Calls without ids get generated ones, in order, as the oracle does.
func TestChatToolCallGeneratesMissingIDsInOrder(t *testing.T) {
	calls := collectToolCalls(t,
		toolCallFrame(map[string]any{"index": float64(0), "function": map[string]any{"name": "a", "arguments": "{}"}}),
		toolCallFrame(map[string]any{"index": float64(1), "function": map[string]any{"name": "b", "arguments": "{}"}}),
	)
	for i, call := range calls {
		if want := "call_" + strconv.Itoa(i+1); call.ID != want {
			t.Fatalf("call %d id = %q, want %q", i, call.ID, want)
		}
	}
}
