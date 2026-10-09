package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
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
Use --path to override.`,
	Args: cobra.ExactArgs(1),
	RunE: runSkillInstall,
}

var skillRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove an installed skill",
	Long: `Remove an installed skill from the configuration.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	RunE: runSkillRemove,
}

var skillListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all installed skills",
	Long:  `List all skills installed from external sources.` + localListHint,
	Args:  cobra.NoArgs,
	RunE:  runSkillList,
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
	RunE: func(_ *cobra.Command, args []string) error {
		lockCheck = false
		return exitStatus(runLockFor(kindSkill, args))
	},
}

func init() {
	SkillCmd.AddCommand(skillUpdateCmd)
	SkillCmd.AddCommand(skillInstallCmd)
	SkillCmd.AddCommand(skillRemoveCmd)
	SkillCmd.AddCommand(skillListCmd)

	specLocal.Bool(skillInstallCmd.Flags(), &skillLocal, localFlagUsage)
	specLocal.Bool(skillRemoveCmd.Flags(), &skillLocal, localFlagUsage)

	// Flags for skill install
	skillInstallCmd.Flags().StringVar(&skillSource, "source", "", "Git URL or local path (required)")
	skillInstallCmd.Flags().StringVar(&skillPath, "path", "", "Path within repo to skill directory (defaults to skills/<name>)")
	skillInstallCmd.Flags().StringVar(&skillRef, "ref", "", "Git reference: branch, tag, or commit hash")
	if err := skillInstallCmd.MarkFlagRequired("source"); err != nil {
		logger.Debug("Failed to mark source flag as required", "error", err)
	}

	addResultFormat(skillInstallCmd.Flags())
	addResultFormat(skillRemoveCmd.Flags())

	// Flags for skill remove
	addYesFlag(skillRemoveCmd.Flags(), &skillForce, "Skip confirmation prompts")

	// Flags for skill list
	addJSONFormat(skillListCmd.Flags(), &skillJSON, "j")
}

func runSkillInstall(cmd *cobra.Command, args []string) error {
	name := args[0]

	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(skillLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	req := &crud.InstallSkillRequest{
		Name:   name,
		Source: skillSource,
		Path:   skillPath,
		Ref:    skillRef,
	}

	if err := op.InstallSkill(ctx, req); err != nil {
		return failMsg("Failed to install skill", err)
	}

	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: "installed_skill", Name: name, Source: incl.RedactURL(skillSource),
		Path: op.ConfigFile(), Local: skillLocal,
	}, false)
}

func runSkillRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := confirmRemovalUnlessYes(skillForce, "installed skill", name, "Operation canceled"); err != nil {
		return fail(err)
	}

	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(skillLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	if err := op.UninstallSkill(ctx, name); err != nil {
		return failMsg("Failed to remove installed skill", err)
	}

	return reportChange(out, out.JSON(), changeResult{
		Status: statusRemoved, Type: "installed_skill", Name: name, Path: op.ConfigFile(), Local: skillLocal,
	}, false)
}

func runSkillList(cmd *cobra.Command, _ []string) error {
	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	skills, err := op.ListInstalledSkills(ctx)
	if err != nil {
		return failMsg("Failed to list installed skills", err)
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
		return writeListJSON(out.Stdout(), output)
	}
	if len(skills) == 0 {
		out.Info("No installed skills found\n")
		logLocalEntriesHint("installed_skills")
		return nil
	}
	out.Result("Installed skills:\n")
	for _, s := range skills {
		out.Result("  • %s [%s]\n", s.Name, s.Type)
		out.Result("    Source: %s\n", incl.RedactURL(s.Source))
		if s.Path != "" {
			out.Result("    Path: %s\n", s.Path)
		}
		if s.Ref != "" {
			out.Result("    Ref: %s\n", s.Ref)
		}
	}
	logLocalEntriesHint("installed_skills")
	return nil
}
