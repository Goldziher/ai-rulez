// Excerpt of https://github.com/QwenLM/qwen-code/blob/v0.25.2/packages/core/src/subagents/builtin-agents.ts (lines 17-17, 40-40, 46-52, 70-72, 126-129, 278-282, 383-404, 405-406)
export const DEFAULT_BUILTIN_SUBAGENT_TYPE = 'general-purpose';
// ...
export const REVIEW_BUILTIN_SUBAGENT_TYPE = 'review-agent';
// ...
export class BuiltinAgentRegistry {
  private static readonly BUILTIN_AGENTS: Array<
    Omit<SubagentConfig, 'level' | 'filePath'>
  > = [
    {
      name: DEFAULT_BUILTIN_SUBAGENT_TYPE,
      description:
// ...
    {
      name: 'Explore',
      description:
// ...
    },
    {
      name: 'statusline-setup',
      description:
// ...
    },
    {
      name: REVIEW_BUILTIN_SUBAGENT_TYPE,
      // Kept to one clause on purpose: `AgentTool.updateDescriptionAndSchema`
      // splices every registered type's description into the Agent tool's own
// ...
    {
      name: 'claude-code',
      description:
        'Delegate to Claude Code through the installed claude-agent-acp adapter, using its own authentication and model settings. Foreground by default.',
      systemPrompt:
        'Complete the delegated task and report the verified result.',
      background: false,
      executor: { kind: 'acp', command: 'claude-agent-acp' },
    },
    {
      name: 'codex',
      description:
        'Delegate one self-contained task to the installed Codex CLI using its own authentication and model settings. Foreground by default; optional background execution, no messages or resume.',
      systemPrompt:
        'Complete the delegated task and report the verified result.',
      background: false,
      executor: { kind: 'codex', command: 'codex' },
    },
  ];

  /**
   * Gets all built-in agent configurations.
// ...
   * @returns Array of built-in subagent configurations
   */
