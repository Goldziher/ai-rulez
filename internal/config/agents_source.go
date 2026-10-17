package config

import (
	"context"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
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
	// sourceKey is the config key that selects the source.
	sourceKey = "source"
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

// AuthoredRoot is the directory the project's own content is addressed from, and
// the one a content file must lie in to count as authored here rather than
// imported: the project root for the "agents" source (AGENTS.md and .agents sit
// beside the config directory), the configuration directory otherwise.
func (c *Config) AuthoredRoot() string {
	if (c.UsesAgentsSource() || c.Implicit) && c.BaseDir != "" {
		return c.BaseDir
	}
	return c.ConfigDir
}

// ConfigPath is the main configuration file. It is not always inside ConfigDir:
// .agents/ai-rulez.toml sits beside the .agents/.ai-rulez state directory that
// ConfigDir names.
func (c *Config) ConfigPath() string {
	if c.Implicit {
		return "" // an implicit configuration has no file
	}
	if c.ConfigFilePath != "" {
		return c.ConfigFilePath
	}
	return filepath.Join(c.ConfigDir, c.ConfigFile)
}

// Layout says where the pieces of a project live, given the config directory a
// command was pointed at (.ai-rulez, .config/ai-rulez or .agents). The CRUD
// commands and the MCP server use it to write content, the config file and the
// machine-local tree to the right place without loading the project.
type Layout struct {
	// ConfigDir is the directory that holds the main config file.
	ConfigDir string
	// ConfigFile is the main config file.
	ConfigFile string
	// ContentDir holds the rules, context, skills, agents, commands, checks and
	// domains directories: .agents for the "agents" source, ConfigDir otherwise.
	ContentDir string
	// StateDir holds the manifest, lock, local/ and config.local.toml.
	StateDir string
	// Agents is true when the content is read from AGENTS.md and .agents.
	Agents bool
	// AgentsMDSplit is true when AGENTS.md is read as one context item per H2
	// heading (agents_md_split).
	AgentsMDSplit bool
	// Contentless is true for the root layout (ai-rulez.toml, no content tree):
	// ContentDir names a directory that does not exist and content cannot be written.
	Contentless bool
}

// LayoutOf resolves the layout of the config directory configDir. It reads the
// config file only to learn its `source`; an unreadable file means the default.
func LayoutOf(configDir string) Layout {
	if base, ok := agentsLayoutBase(configDir); ok {
		agents := filepath.Join(base, AgentsDirName)
		file := filepath.Join(agents, AgentsConfigFilename)
		return Layout{
			ConfigDir: agents, ConfigFile: file,
			ContentDir: agents, StateDir: filepath.Join(agents, agentsStateDirName), Agents: true,
			AgentsMDSplit: readSourceKeys(file).AgentsMDSplit,
		}
	}
	if IsRootConfigDir(configDir) {
		return Layout{
			ConfigDir: configDir, ConfigFile: filepath.Join(configDir, RootConfigFilename),
			ContentDir: filepath.Join(configDir, aiRulezDirName), StateDir: configDir, Contentless: true,
		}
	}
	l := Layout{
		ConfigDir: configDir, ConfigFile: filepath.Join(configDir, configTOMLFilename),
		ContentDir: configDir, StateDir: configDir,
	}
	if keys := readSourceKeys(l.ConfigFile); keys.Source == SourceAgents {
		l.ContentDir = filepath.Join(ProjectBaseDir(configDir), AgentsDirName)
		l.Agents = true
		l.AgentsMDSplit = keys.AgentsMDSplit
	}
	return l
}

// sourceKeys are the config keys that say how the content is read.
type sourceKeys struct {
	Source        string `toml:"source"`
	AgentsMDSplit bool   `toml:"agents_md_split"`
}

// readSourceKeys returns the source keys of the TOML file at path, zero when it
// is missing or the file cannot be read.
func readSourceKeys(path string) sourceKeys {
	data, err := os.ReadFile(path) //nolint:gosec // the project's own config file
	if err != nil {
		return sourceKeys{}
	}
	var doc sourceKeys
	if toml.Unmarshal(data, &doc) != nil {
		return sourceKeys{}
	}
	return doc
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
	if c.UsesAgentsSource() && c.agentsMDOptIn {
		return oops.
			With("field", "agents_md").
			Hint("With the agents source every tool gets its own full file (AGENTS.md is yours and is never written); drop agents_md or set it to false.").
			Errorf("agents_md = true cannot be combined with source = %q", SourceAgents)
	}
	if c.AgentsMDSplit && !c.UsesAgentsSource() {
		return oops.
			With("field", "agents_md_split").
			Hint(`agents_md_split splits the AGENTS.md the agents source reads; set source = "agents" (or keep the configuration in .agents/ai-rulez.toml), or drop it.`).
			Errorf("agents_md_split needs source = %q", SourceAgents)
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
