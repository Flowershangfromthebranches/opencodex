package usage

import (
	"math"

	"github.com/lidge-jun/opencodex-go/internal/providers"
	"github.com/lidge-jun/opencodex-go/internal/types"
)

// JSON keys mirror the TypeScript oracle exactly (src/usage/cost.ts:29-42).
// The GUI reads the lowercase form (gui/src/pages/Logs.tsx:53,208); untagged
// fields serialize as `Total`/`Input`, which made every correctly priced row
// render as an em dash because `cost.total` was undefined (devlog 260729 010).
type CostTokens struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
}

type CostBreakdown struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}
type AttemptCostEstimate struct {
	Ordinal            int           `json:"ordinal"`
	Provider           string        `json:"provider"`
	Model              string        `json:"model"`
	Tokens             CostTokens    `json:"tokens"`
	Price              PriceOverlay  `json:"price"`
	Cost               CostBreakdown `json:"cost"`
	Estimated          bool          `json:"estimated"`
	PriorityMultiplier float64       `json:"priorityMultiplier,omitempty"`
}
type CostEstimate struct {
	Tokens             CostTokens            `json:"tokens"`
	Cost               CostBreakdown         `json:"cost"`
	Price              PriceOverlay          `json:"price"`
	Estimated          bool                  `json:"estimated"`
	PriorityMultiplier float64               `json:"priorityMultiplier,omitempty"`
	Attempts           []AttemptCostEstimate `json:"attempts,omitempty"`
}

var priorityMultipliers = map[string]float64{
	"gpt-5.6-sol": 2, "gpt-5.6-terra": 2, "gpt-5.6-luna": 2,
	"gpt-5.5": 2.5, "gpt-5.4": 2,
}

func PriorityMultiplier(modelID string) float64 {
	if multiplier := priorityMultipliers[modelID]; multiplier > 0 {
		return multiplier
	}
	return 1
}

func EffectiveServiceTier(entry Entry) string {
	if entry.ResponseServiceTier != "" {
		return entry.ResponseServiceTier
	}
	if entry.RequestedServiceTier != "" {
		return entry.RequestedServiceTier
	}
	return entry.ConfiguredServiceTier
}

func NormalizeCostTokens(value types.Usage) (CostTokens, bool) {
	if !validUsage(value) {
		return CostTokens{}, false
	}
	write := value.CacheCreationInputTokens
	read := value.CacheReadInputTokens
	if read == 0 {
		read = value.CachedInputTokens
	}
	candidates := []int{read}
	if value.CacheReadInputTokens == 0 && value.CachedInputTokens > 0 && write > 0 {
		candidates = append(candidates, max(0, value.CachedInputTokens-write))
	}
	if write > value.InputTokens {
		return CostTokens{}, false
	}
	for _, candidate := range candidates {
		if candidate > value.InputTokens-write {
			continue
		}
		return CostTokens{Input: value.InputTokens - write - candidate, Output: value.OutputTokens, CacheRead: candidate, CacheWrite: write}, true
	}
	return CostTokens{}, false
}

func CalculateCost(tokens CostTokens, price Price) CostBreakdown {
	result := CostBreakdown{
		Input:      float64(tokens.Input) * price.Input / 1_000_000,
		Output:     float64(tokens.Output) * price.Output / 1_000_000,
		CacheRead:  float64(tokens.CacheRead) * price.CacheRead / 1_000_000,
		CacheWrite: float64(tokens.CacheWrite) * price.CacheWrite / 1_000_000,
	}
	result.Total = result.Input + result.Output + result.CacheRead + result.CacheWrite
	return result
}

func EstimateCost(provider, model string, value types.Usage, status Status, overlays []PriceOverlay) (CostEstimate, bool) {
	return EstimateCostWithTier(provider, model, value, status, overlays, "")
}

func EstimateCostWithTier(provider, model string, value types.Usage, status Status, overlays []PriceOverlay, serviceTier string) (CostEstimate, bool) {
	tokens, ok := NormalizeCostTokens(value)
	if !ok {
		return CostEstimate{}, false
	}
	// Full resolution chain (jawcode -> overlay -> cross-provider vendor
	// fallback), not the overlay roster alone; nil overlays selects the default
	// roster and enables memoization.
	price, ok := ResolveMatchedPrice(provider, model, overlays)
	if !ok {
		return CostEstimate{}, false
	}
	multiplier := 1.0
	base := BaseProvider(provider)
	if serviceTier == "priority" && (base == "openai" || base == "openai-apikey") {
		multiplier = PriorityMultiplier(model)
	}
	effectivePrice := price.Price
	if multiplier != 1 {
		effectivePrice.Input *= multiplier
		effectivePrice.Output *= multiplier
		effectivePrice.CacheRead *= multiplier
		effectivePrice.CacheWrite *= multiplier
	}
	return CostEstimate{Tokens: tokens, Cost: CalculateCost(tokens, effectivePrice), Price: price, PriorityMultiplier: multiplier,
		Estimated: value.Estimated || status == StatusEstimated || price.Status == PriceVerifiedDerived}, true
}

func EstimateComboCost(attempts []Attempt, overlays []PriceOverlay, serviceTier string) (CostEstimate, bool) {
	if len(attempts) == 0 {
		return CostEstimate{}, false
	}
	result := CostEstimate{}
	for _, attempt := range attempts {
		if attempt.Usage == nil {
			return CostEstimate{}, false
		}
		estimate, ok := EstimateCostWithTier(attempt.Provider, attempt.Model, *attempt.Usage, attempt.UsageStatus, overlays, serviceTier)
		if !ok {
			return CostEstimate{}, false
		}
		result.Tokens.Input += estimate.Tokens.Input
		result.Tokens.Output += estimate.Tokens.Output
		result.Tokens.CacheRead += estimate.Tokens.CacheRead
		result.Tokens.CacheWrite += estimate.Tokens.CacheWrite
		result.Cost.Input += estimate.Cost.Input
		result.Cost.Output += estimate.Cost.Output
		result.Cost.CacheRead += estimate.Cost.CacheRead
		result.Cost.CacheWrite += estimate.Cost.CacheWrite
		result.Cost.Total += estimate.Cost.Total
		result.Estimated = result.Estimated || estimate.Estimated
		result.Attempts = append(result.Attempts, AttemptCostEstimate{Ordinal: attempt.Ordinal, Provider: attempt.Provider, Model: attempt.Model, Tokens: estimate.Tokens, Price: estimate.Price, Cost: estimate.Cost, Estimated: estimate.Estimated, PriorityMultiplier: estimate.PriorityMultiplier})
		if result.PriorityMultiplier == 0 && estimate.PriorityMultiplier != 0 && estimate.PriorityMultiplier != 1 {
			result.PriorityMultiplier = estimate.PriorityMultiplier
		}
	}
	return result, true
}

func TokensPerSecond(outputTokens int, durationMS int64) (float64, bool) {
	if outputTokens <= 0 || durationMS <= 0 {
		return 0, false
	}
	value := float64(outputTokens) / (float64(durationMS) / 1000)
	return value, !math.IsInf(value, 0) && !math.IsNaN(value)
}

// BaseProvider collapses pool and account-log suffixes before pricing or
// grouping. It delegates to the single canonical implementation, an exact port of
// src/providers/label.ts:7-19; the previous hardcoded four-prefix list silently
// failed for pooled providers such as kimi-code-pabcdef, which then lost their
// price entirely and split into their own summary row.
//
// The symbol stays so its ten call sites keep one shared definition across the
// resolver, the tier multiplier, and summary grouping.
func BaseProvider(provider string) string {
	return providers.BaseProviderLabel(provider)
}

// finiteNonNegative mirrors the oracle's rate validation (src/usage/cost.ts:74):
// a negative or non-finite rate is corrupt data, not a discount.
func finiteNonNegative(value float64) bool {
	return !math.IsInf(value, 0) && !math.IsNaN(value) && value >= 0
}
