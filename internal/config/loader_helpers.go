package config

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/oops"
)

// setDefaultPriorities ensures all items have a priority set
func setDefaultPriorities(config *Config) {
	setRulesPriorities(config.Rules)
	setSectionsPriorities(config.Sections)
	setAgentsPriorities(config.Agents)

	if config.UserRulez != nil {
		setRulesPriorities(config.UserRulez.Rules)
		setSectionsPriorities(config.UserRulez.Sections)
		setAgentsPriorities(config.UserRulez.Agents)
	}
}

func setRulesPriorities(rules []Rule) {
	for i := range rules {
		if rules[i].Priority == 0 {
			rules[i].Priority = 1
		}
	}
}

func setSectionsPriorities(sections []Section) {
	for i := range sections {
		if sections[i].Priority == 0 {
			sections[i].Priority = 1
		}
	}
}

func setAgentsPriorities(agents []Agent) {
	for i := range agents {
		if agents[i].Priority == 0 {
			agents[i].Priority = 1
		}
	}
}

// collectIncludedContent gathers all rules, sections, and agents from includes
func (l *configLoader) collectIncludedContent(ctx context.Context, config *Config, baseDir string) ([]Rule, []Section, []Agent, error) {
	var allRules []Rule
	var allSections []Section
	var allAgents []Agent

	allRules = append(allRules, config.Rules...)
	allSections = append(allSections, config.Sections...)
	allAgents = append(allAgents, config.Agents...)

	for _, includePath := range config.Includes {
		rules, sections, agents, err := l.processInclude(ctx, includePath, baseDir)
		if err != nil {
			return nil, nil, nil, err
		}

		allRules = append(allRules, rules...)
		allSections = append(allSections, sections...)
		allAgents = append(allAgents, agents...)
	}

	return allRules, allSections, allAgents, nil
}

// processInclude loads a single include file and returns its content
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

// validateLocalInclude checks if a local include file exists
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
