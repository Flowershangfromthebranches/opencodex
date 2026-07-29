package jawcode

import (
	"strings"
	"testing"
)

// Counts measured from the TypeScript oracle at generation time:
//
//	bun -e '...listJawcodeModelMetadata over every bundle...'
//	-> {"bundles":15,"rows":724,"priced":652}
//
// A generator regression (a scan that matches nothing, a dropped bundle, a
// mangled cost tuple) still compiles and still serves; it just prices less. These
// assertions are the thing that makes such a regression loud.
const (
	oracleProviderCount = 15
	oracleRowCount      = 724
	oraclePricedRows    = 652
	oracleAliasCount    = 18
	oracleVendorCount   = 13
)

func TestBundleRowCountsMatchOracle(t *testing.T) {
	providers := Providers()
	if len(providers) != oracleProviderCount {
		t.Fatalf("provider bundles = %d, want %d (%v)", len(providers), oracleProviderCount, providers)
	}
	if generatedProviderCount != oracleProviderCount {
		t.Fatalf("generator recorded %d providers, want %d", generatedProviderCount, oracleProviderCount)
	}

	rows, priced := 0, 0
	for _, provider := range providers {
		for _, row := range List(provider) {
			rows++
			if row.Cost != nil && HasNonZeroCost(*row.Cost) {
				priced++
			}
		}
	}
	if rows != oracleRowCount {
		t.Fatalf("rows = %d, want %d", rows, oracleRowCount)
	}
	if generatedRowCount != oracleRowCount {
		t.Fatalf("generator recorded %d rows, want %d", generatedRowCount, oracleRowCount)
	}
	if priced != oraclePricedRows {
		t.Fatalf("nonzero-priced rows = %d, want %d", priced, oraclePricedRows)
	}
}

// Spot-checks against prices read directly out of the oracle bundle. These are
// the two models the parity investigation used as worked examples.
func TestKnownModelPrices(t *testing.T) {
	for _, testCase := range []struct {
		provider, model string
		want            Cost
	}{
		{"openai", "gpt-5.6-sol", Cost{Input: 5, Output: 30, CacheRead: 0.5, CacheWrite: 6.25}},
		{"anthropic", "claude-opus-4-6", Cost{Input: 5, Output: 25, CacheRead: 0.5, CacheWrite: 6.25}},
	} {
		row, ok := Metadata(testCase.provider, testCase.model)
		if !ok {
			t.Fatalf("%s/%s missing from the bundle", testCase.provider, testCase.model)
		}
		if row.Cost == nil {
			t.Fatalf("%s/%s carries no cost", testCase.provider, testCase.model)
		}
		if *row.Cost != testCase.want {
			t.Fatalf("%s/%s cost = %+v, want %+v", testCase.provider, testCase.model, *row.Cost, testCase.want)
		}
	}
}

// Go map iteration is randomized, so a lookup that walked `models` directly would
// return a different vendor run to run. Repeating the query proves the resolver
// walks the ordered slice instead.
func TestFindCostByModelIDIsDeterministic(t *testing.T) {
	const modelID = "claude-opus-4-6"
	vendor, cost, ok := FindCostByModelID(modelID)
	if !ok {
		t.Fatalf("%s not resolvable through the vendor fallback", modelID)
	}
	if vendor != "anthropic" {
		t.Fatalf("vendor = %q, want anthropic (first priority entry carrying the model)", vendor)
	}
	for i := 0; i < 1000; i++ {
		gotVendor, gotCost, gotOK := FindCostByModelID(modelID)
		if !gotOK || gotVendor != vendor || gotCost != cost {
			t.Fatalf("iteration %d diverged: vendor=%q cost=%+v ok=%v", i, gotVendor, gotCost, gotOK)
		}
	}
}

// Order is the contract, not just membership: FindCostByModelID returns the first
// nonzero hit, so reordering silently repricies every model served by more than
// one vendor.
func TestCostVendorPriorityOrderIsPinned(t *testing.T) {
	want := []string{
		"anthropic", "openai", "google", "moonshot", "minimax", "deepseek", "xai",
		"zai", "mistral", "cerebras", "azure-openai", "amazon-bedrock", "xiaomi",
	}
	got := CostVendorPriority()
	if len(got) != oracleVendorCount {
		t.Fatalf("vendor priority length = %d, want %d", len(got), oracleVendorCount)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("vendor priority[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// An all-zero row means "not billable here", not "free", so the fallback must
// skip it and keep looking rather than returning a zero price.
func TestFindCostSkipsZeroCostRows(t *testing.T) {
	zeroRows := 0
	for _, provider := range Providers() {
		for _, row := range List(provider) {
			if row.Cost != nil && !HasNonZeroCost(*row.Cost) {
				zeroRows++
				if _, cost, ok := FindCostByModelID(row.ID); ok && !HasNonZeroCost(cost) {
					t.Fatalf("%s/%s returned an all-zero cost from the fallback", provider, row.ID)
				}
			}
		}
	}
	if zeroRows == 0 {
		t.Fatal("expected the catalog to contain all-zero rows; measured 72 at generation time")
	}
}

func TestProviderAliasesResolve(t *testing.T) {
	aliases := ProviderAliases()
	if len(aliases) != oracleAliasCount {
		t.Fatalf("alias count = %d, want %d", len(aliases), oracleAliasCount)
	}
	// Aliases exist so routing ids that are not bundle names still price.
	for alias, want := range map[string]string{
		"anthropic-apikey": "anthropic",
		"gemini":           "google",
		"kimi":             "moonshot",
		"zhipu-bigmodel":   "zai",
	} {
		got, ok := ResolveProvider(alias)
		if !ok || got != want {
			t.Fatalf("ResolveProvider(%q) = %q,%v; want %q,true", alias, got, ok, want)
		}
	}
	if _, ok := ResolveProvider("no-such-provider"); ok {
		t.Fatal("unknown provider resolved; it must fail closed rather than guess a bundle")
	}
	// Every alias must point at a bundle that actually exists.
	for alias, bundle := range aliases {
		if len(List(bundle)) == 0 {
			t.Fatalf("alias %q -> %q, but that bundle has no rows", alias, bundle)
		}
	}
}

func TestListIsSortedAndCaseInsensitiveLookupPrefersExact(t *testing.T) {
	rows := List("anthropic")
	if len(rows) == 0 {
		t.Fatal("anthropic bundle empty")
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].ID >= rows[i].ID {
			t.Fatalf("List not sorted at %d: %q >= %q", i, rows[i-1].ID, rows[i].ID)
		}
	}
	exact := rows[0].ID
	if row, ok := MetadataCaseInsensitive("anthropic", strings.ToUpper(exact)); !ok || row.ID != exact {
		t.Fatalf("case-insensitive lookup of %q failed: %+v ok=%v", exact, row, ok)
	}
}
