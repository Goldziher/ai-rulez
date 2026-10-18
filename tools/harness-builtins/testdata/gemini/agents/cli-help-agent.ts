// Excerpt of https://github.com/google-gemini/gemini-cli/blob/v0.64.0/packages/core/src/agents/cli-help-agent.ts (lines 26-30)
export const CliHelpAgent = (
  context: AgentLoopContext,
): AgentDefinition<typeof CliHelpReportSchema> => ({
  name: 'cli_help',
  kind: 'local',
