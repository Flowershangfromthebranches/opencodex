package openai

import (
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

func parsedToolCalls(t *testing.T, events ...map[string]any) []*types.ToolCall {
	t.Helper()
	calls := map[string]*types.ToolCall{}
	phases := map[string]string{}
	var out []*types.ToolCall
	for _, event := range events {
		parseResponsesStreamEvent(event, calls, phases, func(adapterEvent types.AdapterEvent) bool {
			if adapterEvent.Type == types.EventToolCall {
				out = append(out, adapterEvent.ToolCall)
			}
			return true
		})
	}
	return out
}

// An upstream that answers with a custom_tool_call or tool_search_call is
// reporting a tool the model actually invoked. The parser recognised only
// function_call, so those items were dropped on the floor: the turn looked
// like a clean completion while the requested tool action simply never
// reached the client.
//
// The reverse direction already knows these types -- the request parser at
// responses.go:349 and the bridge both handle them -- so only the response
// path was blind.
func TestResponsesStreamKeepsNonFunctionToolCalls(t *testing.T) {
	for _, testCase := range []struct{ name, itemType, argumentField string }{
		{"custom tool call", "custom_tool_call", "input"},
		{"tool search call", "tool_search_call", "arguments"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			item := map[string]any{
				"type": testCase.itemType, "id": "item-1", "call_id": "call-1", "name": "apply_patch",
				testCase.argumentField: `{"path":"a.txt"}`,
			}
			calls := parsedToolCalls(t,
				map[string]any{"type": "response.output_item.added", "item": item},
				map[string]any{"type": "response.output_item.done", "item": item},
			)
			if len(calls) != 1 {
				t.Fatalf("%s was dropped: got %d calls", testCase.itemType, len(calls))
			}
			if calls[0].ID != "call-1" || calls[0].Name != "apply_patch" {
				t.Fatalf("call = %#v", calls[0])
			}
			if string(calls[0].Arguments) != `{"path":"a.txt"}` {
				t.Fatalf("arguments = %s", calls[0].Arguments)
			}
		})
	}
}

// function_call still behaves exactly as before, including argument deltas
// accumulated across events.
func TestResponsesStreamStillHandlesFunctionCalls(t *testing.T) {
	calls := parsedToolCalls(t,
		map[string]any{"type": "response.output_item.added", "item": map[string]any{
			"type": "function_call", "id": "item-1", "call_id": "call-1", "name": "lookup",
		}},
		map[string]any{"type": "response.function_call_arguments.delta", "item_id": "item-1", "delta": `{"q":`},
		map[string]any{"type": "response.function_call_arguments.delta", "item_id": "item-1", "delta": `"x"}`},
		map[string]any{"type": "response.output_item.done", "item": map[string]any{
			"type": "function_call", "id": "item-1", "call_id": "call-1", "name": "lookup",
		}},
	)
	if len(calls) != 1 {
		t.Fatalf("expected one call, got %d", len(calls))
	}
	if string(calls[0].Arguments) != `{"q":"x"}` {
		t.Fatalf("streamed arguments = %s", calls[0].Arguments)
	}
}

// A web_search_call carries no client-consumable payload, and the oracle
// deliberately drops it rather than surfacing it as a tool the client should
// run (src/responses/parser.ts:492-497).
func TestResponsesStreamIgnoresWebSearchCalls(t *testing.T) {
	item := map[string]any{"type": "web_search_call", "id": "item-1", "call_id": "call-1", "status": "completed"}
	calls := parsedToolCalls(t,
		map[string]any{"type": "response.output_item.added", "item": item},
		map[string]any{"type": "response.output_item.done", "item": item},
	)
	if len(calls) != 0 {
		t.Fatalf("web_search_call surfaced as a tool call: %#v", calls)
	}
}
