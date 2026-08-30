# 050 — WP6: PR #2350 annotate present-but-empty tool outputs

Owner score 52. Author `harryzhou2000`. Adds the provider option
`annotateEmptyToolOutputs`, defaulted on for DeepSeek via the registry.

## State

`MERGEABLE` but `CHANGES_REQUESTED`; ~18 commits behind `dev`. 13 files,
+624/−9, including a new 324-line test suite.

## The one real open blocker

An unrelated provider POST silently resurrects the registry default over an
operator's explicit `false`. This is a genuine data-loss bug, not a style note,
and it is the reason this PR is a fix-then-merge rather than a merge.

The mechanism, traced on `dev`:

1. `src/server/management/provider-routes.ts:550` builds a **fresh** provider
   object from the request body alone:

   ```ts
   const prov = body.provider
     ? stripCodexRuntimeProviderFields(body.provider as OcxProviderConfig)
     : undefined;
   ```

2. Line 588 calls `enrichProviderFromCatalog(name, prov)`, which reaches
   `enrichProviderFromRegistry()` (`src/oauth/key-providers.ts:31-44`). The PR's
   fill in `src/providers/derive.ts` (after `:501-503`) is `=== undefined`-gated,
   which is correct in isolation — but the freshly built `prov` **is**
   `undefined` here, so DeepSeek gets registry `true`.

3. Line 630 writes it through:

   ```ts
   config.providers[name] = stripRegistryOnlyStaticHeaders(name, prov);
   ```

   The stored explicit `false` at `config.providers[name]` is never consulted.

Note `src/server/management/config-routes.ts:252-254` returns 405 for a full
config PUT and redirects callers to this POST, so this is *the* write path — there
is no second route that would preserve the field.

The PR's PATCH validation does not help: PATCH is a different handler.

## Fix

Ownership-based preservation, not truthiness. Before line 588:

```ts
const submittedAnnotateEmptyToolOutputs =
  Object.hasOwn(prov, "annotateEmptyToolOutputs");
```

Immediately after `const existing = config.providers[name];` (line 609):

```ts
if (
  !submittedAnnotateEmptyToolOutputs
  && existing?.annotateEmptyToolOutputs !== undefined
) {
  prov.annotateEmptyToolOutputs = existing.annotateEmptyToolOutputs;
}
```

`Object.hasOwn` is required rather than `!== undefined`: omitted must preserve,
submitted `false` must stay `false`, submitted `true` must win, and a brand-new
DeepSeek row must still receive the registry default.

## Regression test

`tests/management-provider-validation.test.ts`, beside the existing
POST-overwrite tests (`:621-696`), following that file's conventions
(`TEST_DIR`, `OPENCODEX_HOME`, `saveConfig`, `startServer(0)`, stop in
`finally`):

1. POST `deepseek` with `annotateEmptyToolOutputs: false`.
2. POST the same provider again changing an unrelated field, omitting the key.
3. Both responses 200.
4. `loadConfig().providers.deepseek?.annotateEmptyToolOutputs` is still `false`.
5. `routedProviderConfig("deepseek", saved)` still resolves `false` — this is
   the assertion that proves the runtime, not just the file, respects the operator.

## Core/Lab boundary

The PR touches `src/router.ts` — one of the three files that must never reach
`src/lab/`. Its hunk (after `:353-356`) only reads the already-resolved
`registryEntry` and adds **no import**, so the boundary holds.
`tests/core-lab-boundary.test.ts` is the oracle.

## Overlap with WP10

Both this and WP10 add a provider config key. Whichever lands second re-reads the
other's schema shape in `src/config.ts` before rebasing.

## Done

Landed with the regression proving an explicit `false` survives an unrelated
provider POST (criterion c-6).
