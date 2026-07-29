package kiro

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// A stream that ends cleanly without ever producing text, reasoning, or a tool
// call means the model said nothing -- not that the wire failed. Reporting an
// upstream error there makes a resumable turn look broken to the client.
func TestEmptySuccessfulStreamIsIncompleteNotAnError(t *testing.T) {
	stream := smithyStream(t, eventFrame(t, "messageMetadataEvent", map[string]any{
		"conversationId": "conv-empty",
	}))

	events := collect((&Adapter{}).ParseStream(context.Background(), io.NopCloser(bytes.NewReader(stream))))

	var terminal *types.AdapterEvent
	for index := range events {
		if events[index].Type == types.EventIncomplete || events[index].Type == types.EventError {
			terminal = &events[index]
		}
	}
	if terminal == nil {
		t.Fatalf("no terminal event in %#v", events)
	}
	if terminal.Type == types.EventError {
		t.Fatalf("an empty successful stream must not be reported as an upstream error: %#v", *terminal)
	}
	if terminal.Reason != "empty_kiro_stream" {
		t.Fatalf("reason = %q, want empty_kiro_stream", terminal.Reason)
	}
	if !terminal.Retryable {
		t.Fatal("an empty stream is replay-safe: nothing reached the client")
	}
	if terminal.EndTurn {
		t.Fatal("an empty stream did not end the turn")
	}
	if terminal.StatusCode != 0 {
		t.Fatalf("incomplete must not carry an HTTP status, got %d", terminal.StatusCode)
	}
}

// The branch above must stay confined to genuinely empty streams: a turn that
// produced text still finishes normally.
func TestStreamWithTextStillCompletes(t *testing.T) {
	stream := smithyStream(t,
		eventFrame(t, "assistantResponseEvent", map[string]any{"content": "hello"}),
		eventFrame(t, "messageMetadataEvent", map[string]any{"conversationId": "conv-text"}),
	)

	events := collect((&Adapter{}).ParseStream(context.Background(), io.NopCloser(bytes.NewReader(stream))))

	var sawDone bool
	for _, event := range events {
		if event.Type == types.EventIncomplete && event.Reason == "empty_kiro_stream" {
			t.Fatalf("a stream carrying text is not empty: %#v", events)
		}
		if event.Type == types.EventDone {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatalf("expected a done terminal, got %#v", events)
	}
}
