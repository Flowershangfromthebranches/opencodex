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
- Run-turn adapters: preflight support only — the guaranteed pre-dispatch consultation applies,
  but the synthetic tool is never injected because the run-turn loop cannot intercept it.
- Native OpenAI passthrough: no advisor support in PR1. The request path is byte-identical to a
  proxy without the advisor; the limitation is documented, not silently degraded.
- Turns claimed by the web-search or image/video sidecar loops keep the advisor tool un-injected;
  preflight still applies.

## Recursion fence

The consultation executor calls the proxy's own `/v1/chat/completions` on loopback with the
`x-opencodex-advisor-internal: 1` marker header (the same structure as the vision-describe
fence). The Chat surface detects the raw header before its bridge rebuilds headers and carries
the fact into `handleResponses` as `advisorInternal`; a marked request never plans an advisor
consultation. Depth cap 1 holds under combo re-resolution.

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
and both model identities. Thinking/chain-of-thought parts are never included, encrypted
provider content is never decrypted or forwarded, and failure text is redacted and bounded before
it can reach any context. Advice is re-injected as identifiable
`<opencodex_advisor>`-wrapped content with no system authority: manual consultations arrive as
tool results, preflight advice as a marked developer message.

## Provenance and the preflight claim

"Already advised" is decided by PROVENANCE, never by scanning for a bare string:

- manual: a `toolResult` whose `toolName` is the synthetic advisor tool and whose content carries
  the `<opencodex_advisor>` wrapper;
- preflight: a developer message carrying the runtime-owned `<opencodex_advisor_preflight>`
  wrapper.

Ordinary tool output, developer text, user text, and failure notices (`<opencodex_advisor_unavailable>`)
match neither form, so nothing a shell, log, or upstream error body prints can suppress or forge
advice. The guard never composes failure prose itself: `AdvisorPlan.formatUnavailable` owns that
text and neutralizes untrusted fragments.

The preflight ledger is an atomic CLAIM table, not a has-then-mark pair: `claim` returns
`claimed` / `inflight` / `complete` / `cooldown`, and `complete` / `fail` / `release` settle it.
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
  and injected with the `<opencodex_advisor_unavailable>` wrapper, which historyHasAdvisorResult
  deliberately does not match: a failure is not advice and does not permanently suppress the
  policy. No semantic stagnation detection exists in PR1.

## Observability

Every consultation writes one structured `[advisor]` log line (trigger, worker model, advisor
model, duration, status, usage) and the loopback call lands in usage accounting as its own
request under the advisor model. Consultation usage is never merged into the worker's terminal
usage; intercepted worker legs are, via the same usage-merge rule the terminal guard applies.
