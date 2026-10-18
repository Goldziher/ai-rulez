// Excerpt of https://github.com/google-gemini/gemini-cli/blob/v0.64.0/packages/core/src/agents/codebase-investigator.ts (lines 51-56, 66-67)
export const CodebaseInvestigatorAgent = (
  config: Config,
): LocalAgentDefinition<typeof CodebaseInvestigationReportSchema> => {
  // Use Preview Flash model if the main model supports modern features.
  // If the main model is not a modern model, use the default pro model.
  const model = supportsModernFeatures(config.getModel())
// ...
    name: 'codebase_investigator',
    kind: 'local',
