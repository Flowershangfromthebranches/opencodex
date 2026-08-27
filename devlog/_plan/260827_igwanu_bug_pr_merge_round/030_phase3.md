# 030 — wp4: clean approved lane — #2733, #2726, #2747

## #2733 — fix(cli): neutralize usage report terminal controls

Lane **L1**. Head `2a0ab4be6`, ready, **MERGEABLE/CLEAN**, **APPROVED**,
24 checks zero failures, 43 behind. `luvs01`. Labels: `bug`, `review-ready`.

Touches `src/cli/usage-report.ts` (+15/-2) and `tests/cli-usage-report.test.ts`
(+25). This is terminal-escape-sequence neutralization in a report renderer — a
control-character injection fix. No overlap with any in-scope PR.

Cleanest merge in the round: approved, clean, green, with its own regression.
**Merge as-is.**

## #2726 — fix(xai): normalize web search on the Grok CLI proxy

Lane **L1**. Head `790a581cf`, ready, **MERGEABLE/CLEAN**, **APPROVED**,
24 checks zero failures, 63 behind. `olddonkey`. Labels: `bug`, `review-ready`.

Touches `src/adapters/xai-web-search.ts` (+24/-...),
`tests/responses-routed-web-search-fields.test.ts`,
`tests/xai-web-search-compat.test.ts` (+44). No overlap. **Merge as-is.**

Note: `xai` is this operator's default provider (`defaultProvider: xai`), so this
one is worth a live smoke after landing rather than test-only evidence.

## #2747 — fix(tests): reap the recovery proxy instead of trusting `stop`

Lane **L1, gated on wp2**. Head `07b97587`, ready, MERGEABLE, 26 behind,
labels `bug`, `review-ready`. Failing `ci` and `macos`.

The `macos` job (run `33059606933`, job `98534630924`) fails on
`release version line` — the shared baseline, again. The `ci` job fails with
`needed job(s) did not pass`, i.e. it is a fan-in that inherits the same failure.
Unlike #2764 and #2767, #2747's `gates` job is green: its head predates the
release-runbook document that trips `privacy:scan`. Version line only.

Touches exactly one file: `tests/update-stop-first.test.ts` (+46/-18). Test-only,
no `src/` change, no overlap. This is the causal repair of a flaky test — it reaps
the recovery proxy process instead of trusting `stop` to have ended it, which is
exactly the "find the causal issue, don't rerun until green" discipline.

After wp2, re-run CI; expect green with no diff change.

## TESTS

- #2733: `tests/cli-usage-report.test.ts`
- #2726: `tests/xai-web-search-compat.test.ts`,
  `tests/responses-routed-web-search-fields.test.ts`
- #2747: `tests/update-stop-first.test.ts` (the PR *is* the test)

## Verification (C)

```bash
bun test tests/cli-usage-report.test.ts
bun test tests/xai-web-search-compat.test.ts tests/responses-routed-web-search-fields.test.ts
bun test tests/update-stop-first.test.ts
bun x tsc --noEmit
```

#2747 additionally needs the run repeated to show the reap actually removes the
orphan: a single green pass on a formerly-flaky test is weak evidence. Run it
3x and confirm no leaked proxy process survives.
