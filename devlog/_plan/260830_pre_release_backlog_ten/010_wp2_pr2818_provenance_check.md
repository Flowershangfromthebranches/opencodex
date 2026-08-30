# 010 — WP2: PR #2818 candidate-only Codex CLI provenance check

Owner score 55. Closes issue #2811 (51). Author `luvs01`.

## Why this is first

It is the only PR in the entire open set that needs **no code change and no
rebase**. At snapshot: non-draft, `MERGEABLE`, `APPROVED` on the exact head,
1 commit behind `dev`, all four policy/intake checks green. The single missing
gate is that `Cross-platform CI` sits at `action_required` with 0 jobs — fork
PRs do not start repository CI on their own.

Diff at snapshot: 49 files, +2495/−64. The size is almost entirely locale docs
for a read-only CLI verb.

## Execution

1. Re-read live state; `behind_by` must still be small and `mergeStateStatus`
   must not be `DIRTY`.
2. Authorize the `action_required` Cross-platform CI run for the **exact current
   head**. Workflow approval is not review approval — this is only starting CI.
3. Require a `success` conclusion on that head. A green run on an older head does
   not transfer.
4. Squash merge to `dev`.
5. Close #2811 manually — PRs target `dev`, and GitHub only auto-closes on merge
   into the default branch `main`.

## Risk

The verb inspects the installed Codex CLI and reports provenance; it does not
mutate the launcher. It does not touch `scripts/release.ts` or
`.github/workflows/*`. Lifecycle-adjacent, so if CI reveals a service-lifecycle
failure, that is a real signal and not flake.

## Done

Merged into `dev`; exact-head Cross-platform CI conclusion `success`
(criterion c-2).
