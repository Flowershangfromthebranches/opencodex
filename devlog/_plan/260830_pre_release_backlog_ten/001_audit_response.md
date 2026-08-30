# 001 — A-phase audit response (round 1)

Verdict received: **FAIL** — 7 blockers. All 7 are accepted; none are rebutted.
The auditor also independently verified every load-bearing code citation in
`050`, `070`, `080`, `090` and every quoted owner score, finding no material
error. So the diffs are right and the *process* around them was not.

## B1 — incomplete priority inventory (accepted)

Two open enhancements outscore items in the map and were not named:
**#1107 (71)** and **#695 (69)**. The claim "WP10 is the highest-scored" was
therefore wrong as written; #2643's 70 is the highest score *among items in this
unit*, not in the backlog.

Both were assessed during research; the assessment simply never reached the plan.
Recorded exclusions:

- **#695 generic OAuth account-pool failover (69) — DEFER, architectural.**
  Generalizing the 594-line Anthropic engine into a reusable affinity/failover
  layer with provider classification and observability hooks is multi-PR work. The
  owner's own note keeps import, provider UI, and generic 403 handling separate.
  Note WP4 (#2560) is the Anthropic-specific slice of this same area, so the
  release still gets the concrete half.
- **#1107 opt-in authless Codex Desktop routing (71) — DEFER, wrong side of a
  release cut.** Bounded (M) and genuinely implementable, but it introduces an
  **authless routing mode**. Adding a new path that relaxes authentication days
  before a release, when the same cycle already carries two credential-surface
  merges (WP4, WP5) and a new authenticated endpoint (WP8), is poor sequencing
  regardless of score. It is the **first candidate for the next cycle** and is
  recorded as such rather than dropped.

The map stays at eleven phases; the owner asked for ten items.

## B2 — stale merge mechanics (accepted)

`dev` moved to `c2b64dbc3` during the audit. Real drift:

| PR | Plan said | Audit measured |
|---|---:|---|
| #2818 | 1 behind | 6 behind, `MERGEABLE/UNSTABLE/APPROVED` |
| #2498 | 1 behind | **15 behind**, `BLOCKED/CHANGES_REQUESTED` |
| #2560 | 1 behind | **15 behind**, `BLOCKED/CHANGES_REQUESTED` |
| #2083 | 18 behind | **32 behind** |
| #2350 | ~18 behind | **30 behind** |
| #2655 | ~67 behind | **75 behind** |

This is exactly the decay `000_plan.md` predicted, arriving faster than the plan
could be audited. Consequences folded in:

1. #2498 and #2560 have crossed the 10-commit freshness boundary and now need
   rebase handling too, so **every candidate past that boundary** is rebase-first
   — WP3, WP4, WP5, WP6, WP10. Only #2818 (WP2) remains inside it, and it stays
   refresh-and-verify rather than rebase.
2. Contributor branches cannot be pushed to by the maintainer. Where a rebase is
   required on a fork branch, use a **maintainer carry branch**
   (`codex/carry-<pr>-<slug>`) that re-applies the contributor's commits onto
   current `dev`, credits the author by name in the commit trailer, and links the
   original PR. The original PR is then closed with
   `landed-via-maintainer`.
3. Every phase re-measures `behind_by` and `mergeStateStatus` immediately before
   acting. Numbers in `010`-`050` are historical snapshots, not instructions.

## B3 — security notes in a tracked directory (accepted, my error)

`030` and `040` instructed "record the security review in this unit". `devlog/`
is public and tracked; AGENTS.md forbids pre-disclosure security material there,
and explicitly says seniority is not an exemption. Corrected: reviews are drafted
in `.tmp/` (gitignored), and only a **sanitized outcome** — "reviewed, no finding"
or a finding already fixed and public — is committed. Exact-head security approval
remains a merge gate either way.

## B4 — wrong data-plane auth function (accepted, verified independently)

`070` specified `resolveResponsesApiAuth`. Confirmed on `origin/dev`:

- `resolveApiAuth` (`src/server/auth-cors.ts:448`) accepts `x-opencodex-api-key`,
  bearer, **and `x-api-key`** (`:456-457`) for Anthropic-SDK clients.
- `resolveResponsesApiAuth` (`:475`) deliberately rejects `x-api-key`
  (`:487`) because that transport has a credential-collision problem.
- `/v1/models` — the actual peer of a read-only catalog route — uses the general
  path (`src/server/index.ts:1072`) and is allowlisted at `:711`.

A catalog read never forwards a credential upstream, so the Responses-specific
restriction buys nothing and would reject a client holding a perfectly valid data
credential. **WP8 uses `resolveApiAuth`**, matching `/v1/models`, with
`AUTH_MATRIX` and tests written against that. PR #2772 chose
`resolveResponsesApiAuth`; that is now a seventh reason not to take its commits.

Also accepted: `server.md` and `codex-integration.md` exist in all seven locales,
so WP8's docs step covers eight files per page, not one.

## B5 — WP7 Done criterion too narrow (accepted)

Section D deliverables (`x-opencodex-request-id`, `cacheHitRate`,
`vercelGatewayRouting`) and the stale Kiro CLI note were in the body but absent
from the completion gate, so they could have been silently skipped. Criterion c-7
is widened in the goalplan to name all of them plus
`tests/cli-account.test.ts`.

## B6 — retry budget not diff-level (accepted)

"Counted fetch wrapper" is a sketch, not a spec, for the single most load-bearing
change in the unit. `090` gains explicit before/after pseudocode: where the
shared counter increments, how the remaining budget is passed into
`fetchWithResetRetry`, how the terminal transient response body survives
exhaustion, and the mixed reset/503 and cancellation sequences. Also accepted:
`providerManagementConfigError` must call the validator **unconditionally** —
POST/reload can carry the field — while PATCH editing stays out of v1.

## B7 — verification and landing contract insufficient (accepted)

Two real holes:

1. **No PR path for new work.** WP7-WP10 are new implementation; they must land as
   `codex/`-prefixed PRs into `dev` with the full PR template and exact-head CI,
   not as direct pushes. Direct-to-`dev` was never authorized and is not implied
   by merge authorization for the ten items.
2. **Windows.** The auditor is right that standard PR CI skips Windows, so calling
   a green aggregate "Cross-platform CI" overstates it. WP2 (#2818) inspects the
   installed CLI and is platform-sensitive; it gets a dedicated-branch
   `workflow_dispatch` Windows run. Shared server/config/routing phases (WP8,
   WP9, WP10) require remote `typecheck` **plus** the full suite on the SSH host
   or exact-head hosted full CI — focused tests alone are not sufficient for those
   surfaces per AGENTS.md.

`privacy:scan` is required on every credential-adjacent phase (WP4, WP5, WP8,
WP9), and `docs-site` build on WP7.

## Non-blocking items adopted

- `070` said "six defects" and listed five; with B4 it is genuinely six.
- Renaming "version-line decision" to **release-cut/promotion timing**: `dev`
  already declares 2.36.0 and a preview exists, so the version line is settled and
  only the cut timing is the owner's call.
- #1829 has drifted from 0 to 15 behind, reinforcing its exclusion.

## Residual after correction

None of the seven is deferred. Round 2 re-audits the corrected unit.
