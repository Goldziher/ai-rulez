package commands

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

var (
	dryRun             bool
	updateGitignore    bool
	recursive          bool
	noFetch            bool
	profile            string
	configDir          string
	mcpEnv             []string
	mcpEnvFiles        []string
	pluginMode         bool
	noLocal            bool
	allowLocalDrift    bool
	generateForce      bool
	pluginIfConfigured bool
	generateCheck      bool
	generateLocked     bool
	generateFrozen     bool
	generateRole       string
	generateEmitPlan   string
)

var GenerateCmd = &cobra.Command{
	Use:   "generate [config-file]",
	Short: "Generate AI assistant rule files from configuration",
	Long: `Generate AI assistant rule files based on the configuration.
This will create markdown files for various AI assistants like Claude,
Cursor, Devin, etc. based on your configuration.`,
	Aliases: []string{"gen", "g"},
	Args:    cobra.MaximumNArgs(1),
	Run:     runGenerate,
}

func init() {
	GenerateCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Show what would be generated without writing files")
	GenerateCmd.Flags().BoolVar(&generateCheck, "check", false,
		"Verify the committed output matches the sources without writing: list differing files and exit 2 on drift (for CI)")
	GenerateCmd.Flags().BoolVarP(&generateWatch, "watch", "w", false,
		"Generate, then watch the configuration directory and local include sources and regenerate on every change (Ctrl-C to stop)")
	GenerateCmd.Flags().BoolVar(&generateLocked, "locked", false,
		"Require ai-rulez.lock to cover every remote include and installed skill and fetch exactly the pinned commits (for CI)")
	GenerateCmd.Flags().BoolVar(&generateFrozen, "frozen", false,
		"Like --locked, and never use the network: resolve only from the local cache, verified against the lock")
	GenerateCmd.Flags().BoolVarP(&updateGitignore, "gitignore", "i", false, "Update .gitignore files to include generated output patterns")
	GenerateCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Find and process configuration files recursively")
	GenerateCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile to generate, or a comma-separated list to compose several (default: from config or 'default')")
	GenerateCmd.Flags().StringVar(&generateRole, flagRole, "",
		"Generate the slice of content a role selects instead of a profile (see 'ai-rulez roles list'); mutually exclusive with --profile")
	GenerateCmd.Flags().BoolVar(&noFetch, "offline", false, "Skip fetching remote includes, use cached content only")
	GenerateCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	GenerateCmd.Flags().StringArrayVarP(&mcpEnv, "env", "e", nil, "MCP env override in KEY=VALUE form (repeatable)")
	GenerateCmd.Flags().StringArrayVarP(&mcpEnvFiles, "env-file", "E", nil, "Dotenv file for MCP env placeholders (repeatable)")
	GenerateCmd.Flags().StringVar(&generateEmitPlan, "emit-plan", "",
		"Write the generation plan (every file that would be written, merged or removed, with digests; no secrets) as JSON to FILE ('-' for stdout) and apply nothing; see schema/plan.schema.json")
	GenerateCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content (the view a teammate without them sees)")
	GenerateCmd.Flags().BoolVar(&allowLocalDrift, "allow-local-drift", false,
		"Write output even when machine-local config would change files shared with the team")
	GenerateCmd.Flags().BoolVar(&generateForce, "force", false,
		"Overwrite an existing file ai-rulez cannot prove it wrote (a hand-written CLAUDE.md); without it generate refuses that file and exits 1")
	GenerateCmd.Flags().BoolVar(&userScope, "user", false,
		"Generate the user-level config (default ~/.config/ai-rulez, or --config) into the home directories each harness reads: ~/.claude, ~/.agents/skills, ~/.codex, ~/.gemini, ~/.config/opencode, ~/.copilot, ~/.pi/agent")
	addYesFlag(GenerateCmd.Flags(), &assumeYes, "With --user: write without the confirmation prompt; always: do not warn about new hook and MCP commands")
	GenerateCmd.Flags().BoolVar(&pluginMode, "plugin", false, "Generate distributable plugin bundles and a marketplace index from the [plugin] block")
	GenerateCmd.Flags().BoolVar(&pluginIfConfigured, "if-configured", false, "Skip plugin generation when no plugin authoring configuration is present")
}

func runGenerate(cmd *cobra.Command, args []string) {
	progress.SetQuiet(viper.GetBool("quiet"))

	// The lock policy of every load this run makes (before any config loading).
	applyLockFlags()

	exitOn(checkRoleFlags())
	exitOn(checkEmitPlanFlags())

	if runOtherGenerateMode(cmd, args) {
		return
	}

	ctx := cmdContext()
	cfg := loadGenerateConfig(args)
	applyGenerateOverrides(cfg)
	warnFrontmatter(cfg)
	if err := importGate(cfg); err != nil {
		fmtError(err)
		os.Exit(1)
	}
	if pluginMode && pluginIfConfigured && !cfg.HasPluginAuthoring() {
		logger.Info("Skipping plugin generation: no plugin authoring configuration")
		return
	}

	if generateEmitPlan != "" {
		exitOn(emitPlan(ctx, cfg, generateEmitPlan))
		return
	}

	// Create generator
	gen := generator.NewGenerator(cfg)
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetOverwriteUnowned(generateForce)
	gen.SetContext(ctx)
	exitOn(applyRole(gen))
	exitOn(generatePreflight(cfg, gen))

	if pluginMode {
		runPluginGenerate(gen)
		return
	}

	if dryRun {
		if err := printDryRun(gen, ""); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		return
	}

	// Generate files
	if err := gen.Generate(profile); err != nil {
		fmtError(err)
		os.Exit(1)
	}
}

// runOtherGenerateMode runs --watch, --check, --user and --recursive, which do
// not take the single-project path, and reports whether one of them ran.
func runOtherGenerateMode(cmd *cobra.Command, args []string) bool {
	switch {
	case generateWatch:
		if err := runGenerateWatch(watchParentContext(cmd), args); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		return true
	case generateCheck:
		runGenerateCheck(args)
		return true
	case handleUserGenerate(args):
		return true
	case recursive:
		if code := runRecursiveGenerate(); code != 0 {
			os.Exit(code)
		}
		return true
	}
	return false
}

// loadGenerateConfig loads, policy-checks and validates the project and enforces
// the lock; any failure ends the process with its exit code.
func loadGenerateConfig(args []string) *config.Config {
	cfg, err := loadConfigForCommand(cmdContext(), args, append(pluginLoadOptions(pluginMode), config.WithFrontmatterErrors())...)
	if err != nil {
		fmtError(err)
		if (generateLocked || generateFrozen) && errors.Is(err, config.ErrLockViolation) {
			os.Exit(exitDrift) // a missing or disagreeing lock is drift, the same code as a changed source
		}
		os.Exit(1)
	}

	// The organization policy clamped the configuration at load; refuse to
	// generate from one that tried to loosen it.
	if err := policyGate(cfg); err != nil {
		fmtError(err)
		os.Exit(exitCodeFor(err))
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmtError(err)
		os.Exit(1)
	}

	exitOnLockedDrift(enforceLockedContent(cfg))
	exitOnMovedTags(cfg) // only with --verify-tags or [lock] verify_tags: a pinned tag that moved ends the run
	return cfg
}

// emitPlan renders the generation plan without writing any output and writes it
// as JSON to dest ("-" is standard output). Nothing is applied.
func emitPlan(ctx context.Context, cfg *config.Config, dest string) error {
	// The checks a real run starts with that only read: nothing is announced or recorded.
	if err := planPreflight(cfg); err != nil {
		return err
	}
	plan, err := generator.PlanOutputs(ctx, cfg, generator.PlanOptions{Profile: profile, Role: generateRole})
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	data, err := generator.MarshalPlan(plan)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if dest == "-" {
		_, err = os.Stdout.Write(data)
		return oops.Wrapf(err, "write the plan")
	}
	if err := config.WriteFileAtomic(dest, data, 0o644); err != nil {
		return oops.With("path", dest).Wrapf(err, "write the plan")
	}
	return nil
}

// printDryRun prints the generation plan and returns an error when the plan
// holds local drift a real run would refuse, so a blocked dry run exits non-zero.
func printDryRun(gen *generator.Generator, indent string) error {
	plan, err := gen.DryRun(profile)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	for _, line := range plan {
		progress.PrintlnIfNotQuiet(indent + line)
	}
	return gen.DryRunBlocked() //nolint:wrapcheck // already contextual
}

// runPluginGenerate handles `generate --plugin`: it renders (or, with --dry-run,
// previews) the distributable plugin bundles and marketplace index.
func runPluginGenerate(gen *generator.Generator) {
	if dryRun {
		plan, err := gen.DryRunPlugin(profile)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		for _, line := range plan {
			progress.PrintlnIfNotQuiet(line)
		}
		return
	}
	if err := gen.GeneratePlugin(profile); err != nil {
		fmtError(err)
		os.Exit(1)
	}
}

func loadConfigForCommand(ctx context.Context, args []string, opts ...config.LoadOption) (*config.Config, error) {
	if noLocal {
		opts = append(opts, config.WithoutLocal())
	}
	if len(args) > 0 {
		return loadProjectFile(ctx, args[0], opts...)
	}
	if cfgFile != "" {
		return loadProjectFile(ctx, cfgFile, opts...)
	}
	if configDir != "" {
		return loadProjectDir(ctx, ".", configDir, opts...)
	}
	return loadProject(ctx, ".", opts...)
}

// pluginLoadOptions returns the load options for plugin bundle work. Plugin
// bundles are distributable, so machine-local overlays never apply to them.
func pluginLoadOptions(plugin bool) []config.LoadOption {
	if plugin || noLocal {
		return []config.LoadOption{config.WithoutLocal()}
	}
	return nil
}

// configConventionRoot is the generic project-level config directory from the
// pi0/config-dir proposal. The ai-rulez subtree inside it mirrors .ai-rulez/.
const (
	configConventionRoot   = ".config"
	configConventionSubdir = "ai-rulez"
)

func applyGenerateOverrides(cfg *config.Config) {
	if updateGitignore {
		enabled := true
		cfg.Gitignore = &enabled
	}
	if len(mcpEnv) > 0 {
		cfg.MCPEnvOverrides = parseMCPEnvOverrides(mcpEnv)
	}
	if len(mcpEnvFiles) > 0 {
		cfg.MCPEnvFiles = append([]string(nil), mcpEnvFiles...)
	}
}

func parseMCPEnvOverrides(values []string) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			logger.Error("Invalid --env value; expected KEY=VALUE", "value", value)
			os.Exit(1)
		}
		out[key] = val
	}
	return out
}

// checkGenerateCheckFlags rejects flags that make no sense with --check, which
// must never write.
func checkGenerateCheckFlags() error {
	switch {
	case dryRun:
		return oops.Errorf("--check and --dry-run are mutually exclusive: --check already writes nothing")
	case pluginMode:
		return oops.Errorf("--check does not cover plugin bundles; use `ai-rulez verify --plugin`")
	case updateGitignore:
		return oops.Errorf("--check cannot be combined with --gitignore, which writes .gitignore files")
	}
	return nil
}

// applyLockFlags turns --no-fetch, --locked and --frozen into the lock policy
// of this run's loads. generate writes outputs, so an enforced lock is required.
func applyLockFlags() {
	cliLockPolicy = config.LockPolicy{RequireWhenEnforced: true, Offline: noFetch}
	switch {
	case generateFrozen:
		cliLockPolicy.Mode, cliLockPolicy.Offline = config.LockFrozen, true
	case generateLocked:
		cliLockPolicy.Mode = config.LockRequire
	}
}

// runGenerateCheck runs `generate --check` and exits with its code.
func runGenerateCheck(args []string) {
	if code := generateCheckCode(args); code != 0 {
		os.Exit(code)
	}
}

// generateCheckCode is `generate --check`: it first requires the sources to
// match ai-rulez.lock when --locked or --frozen is set or the lock is enforced
// (a lock exists and [lock] enforce is not false), the same gate `generate`
// applies before writing, then compares the generated files. Each root is
// loaded once for both.
func generateCheckCode(args []string) int {
	if err := checkGenerateCheckFlags(); err != nil {
		fmtError(err)
		return 1
	}
	return runDriftCheckGated(args, recursive, driftRender, func(cfg *config.Config) error {
		return enforceLockedContentFor(cfg, true)
	})
}

// importGate scans imported content before anything is written, when
// [lint.security] scan_imports is not "off". Every mode that writes runs it (the
// in-repo run, plugin bundles and --user); only a dry run skips it.
func importGate(cfg *config.Config) error {
	if dryRun {
		return nil
	}
	// The signature gate comes first: unsigned content is refused before it is scanned.
	if err := skillSignatureGate(cfg); err != nil {
		return err
	}
	if err := authoredSecretGate(cfg); err != nil {
		return err
	}
	return enforceScanImports(cfg)
}

// checkRoleFlags rejects --role together with --profile.
func checkRoleFlags() error {
	if generateRole != "" && profile != "" {
		return oops.Hint("A role replaces the profile selection; pass only one").
			Errorf("--role and --profile are mutually exclusive")
	}
	if generateRole != "" && pluginMode {
		return oops.Errorf("--role cannot be combined with --plugin: plugin bundles are built from the full content")
	}
	return nil
}

// checkEmitPlanFlags rejects --emit-plan with the modes that never reach it or
// plan something else: --watch, --check, --user and --recursive return before
// the plan is made, and --plugin renders bundles, not the in-repo outputs.
func checkEmitPlanFlags() error {
	if generateEmitPlan == "" {
		return nil
	}
	for _, f := range []struct {
		name string
		set  bool
	}{{"--watch", generateWatch}, {"--check", generateCheck}, {"--user", userScope}, {"--recursive", recursive}, {"--plugin", pluginMode}} {
		if f.set {
			return oops.Hint("Run --emit-plan on its own, from the project root").
				Errorf("--emit-plan cannot be combined with %s", f.name)
		}
	}
	return nil
}

// applyRole switches a Generator to the role given with --role, if any.
func applyRole(gen *generator.Generator) error {
	if generateRole == "" {
		return nil
	}
	return gen.SetRole(generateRole) //nolint:wrapcheck // already contextual
}

// FormatError renders err for the terminal: the error text, then the oops hint
// when the error carries one. The CLI itself prints through renderError.
func FormatError(err error) string {
	text := errorText(err)
	if hint := errorHintOf(err); hint != "" {
		text += "\n\nHint: " + hint
	}
	return text
}

// errorHint is the hint of an error as printed. An unknown profile in a
// configuration that declares none reads "No profiles are declared", not an
// empty list.
func errorHint(err oops.OopsError) string {
	hint := err.Hint()
	if names, ok := err.Context()["available_profiles"].([]string); ok && len(names) == 0 {
		hint = strings.Replace(hint, "Available profiles: []", "No profiles are declared in this configuration.", 1)
	}
	return hint
}
