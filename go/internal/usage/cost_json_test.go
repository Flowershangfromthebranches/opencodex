package usage

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// The TypeScript oracle (src/usage/cost.ts:29-42) declares these key sets, and
// the GUI reads them verbatim (gui/src/pages/Logs.tsx:53). Untagged Go structs
// serialize as `Total`/`Input`, so a correctly priced row reaches the client
// with no `total` key and renders as an em dash (devlog 260729 010).
var (
	oracleCostBreakdownKeys = []string{"cacheRead", "cacheWrite", "input", "output", "total"}
	oracleCostTokensKeys    = []string{"cacheRead", "cacheWrite", "input", "output"}
)

func marshalledKeys(t *testing.T, value any) []string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &probe); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(probe))
	for key := range probe {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertKeys(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("key set mismatch:\n got %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("key set mismatch:\n got %v\nwant %v", got, want)
		}
	}
}

func TestCostBreakdownJSONKeysMatchOracle(t *testing.T) {
	assertKeys(t, marshalledKeys(t, CostBreakdown{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4, Total: 10}), oracleCostBreakdownKeys)
}

func TestCostTokensJSONKeysMatchOracle(t *testing.T) {
	assertKeys(t, marshalledKeys(t, CostTokens{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}), oracleCostTokensKeys)
}

// Walks the exact access path the GUI uses:
// displayMetrics.cost.estimate.cost.total (gui/src/pages/Logs.tsx:208) and
// .estimate.tokens.input, so a key-casing regression fails here rather than
// silently blanking the cost column.
func TestEstimateCostJSONShapeMatchesOracle(t *testing.T) {
	estimate, ok := EstimateCost("deepseek", "deepseek-chat", types.Usage{InputTokens: 1_000_000}, StatusReported, nil)
	if !ok {
		t.Fatal("expected deepseek/deepseek-chat to be priced by the overlay roster")
	}
	encoded, err := json.Marshal(estimate)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	cost, ok := decoded["cost"].(map[string]any)
	if !ok {
		t.Fatalf("estimate has no `cost` object: %s", encoded)
	}
	total, ok := cost["total"].(float64)
	if !ok {
		t.Fatalf("cost has no lowercase `total` key (GUI reads cost.total): %v", cost)
	}
	if total <= 0 {
		t.Fatalf("expected a positive total, got %v", total)
	}

	tokens, ok := decoded["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("estimate has no `tokens` object: %s", encoded)
	}
	if _, ok := tokens["input"].(float64); !ok {
		t.Fatalf("tokens has no lowercase `input` key: %v", tokens)
	}
}
