# 260830 — Pre-release backlog: ten non-bug items for 2.36.0

> Owner-directed unit. The ranking input is the maintainer's own recorded score,
> `## 리뷰 · 우선순위 NN / 80`, harvested from every open issue (49) and the
> candidate PRs (21) at 2026-08-30T01:45Z. This is not a fresh opinion about what
> matters; it is the owner's scoring made executable.

## Why this unit exists

`main` is 2.35.0. `dev` is 2.36.0 with 105 unreleased commits and no
`v2.36.0` release on GitHub. So 2.36.0 **is** the next release: the useful work
is draining what already accumulated, not opening new surface. Bug PRs are
explicitly out of scope (they are their own train).

## The constraint that orders everything

`dev` moves several times an hour. During the research pass alone it went
`4f6a19643` → `13ba8f11f`. Two consequences, and they are the reason this plan
is sequenced the way it is:

1. **A "behind by N" number is evidence with a shelf life.** Every merge decision
   re-reads it at execution time rather than trusting this document.
2. **Merge-ready PRs are perishable and implementation work is not.** A PR that is
   1 commit behind and approved becomes a conflict if it waits behind three
   implementation cycles. So the merge phases (WP2-WP6) run before the
   implementation phases (WP8-WP10), even though WP10 carries the highest owner
   score (70).

That is the whole ordering rationale. It is not effort bucketing.

## Verification constraints (user-imposed, binding)

- **Local full suites are forbidden.** No `bun run test` on this machine.
- Verification runs on SSH host `lidge` (Linux, bun 1.3.14) in a dedicated
  worktree `/home/lidgeai/ocx-verify`, pinned per phase. Baseline
  `bun run typecheck` there is green (exit 0) at `de4e846e8`.
- `--no-verify` push is pre-approved by the user.
- Merges to `dev` are pre-approved **for these ten items only**.
- Landing strategy per item is the agent's choice among squash merge, cherry-pick,
  commit stacking, or re-implementation.

## Work-phase map (dependency-ordered)

| WP | Item | Owner score | Kind | Doc |
|---|---|---:|---|---|
| WP1 | This roadmap (docs-only cycle) | — | docs | `000_plan.md` |
| WP2 | PR #2818 Codex CLI provenance check | 55 | merge | `010` |
| WP3 | PR #2498 grok-4.20-multi-agent | 61 | merge | `020` |
| WP4 | PR #2560 Anthropic quota-window pool | 50 (issue #2539: 59) | merge + security | `030` |
| WP5 | PR #2083 image_gen → xAI Imagine | 58 | rebase + merge + security | `040` |
| WP6 | PR #2350 empty tool-output annotation | 52 | fix + merge | `050` |
| WP7 | docs/locale parity | — | docs | `060` |
| WP8 | issue #809 least-privilege `GET /v1/catalog` | 66 | implement | `070` |
| WP9 | issue #1168 GLM Coding Plan quota | 65 | implement | `080` |
| WP10 | issue #2643 transient-5xx retry (PR #2655) | 70 | re-implement | `090` |
| WP11 | PR #2952 README asset guard | 42 | verify NOOP | `100` |

WP2-WP11 each depend only on WP1. They do not depend on each other, with one
exception recorded in `050`: WP6 and WP10 both touch provider config schema, so
whichever lands second re-reads the first's shape.

## Scope boundaries

In scope: `src/`, `tests/`, `gui/`, `docs-site/`, this devlog unit, and
`skills/ocx/` when the surface map regenerates.

Out of scope, explicitly:

- `scripts/release.ts` and `.github/workflows/*` — release automation is a
  security-review surface and cutting the release is not this unit's job.
- Promoting `dev` to `main`, tagging, or publishing. **The 2.36.0 version-line
  decision is NEEDS_HUMAN** and stays with the owner.
- Any `src/lab/` import reachable from `src/router.ts`,
  `src/server/lifecycle.ts`, or `src/server/responses/core.ts`
  (`tests/core-lab-boundary.test.ts` enforces this).
- Security findings in tracked directories. Pre-disclosure material goes to
  `.tmp/` per AGENTS.md.

## Items deliberately excluded despite high scores

**PR #2783 quota-reset detection, score 68 — the highest-scoring PR, excluded.**
202 commits behind, CONFLICTING, and five verified blockers remain: webhook
redirect/HTTPS safety, poller cadence/fencing, durable dedupe, auth-mode identity,
and startup activation order. Its own devlog history is also mixed into the PR.
Landing it before a release would be the opposite of release hardening.

**The remote-hub stack #2771→#2789.** All seven are 214+ behind, all
CHANGES_REQUESTED, cumulatively 182 files and +14,564 lines. The roadmap defines
Phase 6 as the release gate, and the Phase 5 dogfood record repairs Phase-1-era
hub-role startup defects — so landing #2772 alone would ship behavior whose known
fixes sit five phases later. **No landable prefix exists.** WP8 reuses #2772's
catalog serializer design without taking its commits.

**PR #1829 durable reset-credit ledger, approved and 0 behind.** Mergeable today,
but 4 files / +2,878 lines of currently dormant infrastructure. Including it is a
release-content judgment call the owner should make, not a mechanical merge. It is
the prerequisite for issue #2275 (53).

**Issue #1820 usage cost/cache metrics, score 56.** Already landed on `dev` via
PR #2365. It ships with the release; it is a release-note line, not work.

## Terminal outcome vocabulary

Per `cxc-loop`: `DONE` verified success, `NOOP` no change needed, `BLOCKED`
external dependency, `UNSAFE` needs an owner risk decision, `NEEDS_HUMAN`
owner judgment required, `BUDGET_EXHAUSTED` against a stated bound. A merge
claim requires an exact-head green CI conclusion; an implementation claim requires
the SSH command tail plus exit code.
