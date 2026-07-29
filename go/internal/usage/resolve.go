package usage

import (
	"strings"
	"sync"

	"github.com/lidge-jun/opencodex-go/internal/jawcode"
	"github.com/lidge-jun/opencodex-go/internal/providers"
)

// Price resolution mirrors src/usage/cost.ts:146-236. Fixed priority:
//
//	1. jawcode exact match on the provider's catalog bundle, nonzero  -> verified
//	2. expected-price overlay (verified beats verified-derived inside FindPrice)
//	3. anything else -> cross-provider vendor fallback: a model follows its
//	   official vendor price whichever provider serves it
//	4. no match -> not priced, and the dashboard shows an em dash
//
// An all-zero jawcode row is NOT a price: zero means "not billable here", not
// "free", so such rows stay overlay candidates instead of short-circuiting.

// resolveCacheLimit mirrors the oracle's 512-entry cap (src/usage/cost.ts:159).
// Usage summaries walk hundreds of thousands of rows that share a handful of
// provider/model keys, so resolving each row from scratch dominated /api/usage
// latency in the TypeScript runtime.
const resolveCacheLimit = 512

var (
	resolveCacheMu sync.RWMutex
	resolveCache   = make(map[string]resolveCacheEntry, resolveCacheLimit)
)

type resolveCacheEntry struct {
	price PriceOverlay
	found bool
}

// ResolveMatchedPrice returns the price for one provider/model pair.
//
// Passing nil for overlays selects the default roster and enables memoization.
// A caller-supplied slice bypasses the cache entirely, because the cache key is
// only (provider, model) and a custom roster would poison it for everyone else.
func ResolveMatchedPrice(provider, model string, overlays []PriceOverlay) (PriceOverlay, bool) {
	if overlays != nil {
		return resolveUncached(provider, model, overlays)
	}
	key := provider + " " + model
	resolveCacheMu.RLock()
	entry, hit := resolveCache[key]
	resolveCacheMu.RUnlock()
	if hit {
		return entry.price, entry.found
	}
	price, found := resolveUncached(provider, model, ExpectedPriceOverlays)
	resolveCacheMu.Lock()
	if len(resolveCache) >= resolveCacheLimit {
		resolveCache = make(map[string]resolveCacheEntry, resolveCacheLimit)
	}
	resolveCache[key] = resolveCacheEntry{price: price, found: found}
	resolveCacheMu.Unlock()
	return price, found
}

func resolveUncached(provider, model string, overlays []PriceOverlay) (PriceOverlay, bool) {
	// Pool and account-log suffixes are diagnostic identity, not a pricing
	// namespace, so they collapse before any lookup (src/usage/cost.ts:152).
	base := BaseProvider(provider)
	if price, ok := resolveExact(base, model, overlays); ok {
		return price, true
	}
	// Antigravity wire ids often have no exact row; retry on the picker/call base
	// model so collapsed usage rows still price (src/usage/cost.ts:174-180).
	if strings.HasPrefix(base, "google-antigravity") {
		if canonical := providers.CanonicalAntigravityUsageModel(model); canonical != model {
			return resolveExact(base, canonical, overlays)
		}
	}
	return PriceOverlay{}, false
}

func resolveExact(provider, model string, overlays []PriceOverlay) (PriceOverlay, bool) {
	// 1. jawcode exact, nonzero only.
	if bundle, ok := jawcode.ResolveProvider(provider); ok {
		if row, ok := jawcode.Metadata(bundle, model); ok && row.Cost != nil {
			cost := *row.Cost
			if validJawcodeCost(cost) && jawcode.HasNonZeroCost(cost) {
				return PriceOverlay{
					Provider: provider, Model: model, JawcodeProvider: bundle,
					Price:  Price{Input: cost.Input, Output: cost.Output, CacheRead: cost.CacheRead, CacheWrite: cost.CacheWrite},
					Source: PriceSourceJawcode, Status: PriceVerified,
				}, true
			}
		}
	}
	// 2. Overlay. FindPrice already prefers verified over verified-derived and
	//    returns nothing for any other status.
	if overlay, ok := FindPrice(provider, model, overlays); ok && validPrice(overlay.Price) && hasNonZeroPrice(overlay.Price) {
		return overlay, true
	}
	// 3. Cross-provider vendor fallback.
	return resolveModelLevelPrice(provider, model)
}

func resolveModelLevelPrice(provider, model string) (PriceOverlay, bool) {
	vendor, cost, ok := jawcode.FindCostByModelID(model)
	// Retry the dash spelling ONLY when the id actually contains a dot
	// (src/usage/cost.ts:226): kiro says "claude-opus-4.6" where the catalog says
	// "claude-opus-4-6". No fuzzy matching beyond this one normalization.
	if !ok && strings.Contains(model, ".") {
		vendor, cost, ok = jawcode.FindCostByModelID(strings.ReplaceAll(model, ".", "-"))
	}
	if !ok {
		return PriceOverlay{}, false
	}
	return PriceOverlay{
		Provider: provider, Model: model, JawcodeProvider: vendor,
		Price:  Price{Input: cost.Input, Output: cost.Output, CacheRead: cost.CacheRead, CacheWrite: cost.CacheWrite},
		Source: PriceSourceJawcode, Status: PriceVerifiedDerived,
	}, true
}

// validJawcodeCost / validPrice mirror validCost4 (src/usage/cost.ts:78-85):
// a negative or non-finite rate is corrupt data, not a discount.
func validJawcodeCost(cost jawcode.Cost) bool {
	return finiteNonNegative(cost.Input) && finiteNonNegative(cost.Output) &&
		finiteNonNegative(cost.CacheRead) && finiteNonNegative(cost.CacheWrite)
}

func validPrice(price Price) bool {
	return finiteNonNegative(price.Input) && finiteNonNegative(price.Output) &&
		finiteNonNegative(price.CacheRead) && finiteNonNegative(price.CacheWrite)
}

func hasNonZeroPrice(price Price) bool {
	return price.Input != 0 || price.Output != 0 || price.CacheRead != 0 || price.CacheWrite != 0
}
