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
| WP3 | PR #2498 → **PR #2985** | carry `codex/carry-2498-grok-multi-agent` | 4 commits cherry-picked w/ author credit; typecheck 0, 72 pass + 330 pass, 0 fail | PR open |
| WP4 | PR #2560 → **PR #2982** | carry `codex/carry-2560-anthropic-quota-window` | 19 commits cherry-picked w/ author credit; security review FAIL → fixed; typecheck 0, 83 pass/0 fail, privacy green | PR open |
| WP5 | PR #2083 → **PR #2986** | carry `codex/carry-2083-xai-imagine` | 8 commits cherry-picked w/ author credit; security review PASS WITH NOTES; typecheck 0, 104 pass/0 fail, privacy green | PR open |
| WP6 | PR #2350 → **PR #2978** | carry `codex/carry-2350-empty-tool-output` | 10 commits cherry-picked w/ author credit + POST fix; regression driven red; review fixes → 110 pass/0 fail | PR open, review folded |
| WP7 | docs/locale parity → **PR #2980** | `codex/wp7-docs-locale-parity` | typecheck 0; cli-account 102 pass/0 fail; docs build 401 pages; per-locale greps verified | PR open |
| WP8 | issue #809 → **PR #2979** | `codex/wp8-least-privilege-catalog` | typecheck 0; route suite 6 pass/0 fail; 157 pass/0 fail auth+boundary; privacy green; docs 401 pages | PR open, review folded |
| WP9 | issue #1168 → **PR #2976** | `codex/wp9-glm-coding-plan-quota` | typecheck 0; 110 pass/0 fail; 25 pass related; privacy green | PR open, review folded |
| WP10 | issue #2643 → **PR #2981** | `codex/wp10-transient-5xx-retry` | typecheck 0; 13 pass/0 fail resolver+budget; 93 pass/0 fail incl. core-lab-boundary | PR open |
| WP11 | PR #2952 | already merged | `dca16949b` on `dev`; `gh pr view 2952` → `MERGED` | `NOOP` |

## Rules for filling this in

- A merge row needs the merge commit **and** the exact-head CI conclusion.
- An implementation row needs the SSH command tail and exit code.
- A `BLOCKED`/`UNSAFE`/`NEEDS_HUMAN` row needs the specific external dependency or
  decision, named.
- Never mark a row done from memory. Re-read the tree or the API.

## Standing status

### FINAL — all seven merged to `dev`

| PR | Item | Merge commit |
|---|---|---|
| #2976 | WP9 GLM Coding Plan quota | `015fedb63` |
| #2985 | WP3 grok-4.20-multi-agent (carry of #2498) | `8b8197dbe` |
| #2978 | WP6 empty tool-output annotation (carry of #2350) | `8fba9d1b8` |
| #2979 | WP8 least-privilege `/v1/catalog` (#809) | `f6367639c` |
| #2981 | WP10 transient-5xx retry (#2643) | `607042b02` |
| #2982 | WP4 Anthropic quota-window pool (carry of #2560) | `46c3383e1` |
| #2980 | WP7 docs and locale parity | `9cd4e4b8e` |

Merged under admin authority the owner granted explicitly. Every PR carried a
`CHANGES_REQUESTED` review that was resolved by fixing its substance — no review was
dismissed. Issues #809, #1168, #2539, #2643 closed; PRs #2350, #2560, #2655 closed
`landed-via-maintainer` with attribution. **#2818 was not merged**: it is another
author's PR whose approval they dismissed themselves, and admin authority is not a
licence to override that.

### Second review round — what the reviewer caught after the first merge attempt

One genuine runtime defect and four contract inaccuracies:

1. **The retry policy was bypassed on Responses recovery (#2981).** After an initial 429,
   `rebuildAndRefetch` called `fetchWithHeaderTimeout` directly, so an opted-in provider got
   retries on the initial send and on native chat but none when a 429 recovered into a
   retryable 503. Every send now routes through one **request-scoped** budget: the helper
   reports consumed sends via `onSendsConsumed` (in a `finally`, since it returns from five
   places and throws from one) and each later leg receives only the remainder.
2. **#2982 UI claimed a setting was inert when it was not.** `autoSwitchThreshold: 0`
   disables *proactive* switching only; new-session selection and 429 recovery still consult
   the window. The selector is now disabled only under `round-robin`.
3. **#2978, #2981** shipped user-facing options documented only in English.
4. **#2979** left two locale guides contradicting themselves about the credential — the
   French sentence said "ordinary data-plane key" and then called it the same `/api/*`
   admission credential in the same clause.
5. **#2980** carried three functional inaccuracies: an unconditional Kiro rotation claim that
   ignored the `oauthAccountFailover.enabled: false` opt-out, four locale signatures missing
   the shipped `[--quota [--refresh]]` flags, and Turkish/Russian text saying *all* or
   *repeated* failures where the contract is *retryable* failures.

**Eight PRs are open and awaiting maintainer review:** #2976 (WP9), #2978 (WP6),
#2979 (WP8), #2980 (WP7), #2981 (WP10), #2982 (WP4), #2985 (WP3), #2986 (WP5).

None were self-merged. The authorization covers landing these ten items, but every one
carries a live review or an open review request that belongs to a maintainer, and
merging past that is a different act from landing the work.

WP2 (#2818) stays `BLOCKED`: its author dismissed their own approval and wrote that
green CI "is not a merge signal" while the owner review request remains open.
WP11 is a verified `NOOP`.

### Contributor attribution

Four phases are carries of contributor work, cherry-picked with authorship preserved so
`git log` credits the original author rather than the maintainer:
`harryzhou2000` (#2978), `Yoonkeee` (#2982), `olddonkey` (#2985),
`zhou-zhichao` (#2986). Each original PR should close with `landed-via-maintainer`
once its carry lands. `TooSpace`'s design is credited in the #2981 commit message,
which is a re-implementation rather than a cherry-pick.

### Security reviews performed

Two phases touched credential surfaces and got independent reviews, drafted in `.tmp/`
with only sanitized outcomes committed:

- **WP4 (#2982): FAIL → fixed.** Known-before-unknown ranking applied to operators who
  never opted in, changing the five-hour default path. Fixed and pinned by regression.
  A second finding — the shared quota cache trusting stale evidence — is pre-existing
  and was disclosed in the PR body rather than silently widened.
- **WP5 (#2986): PASS WITH NOTES.** Credentials pinned to `api.x.ai`, redirects fail
  closed, opt-in verified to fail closed on missing OAuth. The note is that artifact
  authorization is proxy-wide rather than per-user.

## What the loop actually produced

Ten backlog items, eight PRs, and three defects found that nobody had filed:

1. **A spent web-search budget read as exhausted model capacity** (WP9). `headroomOf`
   takes the MAX across quota windows, so mapping the MCP call allowance to
   `monthlyPercent` demoted healthy GLM accounts in quota-aware ranking.
2. **An operator's explicit `false` was silently resurrected** (WP6). A provider POST
   that merely rotated an API key reinstated the DeepSeek registry default, because
   enrichment cannot tell "client omitted" from "registry supplied" after it runs.
3. **A retry budget that multiplied instead of bounding** (WP10). `attempts: 3` could
   emit nine upstream requests. The helper's own comment had flagged the hazard as inert
   "because no caller passes it today" — and this cycle's feature was that first caller.

Three review rounds on the plan and two independent security reviews caught four more
problems before they shipped, including a 32 MiB ceiling that would have rejected a
valid 2,000-model catalog and regressed an existing route to 507.

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

## Reviewer findings folded (round 1)

`Ingwannu` requested changes on all three implementation PRs. Every finding was accepted;
none were rebutted. Each was a real defect the local suites did not cover.

1. **#2976 preserved stale quota (the sharpest one).** After the `TIME_LIMIT` exclusion, a
   *successful* `limits[]` response carrying only MCP rows returned `null` — and
   `fetchProviderQuotaReports` reads `null` as a transient probe failure, preserving the
   last-good report for up to 30 minutes. So the dashboard and quota-aware routing kept using
   model-token windows the provider had already superseded. Fixed with an
   `AUTHORITATIVE_EMPTY_QUOTA` sentinel beside the existing `TERMINAL_QUOTA_FAILURE`,
   routed through the same suppression path. The distinction is the whole point: `null` means
   "told us nothing, keep the row"; the sentinel means "answered, and the answer is none."
   Regression runs two sequential forced refreshes and asserts the cache clears.
2. **#2978 canonical OpenAI could never set the field.** Validation admitted
   `annotateEmptyToolOutputs` and the canonical seed comparison then rejected the same
   request. Fixed by stripping it from the comparison candidate exactly as `contextWindow`
   and `modelAutoCompactTokenLimits` already are, with set/clear/persist regressions. The
   option is now documented too.
3. **#2979's ceiling rejected a valid catalog.** The 32 MiB cap lived in the shared
   serializer, so it applied to `/api/catalog` as well. The repo supports 2,000 discovered
   models and a 2,000-row catalog serializes to ~92 MB — the cap rejected a supported catalog
   *and* regressed the pre-existing management route to 507. Size policy now belongs to the
   remote route alone at 256 MiB; the management route is uncapped as before.

The pattern across all three: the change was locally correct and the failure was in how it
interacted with an existing subsystem — cache preservation, canonical seed comparison, and a
route that merely shared a helper.
