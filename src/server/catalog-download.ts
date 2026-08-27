import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import type { OcxConfig } from "../types";
import { corsHeaders, jsonResponse, type RequestPolicyView } from "./auth-cors";

export const MAX_REMOTE_CATALOG_BYTES = 32 * 1024 * 1024;

export interface SerializedCatalog {
  bytes: Uint8Array<ArrayBuffer>;
  codexVersion?: string;
}

export async function serializePersistedCatalog(): Promise<SerializedCatalog | null> {
  const [{ parseCatalogJson, readCodexCatalogPath }, { loadPersistedCodexRuntime }] = await Promise.all([
    import("../codex/catalog/parsing"),
    import("../codex/runtime"),
  ]);
  const path = readCodexCatalogPath();
  if (!existsSync(path)) return null;
  const catalog = parseCatalogJson(readFileSync(path, "utf8"));
  if (!catalog) throw new Error("Persisted Codex catalog is malformed");
  // TextEncoder types its output over ArrayBufferLike, which Bun's Response BodyInit
  // refuses; the encoder always allocates a plain ArrayBuffer, so the assertion is exact.
  const bytes = new TextEncoder().encode(JSON.stringify(catalog)) as Uint8Array<ArrayBuffer>;
  const codexVersion = loadPersistedCodexRuntime()?.selectedVersion ?? undefined;
  return { bytes, ...(codexVersion ? { codexVersion } : {}) };
}

export function catalogEtag(bytes: Uint8Array): string {
  return `"sha256-${createHash("sha256").update(bytes).digest("base64url")}"`;
}

function ifNoneMatchMatches(value: string | null, etag: string): boolean {
  if (!value) return false;
  return value.split(",").some(raw => {
    const candidate = raw.trim();
    return candidate === "*" || candidate === etag || candidate === `W/${etag}`;
  });
}

export function catalogManagementResponse(
  catalog: SerializedCatalog | null,
  req: Request,
  config: OcxConfig,
): Response {
  if (!catalog) return jsonResponse({ error: "catalog not found" }, 404, req, config);
  const headers = new Headers({
    "Content-Type": "application/json",
    ...corsHeaders(req, config),
  });
  if (catalog.codexVersion) headers.set("x-opencodex-codex-version", catalog.codexVersion);
  return new Response(catalog.bytes, { status: 200, headers });
}

export function catalogDataPlaneResponse(
  catalog: SerializedCatalog | null,
  req: Request,
  policy: RequestPolicyView,
): Response {
  if (!catalog) return jsonResponse({ error: "catalog not found" }, 404, req, policy);
  if (catalog.bytes.byteLength > MAX_REMOTE_CATALOG_BYTES) {
    return new Response(JSON.stringify({
      error: {
        type: "server_error",
        code: "catalog_too_large",
        message: "OpenCodex catalog exceeds the remote download limit",
      },
    }), {
      status: 503,
      headers: { "Content-Type": "application/json", ...corsHeaders(req, policy) },
    });
  }
  const etag = catalogEtag(catalog.bytes);
  const headers = new Headers({
    ETag: etag,
    "Cache-Control": "private, no-cache",
    ...corsHeaders(req, policy),
  });
  if (ifNoneMatchMatches(req.headers.get("If-None-Match"), etag)) {
    return new Response(null, { status: 304, headers });
  }
  headers.set("Content-Type", "application/json");
  return new Response(catalog.bytes, { status: 200, headers });
}
