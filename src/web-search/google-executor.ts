import type { OcxProviderConfig } from "../types";
import { resolveEnvValue } from "../config";
import { signalWithTimeout } from "../lib/abort";
import { redactSecretString } from "../lib/redact";
import { sidecarEnter } from "../lib/sidecar-tracker";
import { fetchWithResetRetry } from "../lib/upstream-retry";
import { readBoundedResponseBody } from "../lib/bounded-body";
import { MAX_SIDECAR_RESPONSE_BYTES, type WebSearchSource } from "./parse";
import { BASE_INSTRUCTION, IMAGE_INSTRUCTION, type SidecarOutcome, type SidecarSettings } from "./executor";

function isRec(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === "object" && !Array.isArray(v);
}

/**
 * Fold a non-streamed Gemini generateContent body into a WebSearchResult. Grounded answers carry
 * `candidates[0].groundingMetadata.groundingChunks[].web.{uri,title}`
 * (ai.google.dev/gemini-api/docs/generate-content/google-search); the answer text is the concat of
 * `candidates[0].content.parts[].text`. A response without groundingMetadata still yields its text —
 * the model may answer from knowledge, mirroring the openai sidecar when it declines to search.
 * Never throws.
 */
export function parseGoogleSearchResponse(payload: unknown): SidecarOutcome {
  if (!isRec(payload)) return { text: "", sources: [], error: "google search returned a non-object body" };
  const candidates = Array.isArray(payload.candidates) ? payload.candidates : [];
  const first = candidates.find(isRec);
  if (!first) {
    const err = isRec(payload.error) && typeof payload.error.message === "string"
      ? payload.error.message
      : "google search produced no candidates";
    return { text: "", sources: [], error: redactSecretString(err) };
  }
  let text = "";
  const content = isRec(first.content) ? first.content : {};
  if (Array.isArray(content.parts)) {
    for (const part of content.parts) {
      if (isRec(part) && typeof part.text === "string") text += part.text;
    }
  }
  const sources: WebSearchSource[] = [];
  const seen = new Set<string>();
  const grounding = isRec(first.groundingMetadata) ? first.groundingMetadata : {};
  if (Array.isArray(grounding.groundingChunks)) {
    for (const chunk of grounding.groundingChunks) {
      if (!isRec(chunk) || !isRec(chunk.web)) continue;
      const uri = chunk.web.uri;
      if (typeof uri !== "string" || uri.length === 0 || seen.has(uri)) continue;
      seen.add(uri);
      const title = chunk.web.title;
      sources.push(typeof title === "string" && title.length > 0 ? { url: uri, title } : { url: uri });
    }
  }
  const trimmed = text.trim();
  if (trimmed.length === 0) {
    return { text: "", sources, error: "google search produced no answer" };
  }
  return { text: trimmed, sources };
}

/**
 * Execute ONE web search on the routed Google provider's OWN credential — google_search grounding
 * via generateContent (the Gemini API key path only; vertex/CCA modes are excluded on purpose:
 * different endpoint shapes, and CCA grounding has no public contract).
 *
 * The request's `tools` array contains ONLY `{ google_search: {} }`. Google documents multi-tool
 * combinations for search+code-execution+url-context, but search + functionDeclarations in one
 * request is NOT a documented guarantee — and the sidecar never needs the routed model's tools
 * anyway. Never throws — returns `{error}` so the caller injects a graceful tool result.
 */
export async function runGoogleWebSearch(
  query: string,
  providerName: string,
  provider: OcxProviderConfig,
  settings: SidecarSettings,
  abortSignal?: AbortSignal,
): Promise<SidecarOutcome> {
  void providerName;
  const apiKey = resolveEnvValue(provider.apiKey)?.trim();
  if (!apiKey || apiKey.startsWith("<") || apiKey === "N/A") {
    return { text: "", sources: [], error: "google search backend requires a Gemini API key on the provider" };
  }
  const base = provider.baseUrl.replace(/\/+$/, "");
  const url = `${base}/v1beta/models/${settings.model}:generateContent`;
  const instruction = settings.describeImages ? BASE_INSTRUCTION + IMAGE_INSTRUCTION : BASE_INSTRUCTION;
  const body = {
    contents: [{ role: "user", parts: [{ text: query }] }],
    systemInstruction: { parts: [{ text: instruction }] },
    tools: [{ google_search: {} }],
  };
  const linkedSignal = signalWithTimeout(settings.timeoutMs, abortSignal);
  const sidecarExit = sidecarEnter("web-search");
  const t0 = Date.now();
  try {
    const res = await fetchWithResetRetry(
      () => fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json", "x-goog-api-key": apiKey },
        body: JSON.stringify(body),
        signal: linkedSignal.signal,
        // Credential-bearing header: do not follow a cross-origin 3xx.
        redirect: "manual",
      }),
      { abortSignal: linkedSignal.signal, label: "google-web-search" },
    );
    if (!res.ok) {
      const t = await res.text().catch(() => "");
      console.warn(`[web-search] google HTTP ${res.status} for query "${query.slice(0, 80)}" (${Date.now() - t0}ms)`);
      return { text: "", sources: [], error: `google search HTTP ${res.status}: ${redactSecretString(t.slice(0, 200))}` };
    }
    // Whole-payload materialization: grounding metadata rides alongside the text, so allow
    // more headroom than the SSE text bound.
    const bounded = await readBoundedResponseBody(res, { maxBytes: 4 * MAX_SIDECAR_RESPONSE_BYTES, signal: linkedSignal.signal });
    if (bounded.oversized || bounded.truncated) {
      return { text: "", sources: [], error: "google search response exceeded the sidecar size budget" };
    }
    let payload: unknown;
    try {
      payload = JSON.parse(bounded.text);
    } catch {
      return { text: "", sources: [], error: "google search returned malformed JSON" };
    }
    return parseGoogleSearchResponse(payload);
  } catch (e) {
    const kind = e instanceof Error && e.name === "TimeoutError" ? "timeout" : "connect_error";
    console.warn(`[web-search] google ${kind} for query "${query.slice(0, 80)}" (${Date.now() - t0}ms)`);
    return { text: "", sources: [], error: e instanceof Error ? e.message : String(e) };
  } finally {
    sidecarExit();
    linkedSignal.cleanup();
  }
}
