package commands

import (
	"os"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

// runInitMinimal creates ai-rulez.toml in the working directory: a configuration
// without a .ai-rulez/ folder. It writes nothing else; the instruction files of
// the project stay as they are.
func runInitMinimal(cmd *cobra.Command, args []string) error {
	if err := checkInitMinimalFlags(cmd); err != nil {
		return failMsg("Invalid flags for --minimal", err)
	}
	projectName := getProjectName(args)
	file := config.RootConfigFilename
	for _, conflict := range initMinimalConflicts(".") {
		logger.Warn(conflict)
	}
	if err := prepareAgentsConfig(cmd, file); err != nil {
		return failMsg("Refusing to replace the existing configuration", err)
	}
	body := templates.InitMinimalConfigTOML(projectName, []string{presetClaude})
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil { //nolint:gosec // a project config file, committed like any other
		return failMsg("Failed to write the configuration", err)
	}

	out := outFor(cmd)
	out.Info("Created %s for %s (no .ai-rulez/ folder)\n\n", file, projectName)
	out.Info("Next steps:\n")
	out.Info("  1. Declare skills, MCP servers and lint settings in %s\n", file)
	out.Info("  2. Run 'ai-rulez generate' to write them into your tools' folders\n")
	out.Info("  3. Run 'ai-rulez lock' to pin remote sources in ai-rulez.lock\n")
	if setupHooks {
		handleHooksSetup()
	}
	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: typeConfig, Name: projectName, Path: file,
	}, true)
}

// checkInitMinimalFlags rejects the flags that scaffold or import a content tree,
// which a configuration without a .ai-rulez/ folder does not have.
func checkInitMinimalFlags(cmd *cobra.Command) error {
	source, err := initSource(cmd)
	if err != nil {
		return err
	}
	switch {
	case source == initSourceAgents:
		return oops.Hint("--minimal and --source agents are two different layouts; pick one.").Errorf("--minimal cannot be combined with --source agents")
	case fromFlag != "":
		return oops.Hint("Import with `ai-rulez init --from ...` (the directory layout), or `convert`.").Errorf("--minimal cannot be combined with --from")
	case domainsFlag != "":
		return oops.Hint("Domains are directories under .ai-rulez/; drop --domains.").Errorf("--minimal cannot be combined with --domains")
	case configDir != "":
		return oops.Hint("--minimal writes ai-rulez.toml in the working directory; drop --config-dir.").Errorf("--minimal cannot be combined with --config-dir")
	}
	return nil
}

// initMinimalConflicts lists, as warnings, the existing configuration that takes
// precedence over the ai-rulez.toml init writes: a config directory wins.
func initMinimalConflicts(base string) []string {
	var out []string
	if name := config.ResolveConfigDirName(base); name != "" && name != config.RootConfigDirName {
		out = append(out, name+"/ already holds a configuration and takes precedence: the "+config.RootConfigFilename+" this writes will be ignored")
	}
	return out
}
