package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var (
	skillSource string
	skillPath   string
	skillRef    string
	skillForce  bool
	skillJSON   bool
)

// SkillCmd is the top-level skill command
var SkillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage installed skills",
	Long:  `Manage named skills installed from external repositories.`,
}

var skillInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a named skill from a git repository or local path",
	Long: `Install a named skill from an external source.

The skill will be fetched dynamically during generation and included
in your generated outputs.

Sources can be:
  - Git URLs: https://github.com/user/repo
  - Local paths: /path/to/local/repo

By default, the skill is expected at skills/<name>/ within the source.
Use --path to override.

Examples:
  ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
  ai-rulez skill install ai-rulez --source https://github.com/Goldziher/ai-rulez
  ai-rulez skill install my-skill --source ./local-repo --path custom/path`,
	Args: cobra.ExactArgs(1),
	Run:  runSkillInstall,
}

var skillRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove an installed skill",
	Long: `Remove an installed skill from the configuration.

Use --force to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runSkillRemove,
}

var skillListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all installed skills",
	Long:  `List all skills installed from external sources.`,
	Args:  cobra.NoArgs,
	Run:   runSkillList,
}

func init() {
	SkillCmd.AddCommand(skillInstallCmd)
	SkillCmd.AddCommand(skillRemoveCmd)
	SkillCmd.AddCommand(skillListCmd)

	// Flags for skill install
	skillInstallCmd.Flags().StringVar(&skillSource, "source", "", "Git URL or local path (required)")
	skillInstallCmd.Flags().StringVar(&skillPath, "path", "", "Path within repo to skill directory (defaults to skills/<name>)")
	skillInstallCmd.Flags().StringVar(&skillRef, "ref", "", "Git reference: branch, tag, or commit hash")
	if err := skillInstallCmd.MarkFlagRequired("source"); err != nil {
		logger.Debug("Failed to mark source flag as required", "error", err)
	}

	// Flags for skill remove
	skillRemoveCmd.Flags().BoolVar(&skillForce, "force", false, "Skip confirmation prompts")

	// Flags for skill list
	skillListCmd.Flags().BoolVar(&skillJSON, "json", false, "Output as JSON")
}

func runSkillInstall(cmd *cobra.Command, args []string) {
	name := args[0]

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	req := &crud.InstallSkillRequest{
		Name:   name,
		Source: skillSource,
		Path:   skillPath,
		Ref:    skillRef,
	}

	if err := op.InstallSkill(ctx, req); err != nil {
		logger.Error("Failed to install skill", "error", err)
		os.Exit(1)
	}

	logger.Info("Skill installed successfully",
		"name", name,
		"source", skillSource,
	)
}

func runSkillRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	if !skillForce {
		if !confirmRemoval("installed skill", name) {
			logger.Info("Operation canceled")
			return
		}
	}

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	if err := op.UninstallSkill(ctx, name); err != nil {
		logger.Error("Failed to remove installed skill", "error", err)
		os.Exit(1)
	}

	logger.Info("Skill removed successfully", "name", name)
}

func runSkillList(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	skills, err := op.ListInstalledSkills(ctx)
	if err != nil {
		logger.Error("Failed to list installed skills", "error", err)
		os.Exit(1)
	}

	if len(skills) == 0 {
		logger.Info("No installed skills found")
		return
	}

	if skillJSON {
		output := make([]map[string]interface{}, len(skills))
		for i, s := range skills {
			output[i] = map[string]interface{}{
				keyName:  s.Name,
				"source": s.Source,
				keyPath:  s.Path,
				"ref":    s.Ref,
				keyType:  s.Type,
			}
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal JSON", "error", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		logger.Info("Installed skills:")
		for _, s := range skills {
			sourceInfo := fmt.Sprintf("[%s]", s.Type)
			logger.Info(fmt.Sprintf("  • %s %s", s.Name, sourceInfo))
			logger.Debug(fmt.Sprintf("    Source: %s", s.Source))
			if s.Path != "" {
				logger.Debug(fmt.Sprintf("    Path: %s", s.Path))
			}
			if s.Ref != "" {
				logger.Debug(fmt.Sprintf("    Ref: %s", s.Ref))
			}
		}
	}
}
