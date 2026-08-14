# Native web-search backends: research (260815)

Goal: the web-search sidecar currently supports two backends — "openai" (ChatGPT
forward gpt-mini replaying the hosted web_search) and "anthropic"
(web_search_20250305 on a stored-OAuth Claude). Both borrow a DIFFERENT
credential than the routed provider. When the routed provider itself has an
official server-side search (xAI, Google Gemini), the proxy should be able to
run the search on that provider's own credential instead.

All findings below were verified against primary official docs on 2026-08-15
(Luna lane sweep, sources opened).

## xAI Grok — official Live/Web Search

- Endpoint: `POST https://api.x.ai/v1/responses` (Responses API is the
  documented REST path for server-side search; chat/completions examples are
  SDK/gRPC only).
- Request: `tools: [{ "type": "web_search" }]`, optional
  `filters.allowed_domains` / `filters.excluded_domains`,
  `enable_image_understanding`, `enable_image_search`.
  No `search_parameters` / `sources` / `web_search_options` in the current
  schema (older shapes are dead).
- Citations: `output_text` blocks carry
  `annotations: [{ type: "url_citation", url, title, start_index, end_index }]`
  — the SAME shape src/web-search/parse.ts already collects
  (`collectAnnotation`). Inline markdown citations are on by default;
  `include: ["no_inline_citations"]` disables.
- Models: current docs demonstrate with `grok-4.6`; flagship family supports it.
- Pricing: $5 / 1,000 successful tool calls, billed per invocation not per URL.
- Source: docs.x.ai/developers/tools/web-search (updated 2026-05-27),
  /developers/tools/citations, /developers/pricing, /developers/models.

## Google Gemini API — google_search grounding

- Endpoint: `POST https://generativelanguage.googleapis.com/v1beta/models/{m}:generateContent`
  (and `:streamGenerateContent` — the adapter's existing endpoints).
- Request: `tools: [{ "google_search": {} }]`.
- Response: `candidates[].groundingMetadata` with `webSearchQueries[]`,
  `searchEntryPoint.renderedContent`, `groundingChunks[].web.{uri,title}`,
  `groundingSupports[].{segment,groundingChunkIndices}`.
- Combination caveat (load-bearing): the `Tool` schema allows both
  `functionDeclarations` and `googleSearch`, but Google documents multi-tool
  only for search+code-execution+url-context; search + function declarations in
  one request is NOT a documented guarantee and community reports show 4xx on
  some models. Design must not send both in one request.
- Pricing: Gemini 3.x $14/1k requests after 5k free/mo (2.5: $35/1k grounded
  prompts). Free tier: not available for paid grounding.
- Source: ai.google.dev/gemini-api/docs/generate-content/google-search,
  /api/generate-content (Tool schema), /docs/pricing, /docs/changelog.

## Google Antigravity (Cloud Code Assist)

- The CCA `v1internal:generateContent` envelope is a private protocol; no
  public doc grants `google_search` grounding through it, and CLIProxyAPI's
  public source shows no antigravity grounding implementation either (its
  `web_search` payload matching is generic config routing).
- The public "Antigravity Agent" docs expose google_search on the PUBLIC Gemini
  Interactions API surface, not on cloudcode-pa.
- Decision: Antigravity is OUT of scope for a native search executor in this
  unit. An antigravity route keeps using the existing sidecar backends. If CCA
  grounding evidence appears later (chase: CLIProxyAPI, agy CLI traffic), a
  google-family executor already gives it a seam.

## Others (surveyed, deferred)

- Moonshot Kimi: `builtin_function` `$web_search` — pass-through arguments
  echo pattern; incompatible with our intercept loop's synthesize-and-inject
  contract without a bespoke sub-loop. Deferred.
- Z.AI GLM: dedicated `POST /api/paas/v4/web_search` endpoint returning
  structured `search_result[]` — a clean future backend, deferred.
- OpenRouter: `openrouter:web_search` tool / `:online` suffix; citations as
  OpenAI `url_citation` annotations. Deferred (openrouter routes are
  openai-chat adapter; could reuse the xai executor shape later).
- DeepSeek / MiniMax / DashScope: no verified official public HTTP search
  contract today.

## Existing seams (verified in-tree)

- `planWebSearch` (src/web-search/index.ts) picks backend "openai" |
  "anthropic" and builds the SidecarPlan consumed by `runWithWebSearch`.
- The loop calls exactly one executor per query
  (loop.ts ~653): `runAnthropicWebSearch` or `runWebSearch`.
- `parse.ts` already normalizes `url_citation` annotations and trailing
  `Sources:` sections into `WebSearchResult { text, sources }`.
- Anthropic executor (anthropic-executor.ts) is the template for a
  provider-credentialed executor: own auth, own wire, folds SSE into
  `SidecarOutcome`, never throws.
