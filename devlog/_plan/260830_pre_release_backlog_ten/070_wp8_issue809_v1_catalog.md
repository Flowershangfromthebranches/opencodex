# 070 — WP8: issue #809 least-privilege `GET /v1/catalog`

Owner score **66** — the highest-scoring item with no implementation anywhere.
`maintainer-sponsored`.

## Problem

A remote Codex client needs the model catalog. Today the only source is
`GET /api/catalog`, which sits behind management auth — so handing a remote client
the catalog means handing it an **admin token**. That is the least-privilege
violation the issue is about.

## Accepted scope, and the exclusion that defines the design

Add a credentialed read-only `GET` (and `HEAD`) `/v1/catalog` authenticated by
the **data plane**, reusing the existing catalog authority and serialization.

The load-bearing exclusion: **no data-token exception inside `/api/catalog` or
anywhere in `/api/*`.** The fix is a new data-plane route, never a widened
management boundary. Also excluded: any mutation under `/v1`, exposing
`models_cache.json`, fabricating a Codex version, a second catalog serializer, and
expanding into the rest of #95.

## Current contract

`/api/catalog` is `src/server/management/model-routes.ts:335-346` inside
`handleModelRoutes` (`:175`), registered at
`src/server/management/route-registry.ts:210-213`. It calls
`readCatalog(readCodexCatalogPath())`, 404s `{ error: "catalog not found" }` on
missing/malformed, else `JSON.stringify(catalog)`, with
`Content-Type: application/json` and optional `x-opencodex-codex-version` from
`loadPersistedCodexRuntime()?.selectedVersion`. It has **no size cap, no ETag, no
Cache-Control** today.

DTO authority is `RawCatalog` (`src/codex/catalog/parsing.ts:146-148`);
`readCatalog` returns `null` on any read/parse failure (`:249-253`). There is no
route-local redactor — safety rests on the generated persisted catalog, which is
why WP8 must not invent a second serializer that could diverge.

## The two admission planes

Management: `src/server/index.ts:1055-1069` intercepts every `/api/*` and runs
`requireManagementAuth` (`src/server/management-auth.ts:445-481`) — admin token or
origin-bound GUI session, CSRF on unsafe mutations.

Data: `resolveResponsesApiAuth` (`src/server/auth-cors.ts:475-488`) accepts the
configured data credential via `x-opencodex-api-key` or a recognized bearer, and
rejects `x-api-key`. `AUTH_MATRIX` is `:399-408`. `/v1/models` at
`src/server/index.ts:1072-1080` is the pattern to copy. Unknown `/v1` paths die at
`:1649-1655`.

## Diff shape

1. New `src/server/catalog-download.ts` — `SerializedCatalog`,
   `serializePersistedCatalog`, `catalogManagementResponse`,
   `catalogDataPlaneResponse`, `catalogEtag`, `MAX_REMOTE_CATALOG_BYTES`.
2. Repoint `model-routes.ts:335-346` at the shared serializer. **Both routes must
   emit the same bytes** — that is what keeps them from drifting.
3. Register `/v1/catalog` after the `/api/*` block and before `/v1/models`:
   `GET`/`HEAD` only, `resolveResponsesApiAuth`, `isAllowedRequestOrigin`,
   `withCors`.
4. Add the `AUTH_MATRIX` row with `/v1/responses` dispositions.
5. Let unsupported methods and `/v1/catalog/` fall through to the existing 404.
6. `HEAD` returns identical status/headers, no body.
7. Leave `/api/*`, `requireManagementAuth`, and the management route registry
   untouched.
8. Do **not** broaden the `unauthenticatedLoopbackListener` allowlist.

## Reuse from PR #2772, and why not the commits

#2772 already implements this at `src/server/catalog-download.ts:6-84` (32 MiB
cap, shared serialization, SHA-256 ETag, conditional 304,
`Cache-Control: private, no-cache`) with tests at
`tests/server-auth.test.ts:3753-3923`. Reuse that **design**.

Do not take the commits: it is 217 behind, targets `codex/remote-hub-design`
rather than `dev`, bundles unrelated runtime-role work, and has six concrete
defects — GET without HEAD, omits `x-opencodex-codex-version` on the data plane,
adds an unrequested `x-opencodex-key-id`, broadens the unauthenticated loopback
allowlist, and adds no `/api/*` denial coverage.

## Tests

- `tests/api-catalog-route.test.ts:39-76` — shared serializer, byte parity with
  the management route, version header, malformed/missing catalog, secret
  sentinels.
- `tests/server-auth.test.ts:946-988` — missing/invalid/admin-as-data credential,
  origin, GET/HEAD, unsupported method, size cap, ETag/304.
- `tests/server-management-auth.test.ts:561-587` — plane separation: the data
  credential must still 401 against `/api/config`, `/api/providers`, OAuth/Codex
  auth mutations, and the stop gate.
- `tests/api-key-attribution.test.ts:455-501` — update the live `AUTH_MATRIX`
  test so `/v1/catalog` cannot pass **vacuously as an unknown 404**. Without this
  the whole suite could go green on a route that was never registered.

## Docs

`structure/05_gui-and-management-api.md:58-83` (credential boundary);
`docs-site/src/content/docs/reference/configuration/server.md:68-83` (row);
`docs-site/src/content/docs/guides/codex-integration.md:268-282` — replace the
admin-token `/api/catalog` workflow with the data-token `/v1/catalog` one.

## Security note

This adds a network-reachable authenticated endpoint. It is credentialed by
design; the review must confirm the catalog bytes carry no API keys, OAuth
material, session tokens, account identifiers, raw provider config, or filesystem
paths, and that no `/api/*` route became reachable with a data credential.

## Done

`GET`/`HEAD /v1/catalog` serves the least-privilege catalog with negative-auth
coverage and no weakening of `/api/*` (criterion c-8).
