# 010 — Design: provider-native web-search backends (xai, google)

## Shape

Extend the EXISTING backend enum rather than inventing a parallel subsystem:

```
OcxWebSearchSidecarConfig.backend?: "openai" | "anthropic" | "xai" | "google"
```

Each new backend is one executor file that follows the anthropic-executor
contract: authenticate with that provider's OWN stored credential, run ONE
query, fold the provider's response into `SidecarOutcome { text, sources,
error? }`, never throw. The loop keeps its single dispatch point.

## Backend: "xai" (Grok Live Search)

- File: `src/web-search/xai-executor.ts`, `runXaiWebSearch(query, providerName,
  provider, settings, signal)`.
- Wire: `POST {baseUrl}/responses` with
  `{ model, input:[{role:"user", content:[{type:"input_text", text:query}]}],
     instructions: BASE_INSTRUCTION(+IMAGE_INSTRUCTION),
     tools:[{type:"web_search"}], stream:false }`.
  Non-streaming: the sidecar result is consumed whole by the loop anyway
  (the anthropic executor streams only because Messages requires it).
- Auth: `Authorization: Bearer <key>` where key = `resolveEnvValue(provider.apiKey)`.
  Key-mode only in v1: the Grok-CLI OAuth transport (cli-chat-proxy.grok.com)
  is a chat-completions surface with a first-party fingerprint; do not assume
  /responses+web_search works there without evidence. Provider eligibility =
  enabled xai-family provider (registry id "xai" or adapter openai-chat with
  api.x.ai baseUrl) holding a usable API key.
- Parse: non-streamed Responses JSON: walk `output[]` for `message` items,
  concat `output_text` blocks, collect `annotations[].url_citation` via the
  same dedupe rule as parse.ts. Strip trailing Sources: via existing helper if
  annotations are empty.
- Model: `cfg.model` when the operator set one AND the xai provider serves it;
  else `DEFAULT_XAI_SIDECAR_MODEL = "grok-4-1-fast"` (cheap, search-capable,
  2M ctx; grok-4.6 accepted but pricier). Bound tokens via `max_output_tokens`.

## Backend: "google" (Gemini google_search grounding)

- File: `src/web-search/google-executor.ts`, `runGoogleWebSearch(query,
  providerName, provider, settings, signal)`.
- Wire: `POST {baseUrl}/v1beta/models/{model}:generateContent` with
  `{ contents:[{role:"user", parts:[{text: instruction+query}]}],
     tools:[{ google_search: {} }] }`.
  CRITICAL: tools contains ONLY google_search — never functionDeclarations —
  honoring the documented multi-tool limitation. The routed model's real tools
  never leak into the sidecar body (the loop already isolates them).
- Auth: `x-goog-api-key` from `resolveEnvValue(provider.apiKey)`. Key-mode
  Gemini API only in v1; vertex/CCA modes excluded (different endpoint shapes
  and undocumented grounding).
- Parse: single JSON response; text = concat `candidates[0].content.parts[].text`,
  sources = `candidates[0].groundingMetadata.groundingChunks[].web` →
  `{ url: uri, title }`, deduped. Grounding absent → sources [], text still
  used (the model may answer from knowledge; that mirrors openai-sidecar
  behavior when it declines to search).
- Model: `cfg.model` if set, else `DEFAULT_GOOGLE_SIDECAR_MODEL =
  "gemini-3.5-flash"` (registry default, search-supported family).

## planWebSearch changes (src/web-search/index.ts)

- `resolveSidecarBackend(explicit)` gains "xai" | "google" passthrough; unset
  still resolves "openai".
- New finders, symmetric with `findAnthropicSidecarProvider`:
  - `findXaiSearchProvider(config)`: first enabled provider with adapter
    "openai-chat" whose baseUrl host is api.x.ai and a non-sentinel apiKey.
  - `findGoogleSearchProvider(config)`: first enabled provider with adapter
    "google", googleMode unset/"gemini", non-sentinel apiKey.
- Explicit backend with no usable provider FAILS CLOSED (no plan) — same rule
  the anthropic backend already enforces (no silent credential borrowing).
- SidecarPlan gains `xaiSidecar? / googleSidecar?` slots (same
  `{providerName, provider}` shape); `describeImages` continues to ride
  `settings`.

## Loop changes (src/web-search/loop.ts)

- `WebSearchLoopDeps.backend` widens to the four-value union; two new optional
  sidecar slots; dispatch becomes a small switch at the single existing call
  site. No other loop behavior changes (budgets, forced-answer pass, failed
  query dedupe all inherited).

## core.ts changes

- Thread the plan's new slots into runWithWebSearch deps (one object spread).
- `shouldResolveOpenAiWebSearchSidecar` already returns false for non-openai
  backends (it checks resolved backend === "openai") — verify and keep: the
  ChatGPT forward resolution must NOT run for xai/google backends.

## Management API / config validation

- config-routes.ts backend validation: accept the two new literals for
  webSearch.backend ONLY (vision.backend stays two-valued — vision sidecar is
  out of scope here).
- Claude-messages override type (`webSearchSidecar` in OcxClaudeCodeConfig)
  widens identically.

## Out of scope (recorded)

- Antigravity/CCA native grounding (no public contract — 000_research).
- Kimi $web_search pass-through, Z.AI endpoint, OpenRouter tool (surveyed,
  future backends; the enum + executor seam makes each a one-file addition).
- Vision-side change is a SEPARATE tiny unit: no code change needed for
  "text,image models accept images" — that already works (models not in
  noVisionModels forward images natively via each adapter). What was missing is
  only the metadata-driven population of noVisionModels, out of this unit.

## Tests (tests/web-search-native-backends.test.ts)

1. resolveSidecarBackend passthrough for all four values + default.
2. findXaiSearchProvider / findGoogleSearchProvider: positive, disabled,
   sentinel-key, wrong-adapter, wrong-host negatives.
3. planWebSearch: explicit xai/google with provider → plan with right slot,
   model default, describeImages inheritance; without provider → undefined
   (fail closed); openai default unaffected.
4. runXaiWebSearch parse: annotations → sources, no-annotation Sources: strip,
   HTTP error → {error}, auth missing → {error}.
5. runGoogleWebSearch parse: groundingChunks → sources, absent grounding →
   text-only, HTTP error → {error}; body contains ONLY google_search tool.
6. Loop dispatch: backend "xai"/"google" routes to the new executors (spy).
