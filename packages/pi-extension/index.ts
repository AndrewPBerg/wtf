import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { isToolCallEventType } from "@earendil-works/pi-coding-agent";

const BYPASS = /\bWTF_OK=1\b|#\s*wtf-ok\b/i;
const RAW_CHECKOUT_ADD = [/\bgit\s+(?:-[^\n\s]+\s+)*worktree\s+add\b/, /\bjj\s+(?:-[^\n\s]+\s+)*workspace\s+add\b/];

export default function (pi: ExtensionAPI) {
  pi.on("tool_call", async (event) => {
    if (!isToolCallEventType("bash", event)) return undefined;

    const command = String(event.input.command ?? "");
    if (BYPASS.test(command)) return undefined;

    if (RAW_CHECKOUT_ADD.some((pattern) => pattern.test(command))) {
      return {
        block: true,
        reason:
          "Use `wtf new <name> --copy-env --no-serve` for agent checkouts. In a colocated repo, add `--vcs jj` when jj is intended. Add `WTF_OK=1` only when raw VCS behavior is required.",
      };
    }

    return undefined;
  });
}
