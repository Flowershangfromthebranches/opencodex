# 040 — wp5: maintainer changes-requested — #2745, #2729

Both are authored by `lidge-jun` (the maintainer) and both carry a detailed
CHANGES_REQUESTED review from Ingwannu naming specific, reproducible defects.
Neither may merge as-is. Lane **L4 (fix-then-land)** for both.

## #2745 — fix(responses): rebind credential identity on every OAuth 429 rotation

Head `a90ab6ee7`, ready, MERGEABLE, 26 behind, 24 checks zero failures.
Touches `src/server/responses/core.ts` (+55/-11) and
`tests/generic-oauth-failover.test.ts` (+78).

**This is an OAuth credential-boundary change — the exact surface `MAINTAINERS.md`
requires security review for.** It does not land autonomously on my judgement
alone; the two reviewer blockers below are also genuine correctness defects.

Reviewer blockers, both concrete:

1. **Cross-account origin bleed.** At both 401 rebuild sites,
   `refreshed.apiBaseUrl ?? getOAuthCredentialApiBaseUrl(route.providerName)`
   falls back to the *active* credential's origin. Generic 429 rotation does not
   promote account B to active, so a legacy B credential with no `apiBaseUrl`
   yields **B's bearer paired with A's origin**. `applyFailoverSnapshot` has the
   same hole: an undefined snapshot origin leaves the previous routed base URL on
   the provider object. Fix: resolve by `refreshed.accountId`, or fail closed to
   the canonical origin — never consult another account's route.
2. **The three new tests are source-text assertions.** They pass even if the
   assignments are unreachable or the wrong snapshot reaches the request. Needs an
   executable A -> 429 -> B regression through the HTTP recovery path asserting the
   second dispatch's bearer/origin pair, the B account/generation replay identity,
   and cleared Cursor continuation state.

`src/server/responses/core.ts` is also touched by #2638 and #2497 —
`git merge-tree` pairwise before a second one lands.

Disposition: fix both blockers, add the behavioral regression, then **hold for
explicit human security sign-off** before merge. If sign-off is not available in
this round, the honest outcome is NEEDS_HUMAN with the work banked on a branch.

## #2729 — fix(claude): derive response.failed status from the classified error

Head `19801d201`, ready, MERGEABLE, 89 behind, 24 checks zero failures.
Touches `src/adapters/cursor/cursor-errors.ts` (+8), `src/claude/outbound.ts`
(+17/-3), `tests/claude-outbound.test.ts` (+73), `tests/cursor-errors.test.ts` (+10).

Reviewer accepts the main diagnosis (Cursor `failed_precondition -> 400` is
correct, 157/157 across eight suites) but found one **error-fidelity regression**:

`httpStatusFromTerminalError` recognizes only `server_error + server_is_overloaded`
before falling through to message inference. A status-less envelope like
`{type:"server_error", code:"upstream_server_error", message:"...malformed tool
call arguments"}` returns **400** because the message contains "malformed".
Before this PR it became a transient 500. Result: Claude Code receives
`invalid_request_error` and **stops retrying a genuine upstream failure**. The
reviewer probed the exact head and got 400.

Fix: structured generic server classifications must win over message keywords —
map `server_error`/`upstream_server_error` and equivalent generic upstream codes to
transient 5xx, while retaining the specific 429/401/403/invalid-request/policy/
cancellation/explicit-overload mappings. Add a status-less regression using a
server-classified message containing an invalid-request keyword, asserting the
Anthropic tail stays `overloaded_error`.

Deferred, not blockers: the dead `translation_buffer_limit` status arm; loss of
the original error code.

No auth surface. This one **can** land autonomously once the fix and regression
are in and the merged-tree suite is green.

## TESTS

- #2745: `tests/generic-oauth-failover.test.ts` — replace source-text assertions
  with an executable failover regression; add the negative A/B legacy-origin case.
- #2729: `tests/claude-outbound.test.ts` — add the status-less
  `upstream_server_error` case asserting a transient 5xx tail.

## Verification (C)

```bash
bun test tests/generic-oauth-failover.test.ts
bun test tests/claude-outbound.test.ts tests/cursor-errors.test.ts
bun x tsc --noEmit
```

For #2729 the load-bearing proof is the new negative case failing on `dev` without
the fix and passing with it.
