package commands

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
)

var (
	skillSource string
	skillPath   string
	skillRef    string
	skillForce  bool
	skillJSON   bool
	skillLocal  bool
)

// SkillCmd is the top-level skill command
var SkillCmd = &cobra.Command{
	Use:   kindSkill,
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

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runSkillRemove,
}

var skillListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all installed skills",
	Long:  `List all skills installed from external sources.` + localListHint,
	Args:  cobra.NoArgs,
	Run:   runSkillList,
}

var skillUpdateCmd = &cobra.Command{
	Use:   "update [name...]",
	Short: "Re-pin installed skills in ai-rulez.lock to their current remote commit",
	Long: `Resolve the named installed skills (all of them without names) from their
remote and record the commit and content digest in .ai-rulez/ai-rulez.lock.
Includes and skills not named keep their pins. See "ai-rulez lock".

A skill that uses a version range (version = ^1.2) keeps a pin that
still satisfies the range: this command re-resolves plain refs (a branch
follows its tip) and never upgrades a range pin. Use "ai-rulez update --kind
skill [name...]" to move range pins to the newest allowed tag.`,
	Run: func(_ *cobra.Command, args []string) {
		lockCheck = false
		if code := runLockFor(kindSkill, args); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	SkillCmd.AddCommand(skillUpdateCmd)
	SkillCmd.AddCommand(skillInstallCmd)
	SkillCmd.AddCommand(skillRemoveCmd)
	SkillCmd.AddCommand(skillListCmd)

	skillInstallCmd.Flags().BoolVar(&skillLocal, "local", false, localFlagUsage)
	skillRemoveCmd.Flags().BoolVar(&skillLocal, "local", false, localFlagUsage)

	// Flags for skill install
	skillInstallCmd.Flags().StringVarP(&skillSource, "source", "s", "", "Git URL or local path (required)")
	skillInstallCmd.Flags().StringVarP(&skillPath, "path", "p", "", "Path within repo to skill directory (defaults to skills/<name>)")
	skillInstallCmd.Flags().StringVarP(&skillRef, "ref", "r", "", "Git reference: branch, tag, or commit hash")
	if err := skillInstallCmd.MarkFlagRequired("source"); err != nil {
		logger.Debug("Failed to mark source flag as required", "error", err)
	}

	// Flags for skill remove
	addYesFlag(skillRemoveCmd.Flags(), &skillForce, "Skip confirmation prompts")

	// Flags for skill list
	addJSONFormat(skillListCmd.Flags(), &skillJSON, "j")
}

func runSkillInstall(cmd *cobra.Command, args []string) {
	name := args[0]

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}
	if skillLocal {
		op = op.Local()
	}

	req := &crud.InstallSkillRequest{
		Name:   name,
		Source: skillSource,
		Path:   skillPath,
		Ref:    skillRef,
	}

	if err := op.InstallSkill(ctx, req); err != nil {
		fatal("Failed to install skill", err)
	}

	logger.Info("Skill installed successfully",
		"name", name,
		"source", incl.RedactURL(skillSource),
	)
}

func runSkillRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	confirmRemovalUnlessYes(skillForce, "installed skill", name, "Operation canceled")

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}
	if skillLocal {
		op = op.Local()
	}

	if err := op.UninstallSkill(ctx, name); err != nil {
		fatal("Failed to remove installed skill", err)
	}

	logger.Info("Skill removed successfully", "name", name)
}

func runSkillList(cmd *cobra.Command, args []string) {
	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}

	skills, err := op.ListInstalledSkills(ctx)
	if err != nil {
		fatal("Failed to list installed skills", err)
	}

	if len(skills) == 0 && !skillJSON {
		logger.Info("No installed skills found")
		logLocalEntriesHint("installed_skills")
		return
	}

	if skillJSON {
		output := make([]map[string]interface{}, len(skills))
		for i, s := range skills {
			output[i] = map[string]interface{}{
				keyName:   s.Name,
				keySource: incl.RedactURL(s.Source),
				keyPath:   s.Path,
				"ref":     s.Ref,
				keyType:   s.Type,
			}
		}
		data, err := jsondoc.Marshal(output)
		if err != nil {
			fatal("Failed to marshal JSON", err)
		}
		fmt.Print(string(data))
	} else {
		logger.Info("Installed skills:")
		for _, s := range skills {
			sourceInfo := fmt.Sprintf("[%s]", s.Type)
			logger.Info(fmt.Sprintf("  • %s %s", s.Name, sourceInfo))
			logger.Debug(fmt.Sprintf("    Source: %s", incl.RedactURL(s.Source)))
			if s.Path != "" {
				logger.Debug(fmt.Sprintf("    Path: %s", s.Path))
			}
			if s.Ref != "" {
				logger.Debug(fmt.Sprintf("    Ref: %s", s.Ref))
			}
		}
		logLocalEntriesHint("installed_skills")
	}
}
