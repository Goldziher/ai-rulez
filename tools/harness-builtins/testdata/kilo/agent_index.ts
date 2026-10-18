// Excerpt of https://github.com/Kilo-Org/kilocode/blob/v7.8.14/packages/opencode/src/kilocode/agent/index.ts (lines 485-485, 515-528, 584-588, 604-608, 642-646)
export function patchAgents(
// ...
  // Rename "build" → "code" for backward compatibility
  if (agents.build) {
    agents.code = {
      ...agents.build,
      name: "code",
      permission: Permission.merge(
        defaults,
        agents.build.permission,
        user,
        Permission.fromConfig({ semantic_search: "allow" }),
      ),
    }
    delete agents.build
  }
// ...
  // Add debug agent
  agents.debug = {
    name: "debug",
    description: "Diagnose and fix software issues with systematic debugging methodology.",
    prompt: PROMPT_DEBUG,
// ...
  // Add orchestrator agent
  agents.orchestrator = {
    name: "orchestrator",
    description: "Coordinate complex tasks by delegating to specialized agents in parallel.",
    prompt: PROMPT_ORCHESTRATOR,
// ...
  // Add ask agent
  const guard = askGuard(kilo.mcpRules, enabled)
  agents.ask = {
    name: "ask",
    description: "Get answers and explanations without making changes to the codebase.",
