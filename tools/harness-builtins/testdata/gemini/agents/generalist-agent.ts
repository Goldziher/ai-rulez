// Excerpt of https://github.com/google-gemini/gemini-cli/blob/v0.64.0/packages/core/src/agents/generalist-agent.ts (lines 22-27)
export const GeneralistAgent = (
  context: AgentLoopContext,
): LocalAgentDefinition<typeof GeneralistAgentSchema> => ({
  kind: 'local',
  name: 'generalist',
  displayName: 'Generalist Agent',
