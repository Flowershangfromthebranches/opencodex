import type { OcxProviderConfig } from "../types";
import { resolveEnvValue } from "../config";
import { signalWithTimeout } from "../lib/abort";
import { redactSecretString } from "../lib/redact";
import { sidecarEnter } from "../lib/sidecar-tracker";
import { fetchWithResetRetry } from "../lib/upstream-retry";
import { readBoundedResponseBody } from "../lib/bounded-body";
import { MAX_SIDECAR_RESPONSE_BYTES, type WebSearchSource } from "./parse";
import { BASE_INSTRUCTION, IMAGE_INSTRUCTION, type SidecarOutcome, type SidecarSettings } from "./executor";

/** Answer budget for the sidecar turn (the injected tool_result is clamped downstream anyway). */
const XAI_MAX_OUTPUT_TOKENS = 8192;

function isRec(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === "object" && !Array.isArray(v);
}

/**
 * Fold a non-streamed xAI Responses body into a WebSearchResult. xAI's official server-side
 * web_search (docs.x.ai/developers/tools/web-search) returns the standard Responses envelope:
 * `output[]` message items whose `output_text` blocks carry `url_citation` annotations
 * (docs.x.ai/developers/tools/citations). Sources are collected from those annotations,
 * de-duplicated by URL. Never throws.
 */
export function parseXaiSearchResponse(payload: unknown): SidecarOutcome {
  if (!isRec(payload)) return { text: "", sources: [], error: "xai search returned a non-object body" };
  const output = Array.isArray(payload.output) ? payload.output : [];
  let text = "";
  const sources: WebSearchSource[] = [];
  const seen = new Set<string>();
  for (const item of output) {
    if (!isRec(item) || item.type !== "message" || !Array.isArray(item.content)) continue;
    for (const block of item.content) {
      if (!isRec(block) || block.type !== "output_text" || typeof block.text !== "string") continue;
      text += block.text;
      if (!Array.isArray(block.annotations)) continue;
      for (const ann of block.annotations) {
        if (!isRec(ann) || ann.type !== "url_citation" || typeof ann.url !== "string" || ann.url.length === 0) continue;
        if (seen.has(ann.url)) continue;
        seen.add(ann.url);
        sources.push(typeof ann.title === "string" && ann.title.length > 0 ? { url: ann.url, title: ann.title } : { url: ann.url });
      }
    }
  }
  const trimmed = text.trim();
  if (trimmed.length === 0) {
    const err = isRec(payload.error) && typeof payload.error.message === "string"
      ? payload.error.message
      : "xai search produced no answer";
    return { text: "", sources, error: redactSecretString(err) };
  }
  return { text: trimmed, sources };
}

/**
 * Execute ONE web search on the routed xAI provider's OWN credential — the xAI-native analog of
 * runWebSearch. Calls the official Responses API with `tools:[{type:"web_search"}]`
 * (docs.x.ai, updated 2026-05-27; the older `search_parameters` shape is dead). Non-streaming:
 * the loop consumes the folded result whole, so SSE buys nothing here. Never throws — returns
 * `{error}` so the caller injects a graceful tool result.
 */
export async function runXaiWebSearch(
  query: string,
  providerName: string,
  provider: OcxProviderConfig,
  settings: SidecarSettings,
  abortSignal?: AbortSignal,
): Promise<SidecarOutcome> {
  void providerName;
  const apiKey = resolveEnvValue(provider.apiKey)?.trim();
  if (!apiKey || apiKey.startsWith("<") || apiKey === "N/A") {
    return { text: "", sources: [], error: "xai search backend requires an xAI API key on the provider" };
  }
  const base = provider.baseUrl.replace(/\/+$/, "");
  const url = `${base}/responses`;
  const body = {
    model: settings.model,
    instructions: settings.describeImages ? BASE_INSTRUCTION + IMAGE_INSTRUCTION : BASE_INSTRUCTION,
    input: [{ role: "user", content: [{ type: "input_text", text: query }] }],
    tools: [{ type: "web_search" }],
    max_output_tokens: XAI_MAX_OUTPUT_TOKENS,
    stream: false,
  };
  const linkedSignal = signalWithTimeout(settings.timeoutMs, abortSignal);
  const sidecarExit = sidecarEnter("web-search");
  const t0 = Date.now();
  try {
    const res = await fetchWithResetRetry(
      () => fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": `Bearer ${apiKey}` },
        body: JSON.stringify(body),
        signal: linkedSignal.signal,
        // Credential-bearing: do not follow a cross-origin 3xx with the Authorization header.
        redirect: "manual",
      }),
      { abortSignal: linkedSignal.signal, label: "xai-web-search" },
    );
    if (!res.ok) {
      const t = await res.text().catch(() => "");
      console.warn(`[web-search] xai HTTP ${res.status} for query "${query.slice(0, 80)}" (${Date.now() - t0}ms)`);
      return { text: "", sources: [], error: `xai search HTTP ${res.status}: ${redactSecretString(t.slice(0, 200))}` };
    }
    // Materializing a whole success payload: allow more than the SSE parser's 64 KiB text bound
    // because the raw envelope also carries citation/annotation metadata around the answer text.
    const bounded = await readBoundedResponseBody(res, { maxBytes: 4 * MAX_SIDECAR_RESPONSE_BYTES, signal: linkedSignal.signal });
    if (bounded.oversized || bounded.truncated) {
      return { text: "", sources: [], error: "xai search response exceeded the sidecar size budget" };
    }
    let payload: unknown;
    try {
      payload = JSON.parse(bounded.text);
    } catch {
      return { text: "", sources: [], error: "xai search returned malformed JSON" };
    }
    return parseXaiSearchResponse(payload);
  } catch (e) {
    const kind = e instanceof Error && e.name === "TimeoutError" ? "timeout" : "connect_error";
    console.warn(`[web-search] xai ${kind} for query "${query.slice(0, 80)}" (${Date.now() - t0}ms)`);
    return { text: "", sources: [], error: e instanceof Error ? e.message : String(e) };
  } finally {
    sidecarExit();
    linkedSignal.cleanup();
  }
}
