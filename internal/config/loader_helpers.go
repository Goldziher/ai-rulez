package config

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/oops"
)

func setDefaultPriorities(config *Config) {
	setRulesPriorities(config.Rules)
	setSectionsPriorities(config.Sections)
	setAgentsPriorities(config.Agents)
}

func setRulesPriorities(rules []Rule) {
	for i := range rules {
		if rules[i].Priority == "" {
			rules[i].Priority = PriorityMedium
		}
	}
}

func setSectionsPriorities(sections []Section) {
	for i := range sections {
		if sections[i].Priority == "" {
			sections[i].Priority = PriorityMedium
		}
	}
}

func setAgentsPriorities(agents []Agent) {
	for i := range agents {
		if agents[i].Priority == "" {
			agents[i].Priority = PriorityMedium
		}
	}
}

func (l *configLoader) collectIncludedContent(ctx context.Context, config *Config, baseDir string) ([]Rule, []Section, []Agent, error) {
	allRules := make([]Rule, len(config.Rules))
	copy(allRules, config.Rules)

	allSections := make([]Section, len(config.Sections))
	copy(allSections, config.Sections)

	allAgents := make([]Agent, len(config.Agents))
	copy(allAgents, config.Agents)

	for _, includePath := range config.Includes {
		rules, sections, agents, err := l.processInclude(ctx, includePath, baseDir)
		if err != nil {
			return nil, nil, nil, err
		}

		allRules = mergeRules(allRules, rules)
		allSections = mergeSections(allSections, sections)
		allAgents = mergeAgents(allAgents, agents)
	}

	return allRules, allSections, allAgents, nil
}

func (l *configLoader) processInclude(ctx context.Context, includePath, baseDir string) ([]Rule, []Section, []Agent, error) {
	resolvedPath := l.resolvePath(includePath, baseDir)

	if !l.isURL(resolvedPath) {
		if err := l.validateLocalInclude(resolvedPath, includePath, baseDir); err != nil {
			return nil, nil, nil, err
		}
	}

	includedConfig, err := l.loadConfigInternal(ctx, resolvedPath, true)
	if err != nil {
		return nil, nil, nil, err
	}

	return includedConfig.Rules, includedConfig.Sections, includedConfig.Agents, nil
}

func (l *configLoader) validateLocalInclude(resolvedPath, includePath, baseDir string) error {
	if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
		return oops.
			With("path", resolvedPath).
			With("include_path", includePath).
			With("resolved_path", resolvedPath).
			Hint(fmt.Sprintf("Check if the include file exists: %s\nVerify the relative path is correct relative to %s\nUse an absolute path if the relative path is unclear", resolvedPath, baseDir)).
			Errorf("include file not found: %s", includePath)
	}
	return nil
}
