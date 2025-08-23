package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/errors"
)

// ResolveTargets resolves target patterns, expanding named target references (@target-name)
// into their actual patterns. Returns an error if a named target is not found.
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

		// Check if this is a named target reference
		if strings.HasPrefix(target, "@") {
			targetName := target[1:] // Remove @ prefix
			if targetName == "" {
				return nil, errors.New(errors.ErrorTypeConfigInvalid, "resolve named target", fmt.Errorf("empty target name after @")).
					WithSuggestion("Named target references should be in the format '@target-name'")
			}

			patterns, exists := namedTargets[targetName]
			if !exists {
				return nil, errors.New(errors.ErrorTypeConfigInvalid, "resolve named target", fmt.Errorf("named target '%s' not found", targetName)).
					WithSuggestion(fmt.Sprintf("Define the named target '%s' in the targets section", targetName)).
					WithSuggestion("Available named targets: " + getAvailableTargetNames(namedTargets))
			}

			// Add all patterns from the named target
			resolved = append(resolved, patterns...)
		} else {
			// Regular inline target pattern
			resolved = append(resolved, target)
		}
	}

	return resolved, nil
}

// getAvailableTargetNames returns a comma-separated list of available target names for error messages
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

// MatchesTarget checks if an output path matches any of the given target patterns.
// If targets is empty or nil, it returns true (applies to all outputs).
// Supports glob patterns like *.md, .claude/*, CLAUDE.md, etc.
func MatchesTarget(outputPath string, targets []string) bool {
	if len(targets) == 0 {
		return true // No targets means applies to all outputs
	}

	// Normalize the output path for comparison
	normalizedPath := filepath.Clean(outputPath)
	baseName := filepath.Base(normalizedPath)
	isDirectory := !strings.Contains(baseName, ".") || strings.HasSuffix(normalizedPath, "/")

	for _, target := range targets {
		// Clean the target pattern
		cleanTarget := strings.TrimSpace(target)
		if cleanTarget == "" {
			continue
		}

		// Check various matching strategies
		if matchesExact(cleanTarget, normalizedPath) {
			return true
		}

		if matchesGlob(cleanTarget, normalizedPath) {
			return true
		}

		if matchesBaseName(cleanTarget, baseName) {
			return true
		}

		// Special handling for directory outputs
		if isDirectory && matchesDirectoryPatterns(cleanTarget, normalizedPath) {
			return true
		}
	}

	return false
}

// matchesExact checks for exact string match
func matchesExact(pattern, path string) bool {
	return pattern == path
}

// matchesGlob checks if path matches the glob pattern
func matchesGlob(pattern, path string) bool {
	matched, err := filepath.Match(pattern, path)
	return err == nil && matched
}

// matchesBaseName checks if the base name matches the pattern
func matchesBaseName(pattern, baseName string) bool {
	matched, err := filepath.Match(pattern, baseName)
	return err == nil && matched
}

// matchesDirectoryPatterns handles special matching for directory outputs
func matchesDirectoryPatterns(pattern, dirPath string) bool {
	// For patterns like .claude/**/*.md, check if directory is under .claude/
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) > 0 {
			prefix := strings.TrimSuffix(parts[0], "/")
			if prefix != "" && (dirPath == prefix || strings.HasPrefix(dirPath, prefix+"/")) {
				return true
			}
		}
	}

	// For patterns like path/*.ext, check if directory is under the path part
	if strings.Contains(pattern, "*") && !strings.Contains(pattern, "**") {
		dir := filepath.Dir(pattern)
		if dir != "." && strings.HasPrefix(dirPath, dir+"/") {
			return true
		}
	}

	// Try matching directory patterns (e.g., .claude/*)
	if strings.Contains(pattern, "/") {
		matched, err := filepath.Match(pattern, dirPath)
		if err == nil && matched {
			return true
		}
	}

	return false
}

// FilterRules filters rules based on target patterns for the given output path.
// Resolves named target references before matching.
func FilterRules(rules []Rule, outputPath string, namedTargets map[string][]string) ([]Rule, error) {
	if len(rules) == 0 {
		return rules, nil
	}

	filtered := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		// Resolve target patterns
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

// FilterSections filters sections based on target patterns for the given output path.
// Resolves named target references before matching.
func FilterSections(sections []Section, outputPath string, namedTargets map[string][]string) ([]Section, error) {
	if len(sections) == 0 {
		return sections, nil
	}

	filtered := make([]Section, 0, len(sections))
	for _, section := range sections {
		// Resolve target patterns
		resolvedTargets, err := ResolveTargets(section.Targets, namedTargets)
		if err != nil {
			return nil, fmt.Errorf("error resolving targets for section '%s': %w", section.Title, err)
		}

		if MatchesTarget(outputPath, resolvedTargets) {
			filtered = append(filtered, section)
		}
	}

	return filtered, nil
}

// FilterAgents filters agents based on target patterns for the given output path.
// Resolves named target references before matching.
func FilterAgents(agents []Agent, outputPath string, namedTargets map[string][]string) ([]Agent, error) {
	if len(agents) == 0 {
		return agents, nil
	}

	filtered := make([]Agent, 0, len(agents))
	for i := range agents {
		// Resolve target patterns
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
