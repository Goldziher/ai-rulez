package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

func ResolveTargets(targets []string, namedTargets map[string][]string) ([]string, error) {
	if len(targets) == 0 {
		return targets, nil
	}

	resolved := make([]string, 0, len(targets))

	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		if strings.HasPrefix(target, "@") {
			targetName := target[1:]
			if targetName == "" {
				return nil, oops.
					Hint("Named target references should be in the format '@target-name'").
					Errorf("empty target name after @")
			}

			patterns, exists := namedTargets[targetName]
			if !exists {
				return nil, oops.
					Hint(fmt.Sprintf("Define the named target '%s' in the targets section\nAvailable named targets: %s", targetName, getAvailableTargetNames(namedTargets))).
					Errorf("named target '%s' not found", targetName)
			}

			resolved = append(resolved, patterns...)
		} else {
			resolved = append(resolved, target)
		}
	}

	return resolved, nil
}

func getAvailableTargetNames(namedTargets map[string][]string) string {
	if len(namedTargets) == 0 {
		return "none defined"
	}

	names := make([]string, 0, len(namedTargets))
	for name := range namedTargets {
		names = append(names, "@"+name)
	}
	return strings.Join(names, ", ")
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
		resolvedTargets, err := ResolveTargets(rule.Targets, namedTargets)
		if err != nil {
			return nil, fmt.Errorf("error resolving targets for rule '%s': %w", rule.Name, err)
		}

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
		resolvedTargets, err := ResolveTargets(section.Targets, namedTargets)
		if err != nil {
			return nil, fmt.Errorf("error resolving targets for section '%s': %w", section.Name, err)
		}

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
		resolvedTargets, err := ResolveTargets(agents[i].Targets, namedTargets)
		if err != nil {
			return nil, fmt.Errorf("error resolving targets for agent '%s': %w", agents[i].Name, err)
		}

		if MatchesTarget(outputPath, resolvedTargets) {
			filtered = append(filtered, agents[i])
		}
	}

	return filtered, nil
}
