/**
 * Stable discovery facade: literal metadata and types have separate pure owners.
 * The transitive dependency boundary is checked by the CLI capability tests.
 * Command modules must never enter this graph: their top-level usage constants
 * can become undefined through an ESM cycle. This is not an execution sandbox.
 */
import { CAPABILITIES as BASE_CAPABILITIES } from "./capabilities-base";
import type { Capability } from "./capability-types";
import { PROVIDER_MODEL_CAPABILITIES } from "./capabilities-provider-models";
import { ACCOUNT_CAPABILITIES } from "./capabilities-accounts";
import { AGENT_ROUTING_CAPABILITIES } from "./capabilities-agents-routing";
import { INTEGRATION_CAPABILITIES } from "./capabilities-integrations";
import { OBSERVE_SYSTEM_CAPABILITIES } from "./capabilities-observe-system";
import { ACCESS_REMOTE_CAPABILITIES } from "./capabilities-access-remote";
import { LAB_CAPABILITIES } from "./capabilities-lab";

export const CAPABILITIES: readonly Capability[] = [
  ...BASE_CAPABILITIES,
  ...PROVIDER_MODEL_CAPABILITIES,
  ...ACCOUNT_CAPABILITIES,
  ...AGENT_ROUTING_CAPABILITIES,
  {
    command: ["advisor"],
    summary: "Inspect and configure the advisor sidecar (expert consultation for routed workers).",
    routes: [
      { method: "GET", path: "/api/advisor/settings" },
      { method: "PUT", path: "/api/advisor/settings" },
    ],
    flags: [{ name: "--json", value: "boolean", summary: "Emit advisor settings as JSON." }],
    mutates: true,
    json: "payload",
    details: [
      "`status` (the default) reads the resolved settings; `on`/`off` toggle the sidecar; `consent` records or revokes context-sharing consent; `set` updates model, effort, policy, or timeout.",
      "`on` does not grant consent. Without current consent it refuses and prints the disclosure. `on --ack-context-sharing` records consent v1 and enables. `consent --revoke` removes consent and stops task-context transfer.",
      "The advisor model may be any routable model string: a bare native model, an explicit `provider/model`, or an account-qualified native model.",
      "`policy: preflight` makes OpenCodex attempt one automatic consultation per task once the task shows orientation evidence (an assistant tool call or a tool result after the latest user message); `policy: manual` consults only when the worker calls the synthetic `advisor` tool. Neither path sends task context without current context-sharing consent.",
    ],
  },
  ...INTEGRATION_CAPABILITIES,
  ...OBSERVE_SYSTEM_CAPABILITIES,
  ...ACCESS_REMOTE_CAPABILITIES,
  ...LAB_CAPABILITIES,
];

export { HEAD_CAPABILITIES } from "./capabilities-base";
export type {
  CapabilityRoute,
  CapabilityFlag,
  CapabilityJsonMode,
  Capability,
  HeadCapability,
} from "./capability-types";

/** Capabilities that drive `route`, for `ocx capabilities --route`. */
export function capabilitiesForRoute(path: string): Capability[] {
  return CAPABILITIES.filter(cap => cap.routes.some(r => r.path === path));
}

/** Every `(method, path)` pair any capability drives. */
export function capabilityRouteKeys(): Set<string> {
  const keys = new Set<string>();
  for (const cap of CAPABILITIES) {
    for (const route of cap.routes) keys.add(`${route.method} ${route.path}`);
  }
  return keys;
}

/** Rendered command path, e.g. `ocx account pause`. */
export function capabilityInvocation(cap: Capability): string {
  return `ocx ${cap.command.join(" ")}`;
}
