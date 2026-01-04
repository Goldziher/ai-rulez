package presets

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

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
func getAllDomainRules(content *config.ContentTreeV3) []config.ContentFile {
	var rules []config.ContentFile
	for _, domain := range content.Domains {
		rules = append(rules, domain.Rules...)
	}
	return rules
}

// getAllDomainContext extracts all context from all domains
func getAllDomainContext(content *config.ContentTreeV3) []config.ContentFile {
	var context []config.ContentFile
	for _, domain := range content.Domains {
		context = append(context, domain.Context...)
	}
	return context
}

// getAllDomainSkills extracts all skills from all domains
func getAllDomainSkills(content *config.ContentTreeV3) []config.ContentFile {
	var skills []config.ContentFile
	for _, domain := range content.Domains {
		skills = append(skills, domain.Skills...)
	}
	return skills
}

// getAllDomainAgents extracts all agents from all domains
func getAllDomainAgents(content *config.ContentTreeV3) []config.ContentFile {
	var agents []config.ContentFile
	for _, domain := range content.Domains {
		agents = append(agents, domain.Agents...)
	}
	return agents
}

// getAllDomainCommands extracts all commands from all domains
func getAllDomainCommands(content *config.ContentTreeV3) []config.ContentFile {
	var commands []config.ContentFile
	for _, domain := range content.Domains {
		commands = append(commands, domain.Commands...)
	}
	return commands
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
