import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { installShellHook, uninstallShellHook } from "../src/server/system-env";

// The shell hook writes into the user's own ~/.zshrc, so these tests point HOME
// at a throwaway directory. Without that they would edit the developer's real
// profile -- which is the very class of accident this feature keeps causing.
let home = "";
let originalHome: string | undefined;

beforeEach(() => {
  originalHome = process.env.HOME;
  home = mkdtempSync(join(tmpdir(), "ocx-shell-hook-"));
  process.env.HOME = home;
});

afterEach(() => {
  if (originalHome === undefined) delete process.env.HOME;
  else process.env.HOME = originalHome;
  rmSync(home, { recursive: true, force: true });
});

const zshrc = () => join(home, ".zshrc");
const readProfile = () => {
  try { return readFileSync(zshrc(), "utf8"); } catch { return ""; }
};

describe.skipIf(process.platform !== "darwin")("shell hook opt-in", () => {
  test("installing writes exactly one hook line", () => {
    writeFileSync(zshrc(), "# user content\n");

    const result = installShellHook();

    expect(result.installed).toBe(true);
    const profile = readProfile();
    expect(profile).toContain("# user content");
    expect(profile).toContain("claude-env.sh");
  });

  // Startup runs on every launch, so a non-idempotent hook would grow the
  // user's profile without bound.
  test("installing twice does not duplicate the hook", () => {
    writeFileSync(zshrc(), "# user content\n");

    installShellHook();
    const second = installShellHook();
    // Count the marker comment, not the sourced path: the hook line mentions
    // the path twice (guard plus source), so counting the path would misreport.
    const occurrences = readProfile().split("# opencodex claude-env hook").length - 1;

    expect(second.installed).toBe(false);
    expect(occurrences).toBe(1);
  });

  // Opting out has to leave the profile as it was found. A hook that survives
  // the setting is routing the user never authorized.
  test("uninstalling removes the hook and preserves user content", () => {
    writeFileSync(zshrc(), "# user content\n");
    installShellHook();

    const result = uninstallShellHook();

    expect(result.removed).toBe(true);
    const profile = readProfile();
    expect(profile).toContain("# user content");
    expect(profile).not.toContain("claude-env.sh");
  });

  test("uninstalling a profile that never had the hook is a no-op", () => {
    writeFileSync(zshrc(), "# user content\n");

    uninstallShellHook();

    expect(readProfile()).toBe("# user content\n");
  });
});
