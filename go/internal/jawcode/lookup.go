// Package jawcode carries the generated model/price catalog mirrored from the
// TypeScript oracle. It exists so the Go runtime can price the same models the
// TypeScript runtime prices; before it existed, Go could only match a ~48-row
// overlay roster and reported the rest as price_unmatched.
//
// The package is intentionally free of dependencies on internal/usage so the
// price resolver can import it without a cycle.
package jawcode

import (
	"sort"
	"strings"
)

// ResolveProvider maps a routing provider id onto its catalog bundle
// (mirrors resolveJawcodeProvider in the oracle). Unknown providers return false
// rather than guessing: a wrong bundle would price a request against the wrong
// vendor's rate card.
func ResolveProvider(provider string) (string, bool) {
	bundle, ok := providerAliases[provider]
	return bundle, ok
}

// Metadata is an exact provider+id lookup (mirrors getJawcodeModelMetadata).
func Metadata(provider, modelID string) (Model, bool) {
	rows, ok := models[provider]
	if !ok {
		return Model{}, false
	}
	row, ok := rows[modelID]
	return row, ok
}

// MetadataCaseInsensitive mirrors getJawcodeModelMetadataCaseInsensitive. The
// exact form is tried first so a case-folded near-match can never shadow a real
// row.
func MetadataCaseInsensitive(provider, modelID string) (Model, bool) {
	if row, ok := Metadata(provider, modelID); ok {
		return row, true
	}
	rows, ok := models[provider]
	if !ok {
		return Model{}, false
	}
	lower := strings.ToLower(modelID)
	for id, row := range rows {
		if strings.ToLower(id) == lower {
			return row, true
		}
	}
	return Model{}, false
}

// List returns a bundle's rows sorted by model id. Map iteration order is
// randomized in Go, so returning the map's natural order would make callers
// nondeterministic.
func List(provider string) []Model {
	rows, ok := models[provider]
	if !ok {
		return nil
	}
	out := make([]Model, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FindCostByModelID is the cross-provider vendor fallback (mirrors
// findJawcodeCostByModelId): a model follows its official vendor price whichever
// provider serves it. It walks costVendorPriority IN ORDER and returns the first
// nonzero cost; an all-zero row means "not billable here", not "free", so it is
// skipped rather than treated as a hit.
func FindCostByModelID(modelID string) (string, Cost, bool) {
	for _, vendor := range costVendorPriority {
		row, ok := Metadata(vendor, modelID)
		if !ok || row.Cost == nil {
			continue
		}
		if HasNonZeroCost(*row.Cost) {
			return vendor, *row.Cost, true
		}
	}
	return "", Cost{}, false
}

// HasNonZeroCost mirrors the oracle's nonzero test.
func HasNonZeroCost(cost Cost) bool {
	return cost.Input != 0 || cost.Output != 0 || cost.CacheRead != 0 || cost.CacheWrite != 0
}

// Providers lists the catalog bundle names, sorted for determinism.
func Providers() []string {
	out := make([]string, 0, len(models))
	for provider := range models {
		out = append(out, provider)
	}
	sort.Strings(out)
	return out
}

// CostVendorPriority exposes a copy of the ordered fallback list for tests that
// pin the order.
func CostVendorPriority() []string {
	return append([]string(nil), costVendorPriority...)
}

// ProviderAliases exposes a copy of the alias map for parity tests.
func ProviderAliases() map[string]string {
	out := make(map[string]string, len(providerAliases))
	for key, value := range providerAliases {
		out[key] = value
	}
	return out
}
