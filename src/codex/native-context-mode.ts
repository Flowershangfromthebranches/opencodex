import { NATIVE_GPT56_ONE_MILLION_MODEL_IDS } from "../types";

/** Per-model catalog settings published for the explicit GPT-5.6 1M opt-in. */
export const NATIVE_GPT56_ONE_MILLION_CONTEXT_WINDOW = 1_000_000;
export const NATIVE_GPT56_ONE_MILLION_AUTO_COMPACT_LIMIT = 900_000;
export const MANAGED_NATIVE_CONTEXT_MARKER = "# Managed by opencodex: native GPT-5.6 1M context";

/** Exact public model allowlist for the official 1M mode. Capability aliases are excluded. */
export const NATIVE_GPT56_ONE_MILLION_MODELS: ReadonlySet<string>
  = new Set(NATIVE_GPT56_ONE_MILLION_MODEL_IDS);

const TARGETS = {
  model_context_window: NATIVE_GPT56_ONE_MILLION_CONTEXT_WINDOW,
  model_auto_compact_token_limit: NATIVE_GPT56_ONE_MILLION_AUTO_COMPACT_LIMIT,
} as const;

type TargetKey = keyof typeof TARGETS;

export type ManagedNativeContextTransformResult =
  | { ok: true; changed: boolean; content: string }
  | { ok: false; changed: false; content: string; error: string };

function fail(content: string, error: string): ManagedNativeContextTransformResult {
  return { ok: false, changed: false, content, error };
}

function targetAssignment(line: string): { key: TargetKey; value: number } | null {
  const match = line.match(/^\s*(model_context_window|model_auto_compact_token_limit)\s*=\s*(\d+)\s*(?:#.*)?$/);
  if (!match) return null;
  return { key: match[1] as TargetKey, value: Number(match[2]) };
}

/**
 * Remove root settings written by the earlier active-model implementation. The current 1M
 * mode lives entirely in per-model catalog rows, so injection never creates this block. Strict
 * marker/value validation prevents migration cleanup from deleting user-owned settings.
 */
export function removeManagedNativeContextMode(input: string): ManagedNativeContextTransformResult {
  const eol = input.includes("\r\n") ? "\r\n" : "\n";
  const content = input.replace(/\r\n/g, "\n");
  const lines = content.split("\n");
  const firstTable = lines.findIndex(line => /^\s*\[/.test(line));
  const rootEnd = firstTable === -1 ? lines.length : firstTable;
  const owned = new Map<TargetKey, number>();

  for (let index = 0; index < lines.length; index += 1) {
    if (lines[index]!.trim() !== MANAGED_NATIVE_CONTEXT_MARKER) continue;
    if (index >= rootEnd) return fail(input, "managed native context marker is not at the TOML root");
    const assignment = targetAssignment(lines[index + 1] ?? "");
    if (!assignment || index + 1 >= rootEnd) {
      return fail(input, "orphaned managed native context marker cannot be updated safely");
    }
    if (owned.has(assignment.key)) {
      return fail(input, `duplicate managed ${assignment.key} cannot be updated safely`);
    }
    if (assignment.value !== TARGETS[assignment.key]) {
      return fail(input, `managed ${assignment.key} has an unexpected value`);
    }
    owned.set(assignment.key, index);
    index += 1;
  }

  const removals = [...owned.values()].sort((a, b) => b - a);
  for (const index of removals) lines.splice(index, 2);

  let output = lines.join("\n");
  if (eol === "\r\n") output = output.replace(/\n/g, "\r\n");
  return { ok: true, changed: output !== input, content: output };
}
