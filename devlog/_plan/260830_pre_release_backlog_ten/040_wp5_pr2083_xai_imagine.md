# 040 — WP5: PR #2083 relay Codex image_gen to xAI Imagine with Grok OAuth

Owner score 58. Author `zhou-zhichao`. `maintainer-sponsored`, `review-ready`.

## State — the best-verified PR in the set

`MERGEABLE / CLEAN`, `APPROVED`, and its exact-head `Cross-platform CI`
**actually executed and passed**: 18 jobs, attempt 2. 24 files / +1003/−65.

The one disqualifier from merging as-is: it is **18 commits behind `dev`**, past
the 10-commit freshness boundary the repository's own readiness gate applies. So
the green CI is real but no longer describes what would land.

## Execution

1. Rebase onto current `dev`. Expect the conflict surface to be image/tool
   normalization in the adapters.
2. Re-run exact-head CI on the rebased head; require `success`. The pre-rebase
   green run does not carry over.
3. Security review — this relays user prompts to an external image endpoint using
   **Grok OAuth credentials or an API key**:
   - the credential must reach only the intended xAI origin, never a
     caller-influenced host;
   - redirects must not be followed into a credential leak (SSRF boundary);
   - no prompt bodies, credentials, or account identifiers in logs;
   - the relay must stay behind its opt-in and not activate for operators who
     never configured image relay.
4. Merge after both the green rebased CI and the recorded review.

## Note on prior review state

The PR is `APPROVED`, but the record shows a second maintainer/owner security
review was still expected on the credential-egress boundary. Treat the rebase as
the natural point to obtain it, since the head changes anyway.

## Done

Rebased, exact-head CI `success`, OAuth image-egress review recorded, merged
(criterion c-5).
