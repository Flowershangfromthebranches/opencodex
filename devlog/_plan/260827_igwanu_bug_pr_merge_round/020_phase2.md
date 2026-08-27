# 020 — wp3: Ingwannu remainder — #2761, #2764, #2767

All three are Ingwannu's, all merge-tree CLEAN, all tsc OK on the merged tree.
Order within the phase: #2761 first (independent), then #2764 and #2767 after the
keystone turns their matrices green.

## #2761 — fix(integrations): ignore JSON object key order in ownership

Lane **L1**. Head `63941b583`, ready, MERGEABLE, 2 behind, **24 checks, zero
failures** — it predates the version-line breakage. Touches:

- `src/integrations/ownership-policy.ts` (+29/-...)
- `src/integrations/ownership.ts`, `src/integrations/state.ts`,
  `src/integrations/writer.ts`
- `structure/09_client-integrations.md` (architecture note)
- `tests/integrations-state.test.ts` (+41), `tests/integrations-writer.test.ts` (+64)

No file overlaps any other in-scope PR. No auth, credential, or workflow surface.
Carries its own regressions. **Merge as-is** once the merged-tree focused suite is
green.

## #2764 — fix(moonshot): intersect nested schema bounds

Lane **L1, gated on wp2**. Head `30247541f`, draft, 0 behind, merge CLEAN, tsc OK.
Currently red on `ci`/`gates`/`macos`/`test 3/4` — **all four from the shared
version-line failure**, `1 fail` in the whole matrix, and that one fail is
`release version line`. Its own suite passes.

Touches `src/adapters/openai-chat.ts`, `structure/04_transports-and-sidecars.md`,
`tests/moonshot-tool-schema.test.ts`. `openai-chat.ts` is touched by no other
in-scope PR this round.

Draft checklist has one unticked box: "Exact-head required CI is green." That box
cannot be ticked by the author — it is false for a reason outside the PR. After
wp2 lands, rebase onto the new `dev`, re-run CI, and the box becomes truthfully
tickable. Then mark ready and merge.

## #2767 — fix(openai): strip unsupported forward cache options

Lane **L1, gated on wp2**. Head `33586cdf7`, draft, 0 behind, merge CLEAN, tsc OK.
Identical CI situation to #2764: `1 fail`, and it is `release version line`.

Touches `src/adapters/openai-responses.ts`, `src/compatibility/openai-responses.ts`,
`structure/08_openai-provider-tiers.md`,
`tests/fixtures/compatibility/openai-codex-forward-gpt56-sol-v1.json`,
`tests/openai-responses-passthrough.test.ts`.

Its unticked box reads "Exact-head required CI is green after the independent base
gate repair" — the author already diagnosed the dependency correctly. Same
treatment as #2764.

**Fixture caution (previous round's finding):** a regenerated fixture once
resurrected a deliberately removed model behind a count-only assertion. This PR
adds a compatibility fixture, so the review must confirm the fixture's contents
are asserted by field, not merely by count.

## TESTS

- #2761: `tests/integrations-state.test.ts`, `tests/integrations-writer.test.ts`
- #2764: `tests/moonshot-tool-schema.test.ts`
- #2767: `tests/openai-responses-passthrough.test.ts`

## Verification (C)

```bash
bun test tests/integrations-state.test.ts tests/integrations-writer.test.ts
bun test tests/moonshot-tool-schema.test.ts
bun test tests/openai-responses-passthrough.test.ts
bun x tsc --noEmit
```

Each on its own merged tree, one suite at a time (machine-wide lock). For #2764
and #2767 the decisive extra evidence is exact-head CI **after** the rebase onto
post-#2766 `dev`: `ci`, `macos`, `test 3/4`, `gates` must all be green.
