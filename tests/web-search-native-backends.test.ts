import { afterEach, describe, expect, test } from "bun:test";

import { parseRequest } from "../src/responses/parser";
import {
  findGoogleSearchProvider,
  findXaiSearchProvider,
  planWebSearch,
  resolveSidecarBackend,
} from "../src/web-search";
import { parseXaiSearchResponse, runXaiWebSearch } from "../src/web-search/xai-executor";
import { parseGoogleSearchResponse, runGoogleWebSearch } from "../src/web-search/google-executor";
import { BASE_INSTRUCTION, IMAGE_INSTRUCTION } from "../src/web-search/executor";
import type { OcxConfig, OcxProviderConfig } from "../src/types";

const routedProvider: OcxProviderConfig = { adapter: "openai-chat", baseUrl: "https://routed.test/v1", apiKey: "routed-key" };
const xaiProvider: OcxProviderConfig = { adapter: "openai-chat", baseUrl: "https://api.x.ai/v1", apiKey: "xai-key" };
const googleProvider: OcxProviderConfig = { adapter: "google", baseUrl: "https://generativelanguage.googleapis.com", apiKey: "goog-key" };

function config(providers: Record<string, OcxProviderConfig>, overrides: Partial<OcxConfig> = {}): OcxConfig {
  return { port: 10100, defaultProvider: "routed", providers: { routed: routedProvider, ...providers }, ...overrides };
}

function parsedWithWebSearch() {
  return parseRequest({ model: "routed/model", input: "Search current docs", stream: true, tools: [{ type: "web_search" }] });
}

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("resolveSidecarBackend native values", () => {
  test("passes xai and google through; unset still defaults to openai", () => {
    expect(resolveSidecarBackend("xai")).toBe("xai");
    expect(resolveSidecarBackend("google")).toBe("google");
    expect(resolveSidecarBackend("anthropic")).toBe("anthropic");
    expect(resolveSidecarBackend(undefined)).toBe("openai");
  });
});

describe("findXaiSearchProvider", () => {
  test("finds an enabled api.x.ai provider with a real key, by host not by name", () => {
    const found = findXaiSearchProvider(config({ mygrok: xaiProvider }));
    expect(found?.providerName).toBe("mygrok");
  });

  test("rejects disabled, sentinel-key, and non-xai-host providers", () => {
    expect(findXaiSearchProvider(config({ x: { ...xaiProvider, disabled: true } }))).toBeUndefined();
    expect(findXaiSearchProvider(config({ x: { ...xaiProvider, apiKey: "<YOUR_KEY>" } }))).toBeUndefined();
    expect(findXaiSearchProvider(config({ x: { ...xaiProvider, apiKey: undefined } }))).toBeUndefined();
    expect(findXaiSearchProvider(config({ x: { ...xaiProvider, baseUrl: "https://api.other.ai/v1" } }))).toBeUndefined();
  });
});

describe("findGoogleSearchProvider", () => {
  test("finds an enabled google-adapter key provider in plain Gemini mode", () => {
    const found = findGoogleSearchProvider(config({ gem: googleProvider }));
    expect(found?.providerName).toBe("gem");
  });

  test("rejects vertex/CCA modes, wrong adapter, and sentinel keys", () => {
    expect(findGoogleSearchProvider(config({ g: { ...googleProvider, googleMode: "vertex" } }))).toBeUndefined();
    expect(findGoogleSearchProvider(config({ g: { ...googleProvider, googleMode: "cloud-code-assist" } }))).toBeUndefined();
    expect(findGoogleSearchProvider(config({ g: { ...googleProvider, adapter: "openai-chat" } }))).toBeUndefined();
    expect(findGoogleSearchProvider(config({ g: { ...googleProvider, apiKey: "N/A" } }))).toBeUndefined();
  });
});

describe("planWebSearch native backends", () => {
  test("explicit xai backend with a usable provider yields an xai plan with the default model", () => {
    const cfg = config({ mygrok: xaiProvider }, { webSearchSidecar: { backend: "xai" } });
    const plan = planWebSearch(cfg, parsedWithWebSearch(), false, routedProvider, "model");
    expect(plan?.backend).toBe("xai");
    expect(plan?.xaiSidecar?.providerName).toBe("mygrok");
    expect(plan?.settings.model).toBe("grok-4-1-fast");
    expect(plan?.forwardSidecar).toBeUndefined();
  });

  test("explicit google backend with a usable provider yields a google plan with the default model", () => {
    const cfg = config({ gem: googleProvider }, { webSearchSidecar: { backend: "google" } });
    const plan = planWebSearch(cfg, parsedWithWebSearch(), false, routedProvider, "model");
    expect(plan?.backend).toBe("google");
    expect(plan?.googleSidecar?.providerName).toBe("gem");
    expect(plan?.settings.model).toBe("gemini-3.5-flash");
  });

  test("cfg.model overrides the per-backend default", () => {
    const cfg = config({ mygrok: xaiProvider }, { webSearchSidecar: { backend: "xai", model: "grok-4.6" } });
    const plan = planWebSearch(cfg, parsedWithWebSearch(), false, routedProvider, "model");
    expect(plan?.settings.model).toBe("grok-4.6");
  });

  test("explicit native backend with NO usable provider fails closed (no plan)", () => {
    expect(planWebSearch(config({}, { webSearchSidecar: { backend: "xai" } }), parsedWithWebSearch(), false, routedProvider, "model")).toBeUndefined();
    expect(planWebSearch(config({}, { webSearchSidecar: { backend: "google" } }), parsedWithWebSearch(), false, routedProvider, "model")).toBeUndefined();
  });

  test("describeImages rides settings when the routed model is in noVisionModels", () => {
    const routedBlind: OcxProviderConfig = { ...routedProvider, noVisionModels: ["model"] };
    const cfg = config({ mygrok: xaiProvider }, { webSearchSidecar: { backend: "xai" } });
    const plan = planWebSearch(cfg, parsedWithWebSearch(), false, routedBlind, "model");
    expect(plan?.settings.describeImages).toBe(true);
  });
});

describe("parseXaiSearchResponse", () => {
  test("collects output_text and url_citation annotations, de-duplicated", () => {
    const out = parseXaiSearchResponse({
      output: [{
        type: "message",
        content: [{
          type: "output_text",
          text: "Answer.[[1]](https://a.test/x)",
          annotations: [
            { type: "url_citation", url: "https://a.test/x", title: "A" },
            { type: "url_citation", url: "https://a.test/x", title: "dup" },
            { type: "url_citation", url: "https://b.test/y" },
          ],
        }],
      }],
    });
    expect(out.text).toContain("Answer.");
    expect(out.sources).toEqual([{ url: "https://a.test/x", title: "A" }, { url: "https://b.test/y" }]);
    expect(out.error).toBeUndefined();
  });

  test("empty output surfaces an error, not a silent blank answer", () => {
    const out = parseXaiSearchResponse({ output: [] });
    expect(out.error).toBeDefined();
    const withMsg = parseXaiSearchResponse({ output: [], error: { message: "quota exceeded" } });
    expect(withMsg.error).toContain("quota");
  });
});

describe("runXaiWebSearch request shape", () => {
  test("posts tools:[{type:web_search}] to {base}/responses with Bearer auth; image instruction rides describeImages", async () => {
    let captured: { url: string; body: Record<string, unknown>; auth: string | null } | undefined;
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      captured = { url: String(input), body: JSON.parse(String(init?.body)), auth: headers.get("authorization") };
      return Response.json({ output: [{ type: "message", content: [{ type: "output_text", text: "ok", annotations: [] }] }] });
    }) as typeof fetch;

    const out = await runXaiWebSearch("test query", "mygrok", xaiProvider,
      { model: "grok-4-1-fast", reasoning: "low", timeoutMs: 5000, describeImages: true });
    expect(out.text).toBe("ok");
    expect(captured?.url).toBe("https://api.x.ai/v1/responses");
    expect(captured?.auth).toBe("Bearer xai-key");
    expect(captured?.body.model).toBe("grok-4-1-fast");
    expect(captured?.body.tools).toEqual([{ type: "web_search" }]);
    expect(captured?.body.stream).toBe(false);
    expect(String(captured?.body.instructions)).toBe(BASE_INSTRUCTION + IMAGE_INSTRUCTION);
  });

  test("missing/sentinel key and HTTP errors degrade to {error} without throwing", async () => {
    const noKey = await runXaiWebSearch("q", "x", { ...xaiProvider, apiKey: "<KEY>" },
      { model: "m", reasoning: "low", timeoutMs: 5000 });
    expect(noKey.error).toContain("API key");

    globalThis.fetch = (async () => new Response("denied", { status: 403 })) as typeof fetch;
    const httpErr = await runXaiWebSearch("q", "x", xaiProvider, { model: "m", reasoning: "low", timeoutMs: 5000 });
    expect(httpErr.error).toContain("HTTP 403");
  });
});

describe("parseGoogleSearchResponse", () => {
  test("collects candidate text and groundingChunks web sources", () => {
    const out = parseGoogleSearchResponse({
      candidates: [{
        content: { parts: [{ text: "Grounded " }, { text: "answer." }] },
        groundingMetadata: {
          webSearchQueries: ["q"],
          groundingChunks: [
            { web: { uri: "https://s.test/1", title: "S1" } },
            { web: { uri: "https://s.test/1", title: "dup" } },
            { web: { uri: "https://s.test/2" } },
          ],
        },
      }],
    });
    expect(out.text).toBe("Grounded answer.");
    expect(out.sources).toEqual([{ url: "https://s.test/1", title: "S1" }, { url: "https://s.test/2" }]);
  });

  test("text without grounding metadata still yields the answer with empty sources", () => {
    const out = parseGoogleSearchResponse({ candidates: [{ content: { parts: [{ text: "From knowledge." }] } }] });
    expect(out.text).toBe("From knowledge.");
    expect(out.sources).toEqual([]);
    expect(out.error).toBeUndefined();
  });

  test("no candidates surfaces the payload error message", () => {
    const out = parseGoogleSearchResponse({ error: { message: "API key not valid" } });
    expect(out.error).toContain("API key not valid");
  });
});

describe("runGoogleWebSearch request shape", () => {
  test("posts ONLY the google_search tool to v1beta generateContent with x-goog-api-key", async () => {
    let captured: { url: string; body: Record<string, unknown>; key: string | null } | undefined;
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      captured = { url: String(input), body: JSON.parse(String(init?.body)), key: headers.get("x-goog-api-key") };
      return Response.json({ candidates: [{ content: { parts: [{ text: "ok" }] } }] });
    }) as typeof fetch;

    const out = await runGoogleWebSearch("test query", "gem", googleProvider,
      { model: "gemini-3.5-flash", reasoning: "low", timeoutMs: 5000 });
    expect(out.text).toBe("ok");
    expect(captured?.url).toBe("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash:generateContent");
    expect(captured?.key).toBe("goog-key");
    // The documented multi-tool caveat: google_search must be the ONLY tool — never
    // functionDeclarations alongside it.
    expect(captured?.body.tools).toEqual([{ google_search: {} }]);
    const contents = captured?.body.contents as Array<{ parts: Array<{ text: string }> }>;
    expect(contents[0].parts[0].text).toBe("test query");
  });

  test("missing key and HTTP errors degrade to {error} without throwing", async () => {
    const noKey = await runGoogleWebSearch("q", "g", { ...googleProvider, apiKey: undefined },
      { model: "m", reasoning: "low", timeoutMs: 5000 });
    expect(noKey.error).toContain("API key");

    globalThis.fetch = (async () => new Response("bad", { status: 400 })) as typeof fetch;
    const httpErr = await runGoogleWebSearch("q", "g", googleProvider, { model: "m", reasoning: "low", timeoutMs: 5000 });
    expect(httpErr.error).toContain("HTTP 400");
  });
});
