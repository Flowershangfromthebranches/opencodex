# 041 — #2729 superseded by #2769, and what the blocker actually was

#2729's diagnosis was right and its review was right, and the two facts compose
into something neither states alone.

## The defect #2729 fixed

Internal `response.failed` envelopes carry the classified `{type, code, message}`
but no numeric status, so every classified failure was flattened into a retryable
`overloaded_error`. A Cursor plan/quota 429 reached Claude Code as "Repeated 529
Overloaded errors". Deriving the status from the classified payload is correct.

## The blocker: the derivation only helps if the classification wins

`httpStatusFromTerminalError` recognized a structured server class for exactly one
code pair — `server_error` + `server_is_overloaded` — and let everything else fall
through to message inference. Reproduced against `origin/dev`:

```
{type:"server_error", code:"upstream_server_error",
 message:"upstream stream produced malformed tool call arguments"}  ->  400
```

Claude Code receives `invalid_request_error` and stops retrying a retryable
upstream failure. That is #2729's own inversion, one layer down: it fixed masking
at the envelope boundary while the status function kept masking underneath.

`classifyError` assigns `upstream_server_error` to **every** 5xx it observes, so
the class is authoritative about blame.

## What I got wrong, and what caught it

My first fix returned a blanket 502 for any structured server class. Nine focused
suites passed — 210/0 — and it was still wrong.

Exact-head CI failed `test 1/4` and `test 4/4`. The cause:
`tests/web-search-timeout-contract.test.ts` asserts `status: 504` for a stalled
routed body, because a stall genuinely **is** a gateway timeout. A blanket 502
flattens 504 and 503 into a less specific status, discarding information the log
surface and the retry policy both read.

The classification is authoritative about **blame**, not about which server status
fits. So the override narrowed to the single verdict that both blames the caller
and stops the retry: **400 only**. 429, 499, 401 and 403 are left alone — each is
a signal the caller routes on, and overriding them trades one misreport for
another.

Final differential against `origin/dev` — exactly two cases move:

| case | before | after |
|---|---|---|
| `upstream_server_error` + "malformed" | 400 | **502** |
| `upstream_server_error` + "invalid request" | 400 | **502** |
| web-search stall | 504 | 504 |
| "temporarily unavailable" | 503 | 503 |
| rate-limit text under server class | 429 | 429 |
| client close under server class | 499 | 499 |
| cyber-policy by message | 400 | 400 |
| auth / permission text under server class | 401 / 403 | 401 / 403 |
| real `invalid_request_error` | 400 | 400 |
| `proxy_error` / no message | 500 / 502 | 500 / 502 |

## The transferable finding

**Nine green focused suites did not catch a defect that one CI shard caught
immediately.** The focused suites covered the function I changed; the failure was
in a suite that consumes it. Scoping tests to the changed file is exactly how a
blast-radius defect hides — the previous round's lesson was that green checks are
not health, and this is its sharper form: green *targeted* checks are not health
either, because you chose the targets.

What made it cheap to recover was the differential probe. Enumerating every arm
before and after, against unpatched `dev`, turns "did I break something" from a
hope into a table.

## Evidence

```
remote full suite (ocx-run e2769b on lidge)   15349 pass / 0 fail, rc=0
nine consuming focused suites                  210 pass / 0 fail
bun x tsc --noEmit                             exit 0
mutation oracle (revert fix, keep test)        45 pass / 1 fail
mutation oracle (with fix)                     46 pass / 0 fail
```

One macOS CI failure remains, and it is not this change:
`CL-07 task effectiveness producer > inactivity timeout is bounded for trusted
route executors`, a wall-clock test in `src/lab/` that references nothing in the
changed path and passes 47/0 locally. A clean dev merge commit (`d1def682d`) failed
the same day on a *different* timing test, `ocx launcher graceful shutdown`, which
is the signature of pre-existing macOS timing flakiness rather than a regression.
Re-run rather than diagnosed — and that distinction is recorded here deliberately,
because "it was flaky" is the claim this repository's standing gates exist to
distrust.
