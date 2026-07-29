package bridge

import (
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

func messageItems(t *testing.T, events []types.AdapterEvent) []map[string]any {
	t.Helper()
	frames, _ := Convert("gpt-5.6-terra", events)
	var messages []map[string]any
	for _, frame := range frames {
		if frame.Type != "response.output_item.done" {
			continue
		}
		item, _ := frame.Data["item"].(map[string]any)
		if item != nil && item["type"] == "message" {
			messages = append(messages, item)
		}
	}
	return messages
}

// Upstream stamps the phase on the item and then streams deltas without
// repeating it. Treating an omitted phase as a CHANGE would split one
// commentary block into two messages and drop the label the client reads to
// tell a preamble from a final answer.
func TestOmittedPhaseKeepsAppendingToTheSameMessage(t *testing.T) {
	messages := messageItems(t, []types.AdapterEvent{
		{Type: types.EventTextDelta, Text: "Running ", Phase: "commentary"},
		{Type: types.EventTextDelta, Text: "the command."},
		{Type: types.EventDone},
	})

	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1 (an omitted phase split the item): %#v", len(messages), messages)
	}
	if messages[0]["phase"] != "commentary" {
		t.Fatalf("phase = %v, want commentary to survive the unphased delta", messages[0]["phase"])
	}
	content, _ := messages[0]["content"].([]any)
	first, _ := content[0].(map[string]any)
	if first["text"] != "Running the command." {
		t.Fatalf("text = %v, want both deltas in one message", first["text"])
	}
}

// An explicit change still splits: commentary and the final answer are
// different items and must not be merged.
func TestExplicitPhaseChangeStillSplitsTheMessage(t *testing.T) {
	messages := messageItems(t, []types.AdapterEvent{
		{Type: types.EventTextDelta, Text: "Running it.", Phase: "commentary"},
		{Type: types.EventTextDelta, Text: "Here is the answer.", Phase: "final_answer"},
		{Type: types.EventDone},
	})

	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2 across an explicit phase change", len(messages))
	}
	if messages[0]["phase"] != "commentary" || messages[1]["phase"] != "final_answer" {
		t.Fatalf("phases = %v / %v", messages[0]["phase"], messages[1]["phase"])
	}
}

// A stream that never mentions a phase stays unlabeled and unsplit.
func TestUnphasedStreamProducesOneUnlabeledMessage(t *testing.T) {
	messages := messageItems(t, []types.AdapterEvent{
		{Type: types.EventTextDelta, Text: "hello "},
		{Type: types.EventTextDelta, Text: "world"},
		{Type: types.EventDone},
	})

	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if _, present := messages[0]["phase"]; present {
		t.Fatalf("invented a phase: %v", messages[0]["phase"])
	}
}
