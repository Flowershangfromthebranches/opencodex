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
| WP2 | PR #2818 | | | pending |
| WP3 | PR #2498 | | | pending |
| WP4 | PR #2560 | | | pending |
| WP5 | PR #2083 | | | pending |
| WP6 | PR #2350 | | | pending |
| WP7 | docs/locale parity | | | pending |
| WP8 | issue #809 | | | pending |
| WP9 | issue #1168 | | | pending |
| WP10 | issue #2643 | | | pending |
| WP11 | PR #2952 | already merged | `dca16949b` on `dev`; `gh pr view 2952` → `MERGED` | `NOOP` |

## Rules for filling this in

- A merge row needs the merge commit **and** the exact-head CI conclusion.
- An implementation row needs the SSH command tail and exit code.
- A `BLOCKED`/`UNSAFE`/`NEEDS_HUMAN` row needs the specific external dependency or
  decision, named.
- Never mark a row done from memory. Re-read the tree or the API.
