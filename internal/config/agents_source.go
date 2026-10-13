package config

import (
	"context"
	"path/filepath"

	"github.com/samber/oops"
)

// The "agents" source reads the project's knowledge from the open standards
// instead of from the .ai-rulez content tree: the root AGENTS.md, and the
// .agents directory (Agent Skills in .agents/skills/<name>/SKILL.md, plus the
// layout ai-rulez documents for what the standards lack). It is selected with
// `source = "agents"`, or implied by keeping the configuration itself in
// .agents/ai-rulez.toml. Output is never written back to what it reads from:
// AGENTS.md and everything under .agents/ are the source, so the generator
// leaves them alone.
const (
	// SourceAIRulez is the default source: the .ai-rulez content tree.
	SourceAIRulez = "ai-rulez"
	// SourceAgents reads AGENTS.md and .agents/ as the content tree.
	SourceAgents = "agents"

	// AgentsDirName is the standard directory the "agents" source reads.
	AgentsDirName = ".agents"
	// AgentsConfigFilename is the configuration file kept inside AgentsDirName.
	AgentsConfigFilename = "ai-rulez.toml"
	// AgentsMDFilename is the root instruction file the "agents" source reads.
	AgentsMDFilename = "AGENTS.md"
	// agentsStateDirName holds the manifest, lock and local state of a project
	// whose configuration lives in .agents/ai-rulez.toml.
	agentsStateDirName = ".ai-rulez"
)

// AgentsSourceReader completes the content tree the "agents" source scanned from
// .agents: it adds the root AGENTS.md as context and drops what an earlier
// generate run wrote there (a generated file is output, never source). The
// importer package supplies it (config cannot import it); see Resolvers.
type AgentsSourceReader func(ctx context.Context, cfg *Config, tree *ContentTree) (*ContentTree, error)

// UsesAgentsSource reports whether the project's content is read from AGENTS.md
// and .agents rather than from the .ai-rulez content tree.
func (c *Config) UsesAgentsSource() bool {
	return c != nil && (c.Source == SourceAgents || c.AgentsLayout)
}

// SourceDir is the directory the authored content tree (rules, context, skills,
// agents, commands, checks, domains) is read from: .agents for the "agents"
// source, the config directory otherwise.
func (c *Config) SourceDir() string {
	if c.UsesAgentsSource() {
		return filepath.Join(c.BaseDir, AgentsDirName)
	}
	return c.ConfigDir
}

// ConfigPath is the main configuration file. It is not always inside ConfigDir:
// .agents/ai-rulez.toml sits beside the .agents/.ai-rulez state directory that
// ConfigDir names.
func (c *Config) ConfigPath() string {
	if c.ConfigFilePath != "" {
		return c.ConfigFilePath
	}
	return filepath.Join(c.ConfigDir, c.ConfigFile)
}

// agentsLayoutBase returns the project directory when dir names the .agents
// directory or its .ai-rulez state directory, and false for any other dir.
func agentsLayoutBase(dir string) (base string, ok bool) {
	dir = filepath.Clean(dir)
	if filepath.Base(dir) == agentsStateDirName {
		parent := filepath.Dir(dir)
		if filepath.Base(parent) == AgentsDirName {
			return filepath.Dir(parent), true
		}
		return "", false
	}
	if filepath.Base(dir) == AgentsDirName {
		return filepath.Dir(dir), true
	}
	return "", false
}

// agentsDirOf returns the .agents directory when dir is .agents or its
// .ai-rulez state directory.
func agentsDirOf(dir string) string {
	dir = filepath.Clean(dir)
	if filepath.Base(dir) == agentsStateDirName {
		return filepath.Dir(dir)
	}
	return dir
}

// IsAgentsConfigDir reports whether dir is the .agents directory or its
// .ai-rulez state directory, the two spellings of the config directory of a
// project configured in .agents/ai-rulez.toml.
func IsAgentsConfigDir(dir string) bool {
	_, ok := agentsLayoutBase(dir)
	return ok
}

// configFileNameIn is the main config file name a config directory holds:
// ai-rulez.toml inside .agents, config.toml everywhere else.
func configFileNameIn(dir string) string {
	if filepath.Base(filepath.Clean(dir)) == AgentsDirName {
		return AgentsConfigFilename
	}
	return configTOMLFilename
}

// validateSource checks the source setting.
func (c *Config) validateSource() error {
	switch c.Source {
	case "", SourceAIRulez, SourceAgents:
	default:
		return oops.
			With("field", "source").
			Hint(`Use source = "ai-rulez" (the .ai-rulez content tree) or source = "agents" (AGENTS.md and .agents/).`).
			Errorf("unknown source %q", c.Source)
	}
	if c.AgentsLayout && c.Source == SourceAIRulez {
		return oops.
			With("field", "source").
			Hint(`A configuration kept in .agents/ai-rulez.toml reads its content from .agents; drop the setting or use source = "agents".`).
			Errorf("source = %q contradicts a configuration in %s/%s", SourceAIRulez, AgentsDirName, AgentsConfigFilename)
	}
	return nil
}

// loadAgentsSource scans the content tree of the "agents" source: the standard
// layout under .agents, completed by the AgentsSourceReader with the root
// AGENTS.md.
func loadAgentsSource(ctx context.Context, scanner *contentScanner, cfg *Config, lo loadOptions) (*ContentTree, error) {
	if lo.resolvers.Agents == nil {
		return nil, oops.
			With("source", cfg.Source).
			Hint("The agents source is configured but the load was given no reader for it; pass config.WithResolvers (the CLI and the MCP server do).").
			Errorf("source %q configured but the agents-source reader is unavailable", SourceAgents)
	}
	tree, err := scanContentTree(scanner, filepath.Join(cfg.BaseDir, AgentsDirName), cfg.BundleExclude)
	if err != nil {
		return nil, err
	}
	completed, err := lo.resolvers.Agents(ctx, cfg, tree)
	if err != nil {
		return nil, oops.With("source", SourceAgents).Wrapf(err, "read AGENTS.md and %s", AgentsDirName)
	}
	return completed, nil
}
