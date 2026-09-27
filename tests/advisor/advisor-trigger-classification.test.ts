import { expect, test } from "bun:test";
import { classifyTool, observedExit } from "../../src/advisor/triggers/classify-tool";
import type { OcxToolCall, OcxToolResultMessage } from "../../src/types";
const call = (name: string, args = {}): OcxToolCall => ({ type: "toolCall", id: "c", name, arguments: args });
const result = (content: string, isError = false): OcxToolResultMessage => ({ role: "toolResult", toolCallId: "c", toolName: "shell", content, isError, timestamp: 0 });
test("conservative command classifier preserves meaningful command differences", () => {
  for (const cmd of [
    "bun test", "bun test tests/foo.test.ts", "npm test", "npm run test", "npm run build",
    "pnpm test", "yarn test", "pytest", "pytest tests/test_a.py", "python -m pytest", "python3 -m pytest",
    "cargo test", "go test ./...", "tsc", "vitest", "jest",
  ]) expect(classifyTool(call("shell", { cmd })).kind).toBe("validation");
  for (const cmd of [
    "cd repo && bun test", "FOO=1 bun test", "bun test | tee log", "bun test > log.txt",
    "bun test; echo done", "echo \"bun test\"", "false", "curl localhost",
  ]) expect(classifyTool(call("shell", { cmd }))).toEqual({ kind: "unknown" });
  for (const name of ["shell", "shell_command", "exec_command", "Bash"]) {
    expect(classifyTool(call(name, { cmd: "bun test" })).kind).toBe("validation");
  }
  expect(classifyTool({ ...call("shell", { cmd: "bun test" }), namespace: "mcp__foo" })).toEqual({ kind: "unknown" });
  expect(classifyTool(call("mcp__foo__shell", { cmd: "bun test" }))).toEqual({ kind: "unknown" });
  expect(classifyTool(call("shell", { cmd: "bun test a.ts" })).fingerprint).not.toBe(classifyTool(call("shell", { cmd: "bun test b.ts" })).fingerprint);
  expect(classifyTool(call("shell", { cmd: "bun test", workdir: "a" })).fingerprint).not.toBe(classifyTool(call("shell", { cmd: "bun test", workdir: "b" })).fingerprint);
  expect(classifyTool(call("exec", { input: 'await tools.exec_command({cmd:"bun test"})' })).kind).toBe("unknown");
});
test("explicit metadata takes priority; arbitrary names and namespaced lookalikes stay unknown", () => {
  expect(classifyTool(call("custom"), { name: "custom", parameters: {}, description: "", cursorStructuredEdit: true }).kind).toBe("mutation");
  expect(classifyTool({ ...call("write_file"), namespace: "mcp__other" }).kind).toBe("unknown");
  expect(classifyTool(call("maybe_test"))).toMatchObject({ kind: "unknown" });
  const classified = classifyTool(call("write_file", { path: "private/path", content: "secret source" }));
  expect(classified.kind).toBe("mutation");
  expect(JSON.stringify(classified)).not.toContain("private/path");
  expect(JSON.stringify(classified)).not.toContain("secret source");
  expect(classified.fingerprint).toBeUndefined();
});
test("exit status requires explicit structured evidence, never stdout prose", () => {
  expect(observedExit(result('{"exit_code":0,"output":"tests failed"}'))).toBe(true);
  expect(observedExit(result('{"exitCode":1}'))).toBe(false);
  expect(observedExit(result("ok", true))).toBe(false);
  expect(observedExit(result("3 tests failed"))).toBeUndefined();
  expect(observedExit(result('{"session_id":42,"exit_code":null}'))).toBeUndefined();
  expect(observedExit(result('output\n{"exit_code":0}'))).toBeUndefined();
  expect(observedExit(result("Wall time: 1 seconds\nProcess exited with code 1\nFinal output:\nfailed"))).toBe(false);
  expect(observedExit(result('x'.repeat(9000)))).toBeUndefined();
});

test("unknown tool metadata never retains an unbounded raw tool name", () => {
  const classified = classifyTool(call("sensitive-".repeat(100000)));
  expect(classified).toEqual({ kind: "unknown" });
  expect(JSON.stringify(classified).length).toBeLessThan(100);
});
