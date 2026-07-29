package usage

type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

type PriceStatus string

const (
	PriceVerified        PriceStatus = "verified"
	PriceVerifiedDerived PriceStatus = "verified-derived"
)

// Mirrors MatchedPrice in src/usage/cost.ts:45-53. Source is the DISCRIMINATOR
// ("jawcode" | "expected") and SourceRef carries the provenance URL. They were a
// single field holding the URL, which made the `Source == "expected"` check in
// request_log_port.go unreachable and left the dashboard rendering a URL where it
// expects an i18n key (gui/src/pages/Logs.tsx:771).
type PriceOverlay struct {
	Provider        string      `json:"provider"`
	Model           string      `json:"modelId"`
	JawcodeProvider string      `json:"jawcodeProvider,omitempty"`
	Price           Price       `json:"cost4"`
	Source          string      `json:"source"`
	SourceRef       string      `json:"sourceRef,omitempty"`
	VerifiedAt      string      `json:"verifiedAt,omitempty"`
	Status          PriceStatus `json:"status"`
}

// Price source discriminators, matching the oracle's MatchedPrice.source union.
const (
	PriceSourceJawcode  = "jawcode"
	PriceSourceExpected = "expected"
)

var (
	gemini31Pro         = Price{Input: 2, Output: 12, CacheRead: .2}
	gemini36Flash       = Price{Input: 1.5, Output: 7.5, CacheRead: .15}
	minimaxM21Highspeed = Price{Input: .6, Output: 2.4, CacheRead: .03, CacheWrite: .375}
	kimiK3              = Price{Input: 3, Output: 15, CacheRead: .3, CacheWrite: 3}
	kimiK27Code         = Price{Input: .95, Output: 4, CacheRead: .19, CacheWrite: .95}
	kimiK27Highspeed    = Price{Input: 1.9, Output: 8, CacheRead: .38, CacheWrite: 1.9}
	kimiK26             = Price{Input: .95, Output: 4, CacheRead: .16, CacheWrite: .95}
	kimiK25             = Price{Input: .6, Output: 3, CacheRead: .1, CacheWrite: .6}
	claudeSonnet46      = Price{Input: 3, Output: 15, CacheRead: .3, CacheWrite: 3.75}
	claudeOpus46        = Price{Input: 5, Output: 25, CacheRead: .5, CacheWrite: 6.25}
	qwen38Routeway      = Price{Input: 1.5, Output: 5, CacheRead: .15}
)

const (
	geminiPricing    = "https://ai.google.dev/gemini-api/docs/pricing"
	minimaxPricing   = "https://platform.minimax.io/docs/guides/pricing-paygo"
	deepseekPricing  = "https://api-docs.deepseek.com/quick_start/pricing-details-usd"
	kimiPricing      = "https://platform.kimi.ai/docs/pricing"
	anthropicPricing = "https://platform.claude.com/docs/en/about-claude/pricing"
)

// ExpectedPriceOverlays mirrors the verified repository price roster. Lookup
// remains exact and never invents fuzzy or case-folded billing identities.
var ExpectedPriceOverlays = []PriceOverlay{
	{Provider: "anthropic", Model: "claude-opus-5", Price: claudeOpus46, SourceRef: "user-confirmed: claude-opus-5 matches Claude Opus 4.6", VerifiedAt: "2026-07-25", Status: PriceVerifiedDerived},
	{Provider: "cursor", Model: "claude-opus-5", Price: claudeOpus46, SourceRef: "user-confirmed: claude-opus-5 matches Claude Opus 4.6", VerifiedAt: "2026-07-25", Status: PriceVerifiedDerived},
	{Provider: "kiro", Model: "claude-opus-5", Price: claudeOpus46, SourceRef: "user-confirmed: claude-opus-5 matches Claude Opus 4.6", VerifiedAt: "2026-07-25", Status: PriceVerifiedDerived},
	{Provider: "minimax", Model: "MiniMax-M2.1-highspeed", Price: minimaxM21Highspeed, SourceRef: minimaxPricing, VerifiedAt: "2026-07-20", Status: PriceVerified},
	{Provider: "minimax-cn", Model: "MiniMax-M2.1-highspeed", Price: minimaxM21Highspeed, SourceRef: minimaxPricing, VerifiedAt: "2026-07-20", Status: PriceVerified},
	{Provider: "deepseek", Model: "deepseek-chat", Price: Price{Input: .27, Output: 1.1, CacheRead: .07}, SourceRef: deepseekPricing, VerifiedAt: "2026-07-20", Status: PriceVerified},
	{Provider: "deepseek", Model: "deepseek-reasoner", Price: Price{Input: .55, Output: 2.19, CacheRead: .14}, SourceRef: deepseekPricing, VerifiedAt: "2026-07-20", Status: PriceVerified},
	{Provider: "google", Model: "gemini-3.6-flash", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerified},
	{Provider: "google-antigravity", Model: "gemini-3.6-flash", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerified},
	{Provider: "google-antigravity", Model: "gemini-3.1-pro", Price: gemini31Pro, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerified},
	{Provider: "google-antigravity", Model: "gemini-3.1-pro-low", Price: gemini31Pro, SourceRef: geminiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.1-pro-high", Price: gemini31Pro, SourceRef: geminiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.1-pro-preview", Price: gemini31Pro, SourceRef: geminiPricing, VerifiedAt: "2026-07-20", Status: PriceVerified},
	{Provider: "google-antigravity", Model: "gemini-pro-agent", Price: gemini31Pro, SourceRef: geminiPricing, VerifiedAt: "2026-07-23", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.6-flash-low", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.6-flash-medium", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.6-flash-high", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.5-flash-extra-low", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.5-flash-low", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.5-flash-mid", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3.5-flash-high", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "gemini-3-flash-agent", Price: gemini36Flash, SourceRef: geminiPricing, VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "google-antigravity", Model: "claude-sonnet-4-6", Price: claudeSonnet46, SourceRef: anthropicPricing, VerifiedAt: "2026-07-23", Status: PriceVerified},
	{Provider: "google-antigravity", Model: "claude-opus-4-6-thinking", Price: claudeOpus46, SourceRef: anthropicPricing, VerifiedAt: "2026-07-23", Status: PriceVerified},
	{Provider: "google-antigravity", Model: "claude-opus-4-6", Price: claudeOpus46, SourceRef: anthropicPricing, VerifiedAt: "2026-07-23", Status: PriceVerified},
	{Provider: "google-antigravity", Model: "gpt-oss-120b-medium", Price: Price{Input: .03, Output: .15}, SourceRef: "https://openrouter.ai/openai/gpt-oss-120b/providers", VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi", Model: "k3", Price: kimiK3, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi", Model: "k3[1m]", Price: kimiK3, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi", Model: "kimi-k2.7-code", Price: kimiK27Code, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi", Model: "kimi-k2.7-code-highspeed", Price: kimiK27Highspeed, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi", Model: "kimi-k2.6", Price: kimiK26, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi", Model: "kimi-k2.5", Price: kimiK25, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi", Model: "kimi-for-coding", Price: kimiK27Code, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "moonshot", Model: "kimi-k3", Price: kimiK3, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "moonshot", Model: "kimi-k2.7-code", Price: kimiK27Code, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "moonshot", Model: "kimi-k2.7-code-highspeed", Price: kimiK27Highspeed, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "moonshot", Model: "kimi-k2.6", Price: kimiK26, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "moonshot", Model: "kimi-k2.5", Price: kimiK25, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi-code", Model: "k3", Price: kimiK3, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi-code", Model: "k3[1m]", Price: kimiK3, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi-code", Model: "kimi-k2.7-code", Price: kimiK27Code, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi-code", Model: "kimi-k2.7-code-highspeed", Price: kimiK27Highspeed, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi-code", Model: "kimi-k2.6", Price: kimiK26, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi-code", Model: "kimi-k2.5", Price: kimiK25, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "kimi-code", Model: "kimi-for-coding", Price: kimiK27Code, SourceRef: kimiPricing, VerifiedAt: "2026-07-20", Status: PriceVerifiedDerived},
	{Provider: "alibaba-token-plan", Model: "qwen3.8-max-preview", Price: qwen38Routeway, SourceRef: "https://routeway.ai/models/qwen3.8-max-preview", VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "alibaba-token-plan-intl", Model: "qwen3.8-max-preview", Price: qwen38Routeway, SourceRef: "https://routeway.ai/models/qwen3.8-max-preview", VerifiedAt: "2026-07-22", Status: PriceVerifiedDerived},
	{Provider: "cursor", Model: "auto", Price: Price{Input: 1.25, Output: 6, CacheRead: .25, CacheWrite: 1.25}, SourceRef: "https://docs.cursor.com/account/pricing", VerifiedAt: "2026-07-20", Status: PriceVerified},
}

// FindPrice is the overlay stage of the resolution chain. It returns ONLY
// verified or verified-derived rows: PriceStatus is a bare string type, so a
// caller can construct any status it likes, and this filter is what keeps the
// lookup fail-closed (src/usage/expected-prices.ts:141-143).
func FindPrice(provider, model string, overlays []PriceOverlay) (PriceOverlay, bool) {
	provider = BaseProvider(provider)
	var derived *PriceOverlay
	for index := range overlays {
		row := overlays[index]
		if row.Provider != provider || row.Model != model {
			continue
		}
		if row.Status == PriceVerified {
			row.Source = PriceSourceExpected
			return row, true
		}
		if row.Status == PriceVerifiedDerived && derived == nil {
			copy := row
			derived = &copy
		}
	}
	if derived != nil {
		row := *derived
		row.Source = PriceSourceExpected
		return row, true
	}
	return PriceOverlay{}, false
}
