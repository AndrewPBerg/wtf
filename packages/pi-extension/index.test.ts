import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import wtfWorktrees from "./index";
import { createMockPi } from "./test/mocks/pi-coding-agent";

describe("wtf-worktrees", () => {
  it("enforces checkout policy without adding an always-on prompt hook", () => {
    const pi = createMockPi();
    wtfWorktrees(pi);

    expect(pi.events.has("before_agent_start")).toBe(false);
  });

  it.each(["git worktree add ../trial", "jj workspace add ../trial"])("blocks raw checkout creation: %s", async (command) => {
    const pi = createMockPi();
    wtfWorktrees(pi);

    const result = await pi.events.get("tool_call")?.[0]?.({
      toolName: "bash",
      input: { command },
    });

    expect(result?.block).toBe(true);
    expect(result?.reason).toContain("wtf new");
  });

  it("allows WTF to choose the backend", async () => {
    const pi = createMockPi();
    wtfWorktrees(pi);

    const result = await pi.events.get("tool_call")?.[0]?.({
      toolName: "bash",
      input: { command: "wtf new trial --vcs jj --copy-env --no-serve" },
    });

    expect(result).toBeUndefined();
  });

  it("is a thin WTF policy adapter", () => {
    const source = readFileSync(fileURLToPath(new URL("./index.ts", import.meta.url)), "utf8");

    expect(source).toContain("wtf new");
    expect(source).not.toMatch(/agent[ _-]?bridge|workunit/i);
    expect(source).not.toMatch(/workspace (?:list|remove|forget)|worktree (?:list|remove)/i);
  });

  it("does not keyword-block dotenv usage", async () => {
    const pi = createMockPi();
    wtfWorktrees(pi);

    const result = await pi.events.get("tool_call")?.[0]?.({
      toolName: "bash",
      input: { command: 'python -c "from dotenv import load_dotenv; load_dotenv()"' },
    });

    expect(result).toBeUndefined();
  });
});
