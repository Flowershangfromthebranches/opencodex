import type { OcxTool, OcxToolCall, OcxToolResultMessage } from "../../types";
import type { ToolSemanticClass } from "./types";

const LIMIT = 8192;
export interface ClassifiedTool { kind: ToolSemanticClass; fingerprint?: string; target?: string; tool: string }
/** Bounded irreversible identifiers: no commands, paths, source, or outputs retained in state. */
export function fingerprint(value: string): string {
  return new Bun.CryptoHasher("sha256").update(value).digest("hex");
}
export function classifyTool(call: OcxToolCall, metadata?: OcxTool): ClassifiedTool {
  const tool = call.name;
  const unknown: ClassifiedTool = { kind: "unknown", tool };
  const args = call.arguments;
  if (!args || typeof args !== "object") return unknown;
  let kind: ToolSemanticClass = "unknown";
  let identity: string | undefined;
  let target: string | undefined;
  // Explicit runtime semantics take precedence. Do not infer MCP semantics from a suffix.
  if (metadata?.advisor || metadata?.imageGeneration || metadata?.videoGeneration) return unknown;
  if (metadata?.webSearch || metadata?.toolSearch) kind = "search";
  else if (metadata?.cursorStructuredEdit) kind = "mutation";
  else if (call.namespace) return unknown;
  else if (["edit_file", "write_file", "apply_patch", "Edit", "Write"].includes(tool)) kind = "mutation";
  else if (["read_file", "Read", "cat"].includes(tool)) kind = "read";
  else if (["grep", "search", "list", "Grep", "Glob"].includes(tool)) kind = "search";
  else if (["shell", "shell_command", "exec_command", "Bash"].includes(tool)) {
    const command = args.cmd ?? args.command;
    if (typeof command !== "string" || command.length > LIMIT) return unknown;
    // Compound commands, substitutions and redirections can hide a different exit status.
    // Quoting is kept byte-for-byte: different experiments must not collapse into one.
    const cmd = command.trim();
    if (/[;&|<>`$\n\r]/.test(cmd)) return unknown;
    if (/^(?:(?:bun|npm|pnpm|yarn) (?:run )?(?:test|lint|typecheck|build)(?: |$)|(?:pytest|jest|vitest|tsc)(?: |$)|(?:cargo|go) (?:test|build|check)(?: |$)|python(?:3)? -m pytest(?: |$))/.test(cmd)) kind = "validation";
    else if (/^(?:cat|head|tail|ls|rg|grep)(?: |$)/.test(cmd)) kind = "read";
    else if (/^git (?:diff|status|log|show)(?: |$)/.test(cmd)) kind = "diagnostic";
    else if (/^(?:cp|mv|rm|touch) [\w./ -]+$/.test(cmd)) kind = "mutation";
    else kind = "execution";
    const workdir = args.workdir ?? args.cwd ?? "";
    if (typeof workdir !== "string" || workdir.length > LIMIT) return unknown;
    identity = `${workdir}\0${cmd}`;
  }
  if (kind === "mutation" && identity === undefined) {
    const path = args.path ?? args.file_path ?? args.filename;
    if (typeof path === "string" && path.length <= LIMIT) target = fingerprint(path);
    // Different edits to one file are NOT equivalent actions. Retain only the target digest;
    // mutation-count policy covers these without claiming the edits are identical.
    return { kind, tool, target: target ?? "unspecified" };
  }
  return { kind, tool, ...(identity === undefined ? {} : { fingerprint: fingerprint(`${tool}\0${identity}`) }) };
}
/** Read only a bounded structured envelope, never search arbitrary stdout for success prose. */
export function observedExit(result: OcxToolResultMessage): boolean | undefined {
  if (result.isError) return false;
  if (result.containsEncryptedContent) return undefined;
  const content = result.content;
  const text = typeof content === "string" ? content : content.length === 1 && content[0]?.type === "text" ? content[0].text : undefined;
  if (typeof text !== "string" || text.length > LIMIT) return undefined;
  try {
    const value = JSON.parse(text);
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
    const code = value.exit_code ?? value.exitCode;
    if (typeof code === "number" && Number.isInteger(code)) return code === 0;
    if (value.isError === true) return false;
  } catch { /* Plain tool envelopes are supported only with an anchored, complete header. */ }
  const match = /^(?:Chunk ID: [^\n]+\n)?Wall time: [^\n]+\n(?:Process exited with code|Exit code:) (\d+)\n(?:Final output:|Output:)\n/.exec(text);
  return match ? Number(match[1]) === 0 : undefined;
}
