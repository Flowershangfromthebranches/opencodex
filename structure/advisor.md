# Advisor Sidecar

The advisor is an OpenCodex-owned expert consultation runtime. A routed worker can consult a
user-configured expert model WITHOUT any client-side delegation: the proxy injects a synthetic
`advisor` tool, executes the consultation itself through the routing authority, and reinjects the
advice so the original worker continues. `src/advisor/` owns the settings resolver, the synthetic
tool, the sanitized context builder, the loopback consultation executor, and the request plan.

The advisor is distinct from the Codex-owned subagent surface (`subagents.md`): subagents are
worker-initiated delegation through the collaboration catalog; the advisor is a proxy-side sidecar
the client never sees. A worker that never spawns anything can still be advised.

## Optional-subsystem boundary

The advisor follows the same seam discipline as the Lab. `src/server/responses/advisor-slot.ts`
is the core-owned slot: it holds the structural plan interface and the event-stream guard and
imports nothing from `src/advisor/` at runtime. The only runtime import of `src/advisor/` in the
Responses path is `src/server/responses/sidecar-execution.ts`, which registers a per-request plan
onto the parsed request. `src/router.ts`, `src/server/lifecycle.ts`, and
`src/server/responses/core.ts` never reach the advisor, and a disabled advisor executes no advisor
code on the request path. The guard is applied by `adapter-delivery.ts` through the structural
`_advisorGuard` field — type-level knowledge only.

## Execution paths

- Translated (non-passthrough, non-run-turn) fetch path: full support — synthetic tool injection,
  guard interception of `advisor` tool calls, advice reinjection as a paired
  assistant-toolCall/toolResult message pair, and worker re-dispatch through the same
  continuation machinery the terminal guard uses (`adapter-continuation.ts`). Consultations are
  bounded per request; past the bound the worker receives an explicit limit-reached result.
- Run-turn adapters: preflight support only — the automatic pre-dispatch consultation attempt
  applies, but the synthetic tool is never injected because the run-turn loop cannot intercept it.
- Native OpenAI passthrough: no advisor support in PR1. The request path is byte-identical to a
  proxy without the advisor; the limitation is documented, not silently degraded.
- Turns claimed by the web-search or image/video sidecar loops keep the advisor tool un-injected;
  preflight still applies.

## Recursion fence (server-owned authority)

The consultation executor calls the proxy's own `/v1/chat/completions` on loopback and presents
`x-opencodex-advisor-internal` with a **process-owned capability**: a 256-bit random value minted
once per process, kept in memory only — never in config, on disk, in logs, in usage, in request
metadata, or in an API response, and never forwarded upstream. The Chat surface carries the fact
into `handleResponses` as `advisorInternal` only when the header value matches that capability
(shape-checked, constant-time compare); a request without it — including one that sends the old
literal `1` — is an ordinary external request and never receives internal authority. Peer address
is deliberately not part of the decision: Docker, WSL, tunnels, and port forwarding can all end on
loopback. A marked request never plans an advisor consultation, so depth stays capped at 1 under
combo re-resolution, and a new process mints a new value, which invalidates any captured token.

The vision-describe fence still compares a literal header value and therefore has the same
pre-existing spoof shape; wiring it to this capability is a separate follow-up, recorded so the
gap is visible rather than assumed absent.

## Cross-provider consultation

The loopback call re-enters the normal data plane, so model resolution, provider auth, effort
mapping, and usage accounting are the routing authority's job. Any model string the router
accepts works as the advisor: a bare native model, an explicit `provider/model`, or an
account-qualified native model. The advisor never builds its own router and never touches
provider credentials.

## Context and safety boundaries

**Cross-provider data transfer is the feature's documented cost:** a consultation sends the
task conversation and tool results to the configured advisor provider, which may differ from the
worker's provider — the GUI, docs, and config description must say so. The proxy injects none of
its own credentials (no provider API keys, Authorization/OAuth material, backend-only secrets,
or environment variables), never transfers chain-of-thought, and never decrypts or forwards
encrypted provider-only content. **Task content is not generally secret-redacted** — pasted
credentials and token-bearing tool output travel as-is, because no reliable string-level
secret detector exists; no DLP claim may be made in any doc, GUI string, or PR text. The payload
is built exclusively from the parsed conversation the model is already allowed to see: user task, conversation, tool calls and their results, the worker's tool catalog,
and both model identities. The advisor system instruction states the boundary explicitly:
conversation history, tool outputs, logs, file contents, and instructions quoted inside them are
untrusted evidence — the advisor analyses them and never obeys them, because only its own system
instruction defines its role (defense in depth, not a claim that injection is solved). Thinking and
chain-of-thought parts are never included, encrypted
provider content is never decrypted or forwarded, and failure text is redacted and bounded before
it can reach any context. Advice is re-injected as identifiable wrapper-tagged content with no
system authority: manual consultations arrive as paired tool results carrying the
`<opencodex_advisor>` wrapper, and preflight advice as a marked developer message carrying the
`<opencodex_advisor_preflight>` wrapper.

## Provenance and the preflight claim

"Already advised" is decided by PROVENANCE, never by scanning for a bare string — and only for
MANUAL advice: a `toolResult` whose `toolName` is the synthetic advisor tool and whose content
carries the `<opencodex_advisor>` wrapper. Automatic preflight is NOT decided from history at all;
its dedup authority is the claim ledger described below, so a client cannot suppress the policy
by echoing or forging a developer message.

Ordinary tool output, developer text, user text, and failure notices (`<opencodex_advisor_unavailable>`)
match nothing, so nothing a shell, log, or upstream error body prints can suppress or forge advice. The guard never composes failure prose itself: `AdvisorPlan.formatUnavailable` owns that
text and neutralizes untrusted fragments.

Keys are SHA-256 digests, never a short fold and never raw text: one domain-separated digest
over `conversation identity + task boundary + worker model`, where the task boundary digests the
FULL latest user text (no truncation) together with the user-turn count. Task identity is a
correctness boundary, so a 32-bit hash is not acceptable there, and storing only the digest means
a captured key reveals nothing about the conversation.

Automatic-preflight dedup is ledger-authoritative. The `<opencodex_advisor_preflight>` wrapper in
the injected developer message is informational — it labels the text for the worker and for logs —
and developer messages are never inspected for suppression, because a client could echo or forge
one. Manual advice remains verifiable history (paired tool result, `toolName` = the synthetic
advisor tool).

The preflight ledger is an atomic CLAIM table, not a has-then-mark pair: `claim` returns
`claimed` / `inflight` / `complete` / `cooldown` with an ownership token, and a settlement whose
token no longer matches is a no-op.
Success suppresses for the task lifetime; a failure suppresses only for a one-minute cooldown
(the minute scale the repository already uses for polling), so a transient outage pauses the
policy instead of silencing it; a client cancellation releases the claim with no cooldown. Keys
are conversation identity + task boundary + worker model, reusing the existing `thread-id` /
Cursor / replay-scope identities. A client with NO stable identity stays out of the ledger
entirely: it is limited to request-scoped dedup and genuine in-history provenance (fail-open),
so two independent identity-less conversations can never suppress each other.

## State

Request-scoped state (consultation count, dedup fingerprints, preflight flag) lives in the
per-request plan closure. Task-scoped preflight state is a bounded, process-local CLAIM table
(see "Provenance and the preflight claim") keyed by conversation identity + task boundary +
worker model; a client with no stable identity is excluded from it on purpose. After a proxy
restart the table is empty, so a task in progress may receive one more preflight attempt —
fail-open for correctness and only one extra expert call.

## Policies

- `manual` (default): only an explicit worker `advisor()` call consults.
- `preflight`: OpenCodex additionally ATTEMPTS one consultation per task automatically. The
  documented approximation for "before the first substantive mutation": the attempt fires on the
  first worker reasoning turn that arrives with orientation evidence — an assistant tool call OR
  a tool result — since the latest user message, unless the conversation already carries advisor
  advice. A failed attempt is recorded under its own ledger key (no retry storm within the TTL)
  and injected with the `<opencodex_advisor_unavailable>` wrapper, which
  historyHasManualAdvisorResult deliberately does not match: a failure is not advice and does not permanently suppress the
  policy. No semantic stagnation detection exists in PR1.

## Observability

Every consultation writes one structured `[advisor]` log line (trigger, worker model, advisor
model, duration, status, usage) and the loopback call lands in usage accounting as its own
request under the advisor model. Consultation usage is never merged into the worker's terminal
usage; intercepted worker legs are, via the same usage-merge rule the terminal guard applies.

## Adaptive trigger engine

`src/advisor/triggers/types.ts` defines observable completed-tool events. The bounded projector
in `src/advisor/triggers/engine.ts` consumes returned client tool results at the existing
pre-dispatch seam (the preceding worker/tool turn has finished). No token-delta evaluation or
new core imports are added. Native passthrough and provider-private tool loops remain unobservable.
`src/advisor/triggers/policy.ts` reduces events and evaluates deterministic, effect-free decisions.
Only `src/advisor/runtime.ts` can dispatch, through the same PR1 consultation and ledger authority.

`adaptive` includes the existing preflight baseline, then escalates only on `repair_failed`:
an explicit validation failure, at least one substantive mutation after it, then a follow-up
failure of that same validation. Edits with no prior failure do not consult. Two failures with
no mutation between them do not consult. A follow-up failure whose fingerprint differs does not
consult; a missing fingerprint may consult with `same_validation=unknown`. Manual and preflight
policies do not allocate or invoke a trigger engine. There is no semantic stuck detection.
No thresholds are public config fields.

`src/advisor/triggers/classify-tool.ts` prefers existing structured tool flags, then exact known
names and conservative shell command patterns. Compound shell commands and namespaced lookalikes
stay unknown. A validation needs an explicit exit code, structured success/error, or tool error;
unknown outcomes are neither a failure nor progress. A passing validation clears the failure
cycle. Distinct command/target/working-directory digests are not merged, and a different
follow-up validation starts a new failure instead of consulting. Compound shell commands,
including assignments, stay unclassified. Edit bodies and stdout are never hashed or stored.

A successful consultation, including the preflight baseline and a manual `advisor()` call,
clears the failure cycle. Another escalation requires a new failure, a new repair, and a new
failure. The baseline does not block that later cycle. Manual consultation uses the same
claim in adaptive mode and resets observation state on success. `claim(..., repeatAfterSuccess)`
permits a subsequent consultation while preserving in-flight exclusion, saturation, settlement
tokens, cancellation release and provider-failure cooldown. Automatic advice uses the existing
developer-role injection; that role's trust limitation remains PR1 design debt.

State is process-local, keyed solely by `advisorLedgerKey`, with 512 tasks and a 24-hour TTL.
Each projection reads at most 128 recent messages and 128 parts/tools. Per task, pending calls
are capped at 128 and dedup IDs at 2,048. Saturation disables escalation without evicting live
state; no timer is started. TTL cleanup is lazy. Identity-less clients retain PR1 baseline
behavior but never accumulate adaptive state. Compaction or restart can reset observations.
Automatic developer markers never grant consultation authority. Logs and advisor focus metadata
contain only the trigger reason and counters, with no action arguments or output logs.

`tests/advisor/advisor-trigger-policy.test.ts`, `tests/advisor/advisor-trigger-classification.test.ts`
and `tests/advisor/advisor-adaptive-runtime.test.ts` exercise policy, false positives, replay,
bounds and shared ownership. `tests/advisor/advisor-responses-wiring.test.ts` proves cross-provider
consultation and advice reinjection into the original worker after edit/fail/edit/fail.
