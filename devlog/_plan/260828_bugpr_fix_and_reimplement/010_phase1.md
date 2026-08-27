# 010 — wp2 FIX lane: #2747 and #2740

Both are correct code sitting on a stale base. Neither needs a logic change; both
need a rebase I am now authorized to push.

## #2747 — reap the recovery proxy instead of trusting `stop`

Head `07b975873`, **already approved by me**, 39 behind, merge CLEAN, tsc OK,
15/0 on its own suite on the merged tree. Fork `olddonkey/fix/update-recovery-orphan-cleanup`,
`maintainerCanModify: true`.

Its `ci`/`macos` red is the pre-#2766 `release version line` failure, and a rerun
cannot clear it because a rerun replays the same commit.

ACTION: rebase `pr2747-r3` onto `origin/dev`, force-push to the fork branch, let CI
run at the new head, merge when green.

**Fork-push discipline.** The previous round declined this and said so; the user has
now authorized it. Constraints that still apply: rebase only — never squash, reword,
or drop the author's commits; verify `git diff` between old and new head touches
nothing but the rebase; state on the PR that the branch was rebased and why.

## #2740 — atomically commit cleanup run metadata

Head `f07ee36f2`, draft, 39 behind, merge CLEAN, tsc OK, 2/0 on the merged tree.
Fork `luvs01/fix/storage-policy-metadata-race`, `maintainerCanModify: true`.

Mutation oracle already proven in the previous round: revert only
`src/storage/policy.ts` + `src/storage/policy-job.ts` and the race test goes 0 pass /
2 fail. The test drives the interleave through
`setPersistedConfigMutationBeforeCommitForTests`, so it is deterministic rather than
timing-dependent — it will not become a flake later.

It has only 5 checks, none of which compile or test. The merged-tree run is its first
real evidence.

ACTION: rebase onto `origin/dev`, force-push, `gh pr ready` so the full matrix runs,
merge when green.

## Ordering

Independent — `tests/update-stop-first.test.ts` vs `src/storage/*`. No pairwise
merge-tree needed. Land #2747 first (already approved), then #2740.

## TESTS

- #2747: `tests/update-stop-first.test.ts` (the PR is the test).
- #2740: `tests/storage-policy-config-race.test.ts`.

## Verification (C)

```bash
bun x tsc --noEmit
bun test tests/update-stop-first.test.ts          # expect 15/0
bun test tests/storage-policy-config-race.test.ts # expect 2/0
```

Plus exact-head CI green after each rebase, and for #2740 the mutation oracle
re-run on the rebased tree rather than the remembered one.
