# 110 — Execution ledger

Append-only record of what each work-phase actually did. One row per phase, filled
at that phase's D close with the evidence that justified the outcome. A phase with
no row here did not happen, regardless of what any other document claims.

## Roadmap lock (WP1)

| Field | Value |
|---|---|
| Branch | `codex/pre-release-backlog-260830` |
| Base | `13ba8f11f` (`origin/dev` at unit creation) |
| Commits | `6c2590957` roadmap · `4e2eb9f57` audit corrections · `e9a6b53ab` residuals |
| Docs-only proof | `git diff --name-only 13ba8f11f..HEAD` → 12 files, **0** outside the unit |
| Audit rounds | 3 — `FAIL` (7 blockers) → `FAIL` (5 residual + 4 new) → `NEAR-PASS` (2 residual, both closed) |
| Outcome | `DONE` |

What the audit changed, in one line each: the two highest-scoring backlog items
(#1107 at 71, #695 at 69) were missing and are now recorded exclusions; `dev`
drifted far enough mid-audit that every candidate past the 10-commit boundary
became rebase-first; security-review notes moved out of this public directory into
`.tmp/`; WP8's admission function was wrong (`resolveResponsesApiAuth` rejects
`x-api-key`); WP10's budget fix and cancellation semantics needed real pseudocode;
and the correction commit had landed on the wrong worktree.

What the audit did **not** find: a single incorrect file path, line number, symbol,
or owner score. The three defects the plan discovered in existing code were
independently confirmed.

## Implementation phases

| WP | Item | Landing | Evidence | Outcome |
|---|---|---|---|---|
| WP2 | PR #2818 | not landed | exact-head CI green (23 pass) but `BLOCKED`/`REVIEW_REQUIRED`; see below | `BLOCKED` |
| WP3 | PR #2498 | | | pending |
| WP4 | PR #2560 | | | pending |
| WP5 | PR #2083 | | | pending |
| WP6 | PR #2350 → **PR #2978** | carry branch `codex/carry-2350-empty-tool-output` | 10 commits cherry-picked w/ author credit + POST fix; typecheck 0, 109 pass/0 fail, regression driven red | PR open, CI green |
| WP7 | docs/locale parity | | | pending |
| WP8 | issue #809 → **PR #2979** | `codex/wp8-least-privilege-catalog` | typecheck 0; 5 pass route suite; 157 pass/0 fail auth+boundary; privacy green; docs build 401 pages | PR open, CI running |
| WP9 | issue #1168 → **PR #2976** | `codex/wp9-glm-coding-plan-quota` | typecheck 0; 109 pass/0 fail; 25 pass related; privacy green; CI 21 pass/0 fail | PR open, CI green |
| WP10 | issue #2643 | | | pending |
| WP11 | PR #2952 | already merged | `dca16949b` on `dev`; `gh pr view 2952` → `MERGED` | `NOOP` |

## Rules for filling this in

- A merge row needs the merge commit **and** the exact-head CI conclusion.
- An implementation row needs the SSH command tail and exit code.
- A `BLOCKED`/`UNSAFE`/`NEEDS_HUMAN` row needs the specific external dependency or
  decision, named.
- Never mark a row done from memory. Re-read the tree or the API.

## WP2 — why it is BLOCKED rather than merged

The plan called this the one PR needing no code change and no rebase. Re-measuring at
execution time, as `000_plan.md` requires, found that had stopped being true:

- head moved `850249f221` → `6ea39f2b4` (three new commits within ~35 minutes: Flatpak
  path-component classification, Windows-only Scoop scoping);
- `Ingwannu`'s approval was **DISMISSED** at 2026-08-29T17:36:15 because the new head was
  not patch-equivalent to the approved slice;
- at 2026-08-30T03:19:25 they wrote: *"I will restore an approval only after those exact-head
  jobs are green and no new final-head finding appears. The owner review request also remains
  open, so this is not a merge signal."*

Exact-head CI has since gone fully green (23 pass, macOS 11m48s). But a reviewer who
dismissed their own approval and said in writing that green CI is not a merge signal has
not been superseded by that CI turning green. Merging here would be overriding a live
review, which the merge authorization for these ten items does not extend to.

**What unblocks it:** `Ingwannu` restoring the approval on `6ea39f2b4` (or its successor),
plus the still-open owner review. Then it is a squash merge and a manual close of #2811.
A Windows `workflow_dispatch` run is still required per `010`.

## Discovered defects worth recording

Three defects were found while implementing, none of which were in the plan:

1. **WP9 was worse than a mislabelled bar.** `headroomOf()` takes the MAX across quota
   windows, so mapping the MCP call allowance to `monthlyPercent` meant a spent web-search
   budget read as *exhausted model capacity* and demoted a healthy account in quota-aware
   ranking. The regression pins that specific case.
2. **WP8's first auth test was vacuous.** `isApiAuthRequired` returns false for a loopback
   bind, so on `127.0.0.1` every data-plane request is admitted as `kind: "loopback"`. The
   test passed while asserting nothing until it was rebound to `0.0.0.0`.
3. **`AUTH_MATRIX` could not see a new read-only row.** Its `isGet` check named only
   `/v1/models`, so `/v1/catalog` would have been POSTed into a 405. The missing-catalog
   404 is now pinned to code `catalog_not_found` so a deleted route cannot pass vacuously.
