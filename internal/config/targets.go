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

	for _, target := range targets {
		// Clean the target pattern
		cleanTarget := strings.TrimSpace(target)
		if cleanTarget == "" {
			continue
		}

		// Try exact match first
		if cleanTarget == normalizedPath {
			return true
		}

		// Try glob pattern match
		matched, err := filepath.Match(cleanTarget, normalizedPath)
		if err == nil && matched {
			return true
		}

		// Try matching with the base name only (for patterns like *.md)
		baseName := filepath.Base(normalizedPath)
		matched, err = filepath.Match(cleanTarget, baseName)
		if err == nil && matched {
			return true
		}

		// Try matching directory patterns (e.g., .claude/*)
		if strings.Contains(cleanTarget, "/") {
			// For directory-based patterns, check if the path matches
			matched, err = filepath.Match(cleanTarget, normalizedPath)
			if err == nil && matched {
				return true
			}
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
