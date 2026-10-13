package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

// The values of `init --source`.
const (
	initSourceAIRulez = config.SourceAIRulez
	initSourceAgents  = config.SourceAgents
)

// initSource returns the value of init's --source, rejecting anything but the two
// sources ai-rulez reads.
func initSource(cmd *cobra.Command) (string, error) {
	source, err := cmd.Flags().GetString("source")
	if err != nil {
		return "", oops.Wrapf(err, "read --source")
	}
	switch source {
	case "", initSourceAIRulez, initSourceAgents:
		return source, nil
	}
	return "", oops.Hint(`Use --source ai-rulez (the .ai-rulez tree, the default) or --source agents (AGENTS.md and .agents/).`).
		Errorf("unknown --source %q", source)
}

// runInitAgents initializes a project whose content is AGENTS.md and .agents. It
// writes only the configuration (.agents/ai-rulez.toml, or config.toml with
// source = "agents" in the directory --config-dir names) and a starter AGENTS.md
// when there is none: the content directories belong to the standards and are
// never scaffolded. With --from the existing tool files are converted into
// .agents (see `convert --into .agents`).
func runInitAgents(cmd *cobra.Command, args []string) error {
	projectName := getProjectName(args)
	dir := agentsInitDir()
	layout := config.LayoutOf(dir)
	inAgents := config.IsAgentsConfigDir(dir)
	for _, conflict := range agentsInitConflicts(".", inAgents) {
		logger.Warn(conflict)
	}

	if fromFlag != "" {
		return runInitAgentsImport(cmd, dir, inAgents)
	}
	if err := prepareAgentsConfig(cmd, layout.ConfigFile); err != nil {
		return failMsg("Refusing to replace the existing configuration", err)
	}
	if err := os.MkdirAll(filepath.Dir(layout.ConfigFile), 0o755); err != nil {
		return failMsg("Failed to create the configuration directory", err)
	}
	body := templates.InitAgentsConfigTOML(projectName, []string{presetClaude}, inAgents)
	if err := os.WriteFile(layout.ConfigFile, []byte(body), 0o644); err != nil {
		return failMsg("Failed to write the configuration", err)
	}
	created := []string{layout.ConfigFile}
	if !skipContentFlag {
		if md, err := writeStarterAgentsMD(projectName); err != nil {
			return failMsg("Failed to write AGENTS.md", err)
		} else if md != "" {
			created = append(created, md)
		}
	}

	out := outFor(cmd)
	out.Info("Created %s for %s (source = agents)\n\n", filepath.ToSlash(layout.ConfigFile), projectName)
	out.Info("Your content is AGENTS.md and the %s directory; ai-rulez never writes to them.\n", config.AgentsDirName)
	out.Info("\nNext steps:\n")
	out.Info("  1. Write your instructions in AGENTS.md and your skills in %s/skills/<name>/SKILL.md\n", config.AgentsDirName)
	out.Info("  2. Edit %s to choose the presets to generate for\n", filepath.ToSlash(layout.ConfigFile))
	out.Info("  3. Run 'ai-rulez generate' to translate them to the other tools\n")
	if setupHooks {
		handleHooksSetup()
	}
	logger.Debug("Initialized an agents-source project", "files", created)
	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: typeConfig, Name: projectName, Path: layout.ConfigFile,
	}, true)
}

// agentsInitConflicts lists, as warnings, the existing configuration the one init
// writes will shadow or be shadowed by: .agents/ai-rulez.toml takes precedence over
// any config.toml, so only one of them is ever read. base is the project directory.
func agentsInitConflicts(base string, inAgents bool) []string {
	var out []string
	if inAgents {
		for _, dir := range []string{".ai-rulez", ".config/ai-rulez"} {
			rel := dir + "/config.toml"
			if info, err := os.Stat(filepath.Join(base, filepath.FromSlash(rel))); err == nil && !info.IsDir() {
				out = append(out, rel+" is ignored once "+config.AgentsDirName+"/"+config.AgentsConfigFilename+" exists: it takes precedence")
			}
		}
		return out
	}
	existing := filepath.Join(base, config.AgentsDirName, config.AgentsConfigFilename)
	if info, err := os.Stat(existing); err == nil && !info.IsDir() {
		out = append(out, config.AgentsDirName+"/"+config.AgentsConfigFilename+" takes precedence over the config.toml this writes, which will be ignored")
	}
	return out
}

// agentsInitDir is the directory the agents-source config goes in: --config-dir
// when given, else .agents.
func agentsInitDir() string {
	if configDir != "" {
		return filepath.Clean(filepath.FromSlash(configDir))
	}
	return config.AgentsDirName
}

// prepareAgentsConfig refuses to replace an existing configuration file unless
// --force is given, which keeps the old file as <file>.bak-<timestamp>. The
// directory itself holds the project's content and is never moved.
func prepareAgentsConfig(cmd *cobra.Command, file string) error {
	if _, err := os.Stat(file); err != nil {
		return nil //nolint:nilerr // nothing there: nothing to protect
	}
	if !initForceFlag(cmd) {
		return oops.Hint("Pass --force to replace it (the old file is kept as "+file+".bak-<timestamp>), or edit it by hand.").
			Errorf("%s already exists", file)
	}
	backup := file + ".bak-" + time.Now().Format("20060102-150405")
	if err := os.Rename(file, backup); err != nil {
		return oops.Wrapf(err, "move the existing %s to %s", file, backup)
	}
	logger.Info("Existing " + file + " moved to " + backup)
	return nil
}

// writeStarterAgentsMD writes AGENTS.md when the project has none and returns its
// path, or "" when one exists (it is the source and is left alone).
func writeStarterAgentsMD(projectName string) (string, error) {
	path := config.AgentsMDFilename
	if _, err := os.Lstat(path); err == nil {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(templates.InitAgentsMD(projectName)), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// runInitAgentsImport converts the tool files named by --from into .agents. A
// config in any other directory is not supported here: convert keeps the content
// beside the config it writes, which for the agents source must be .agents.
func runInitAgentsImport(cmd *cobra.Command, dir string, inAgents bool) error {
	if !inAgents {
		return failMsg("Cannot import into "+dir, oops.
			Hint("Use `ai-rulez init --source agents --from ...` (config in .agents/ai-rulez.toml), or `convert --into .agents`.").
			Errorf("--from with --source agents writes %s/%s", config.AgentsDirName, config.AgentsConfigFilename))
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return failMsg("Failed to get working directory", err)
	}
	if err := previewInitImportInto(watchParentContext(cmd), workingDir, config.AgentsDirName); err != nil {
		return failMsg("Failed to import from sources", err)
	}
	report, err := importer.Convert(watchParentContext(cmd), initConvertOptions(workingDir, config.AgentsDirName, true))
	if report != nil {
		if werr := report.WriteText(os.Stdout); werr != nil && err == nil {
			err = oops.Wrapf(werr, "print the convert report")
		}
	}
	if err == nil && !report.Written {
		err = oops.Errorf("the imported content failed the security scan or validation; nothing was written")
	}
	if err != nil {
		return failMsg("Failed to import from sources", err)
	}
	logger.Info("✅ Successfully imported content to " + config.AgentsDirName + "/ (source = agents)")
	logger.Info("   Sources: " + fromFlag)
	logger.Info("\nNext steps:")
	logger.Info("  1. Review the report above and the imported content in " + config.AgentsDirName + "/")
	logger.Info("  2. Edit " + config.AgentsDirName + "/" + config.AgentsConfigFilename + " to customize presets")
	logger.Info("  3. Run 'ai-rulez generate' to create tool-specific outputs")
	return nil
}
