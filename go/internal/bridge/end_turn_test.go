package bridge

import (
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

func completedResponseFor(t *testing.T, events []types.AdapterEvent) map[string]any {
	t.Helper()
	frames, _ := Convert("gpt-5.6-terra", events)
	for i := len(frames) - 1; i >= 0; i-- {
		if !strings.Contains(frames[i].Type, "completed") && !strings.Contains(frames[i].Type, "incomplete") {
			continue
		}
		if response, ok := frames[i].Data["response"].(map[string]any); ok {
			return response
		}
	}
	t.Fatal("no terminal frame carried a response")
	return nil
}

// end_turn:false is the model saying "I am not done". Codex reads it to decide
// whether to run another turn, so dropping it ends a turn that was still going:
// the commentary message becomes the final answer and the tool call that was
// about to follow never runs.
func TestCompletedCarriesAnExplicitFalseEndTurn(t *testing.T) {
	response := completedResponseFor(t, []types.AdapterEvent{
		{Type: types.EventTextDelta, Text: "Running the command now."},
		{Type: types.EventDone, EndTurn: false, EndTurnSet: true},
	})

	value, present := response["end_turn"]
	if !present {
		t.Fatalf("end_turn was dropped: %v", response)
	}
	if value != false {
		t.Fatalf("end_turn = %v, want false", value)
	}
}

func TestCompletedCarriesAnExplicitTrueEndTurn(t *testing.T) {
	response := completedResponseFor(t, []types.AdapterEvent{
		{Type: types.EventTextDelta, Text: "All done."},
		{Type: types.EventDone, EndTurn: true, EndTurnSet: true},
	})

	if response["end_turn"] != true {
		t.Fatalf("end_turn = %v, want true", response["end_turn"])
	}
}

// Absent upstream must stay absent. Emitting false where upstream said nothing
// would tell Codex to keep going when it should stop.
func TestCompletedOmitsEndTurnWhenUpstreamDidNotSendIt(t *testing.T) {
	response := completedResponseFor(t, []types.AdapterEvent{
		{Type: types.EventTextDelta, Text: "All done."},
		{Type: types.EventDone},
	})

	if _, present := response["end_turn"]; present {
		t.Fatalf("end_turn was invented: %v", response)
	}
}

// A truncated turn carries the same signal, matching the oracle.
func TestIncompleteCarriesEndTurn(t *testing.T) {
	response := completedResponseFor(t, []types.AdapterEvent{
		{Type: types.EventTextDelta, Text: "partial"},
		{Type: types.EventIncomplete, Reason: "max_output_tokens", EndTurn: false, EndTurnSet: true},
	})

	if response["end_turn"] != false {
		t.Fatalf("end_turn = %v, want false", response["end_turn"])
	}
}
