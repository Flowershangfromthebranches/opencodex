package usage

import (
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// Each stage of the chain gets an input that can ONLY be answered by that stage,
// so a stage that stops firing fails its own case instead of hiding behind the
// others (C-ACTIVATION-GROUNDING-01).
func TestResolveChainStagesEachFire(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		provider, model string
		wantSource      string
		wantStatus      PriceStatus
		wantInput       float64
		wantJawcode     string
	}{
		{
			// Stage 1 needs a provider that is an ALIAS KEY: resolveJawcodeProvider
			// maps routing ids onto bundles, and "openai"/"deepseek"/"zai" are
			// bundle names that are NOT alias keys, so the oracle skips stage 1 for
			// them and reaches the same rate through the vendor fallback. "kimi" is
			// an alias key (-> moonshot bundle), so a verified hit here proves the
			// catalog branch itself ran.
			name: "stage1 jawcode exact via alias", provider: "anthropic", model: "claude-sonnet-4-6",
			wantSource: PriceSourceJawcode, wantStatus: PriceVerified, wantInput: 3, wantJawcode: "anthropic",
		},
		{
			// Stage 2: deepseek-chat is an overlay row; the catalog's deepseek
			// bundle does not carry it, so only the overlay can answer.
			name: "stage2 overlay verified", provider: "deepseek", model: "deepseek-chat",
			wantSource: PriceSourceExpected, wantStatus: PriceVerified, wantInput: 0.27,
		},
		{
			// Stage 2 again, derived tier.
			name: "stage2 overlay derived", provider: "anthropic", model: "claude-opus-5",
			wantSource: PriceSourceExpected, wantStatus: PriceVerifiedDerived, wantInput: 5,
		},
		{
			// Stage 3: kiro is not a catalog bundle and has no overlay row for
			// this id; only the cross-provider vendor fallback can price it, and
			// only after the dot-to-dash retry.
			name: "stage3 vendor fallback with dot normalization", provider: "kiro", model: "claude-opus-4.6",
			wantSource: PriceSourceJawcode, wantStatus: PriceVerifiedDerived, wantInput: 5, wantJawcode: "anthropic",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			price, ok := ResolveMatchedPrice(testCase.provider, testCase.model, nil)
			if !ok {
				t.Fatalf("%s/%s did not resolve", testCase.provider, testCase.model)
			}
			if price.Source != testCase.wantSource {
				t.Fatalf("source = %q, want %q", price.Source, testCase.wantSource)
			}
			if price.Status != testCase.wantStatus {
				t.Fatalf("status = %q, want %q", price.Status, testCase.wantStatus)
			}
			if testCase.wantInput != 0 && price.Price.Input != testCase.wantInput {
				t.Fatalf("input rate = %v, want %v", price.Price.Input, testCase.wantInput)
			}
			if testCase.wantJawcode != "" && price.JawcodeProvider != testCase.wantJawcode {
				t.Fatalf("jawcodeProvider = %q, want %q", price.JawcodeProvider, testCase.wantJawcode)
			}
		})
	}
}

// Stage 4: an unknown pair must fail closed rather than inventing a price.
func TestResolveUnknownFailsClosed(t *testing.T) {
	if price, ok := ResolveMatchedPrice("no-such-provider", "no-such-model", nil); ok {
		t.Fatalf("unknown pair resolved to %+v", price)
	}
}

// The ordering itself is the contract: when the catalog and the overlay both
// answer, the catalog wins.
func TestJawcodeBeatsOverlay(t *testing.T) {
	const provider, model = "deepseek", "deepseek-chat"
	custom := []PriceOverlay{{
		Provider: provider, Model: model, Status: PriceVerified,
		Price: Price{Input: 999, Output: 999},
	}}
	price, ok := ResolveMatchedPrice(provider, model, custom)
	if !ok {
		t.Fatal("did not resolve")
	}
	// deepseek-chat is not in the catalog, so the overlay legitimately wins here.
	if price.Price.Input != 999 {
		t.Fatalf("overlay should answer for a non-catalog model, got %v", price.Price.Input)
	}
	// A provider that IS an alias key resolves through the catalog, which must beat
	// a competing overlay. (openai is a bundle name but not an alias key, so it
	// would legitimately answer from the overlay instead - see the chain test.)
	competing := []PriceOverlay{{
		Provider: "anthropic", Model: "claude-sonnet-4-6", Status: PriceVerified,
		Price: Price{Input: 999, Output: 999}, SourceRef: "competing",
	}}
	price, ok = ResolveMatchedPrice("anthropic", "claude-sonnet-4-6", competing)
	if !ok {
		t.Fatal("catalog model did not resolve")
	}
	if price.Source != PriceSourceJawcode || price.Price.Input == 999 {
		t.Fatalf("catalog did not win: source=%q input=%v", price.Source, price.Price.Input)
	}
}

// An all-zero catalog row means "not billable here", not "free", so it must not
// short-circuit the chain; the overlay stays eligible.
func TestZeroCostCatalogRowFallsThrough(t *testing.T) {
	// zai/glm-4.7 is an all-zero catalog row (72 such rows exist).
	custom := []PriceOverlay{{
		Provider: "zai", Model: "glm-4.7", Status: PriceVerified,
		Price: Price{Input: 1.5, Output: 6}, SourceRef: "test",
	}}
	price, ok := ResolveMatchedPrice("zai", "glm-4.7", custom)
	if !ok {
		t.Fatal("did not resolve")
	}
	if price.Source != PriceSourceExpected || price.Price.Input != 1.5 {
		t.Fatalf("all-zero catalog row short-circuited: source=%q input=%v", price.Source, price.Price.Input)
	}
}

// PriceStatus is a bare string type, so a caller can construct any status. The
// overlay filter is the only thing keeping the lookup fail-closed. An unverified
// row must be IGNORED - after which ordinary fallback applies, which is what the
// oracle actually does (it is not a hard null; src/usage/cost.ts:205-208).
func TestUnverifiedOverlayIsIgnoredThenFallsBack(t *testing.T) {
	custom := []PriceOverlay{{
		Provider: "kiro", Model: "claude-opus-4.6", Status: PriceStatus("unverified"),
		Price: Price{Input: 999, Output: 999},
	}}
	price, ok := ResolveMatchedPrice("kiro", "claude-opus-4.6", custom)
	if !ok {
		t.Fatal("expected the vendor fallback to answer after the unverified row was ignored")
	}
	if price.Price.Input == 999 {
		t.Fatal("unverified overlay was used; the filter is not fail-closed")
	}
	if price.Source != PriceSourceJawcode || price.JawcodeProvider != "anthropic" {
		t.Fatalf("expected the anthropic vendor fallback, got source=%q vendor=%q", price.Source, price.JawcodeProvider)
	}
}

// The end-to-end effect: a catalog model now produces a real dollar figure where
// it previously reported price_unmatched.
func TestCatalogModelNowPrices(t *testing.T) {
	estimate, ok := EstimateCost("openai", "gpt-5.6-sol", types.Usage{InputTokens: 1_000_000}, StatusReported, nil)
	if !ok {
		t.Fatal("gpt-5.6-sol still unpriced")
	}
	if estimate.Cost.Total != 5 {
		t.Fatalf("total = %v, want 5 (1M input at $5/1M)", estimate.Cost.Total)
	}
	if estimate.Price.Source != PriceSourceJawcode {
		t.Fatalf("source = %q, want jawcode", estimate.Price.Source)
	}
}

// Pool suffixes must collapse identically for the resolver, the tier multiplier
// and summary grouping - one definition, not two.
func TestPooledProviderPricesThroughSharedNormalization(t *testing.T) {
	price, ok := ResolveMatchedPrice("kimi-code-pabcdef", "k3", nil)
	if !ok {
		t.Fatal("pooled provider lost its price; BaseProvider did not collapse the account suffix")
	}
	if price.Price.Input != 3 {
		t.Fatalf("input rate = %v, want 3", price.Price.Input)
	}
	if got := BaseProvider("chatgpt"); got != "openai" {
		t.Fatalf("BaseProvider(chatgpt) = %q, want openai", got)
	}
}

// Memoization must not leak a custom-overlay answer into the default cache.
func TestCustomOverlaysBypassTheCache(t *testing.T) {
	const provider, model = "deepseek", "deepseek-chat"
	base, ok := ResolveMatchedPrice(provider, model, nil)
	if !ok {
		t.Fatal("baseline did not resolve")
	}
	custom := []PriceOverlay{{
		Provider: provider, Model: model, Status: PriceVerified,
		Price: Price{Input: 42}, SourceRef: "custom",
	}}
	if got, _ := ResolveMatchedPrice(provider, model, custom); got.Price.Input != 42 {
		t.Fatalf("custom overlay ignored: %v", got.Price.Input)
	}
	again, ok := ResolveMatchedPrice(provider, model, nil)
	if !ok || again.Price.Input != base.Price.Input {
		t.Fatalf("default lookup was poisoned by the custom overlay: %v vs %v", again.Price.Input, base.Price.Input)
	}
}
