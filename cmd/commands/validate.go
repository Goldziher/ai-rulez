package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

var validateRecursive bool

var ValidateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate AI rules configuration and content",
	Long: `Validate an AI rules configuration for syntax errors, schema compliance and
structural issues, then run the content checks: globs that match nothing, dead
links and references, missing hooks, oversize or duplicate content and the
security rules (see the [lint] config table).

Pass --config-only to check the configuration file alone. Pass --strict to make
warnings fail the run (the same as --fail-on warning).

Exit codes: 0 valid, 1 the configuration is invalid or could not be loaded,
2 findings at or above --fail-on (default error).`,
	Aliases: []string{"val", "v", "check"},
	Args:    cobra.MaximumNArgs(1),
	PreRunE: func(*cobra.Command, []string) error { return validatePreRun() },
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmdContext()
		if validateExplain != "" {
			if err := runExplain(cmd.OutOrStdout(), validateExplain, validateFormat); err != nil {
				fmtError(err)
				os.Exit(1)
			}
			return
		}
		if validateShowPolicy {
			os.Exit(runShowPolicy(ctx, args, cmd.OutOrStdout()))
		}
		if err := checkStrictFlags(); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if err := applyRepoRoot(); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		// JSON output must be the only thing on stdout.
		progress.SetQuiet(viper.GetBool("quiet") || structuredFormat(validateFormat))

		if validateRecursive {
			if len(args) > 0 {
				fmtError(oops.Errorf("validate --recursive does not take a config file argument"))
				os.Exit(1)
			}
			if code := runRecursiveValidate(); code != 0 {
				os.Exit(code)
			}
			return
		}

		cfg, err := loadConfigForCommand(ctx, args, config.WithFrontmatterErrors())
		if err != nil {
			logger.Error("Failed to load config")
			fmtError(err)
			os.Exit(1)
		}

		// Validate the raw file against the JSON schema first so a key the
		// struct would silently drop (and a value outside an enum) is reported.
		if cfg.ConfigFile != "" {
			configPath := filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
			if err := schema.ValidateFile(configPath); err != nil {
				logger.Error("Configuration failed schema validation", "path", configPath)
				fmtError(schemaFailure(cfg, err))
				os.Exit(1)
			}
		}

		if err := validateLocalOverlay(cfg); err != nil {
			logger.Error("Local overlay failed schema validation", "path", cfg.LocalOverlay.Path)
			fmtError(schemaFailure(cfg, err))
			os.Exit(1)
		}
		if cfg.LocalOverlay != nil {
			progress.PrintIfNotQuiet("%s\n", localOverlaySummary(cfg))
		}

		if err := cfg.Validate(); err != nil {
			logger.Error("Configuration validation failed", "path", cfg.ConfigDir)
			fmtError(err)
			os.Exit(1)
		}
		if err := checkLocalIncludes(cfg); err != nil {
			logger.Error("Configuration validation failed", "path", cfg.ConfigDir)
			fmtError(err)
			os.Exit(1)
		}

		if !validateStrict {
			// A strict run reports the same attempts as AR74x findings.
			if err := policyGate(cfg); err != nil {
				fmtError(err)
				os.Exit(1)
			}
		}

		logger.Success("Configuration is valid", "path", cfg.ConfigDir)
		warnWorktreeMarketplace(cfg)
		if validateStrict {
			if code := runStrictSingle(cfg); code != 0 {
				os.Exit(code)
			}
			return
		}
		presets.WarnDuplicateContent(cfg.Log(), cfg.Content)
		warnUnpinned(cfg)
		warnFrontmatter(cfg)
		displayConfigurationSummary(cfg)
	},
}

func init() {
	ValidateCmd.Flags().BoolVarP(&validateRecursive, "recursive", "r", false, "Validate every configuration file found recursively")
	ValidateCmd.Flags().BoolVar(&validateConfigOnly, "config-only", false, "Check the configuration file only and skip the content checks")
	ValidateCmd.Flags().BoolVar(&validateWarnings, "strict", false, "Fail on warnings as well as errors (the same as --fail-on warning)")
	ValidateCmd.Flags().BoolVar(&validateVerifiers, "verifiers", false, "Also evaluate the verifiers (never a command or a model) and report them as AR9H findings")
	ValidateCmd.Flags().BoolVar(&validateExtern, "external", false, "Also run the scanners configured in [[lint.external]] and merge their findings")
	ValidateCmd.Flags().StringVar(&validateApprovalsBase, "approvals-base", "", "Report approvals added since this git revision for content that also changed since it (AR716)")
	ValidateCmd.Flags().StringSliceVar(&validateAllowEgress, "allow-egress", nil, "With --external, allow the named [[lint.external]] scanners that declare egress = true to run (repeatable)")
	addFormatFlag(ValidateCmd.Flags(), &validateFormat, "", formatText, lint.Formats()...) // --format implies --strict
	ValidateCmd.Flags().StringVar(&validateLintProfile, "lint-profile", "", "Lint preset: default, strict or permissive (overrides [lint] profile; distinct from the generation --profile)")
	ValidateCmd.Flags().StringSliceVar(&validateAnalyzers, "analyzer", nil, "Run only these analyzers (repeatable or comma-separated; replaces [lint] analyzers): "+strings.Join(lint.AnalyzerNames(), ", "))
	ValidateCmd.Flags().StringVar(&validateOutput, "output", "", "Write the report to this file instead of stdout")
	ValidateCmd.Flags().StringVar(&validateFailOn, "fail-on", "", "Lowest severity that exits 2: error (default), warning, info or none")
	ValidateCmd.Flags().StringVar(&validateExplain, "explain", "", "Print what a rule (code or name, for example AR001) checks, why, examples and how to suppress it, then exit")
	addBaselineFlags(ValidateCmd)
	addChangedFlags(ValidateCmd)
	addFixFlags(ValidateCmd)
	ValidateCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content (the view a teammate without them sees)")
	ValidateCmd.Flags().StringVar(&validateRepoRoot, "repo-root", "", "Repository root that repo-relative paths and git-tracked globs resolve against (env AI_RULEZ_REPO_ROOT; default: the git toplevel, else the config's parent directory)")
	ValidateCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// validatePreRun resolves --config-only and --strict into the internal state.
func validatePreRun() error {
	if validateWarnings && validateConfigOnly {
		return oops.Errorf("--strict (warnings fail) needs the content checks: drop --config-only")
	}
	validateStrict = !validateConfigOnly
	if validateWarnings {
		if validateFailOn != "" && validateFailOn != "warning" {
			return oops.Errorf("--strict means --fail-on warning and conflicts with --fail-on %s", validateFailOn)
		}
		validateFailOn = "warning"
	}
	return nil
}

// runRecursiveValidate validates every discovered config, reports all failures,
// and returns 1 if any config is invalid (0 otherwise, including when none exist).
func runRecursiveValidate() int {
	configFiles := findConfigFilesRecursively()
	if len(configFiles) == 0 {
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}

	var failed []string
	var reports []*lint.Report
	var cfgs []*config.Config
	if len(validateAllowEgress) > 0 {
		var all []*config.Config
		for _, configPath := range configFiles {
			if cfg, err := validateConfigFile(configPath); err == nil {
				all = append(all, cfg)
			}
		}
		if err := checkAllowEgress(all...); err != nil {
			fmtError(err)
			return 1
		}
	}
	for _, configPath := range configFiles {
		cfg, err := validateConfigFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %s\n", configPath)
			fmtError(err)
			failed = append(failed, configPath)
			continue
		}
		progress.PrintIfNotQuiet("✅ %s\n", configPath)
		if !validateStrict {
			warnUnpinned(cfg)
			warnFrontmatter(cfg)
		}
		if validateStrict {
			report, lerr := strictLint(cfg)
			if lerr != nil {
				fmt.Fprintf(os.Stderr, "❌ %s\n", configPath)
				fmtError(lerr)
				failed = append(failed, configPath)
				continue
			}
			reports, cfgs = append(reports, report), append(cfgs, cfg)
		}
	}
	strictCode := 0
	if validateStrict {
		strictCode = reportStrict(reports, cfgs)
	}

	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ %d of %d config(s) invalid\n", len(failed), len(configFiles))
		return 1
	}
	progress.PrintIfNotQuiet("\nAll %d config(s) are valid\n", len(configFiles))
	return strictCode
}

// validateConfigFile applies the same checks as single-root validate (schema,
// then structural validation) to one config file.
func validateConfigFile(configPath string) (*config.Config, error) {
	cfg, err := loadProjectFile(cmdContext(), configPath, pluginLoadOptions(false)...)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateFile(configPath); err != nil {
		return nil, schemaFailure(cfg, err)
	}
	if err := validateLocalOverlay(cfg); err != nil {
		return nil, schemaFailure(cfg, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := checkLocalIncludes(cfg); err != nil {
		return nil, err
	}
	if !validateStrict { // a strict run reports the attempts as AR74x findings
		if err := policyGate(cfg); err != nil {
			return nil, err
		}
	}
	warnWorktreeMarketplace(cfg)
	return cfg, nil
}

// schemaFailure rewords a schema validation error as the findings `generate`
// prints for the same problems: one line per unknown key with the closest known
// key, instead of the raw "- - additionalProperties: ..." list. An error the
// findings do not explain is returned unchanged.
func schemaFailure(cfg *config.Config, err error) error {
	findings, ferr := config.SchemaFindings(cfg)
	if ferr != nil || len(findings) == 0 {
		return err
	}
	lines := make([]string, len(findings))
	var files []string
	for i, f := range findings {
		lines[i] = f.String()
		if !slices.Contains(files, f.File) {
			files = append(files, f.File)
		}
	}
	return oops.With("errors", lines).
		Hint("Fix the keys above; a \"did you mean\" names the closest known key").
		Errorf("configuration has %d unknown or invalid key(s) in %s", len(lines), strings.Join(files, ", "))
}

// validateLocalOverlay checks the config.local.* overlay, when one was merged,
// against the local overlay schema. The returned error names the overlay file.
func validateLocalOverlay(cfg *config.Config) error {
	if cfg.LocalOverlay == nil {
		return nil
	}
	if err := schema.ValidateLocalFile(cfg.LocalOverlay.Path); err != nil {
		return oops.With("path", cfg.LocalOverlay.Path).Wrapf(err, "local overlay %s", cfg.LocalOverlay.Path)
	}
	return nil
}

func displayConfigurationSummary(cfg *config.Config) {
	logger.Info("\nConfiguration summary:")

	if cfg.Content != nil {
		logger.Info("  - Rules:", "count", len(cfg.Content.Rules))
		logger.Info("  - Context files:", "count", len(cfg.Content.Context))
		logger.Info("  - Skills:", "count", len(cfg.Content.Skills))
		logger.Info("  - Domains:", "count", len(cfg.Content.Domains))

		var totalDomainRules, totalDomainContext, totalDomainSkills int
		for _, domain := range cfg.Content.Domains {
			totalDomainRules += len(domain.Rules)
			totalDomainContext += len(domain.Context)
			totalDomainSkills += len(domain.Skills)
		}
		if totalDomainRules > 0 {
			logger.Info("    - Domain rules:", "count", totalDomainRules)
		}
		if totalDomainContext > 0 {
			logger.Info("    - Domain context:", "count", totalDomainContext)
		}
		if totalDomainSkills > 0 {
			logger.Info("    - Domain skills:", "count", totalDomainSkills)
		}
	}

	logger.Info("  - Presets:", "count", len(cfg.Presets))
}

// localOverlaySummary is the one-line overlay description validate prints: the
// path and how many key paths the overlay overrides, adds and removes. It never
// includes a value, since the overlay may hold secrets.
func localOverlaySummary(cfg *config.Config) string {
	line := "local overlay: " + cfg.LocalOverlay.Path
	_, changes, err := config.DescribeLocalOverlayAt(cfg.ConfigDir)
	if err != nil {
		return line
	}
	var overridden, added, removed int
	for _, c := range changes {
		if isEntryNamePath(c.Path) {
			continue // the identity of a list entry is not a setting
		}
		isRemove, _ := c.Local.(bool) //nolint:errcheck // a non-bool value is not a remove marker
		switch {
		case strings.HasSuffix(c.Path, ".remove") && isRemove:
			removed++
		case c.HasShared:
			overridden++
		default:
			added++
		}
	}
	return fmt.Sprintf("%s (%d overridden, %d added, %d removed)", line, overridden, added, removed)
}

// isEntryNamePath reports whether path is "<named list>.<entry>.name".
func isEntryNamePath(path string) bool {
	list, rest, ok := strings.Cut(path, ".")
	return ok && isNamedListPath(list) && strings.HasSuffix(rest, ".name")
}

// ScanCmd runs the security checks only: strict validation restricted to the
// AR0xx rules, with the same flags, output and exit codes.
var ScanCmd = &cobra.Command{
	Use:   "scan [config-file]",
	Short: "Scan skills, rules and scripts for secrets, hidden text, injection and risky shell",
	Long: `Run the deterministic security checks of "validate" on their own:
secret patterns, hidden or bidirectional characters, prompt-injection phrases,
HTML comments that carry instructions, curl-pipe-shell, eval and base64 payloads,
credential access, unrestricted allowed-tools, outbound hosts outside an
allow-list, and unpinned remote sources. By default nothing is fetched or
executed. With --external the scanners configured in [[lint.external]] are run
as programs (those that declare egress = true also need --allow-egress).

Configure it in [lint] and [lint.security]. Exit codes: 0 clean, 1 the
configuration could not be loaded, 2 findings at or above --fail-on.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		validateStrict, strictSecurityOnly = true, true
		ValidateCmd.Run(cmd, args)
	},
}

func init() {
	ScanCmd.Flags().BoolVarP(&validateRecursive, "recursive", "r", false, "Scan every configuration file found recursively")
	ScanCmd.Flags().BoolVar(&validateExtern, "external", false, "Also run the scanners configured in [[lint.external]] and merge their findings")
	ScanCmd.Flags().StringSliceVar(&validateAllowEgress, "allow-egress", nil, "With --external, allow the named [[lint.external]] scanners that declare egress = true to run (repeatable)")
	addFormatFlag(ScanCmd.Flags(), &validateFormat, "", formatText, lint.Formats()...)
	ScanCmd.Flags().StringVar(&validateLintProfile, "lint-profile", "", "Lint preset: default, strict or permissive (overrides [lint] profile)")
	ScanCmd.Flags().StringVar(&validateOutput, "output", "", "Write the report to this file instead of stdout")
	ScanCmd.Flags().StringVar(&validateFailOn, "fail-on", "", "Lowest severity that exits 2: error (default), warning, info or none")
	addBaselineFlags(ScanCmd)
	addChangedFlags(ScanCmd)
	ScanCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	ScanCmd.Flags().StringVar(&validateRepoRoot, "repo-root", "", "Repository root that repo-relative paths and git-tracked globs resolve against (env AI_RULEZ_REPO_ROOT; default: the git toplevel, else the config's parent directory)")
	ScanCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}
