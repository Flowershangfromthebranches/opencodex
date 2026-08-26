type Schema = Record<string, unknown>;

const MAX_SCHEMA_DEPTH = 64;
const MAX_SCHEMA_NODES = 10_000;
const MAX_REF_EXPANSIONS = 64;

interface NormalizeState {
  nodes: number;
  refExpansions: number;
  activeRefs: Set<string>;
}

function isRecord(value: unknown): value is Schema {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function decodePointerToken(token: string): string {
  return token.replace(/~1/g, "/").replace(/~0/g, "~");
}

function lookupLocalRef(root: Schema, ref: string): unknown {
  if (!ref.startsWith("#/")) return undefined;
  let current: unknown = root;
  for (const token of ref.slice(2).split("/").map(decodePointerToken)) {
    if (!isRecord(current) || !Object.hasOwn(current, token)) return undefined;
    current = current[token];
  }
  return current;
}

function schemaValuesEqual(left: unknown, right: unknown): boolean {
  try {
    return JSON.stringify(left) === JSON.stringify(right);
  } catch {
    return false;
  }
}

/**
 * A 2020-12 ref and its siblings are conjunctive. Moonshot does not expose a documented
 * composition subset that we can rely on, so never approximate conjunction by overwriting a
 * referenced assertion. Equal duplicate assertions and non-conflicting keys are safe to inline;
 * any conflicting duplicate makes the caller retain the pure ref instead.
 */
function mergeCompatibleRefSiblings(target: Schema, siblings: Schema): Schema | undefined {
  const merged: Schema = { ...target };
  for (const [key, value] of Object.entries(siblings)) {
    if (Object.hasOwn(target, key) && !schemaValuesEqual(target[key], value)) return undefined;
    merged[key] = value;
  }
  return merged;
}

function normalizeNode(
  value: unknown,
  root: Schema,
  state: NormalizeState,
  depth: number,
): unknown | undefined {
  state.nodes += 1;
  if (depth > MAX_SCHEMA_DEPTH || state.nodes > MAX_SCHEMA_NODES) return undefined;

  if (Array.isArray(value)) {
    const out: unknown[] = [];
    for (const item of value) {
      const normalized = normalizeNode(item, root, state, depth + 1);
      if (normalized === undefined) return undefined;
      out.push(normalized);
    }
    return out;
  }
  if (!isRecord(value)) return value;

  const ref = typeof value.$ref === "string" ? value.$ref : undefined;
  const siblingEntries = ref === undefined
    ? []
    : Object.entries(value).filter(([key]) => key !== "$ref");
  if (ref !== undefined && siblingEntries.length > 0) {
    const target = lookupLocalRef(root, ref);
    if (
      isRecord(target)
      && !state.activeRefs.has(ref)
      && state.refExpansions < MAX_REF_EXPANSIONS
    ) {
      state.activeRefs.add(ref);
      state.refExpansions += 1;
      const normalizedTarget = normalizeNode(target, root, state, depth + 1);
      state.activeRefs.delete(ref);
      const normalizedOverlay = normalizeNode(Object.fromEntries(siblingEntries), root, state, depth + 1);
      if (isRecord(normalizedTarget) && isRecord(normalizedOverlay)) {
        const merged = mergeCompatibleRefSiblings(normalizedTarget, normalizedOverlay);
        if (merged !== undefined) return merged;
        return { $ref: ref };
      }
      if (normalizedTarget === undefined || normalizedOverlay === undefined) return undefined;
    }

    // Moonshot accepts a standalone ref but rejects validation siblings beside it. If the
    // target cannot be expanded safely, preserve identity and drop the ambiguous overlay.
    return { $ref: ref };
  }

  const out: Schema = {};
  for (const [key, item] of Object.entries(value)) {
    const normalized = normalizeNode(item, root, state, depth + 1);
    if (normalized === undefined) return undefined;
    out[key] = normalized;
  }
  return out;
}

/**
 * Normalize first-party Moonshot tool schemas without changing other OpenAI-chat gateways.
 * Over-limit schemas fail closed by omitting the tool instead of publishing a partial rewrite.
 */
export function normalizeMoonshotToolParameters(parameters: unknown): Schema | undefined {
  const root: Schema = isRecord(parameters)
    ? (parameters.type === "object" ? parameters : { ...parameters, type: "object" })
    : { type: "object", properties: {} };
  const normalized = normalizeNode(root, root, {
    nodes: 0,
    refExpansions: 0,
    activeRefs: new Set(),
  }, 0);
  return isRecord(normalized) ? normalized : undefined;
}
