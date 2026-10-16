package judge

import "embed"

// assets holds the role prompts, message templates and output schemas. They are text files so that
// they can be read, diffed and reviewed as prose; changing one changes the PromptVersion.
//
//go:embed prompts schemas
var assets embed.FS
