import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { systemdInstallCleanupStatus } from "../src/service";

const source = readFileSync(join(import.meta.dir, "../src/service.ts"), "utf8");

describe("systemd install cleanup status hardening", () => {
  test("only treats literal not-found as confirmed unit absence", () => {
    expect(systemdInstallCleanupStatus({ show: () => "LoadState=not-found\n" })).toBeNull();
    expect(systemdInstallCleanupStatus({ show: () => "LoadState=loaded\n" })).toBe("loaded");
    expect(source).toContain('return loadState === "not-found" ? null : loadState;');
    expect(source).not.toContain('return !loadState || loadState === "not-found" ? null : loadState;');
  });

  test("uses the key/value output supported by systemd 219", () => {
    const helper = source.slice(
      source.indexOf("export function systemdInstallCleanupStatus"),
      source.indexOf("function platformOps"),
    );

    expect(helper).toContain("systemctl --user show ${TASK} -p LoadState");
    expect(helper).not.toContain("--value");
  });

  test("fails closed on missing, empty, or legacy bare-value output", () => {
    for (const output of ["", "LoadState=\n", "not-found\n", "ActiveState=inactive\n"]) {
      expect(() => systemdInstallCleanupStatus({ show: () => output }))
        .toThrow("systemd service status could not be verified");
    }
  });
});
