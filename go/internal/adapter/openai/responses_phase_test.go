package openai

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// The exact upstream shape captured from chatgpt.com/backend-api/codex for a
// gpt-5.6 turn: commentary message first, then the tool call it announced.
const commentaryThenToolCallSSE = `event: response.output_item.added
data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","phase":"commentary","content":[]}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"delta":"Running the command."}

event: response.output_item.done
data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Running the command."}]}}

event: response.output_item.added
data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec_command","arguments":""}}

event: response.output_item.done
data: {"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"echo hello\"}"}}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[]}}

`

func collectStreamEvents(t *testing.T, sse string) []types.AdapterEvent {
	t.Helper()
	adapter := &ResponsesAdapter{}
	stream := adapter.ParseStream(context.Background(), io.NopCloser(strings.NewReader(sse)))
	var events []types.AdapterEvent
	for event := range stream {
		events = append(events, event)
	}
	return events
}

// Codex distinguishes a preamble from a final answer by the message phase. Drop
// it and the commentary looks terminal, so the turn ends before the tool call
// that was following it ever runs -- which is exactly how gpt-5.6 subagents
// stalled while gpt-5.5 (which emits no preamble) worked.
func TestCommentaryPhaseSurvivesToTheTextDelta(t *testing.T) {
	var sawCommentary bool
	for _, event := range collectStreamEvents(t, commentaryThenToolCallSSE) {
		if event.Type == types.EventTextDelta && event.Phase == "commentary" {
			sawCommentary = true
		}
	}
	if !sawCommentary {
		t.Fatal("the commentary phase was dropped, so the preamble reads as a final answer")
	}
}

// The tool call the commentary announced must still arrive.
func TestToolCallAfterCommentaryIsStillEmitted(t *testing.T) {
	var sawToolCall bool
	for _, event := range collectStreamEvents(t, commentaryThenToolCallSSE) {
		if event.Type == types.EventToolCall && event.ToolCall != nil && event.ToolCall.Name == "exec_command" {
			sawToolCall = true
		}
	}
	if !sawToolCall {
		t.Fatal("the announced tool call never reached the client")
	}
}

// A message with no phase must stay unlabeled rather than being guessed at.
func TestMessageWithoutAPhaseStaysUnlabeled(t *testing.T) {
	sse := strings.ReplaceAll(commentaryThenToolCallSSE, `,"phase":"commentary"`, "")
	for _, event := range collectStreamEvents(t, sse) {
		if event.Type == types.EventTextDelta && event.Phase != "" {
			t.Fatalf("invented a phase: %q", event.Phase)
		}
	}
}

// The non-streaming path reads the same field off the completed item.
func TestUnaryMessageCarriesItsPhase(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"status": "completed",
		"output": []any{map[string]any{
			"type": "message", "role": "assistant", "phase": "commentary",
			"content": []any{map[string]any{"type": "output_text", "text": "Running it."}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &ResponsesAdapter{}
	events, err := adapter.ParseUnary(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == types.EventTextDelta {
			if event.Phase != "commentary" {
				t.Fatalf("phase = %q, want commentary", event.Phase)
			}
			return
		}
	}
	t.Fatal("no text event was produced")
}
