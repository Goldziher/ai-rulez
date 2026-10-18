// Excerpt of https://github.com/cline/cline/blob/v4.1.25/apps/vscode/webview-ui/src/components/chat/chat-view/hooks/useMessageHandlers.ts
// `/compact` and its aliases `/smol` and `/newtask` run a real SDK manual
// compaction via the condense RPC. Sending the literal text to the model would
// make it improvise a fake summary instead of compacting the context window
// (CLINE-2503). `/newtask` aliases compaction because condensing achieves its
// goal (continue working with a fresh, summarized context) without the legacy
// new_task tool.
function isCompactionCommand(text: string): boolean {
  return text === "/compact" || text === "/smol" || text === "/newtask";
}
