# 020 — WP3: PR #2498 expose grok-4.20-multi-agent on the Responses lane

Owner score **61** — the highest-scoring PR that is actually landable. Author
`olddonkey`.

## State

Non-draft, `MERGEABLE`, 1 commit behind, 9 files / +224/−12. Policy checks green.
Two gates: `Cross-platform CI` is `action_required`, and a
`CHANGES_REQUESTED` review from `Ingwannu` is still recorded.

## The review question, stated honestly

A later reviewer pass found no remaining conceptual blocker, and the head was
rebased after the objection. But **a `lidge-jun` approval does not erase another
reviewer's blocking review** — GitHub keeps `CHANGES_REQUESTED` as the
`reviewDecision` until it is superseded by that reviewer or dismissed. So this
needs one deliberate action beyond CI, and that action is a judgment call about
another maintainer's objection, not a formality.

Read the objection at execution time and choose:

- the concern is addressed on the current head → have it superseded or dismiss it,
  recording which commit addressed what; or
- the concern still stands → it is a real blocker; fix it and re-request review.

Do not merge while `CHANGES_REQUESTED` is live.

## Execution

1. Refresh live state and re-read every review thread.
2. Resolve the stale review per the choice above.
3. Authorize exact-head Cross-platform CI; require `success`.
4. Squash merge to `dev`.

## Risk

Provider/catalog/routing surface. Adds a model entry plus inbound effort handling
on the Responses lane. Not an auth or credential surface; no release-automation
files touched.

## Done

Merged with exact-head CI `success` and the stale review resolved rather than
bypassed (criterion c-3).
