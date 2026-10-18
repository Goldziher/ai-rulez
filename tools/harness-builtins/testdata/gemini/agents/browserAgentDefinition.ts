// Excerpt of https://github.com/google-gemini/gemini-cli/blob/v0.64.0/packages/core/src/agents/browser/browserAgentDefinition.ts (lines 27-27, 132-145)
export const BROWSER_AGENT_NAME = 'browser_agent';
// ...
export const BrowserAgentDefinition = (
  config: Config,
  visionEnabled = false,
): LocalAgentDefinition<typeof BrowserTaskResultSchema> => {
  // Use Preview Flash model if the main model is any of the preview models.
  // If the main model is not a preview model, use the default flash model.
  const model = isPreviewModel(config.getModel(), config)
    ? PREVIEW_GEMINI_FLASH_MODEL
    : DEFAULT_GEMINI_FLASH_MODEL;

  return {
    name: BROWSER_AGENT_NAME,
    kind: 'local',
    experimental: true,
