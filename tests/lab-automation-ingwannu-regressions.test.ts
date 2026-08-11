import { afterEach, describe, expect, test } from "bun:test";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { OcxConfig } from "../src/types";
import { defaultLabAutomationPolicyV1 } from "../src/lab/automation/policy";
import * as persistence from "../src/lab/automation/persistence";
import {
  isLabAutomationSchedulerRunning,
  resetLabAutomationSchedulerStateForTests,
  runLabAutomationTick,
  setLabAutomationDispatchDeps,
  startLabAutomationScheduler,
} from "../src/lab/automation/orchestrator";

const HOMES: string[] = [];

function tempHome(): string {
  const dir = join(tmpdir(), `ocx-lab-ingwannu-${process.pid}-${Math.random().toString(16).slice(2)}`);
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  HOMES.push(dir);
  return dir;
}

function emptyConfig(): OcxConfig {
  return { providers: {} } as OcxConfig;
}

afterEach(() => {
  resetLabAutomationSchedulerStateForTests();
  for (const dir of HOMES.splice(0)) rmSync(dir, { recursive: true, force: true });
});

describe("CL-08 Ingwannu regressions", () => {
  test("same-root successor survives predecessor runtime release", async () => {
    const home = tempHome();
    let firstLoads = 0;
    let secondLoads = 0;

    const releaseFirst = setLabAutomationDispatchDeps({
      configDir: home,
      loadConfig: () => {
        firstLoads += 1;
        return emptyConfig();
      },
    }) as unknown;
    expect(typeof releaseFirst).toBe("function");
    if (typeof releaseFirst !== "function") return;

    startLabAutomationScheduler(home);
    expect(isLabAutomationSchedulerRunning(home)).toBe(true);

    const releaseSecond = setLabAutomationDispatchDeps({
      configDir: home,
      loadConfig: () => {
        secondLoads += 1;
        return emptyConfig();
      },
    }) as unknown;
    expect(typeof releaseSecond).toBe("function");
    if (typeof releaseSecond !== "function") return;

    startLabAutomationScheduler(home);
    releaseFirst();
    expect(isLabAutomationSchedulerRunning(home)).toBe(true);

    await runLabAutomationTick(home);
    expect(firstLoads).toBe(0);
    expect(secondLoads).toBeGreaterThan(0);

    releaseSecond();
    expect(isLabAutomationSchedulerRunning(home)).toBe(false);
  });

  test("failed combined automation config commit leaves the prior generation effective", async () => {
    const home = tempHome();
    const saveConfig = Reflect.get(persistence, "saveLabAutomationConfig") as unknown;
    const loadConfig = Reflect.get(persistence, "loadLabAutomationConfig") as unknown;
    const setCommitFault = Reflect.get(persistence, "setLabAutomationConfigCommitFaultForTests") as unknown;

    expect(typeof saveConfig).toBe("function");
    expect(typeof loadConfig).toBe("function");
    expect(typeof setCommitFault).toBe("function");
    if (typeof saveConfig !== "function" || typeof loadConfig !== "function" || typeof setCommitFault !== "function") return;

    const initialPolicy = {
      ...defaultLabAutomationPolicyV1(),
      enabled: true,
      layers: {
        protocolConformance: false,
        liveRouteCompatibility: true,
        taskEffectiveness: false,
      },
    };
    const initialRoutes = persistence.defaultLabAutomationRoutesV1();
    saveConfig(initialPolicy, initialRoutes, home);

    const nextPolicy = {
      ...initialPolicy,
      failureCooldownMs: initialPolicy.failureCooldownMs + 1,
    };
    const nextRoutes = {
      schemaVersion: 1 as const,
      routes: [{ providerName: "provider-new", modelId: "model-new" }],
    };

    setCommitFault("before_publish");
    expect(() => saveConfig(nextPolicy, nextRoutes, home)).toThrow();
    setCommitFault(null);

    const effective = loadConfig(home) as {
      policy: typeof initialPolicy;
      routes: typeof initialRoutes;
    };
    expect(effective.policy).toEqual(initialPolicy);
    expect(effective.routes).toEqual(initialRoutes);

    await runLabAutomationTick(home);
    expect(persistence.loadLabAutomationState(home).runs).toHaveLength(0);
  });

  test("CL-08 plan has no trailing whitespace", () => {
    const text = readFileSync(
      join(import.meta.dir, "..", "devlog", "_plan", "260807_compatibility_lab", "008_cl08_automation.md"),
      "utf8",
    );
    const offenders = text
      .split("\n")
      .map((line, index) => ({ line, lineNumber: index + 1 }))
      .filter(({ line }) => /[ \t]+$/.test(line));
    expect(offenders).toEqual([]);
  });
});
