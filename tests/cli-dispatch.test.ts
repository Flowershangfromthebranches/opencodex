import { describe, expect, spyOn, test } from "bun:test";
import { CLI_COMMANDS } from "../src/cli/registry";
import { DISPATCH_ALIASES, DISPATCH_COMMANDS, dispatchCommand, resolveDispatchCommand } from "../src/cli/dispatch";
import type { CliDispatchDeps } from "../src/cli/dispatch";
import { runGuiCommand } from "../src/cli/gui";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

/** Minimal fake deps. dispatchCommand only touches deps for real command
 * runners, which these tests never invoke, so an empty object is enough. */
const fakeDeps = {} as unknown as CliDispatchDeps;

describe("CLI dispatch command coverage", () => {
  test("every non-hidden registry command is dispatchable", () => {
    const aliasResolved = new Set([...DISPATCH_COMMANDS, ...DISPATCH_ALIASES.keys()]);
    const missing = CLI_COMMANDS.filter(entry => {
      if (entry.hidden) return false;
      // A visible command counts as dispatchable when it is a direct runner
      // key or an alias that resolves to one (setup/eject/remove/model).
      return !aliasResolved.has(entry.name);
    }).map(entry => entry.name);
    expect(missing).toEqual([]);
  });

  test("every dispatch alias resolves to a dispatchable command", () => {
    for (const [alias, target] of DISPATCH_ALIASES) {
      expect(DISPATCH_COMMANDS).toContain(target);
      expect(alias).not.toBe(target);
    }
  });
});

describe("CLI dispatch aliases", () => {
  test("canonical alias pairs resolve to their command", () => {
    expect(DISPATCH_ALIASES.get("setup")).toBe("init");
    expect(DISPATCH_ALIASES.get("eject")).toBe("restore");
    expect(DISPATCH_ALIASES.get("remove")).toBe("uninstall");
    expect(DISPATCH_ALIASES.get("model")).toBe("models");
  });

  test("resolveDispatchCommand maps each alias to its canonical runner key", () => {
    // The same resolver dispatchCommand uses for runner selection, exercised
    // at the resolution level so a regression in the lookup is caught.
    expect(resolveDispatchCommand("setup")).toBe("init");
    expect(resolveDispatchCommand("eject")).toBe("restore");
    expect(resolveDispatchCommand("remove")).toBe("uninstall");
    expect(resolveDispatchCommand("model")).toBe("models");
    // Canonical names resolve to themselves; unknown names resolve undefined.
    expect(resolveDispatchCommand("init")).toBe("init");
    expect(resolveDispatchCommand("definitely-not-a-command")).toBeUndefined();
    expect(resolveDispatchCommand(undefined)).toBeUndefined();
  });

  test("resolveDispatchCommand rejects inherited Object property names", () => {
    // commandRunners is a normal object; inherited names (__proto__,
    // constructor, toString) must not resolve as valid commands.
    expect(resolveDispatchCommand("__proto__")).toBeUndefined();
    expect(resolveDispatchCommand("constructor")).toBeUndefined();
    expect(resolveDispatchCommand("toString")).toBeUndefined();
  });
});

describe("dispatchCommand exit codes", () => {
  test("invalid client state refuses sync before local proxy discovery", async () => {
    const home = mkdtempSync(join(tmpdir(), "ocx-dispatch-client-invalid-"));
    const previous = process.env.OPENCODEX_HOME;
    let discoveries = 0;
    try {
      process.env.OPENCODEX_HOME = home;
      writeFileSync(join(home, "config.json"), JSON.stringify({
        port: 10100,
        providers: {},
        defaultProvider: "openai",
        runtimeRole: "client",
        client: { apiKeyId: "half-present" },
      }), "utf8");
      const args = ["sync"];
      const deps = {
        ...fakeDeps,
        args,
        findLiveProxy: async () => { discoveries += 1; return null; },
      };
      expect(await dispatchCommand({ kind: "command", command: "sync", args }, deps)).toBe(1);
      expect(discoveries).toBe(0);
    } finally {
      if (previous === undefined) delete process.env.OPENCODEX_HOME;
      else process.env.OPENCODEX_HOME = previous;
      rmSync(home, { recursive: true, force: true });
    }
  });

  test("returns 0 for help forms", async () => {
    expect(await dispatchCommand({ kind: "help", command: "help", args: ["help"] }, fakeDeps)).toBe(0);
    expect(await dispatchCommand({ kind: "help", command: "--help", args: ["--help"] }, fakeDeps)).toBe(0);
    expect(await dispatchCommand({ kind: "help", command: "-h", args: ["-h"] }, fakeDeps)).toBe(0);
    expect(await dispatchCommand({ kind: "command", command: undefined, args: [] }, fakeDeps)).toBe(0);
  });

  test("returns 1 for an unknown command", async () => {
    const head = { kind: "command" as const, command: "definitely-not-a-command", args: ["definitely-not-a-command"] };
    expect(await dispatchCommand(head, fakeDeps)).toBe(1);
  });

  test("returns 1 for inherited Object property names", async () => {
    for (const name of ["__proto__", "constructor", "toString"]) {
      const head = { kind: "command" as const, command: name, args: [name] };
      expect(await dispatchCommand(head, fakeDeps), `${name} must be unknown`).toBe(1);
    }
  });

  test("forwards service arguments and preserves handler exit codes", async () => {
    const previousExitCode = process.exitCode;
    try {
      process.exitCode = 7;
      const successCalls: string[][] = [];
      const successDeps = {
        ...fakeDeps,
        args: ["service", "install", "--scheduler"],
        serviceCommand: async (...args: string[]) => {
          successCalls.push(args);
        },
      };

      expect(await dispatchCommand(
        { kind: "command", command: "service", args: successDeps.args },
        successDeps,
      )).toBe(0);
      expect(successCalls).toEqual([["install", "--scheduler"]]);

      for (const expected of [1, 2]) {
        const calls: string[][] = [];
        const deps = {
          ...fakeDeps,
          args: ["service", "install", "--scheduler"],
          serviceCommand: async (...args: string[]) => {
            calls.push(args);
            process.exitCode = expected;
          },
        };

        expect(await dispatchCommand(
          { kind: "command", command: "service", args: deps.args },
          deps,
        )).toBe(expected);
        expect(calls).toEqual([["install", "--scheduler"]]);
      }
    } finally {
      process.exitCode = previousExitCode ?? 0;
    }
  });
});

describe("GUI command delegation", () => {
  const config = {
    port: 10100,
    runtimeRole: "hub" as const,
    hub: { managementPublicOrigin: "https://hub.example.test" },
    corsAllowOrigins: ["https://dashboard.example.test"],
    providers: {},
    defaultProvider: "openai",
  };

  test("keeps the default open behavior and requires an explicit pairing origin", async () => {
    let opens = 0;
    const deps = {
      loadConfig: () => config,
      openDefaultGui: async () => { opens += 1; return 0; },
    };
    expect(await runGuiCommand([], deps)).toBe(0);
    expect(opens).toBe(1);
    expect(await runGuiCommand(["pair"], deps)).toBe(1);
    expect(await runGuiCommand(["pair", "--origin", "https://dashboard.example.test", "extra"], deps)).toBe(1);
  });

  test("prints a created grant once and maps remote API refusal to exit 1 without echoing response data", async () => {
    const stdout: string[] = [];
    const stderr: string[] = [];
    const logSpy = spyOn(console, "log").mockImplementation(value => { stdout.push(String(value)); });
    const errorSpy = spyOn(console, "error").mockImplementation(value => { stderr.push(String(value)); });
    try {
      const base = {
        loadConfig: () => config,
        openDefaultGui: async () => 0,
        findLiveProxy: async () => ({ pid: 4242, port: 10100, source: "runtime" as const }),
      };
      const grant = `ocx_pair_${"C".repeat(43)}`;
      expect(await runGuiCommand(["pair", "--origin", "https://dashboard.example.test", "--json"], {
        ...base,
        requestPairingGrant: async () => ({
          kind: "created",
          grant,
          browserOrigin: "https://dashboard.example.test",
          serverOrigin: "https://hub.example.test",
          expiresAt: 1_800_000_300_000,
        }),
      })).toBe(0);
      expect(stdout.join(" ").split(grant)).toHaveLength(2);

      stdout.length = 0;
      expect(await runGuiCommand(["pair", "--origin", "https://dashboard.example.test"], {
        ...base,
        requestPairingGrant: async () => ({ kind: "unavailable", reason: "rejected" }),
      })).toBe(1);
      expect(`${stdout.join(" ")} ${stderr.join(" ")}`).not.toContain("remote-response-secret");
    } finally {
      logSpy.mockRestore();
      errorSpy.mockRestore();
    }
  });
});
