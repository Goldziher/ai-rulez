package config

import (
	"path/filepath"
	"strings"
)

// resolveTargets expands named targets (those starting with @) using the namedTargets map
func resolveTargets(targets []string, namedTargets map[string][]string) []string {
	if len(targets) == 0 {
		return targets
	}

	resolved := make([]string, 0, len(targets))
	for _, target := range targets {
		cleanTarget := strings.TrimSpace(target)
		if cleanTarget == "" {
			continue
		}

		if strings.HasPrefix(cleanTarget, "@") {
			namedTarget := cleanTarget[1:] // Remove the @ prefix
			if patterns, exists := namedTargets[namedTarget]; exists {
				resolved = append(resolved, patterns...)
			}
			// If named target doesn't exist, ignore it (could log a warning)
		} else {
			resolved = append(resolved, cleanTarget)
		}
	}

	return resolved
}

func MatchesTarget(outputPath string, targets []string) bool {
	if len(targets) == 0 {
		return true
	}

	normalizedPath := filepath.Clean(outputPath)
	baseName := filepath.Base(normalizedPath)
	isDirectory := !strings.Contains(baseName, ".") || strings.HasSuffix(normalizedPath, "/")

	for _, target := range targets {
		cleanTarget := strings.TrimSpace(target)
		if cleanTarget == "" {
			continue
		}

		if matchesExact(cleanTarget, normalizedPath) {
			return true
		}

		if matchesGlob(cleanTarget, normalizedPath) {
			return true
		}

		if matchesBaseName(cleanTarget, baseName) {
			return true
		}

		if isDirectory && matchesDirectoryPatterns(cleanTarget, normalizedPath) {
			return true
		}
	}

	return false
}

func matchesExact(pattern, path string) bool {
	return pattern == path
}

func matchesGlob(pattern, path string) bool {
	matched, err := filepath.Match(pattern, path)
	return err == nil && matched
}

func matchesBaseName(pattern, baseName string) bool {
	matched, err := filepath.Match(pattern, baseName)
	return err == nil && matched
}

func matchesDirectoryPatterns(pattern, dirPath string) bool {
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) > 0 {
			prefix := strings.TrimSuffix(parts[0], "/")
			if prefix != "" && (dirPath == prefix || strings.HasPrefix(dirPath, prefix+"/")) {
				return true
			}
		}
	}

	if strings.Contains(pattern, "*") && !strings.Contains(pattern, "**") {
		dir := filepath.Dir(pattern)
		if dir != "." && (dirPath == dir || strings.HasPrefix(dirPath, dir+"/")) {
			return true
		}
	}

	if strings.Contains(pattern, "/") {
		matched, err := filepath.Match(pattern, dirPath)
		if err == nil && matched {
			return true
		}
	}

	return false
}

func FilterRules(rules []Rule, outputPath string, namedTargets map[string][]string) ([]Rule, error) {
	if len(rules) == 0 {
		return rules, nil
	}

	filtered := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		resolvedTargets := resolveTargets(rule.Targets, namedTargets)
		if MatchesTarget(outputPath, resolvedTargets) {
			filtered = append(filtered, rule)
		}
	}

	return filtered, nil
}

func FilterSections(sections []Section, outputPath string, namedTargets map[string][]string) ([]Section, error) {
	if len(sections) == 0 {
		return sections, nil
	}

	filtered := make([]Section, 0, len(sections))
	for _, section := range sections {
		resolvedTargets := resolveTargets(section.Targets, namedTargets)
		if MatchesTarget(outputPath, resolvedTargets) {
			filtered = append(filtered, section)
		}
	}

	return filtered, nil
}

func FilterAgents(agents []Agent, outputPath string, namedTargets map[string][]string) ([]Agent, error) {
	if len(agents) == 0 {
		return agents, nil
	}

	filtered := make([]Agent, 0, len(agents))
	for i := range agents {
		resolvedTargets := resolveTargets(agents[i].Targets, namedTargets)
		if MatchesTarget(outputPath, resolvedTargets) {
			filtered = append(filtered, agents[i])
		}
	}

	return filtered, nil
}

func FilterMCPServers(mcpServers []MCPServer, outputPath string, namedTargets map[string][]string) ([]MCPServer, error) {
	if len(mcpServers) == 0 {
		return mcpServers, nil
	}

	filtered := make([]MCPServer, 0, len(mcpServers))
	for _, server := range mcpServers {
		resolvedTargets := resolveTargets(server.Targets, namedTargets)
		if MatchesTarget(outputPath, resolvedTargets) {
			filtered = append(filtered, server)
		}
	}

	return filtered, nil
}

func FilterCommands(commands []Command, outputPath string, namedTargets map[string][]string) ([]Command, error) {
	if len(commands) == 0 {
		return commands, nil
	}

	filtered := make([]Command, 0, len(commands))
	for _, cmd := range commands {
		resolvedTargets := resolveTargets(cmd.Targets, namedTargets)
		if MatchesTarget(outputPath, resolvedTargets) {
			filtered = append(filtered, cmd)
		}
	}

	return filtered, nil
}
