package config

import (
	"path/filepath"
	"slices"
	"strings"
)

type Targeted interface {
	Rule | Section | Agent | MCPServer | Command
}

func getTargets[T Targeted](item T) []string {
	switch v := any(item).(type) {
	case Rule:
		return v.Targets
	case Section:
		return v.Targets
	case Agent:
		return v.Targets
	case MCPServer:
		return v.Targets
	case Command:
		return v.Targets
	}
	return nil
}

func Filter[T Targeted](items []T, outputPath string, namedTargets map[string][]string) ([]T, error) {
	if len(items) == 0 {
		return items, nil
	}

	filtered := slices.DeleteFunc(slices.Clone(items), func(item T) bool {
		resolvedTargets := resolveTargets(getTargets(item), namedTargets)
		return !MatchesTarget(outputPath, resolvedTargets)
	})

	return filtered, nil
}

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
			namedTarget := cleanTarget[1:]
			if patterns, exists := namedTargets[namedTarget]; exists {
				resolved = append(resolved, patterns...)
			}
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

		cleanTarget = filepath.FromSlash(cleanTarget)

		if matchesExact(cleanTarget, normalizedPath) {
			return true
		}

		if matchesGlob(cleanTarget, normalizedPath) {
			return true
		}

		pathSep := string(filepath.Separator)
		if !strings.Contains(cleanTarget, pathSep) && strings.HasPrefix(cleanTarget, "*.") {
			if !strings.Contains(normalizedPath, pathSep) && matchesBaseName(cleanTarget, baseName) {
				return true
			}
		} else if matchesBaseName(cleanTarget, baseName) {
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
	pathSep := string(filepath.Separator)

	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) > 0 {
			prefix := strings.TrimSuffix(parts[0], pathSep)
			if prefix != "" && (dirPath == prefix || strings.HasPrefix(dirPath, prefix+pathSep)) {
				return true
			}
		}
	}

	if strings.Contains(pattern, "*") && !strings.Contains(pattern, "**") {
		dir := filepath.Dir(pattern)
		if dir != "." && (dirPath == dir || strings.HasPrefix(dirPath, dir+pathSep)) {
			return true
		}
	}

	if strings.Contains(pattern, pathSep) {
		matched, err := filepath.Match(pattern, dirPath)
		if err == nil && matched {
			return true
		}
	}

	return false
}

func FilterRules(rules []Rule, outputPath string, namedTargets map[string][]string) ([]Rule, error) {
	return Filter(rules, outputPath, namedTargets)
}

func FilterSections(sections []Section, outputPath string, namedTargets map[string][]string) ([]Section, error) {
	return Filter(sections, outputPath, namedTargets)
}

func FilterAgents(agents []Agent, outputPath string, namedTargets map[string][]string) ([]Agent, error) {
	return Filter(agents, outputPath, namedTargets)
}

func FilterMCPServers(mcpServers []MCPServer, outputPath string, namedTargets map[string][]string) ([]MCPServer, error) {
	return Filter(mcpServers, outputPath, namedTargets)
}

func FilterCommands(commands []Command, outputPath string, namedTargets map[string][]string) ([]Command, error) {
	return Filter(commands, outputPath, namedTargets)
}
