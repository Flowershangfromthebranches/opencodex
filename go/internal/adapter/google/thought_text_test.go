package google

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// Gemini marks some answer parts with `thought: true`. The oracle does not
// branch on that flag at all -- every `part.text` becomes a text_delta
// (src/adapters/google.ts:496, :667). Go reclassified those parts as reasoning,
// so a client that hides reasoning (most of them) showed nothing where the
// oracle showed the answer.
func TestGoogleThoughtTextStaysVisibleText(t *testing.T) {
	body := `{"candidates":[{"content":{"parts":[
		{"text":"weighing the options","thought":true},
		{"text":"the answer is 42"}
	]}}]}`

	adapter := NewAdapter(ModeAIStudio, &types.Transport{BaseURL: "https://example.test"}, nil)
	events, err := adapter.ParseUnary(context.Background(), []byte(body))
	if err != nil {
		t.Fatal(err)
	}

	var text []string
	for _, event := range events {
		if event.Type == types.EventReasoning {
			t.Fatalf("a thought part was hidden as reasoning: %q", event.Reasoning)
		}
		if event.Type == types.EventTextDelta {
			text = append(text, event.Text)
		}
	}
	joined := strings.Join(text, "|")
	if !strings.Contains(joined, "weighing the options") {
		t.Fatalf("thought text never reached the client: %q", joined)
	}
	if !strings.Contains(joined, "the answer is 42") {
		t.Fatalf("ordinary text was lost: %q", joined)
	}
}

// Same rule on the streaming path.
func TestGoogleThoughtTextStaysVisibleTextWhenStreaming(t *testing.T) {
	frame, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{
		"content": map[string]any{"parts": []any{map[string]any{"text": "thinking out loud", "thought": true}}},
	}}})

	adapter := NewAdapter(ModeAIStudio, &types.Transport{BaseURL: "https://example.test"}, nil)
	body := struct {
		*strings.Reader
	}{strings.NewReader("data: " + string(frame) + "\n\n")}

	for event := range adapter.ParseStream(context.Background(), nopCloser{body}) {
		if event.Type == types.EventReasoning {
			t.Fatalf("a streamed thought part was hidden as reasoning: %q", event.Reasoning)
		}
	}
}

type nopCloser struct{ r interface{ Read([]byte) (int, error) } }

func (n nopCloser) Read(p []byte) (int, error) { return n.r.Read(p) }
func (nopCloser) Close() error                 { return nil }
