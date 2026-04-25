package presets

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
)

const agentDelegationBuiltin = "agent-delegation"

// configFileName returns the actual config filename from the Config,
// falling back to "config.toml" (V4 default) if not set.
func configFileName(cfg *config.Config) string {
	if cfg.ConfigFile != "" {
		return cfg.ConfigFile
	}
	return "config.toml"
}

// combineContentFiles combines multiple ContentFile slices
func combineContentFiles(slices ...[]config.ContentFile) []config.ContentFile {
	var total int
	for _, slice := range slices {
		total += len(slice)
	}

	result := make([]config.ContentFile, 0, total)
	for _, slice := range slices {
		result = append(result, slice...)
	}
	return result
}

// getAllDomainRules extracts all rules from all domains
func getAllDomainRules(content *config.ContentTree) []config.ContentFile {
	var rules []config.ContentFile
	for _, domain := range content.Domains {
		rules = append(rules, domain.Rules...)
	}
	return rules
}

// getAllDomainContext extracts all context from all domains,
// excluding the agent-delegation builtin (rendered separately in the Agents section).
func getAllDomainContext(content *config.ContentTree) []config.ContentFile {
	var context []config.ContentFile
	for name, domain := range content.Domains {
		if name == agentDelegationBuiltin {
			continue
		}
		context = append(context, domain.Context...)
	}
	return context
}

// getAllDomainSkills extracts all skills from all domains
func getAllDomainSkills(content *config.ContentTree) []config.ContentFile {
	var skills []config.ContentFile
	for _, domain := range content.Domains {
		skills = append(skills, domain.Skills...)
	}
	return skills
}

// getAllDomainAgents extracts all agents from all domains
func getAllDomainAgents(content *config.ContentTree) []config.ContentFile {
	var agents []config.ContentFile
	for _, domain := range content.Domains {
		agents = append(agents, domain.Agents...)
	}
	return agents
}

// getAllDomainCommands extracts all commands from all domains
func getAllDomainCommands(content *config.ContentTree) []config.ContentFile {
	var commands []config.ContentFile
	for _, domain := range content.Domains {
		commands = append(commands, domain.Commands...)
	}
	return commands
}

// renderAgentsSection writes an "## Agents" section listing all available
// subagents with delegation and parallelization instructions.
// The section is only rendered when the agent-delegation builtin is enabled
// and at least one agent exists.
func renderAgentsSection(builder *strings.Builder, content *config.ContentTree, agents []config.ContentFile) {
	delegationDomain, ok := content.Domains[agentDelegationBuiltin]
	if !ok || len(agents) == 0 {
		return
	}

	builder.WriteString("## Agents\n\n")

	// Render instruction text from the builtin's context files
	for _, ctx := range delegationDomain.Context {
		processedContent := markdown.ProcessEmbeddedContent(ctx.Content)
		builder.WriteString(processedContent)
		builder.WriteString("\n\n")
	}

	for _, agent := range agents {
		description := ""
		if agent.Metadata != nil {
			description = agent.Metadata.Extra["description"]
		}
		builder.WriteString("- **")
		builder.WriteString(agent.Name)
		builder.WriteString("**")
		if description != "" {
			builder.WriteString(": ")
			builder.WriteString(description)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

// filterContentByExplicitTargets returns only content files that have explicit
// targets matching the given output path. Content with no targets is excluded,
// since untargeted content is already present in the main output file (e.g. CLAUDE.md).
func filterContentByExplicitTargets(files []config.ContentFile, outputPath, baseDir string) []config.ContentFile {
	var result []config.ContentFile
	for _, f := range files {
		if f.Metadata != nil && len(f.Metadata.Targets) > 0 {
			if shouldIncludeInOutput(f.Metadata.Targets, outputPath, baseDir) {
				result = append(result, f)
			}
		}
	}
	return result
}

// sanitizeName removes special characters from names for use in filenames
func sanitizeName(name string) string {
	// Replace spaces and special chars with dashes
	replacer := strings.NewReplacer(
		" ", "-",
		"_", "-",
		"/", "-",
		"\\", "-",
	)
	sanitized := replacer.Replace(name)
	// Remove any remaining non-alphanumeric chars except dashes
	var builder strings.Builder
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "-")
}
