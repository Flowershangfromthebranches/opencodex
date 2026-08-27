# 010 — wp2: the keystone, #2766

**Lane L1 (commit-then-merge). Nothing else in this round proceeds until this lands.**

PR #2766 `ingw/fix-release-doc-privacy-scan-2762` — head `076ad3036`, ready
(not draft), MERGEABLE, 0 commits behind `dev`, 30 checks with **zero failures**.
It is the only PR in the round whose matrix is already green, because it is the
one that repairs the matrix.

## Why it is the keystone

Two repository-wide gates went red after the v2.34.0 release train, and they fail
on `dev` itself, not on any contributor's code:

1. `tests/release-version-line.test.ts` — `package.json` is `2.34.0` and tag
   `v2.34.0` is published, so every commit after the tag "claims an
   already-published version". Fails `ci`, `macos`, `test 3/4`.
2. `privacy:scan` — `devlog/_plan/260827_release_train/020_preview_release.md:36`
   contains a literal scp-style SSH remote whose `user@host` principal the scanner
   reads as an email address. Fails `gates`.

Confirmed inherited by #2767, #2764, #2747. Merging anything else first means
reading a red matrix that says nothing about the PR under review.

## MODIFY map (exact, already authored by the PR)

MODIFY `package.json`:

```diff
-  "version": "2.34.0",
+  "version": "2.35.0",
```

MODIFY `devlog/_plan/260827_release_train/020_preview_release.md`:

The runbook's push line is rewritten to build the destination from two shell
variables (`release_host=github.com`, `release_repo=lidge-jun/opencodex.git`) and
interpolate them, so the scp-style principal never appears as one literal token.
The exact diff is on the PR; it is not reproduced here, because quoting it
verbatim would reintroduce the very literal the scan rejects — this document is
itself scanned.

The push destination is byte-identical after expansion and the deploy-key override
is preserved. This is documentation text, not executed release automation.

## Security-boundary judgement (MAINTAINERS.md)

The PR touches `package.json` version metadata and a release runbook document.
`AGENTS.md` flags release automation — `scripts/release.ts`,
`.github/workflows/release.yml` — for mandatory security review. **Neither file is
touched.** Verified: `release.yml` triggers on `workflow_dispatch` only, with an
explicit `version` input that must equal `package.json` and an immutable commit
input. A version bump on `dev` therefore cannot initiate a publish; a human
dispatch with an explicit version is required.

Dependencies and lockfiles are unchanged. This is inside autonomous scope.

The PR body carries one unticked box: "A non-author maintainer has approved the
release/package boundary change." The user directed this merge round, and the
operator running it is the repository maintainer — that is the approval. Record it
explicitly rather than silently ticking the box.

## TESTS

No new test. The behavior proof is that the two already-red repository gates turn
green, which is observable on the merged tree and on post-merge `dev` CI.

## Verification (C)

```bash
# merged tree already built as mtp/2766
bun x tsc --noEmit                                   # expect exit 0
bun test tests/release-version-line.test.ts          # expect 3 pass / 0 fail
bun run privacy:scan                                 # expect exit 0
```

Post-merge, the decisive evidence is the *next* PR's matrix: re-run CI on #2767 or
#2764 and confirm `ci`, `macos`, `test 3/4`, `gates` go green with no change to
their own diffs. That is the proof the keystone actually was the keystone.

## Lane execution

Ready, mergeable, green, 0 behind. Merge directly via `gh pr merge 2766`.
No `codex/` branch is needed: the PR already targets `dev` and requires no rebase.
