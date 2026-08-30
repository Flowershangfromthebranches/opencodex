# 030 — WP4: PR #2560 Anthropic quota-window account-pool routing

PR score 50; the issue it closes (#2539) scores **59**. Author `Yoonkeee`.
Labelled `maintainer-sponsored` and `review-ready`.

## State

Non-draft, `MERGEABLE`, 1 commit behind, 31 files / +937/−66. Policy checks
green, `Cross-platform CI` `action_required`, review
`CHANGES_REQUESTED`. The recorded objection was freshness of the quota evidence
used for selection; the head has since been rebased and the author addressed it.

## Owner-accepted scope (from issue #2539)

Opt-in selector that routes by **weekly** usage window, with the five-hour default
preserved. Anything that changes default routing behavior is out of scope.

## Security review — mandatory, not optional

This is an OAuth/account-routing change, which `MAINTAINERS.md` places in the
explicit security-review class. The review must answer, on the exact head:

1. Does the selector ever widen which account a request may reach beyond what the
   operator configured?
2. Is quota evidence used for selection **fenced** — can a stale or absent weekly
   window silently promote an exhausted account?
3. Are account identifiers kept out of logs? `privacy:scan` must stay green.
4. Does the default path change for an operator who does not opt in? It must not.

A green CI run does not answer any of these.

**Where the review is written.** Draft it in `.tmp/` (gitignored), not in this
unit. `devlog/` is public and tracked, and AGENTS.md forbids pre-disclosure
security material there — explicitly including maintainer-authored triage. Only a
sanitized outcome ("reviewed, no finding", or a finding already fixed and public)
is committed here.

## Execution

1. Refresh live state. As of the audit this is **15 commits behind**, past the
   10-commit freshness boundary, so a rebase is required — the "1 behind" figure
   above is a stale snapshot. On a fork branch, use a
   `codex/carry-2560-anthropic-quota-window` carry branch with author credit.
2. Run the security review above, drafting it in `.tmp/`; commit only a sanitized
   outcome here.
3. Resolve the stale `CHANGES_REQUESTED` (superseded by the rebase, or still live).
4. Authorize exact-head CI; require `success`.
5. Squash merge; close #2539 manually.

## Done

Merged with exact-head CI `success` **and** a sanitized OAuth/account-routing
review outcome committed here, the full review having been drafted in `.tmp/`
(criterion c-4). `privacy:scan` green.
