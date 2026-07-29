package openai

// CatalogModelSupportsReasoningSummaries reports whether the ACTIVE Codex
// catalog says a model supports reasoning summaries, and whether the catalog
// knew the model at all.
//
// A package-level hook rather than a direct call: internal/codex already
// imports this package, so reading the catalog from here directly would be an
// import cycle. The runtime installs the real implementation at startup; when
// it is nil (unit tests, CLI paths that never serve requests) the sanitizer
// falls back to provider config alone, which is the behaviour that shipped
// before this guard existed.
var CatalogModelSupportsReasoningSummaries func(modelID string) (supported bool, known bool)
