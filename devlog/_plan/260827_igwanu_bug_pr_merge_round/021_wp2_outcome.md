# 021 — wp2 outcome: keystone landed, hypothesis proven

`dev` advanced `8b1b65b8d` -> `50e955604`. Six PRs merged.

| PR | lane | merge commit | evidence |
|---|---|---|---|
| #2766 | L1 keystone | `913f844ef` | full suite 15334/0 on merged tree |
| #2733 | L1 | `ae5d3993c` | approved, clean, mutation-verified oracle |
| #2726 | L1 | `0821ce951` | approved, clean, mutation-verified oracle |
| #2761 | L1 | `d1def682d` | approved by me at exact head, 92/0 focused |
| #2764 | L1 | `3b5302410` | rebased, 26 green / 0 fail, no diff change |
| #2767 | L1 | `50e955604` | rebased, 26 green / 0 fail, no diff change |

## The keystone claim was proven, not assumed

wp1 predicted that #2767, #2764 and #2747 were red for a reason that had nothing
to do with their code. That is a falsifiable claim, and this phase ran the
experiment rather than asserting the conclusion — but the experiment actually ran
on only two of the three. See "#2747 is not part of the proof" below.

Before: each showed `test 3/4` and `macos` failing with exactly `1 fail`, and that
one failure was `release version line`; `gates` failed on `privacy:scan`; `ci` was
the fan-in over both.

The intervention was controlled. #2766 merged, then #2764 and #2767 were rebased
onto the new `dev` **with no change to their own diffs** — verified by diffing the
rebased head against the old head and confirming the only delta was #2766's own
two files.

After: **26 success, 0 failures** on both. Patch identity survives the rebase —
`7d644cbafe221eea27fa1369b07cc048a7461d5f` for #2764 and
`719d0d986d1dbb370919e8c15ff4a236d5b4c2de` for #2767 — and restricting the
comparison to either PR's own changed paths yields an empty diff.

One PR changed a version string and a documentation line, and four required jobs
on **two** unrelated PRs went green. Had the round merged in author order or
"cleanest first", both would have been re-run, re-diagnosed, or bounced back to
their author for a defect neither had.

### #2747 is not part of the proof

It carries the same failure signature, but it never went green, and an earlier
draft of this document implied otherwise.

`gh run rerun --failed` re-runs the **same commit**. Run `33059606933` attempt 4
completed `failure`, with `macos` still reporting the `2.34.0` / `v2.34.0`
collision and `ci` failing as its fan-in. A re-run cannot pick up a new base —
only a rebase can, and this PR's head lives on a contributor's fork.

That run was already terminal and red about 70 seconds before this document was
first committed, so "in flight" was wrong when written, not merely overtaken by
events. The correct status is **diagnosed, awaiting an author rebase**, requested
on the PR with the #2764/#2767 result attached as evidence.

The lesson generalizes past this round: a re-run tests the same tree twice. When
the fix landed somewhere else, only a rebase moves the evidence.

## What the round would have gotten wrong without the A gate

The plan as first written would have merged #2766 without the non-author approval
`MAINTAINERS.md` requires, on the reasoning that a user instruction to run the
round supplied it. The reviewer refused that, correctly: a round-level instruction
is not an exact-head PR approval, and `package.json` is a restricted surface per
`.github/scripts/pr-sponsored-surface.cjs`.

The gate turned out to be satisfiable — the operator authenticates as
`lidge-jun`, project owner, and every one of these PRs was authored by
`Ingwannu` or a community contributor, so no approval was a self-approval. Each
approval names the exact head it applies to and the evidence behind it. (#2766
carries two approval events, one before and one after a check re-run; the
effective approval is the latest one, at the merged head.)

That is the difference between a gate being satisfied and a gate being skipped,
and from the outside the merge log would look identical either way.

## Fork branches are not writable, and that is a real constraint

#2747 is `olddonkey`'s fork. Pushing its rebase created a same-named branch on the
origin instead of updating the PR, which was deleted immediately once observed
(`git push origin --delete`, confirmed `0` remaining refs).

The correct action for a fork PR whose failure was environmental is to re-run its
CI against the repaired base, not to rewrite the contributor's branch. Done via
`gh run rerun --failed`.

## Carried into wp3

Seven bug PRs remain: #2747 (approved; blocked on an author rebase, not on its
own code), #2745, #2740, #2729, #2693, #2638, #2497. Three of those (#2745,
#2638, #2497) are the credential/OAuth surface and share
`src/server/responses/core.ts`; pairwise `git merge-tree` is mandatory before any
second one of them lands.
