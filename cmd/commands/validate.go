package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

var validateRecursive bool

// applyValidateQuiet silences progress output when --quiet is set or the
// output is structured: JSON must be the only thing on stdout.
func applyValidateQuiet() {
	applyProgressMode(viper.GetBool("quiet") || structuredFormat(validateFormat))
}

var ValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate AI rules configuration and content",
	Long: `Validate an AI rules configuration for syntax errors, schema compliance and
structural issues, then run the content checks: globs that match nothing, dead
links and references, missing hooks, oversize or duplicate content and the
security rules (see the [lint] config table).

Pass --config-only to check the configuration file alone. Pass --strict to make
warnings fail the run (the same as --fail-on warning).

A project with no configuration file is linted too: the files that follow a
convention (AGENTS.md, CLAUDE.md, SKILL.md, .claude/, .agents/, .cursor/rules,
plugin manifests, hooks and MCP files) are taken as hand-written and checked,
and nothing is written. --path and --glob replace that detection with the files
you name; generate, clean and lock still need a configuration.

To leave files out: --exclude <glob> (repeatable, works without a configuration)
drops matching files from that detection, and [lint] ignore_paths in config.toml
drops the findings of matching source files. AR808 (reserved-name) checks a
hand-written skill only in the directories its harness loads from (.claude/skills,
a plugin's skills/, a local skill source); [lint] skill_roots replaces that list.

Exit codes: 0 valid, 1 the configuration is invalid or could not be loaded,
2 findings at or above --fail-on (default error).`,
	Aliases: []string{"val"},
	Args:    cobra.NoArgs,
	PreRunE: func(*cobra.Command, []string) error { return validatePreRun() },
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmdContext()
		if validateExplain != "" {
			return fail(runExplain(cmd.OutOrStdout(), validateExplain, validateFormat))
		}
		if validateShowPolicy {
			return exitStatus(runShowPolicy(ctx, cmd.OutOrStdout()))
		}
		if err := checkStrictFlags(); err != nil {
			return fail(err)
		}
		if err := applyRepoRoot(); err != nil {
			return fail(err)
		}
		// JSON output must be the only thing on stdout.
		applyValidateQuiet()

		if validateRecursive {
			return exitStatus(runRecursiveValidate())
		}

		load := loadLintConfig
		if !validateStrict {
			load = loadConfigForCommand // --config-only checks a configuration file: without one there is nothing to check
		}
		cfg, err := load(ctx, config.WithFrontmatterErrors())
		if err != nil {
			return fail(err)
		}
		warnLegacyLayout(cfg.ConfigDir)

		// Validate the raw file against the JSON schema first so a key the
		// struct would silently drop (and a value outside an enum) is reported.
		if cfg.ConfigFile != "" {
			configPath := cfg.ConfigPath()
			if err := schema.ValidateFile(configPath); err != nil {
				return fail(schemaFailure(cfg, err))
			}
		}

		if err := validateLocalOverlay(cfg); err != nil {
			return fail(schemaFailure(cfg, err))
		}
		if cfg.LocalOverlay != nil {
			progress.PrintIfNotQuiet("%s\n", localOverlaySummary(cfg))
		}

		cfg.DeferMalformedFrontmatter = validateStrict
		if err := cfg.Validate(); err != nil {
			return fail(err)
		}
		if err := checkLocalIncludes(cfg); err != nil {
			return fail(err)
		}

		if !validateStrict {
			// A strict run reports the same attempts as AR74x findings.
			if err := policyGate(ctx, cfg); err != nil {
				return fail(err)
			}
		}

		warnWorktreeMarketplace(cfg)
		if cfg.Implicit {
			// No configuration: there is no root to compare nested roots with and no
			// "valid configuration" to announce; the lint report is the result.
			return exitStatus(runStrictSingle(cfg))
		}
		// A non-recursive run checks one root: name the nested roots it skips so a
		// CI that omits --recursive does not silently pass over them.
		warnNestedRootsNotChecked(cfg)
		// A strict run defers malformed frontmatter to the AR306 finding below, which
		// would contradict a success line printed here.
		valid := len(cfg.MalformedFrontmatterPaths()) == 0
		if validateStrict {
			// "valid" is never printed for a run that exits 2.
			code := runStrictSingle(cfg)
			if code == 0 && valid {
				logger.Success("Configuration is valid", "path", cfg.ConfigDir)
			}
			return exitStatus(code)
		}
		if valid {
			logger.Success("Configuration is valid", "path", cfg.ConfigDir)
		}
		presets.WarnDuplicateContent(cfg.Log(), cfg.Content, cfg)
		warnUnpinned(cfg)
		warnFrontmatter(ctx, cfg)
		displayConfigurationSummary(cfg)
		return nil
	},
}

func init() {
	specRecursive.Bool(ValidateCmd.Flags(), &validateRecursive, "Validate every configuration file found recursively")
	ValidateCmd.Flags().BoolVar(&validateOffline, "offline", false, "Skip fetching remote includes, use cached content only (as generate --offline)")
	ValidateCmd.Flags().BoolVar(&validateConfigOnly, "config-only", false, "Check the configuration file only and skip the content checks")
	ValidateCmd.Flags().BoolVar(&validateWarnings, "strict", false, "Fail on warnings as well as errors (the same as --fail-on warning)")
	ValidateCmd.Flags().BoolVar(&validateVerifiers, "verifiers", false, "Also evaluate the verifiers (never a command or a model) and report them as AR9H findings")
	ValidateCmd.Flags().BoolVar(&validateExtern, "external", false, "Also run the scanners configured in [[lint.external]] and merge their findings")
	ValidateCmd.Flags().StringVar(&validateApprovalsBase, "approvals-base", "", "Report approvals added since this git revision for content that also changed since it (AR716)")
	ValidateCmd.Flags().StringSliceVar(&validateAllowEgress, "allow-egress", nil, "With --external, allow the named [[lint.external]] scanners that declare egress = true to run (repeatable)")
	addFormatFlag(ValidateCmd.Flags(), &validateFormat, "", formatText, lint.Formats()...) // --format implies --strict
	ValidateCmd.Flags().StringVar(&validateLintProfile, "lint-profile", "", "Lint preset: default, strict or permissive (overrides [lint] profile; distinct from the generation --profile)")
	ValidateCmd.Flags().StringSliceVar(&validateAnalyzers, "analyzer", nil, "Run only these analyzers (repeatable or comma-separated; replaces [lint] analyzers): "+strings.Join(lint.AnalyzerNames(), ", "))
	specOutput.String(ValidateCmd.Flags(), &validateOutput, "Write the report to this file instead of stdout")
	ValidateCmd.Flags().StringVar(&validateFailOn, "fail-on", "", "Lowest severity that exits 2: error (default), warning, info or none")
	ValidateCmd.Flags().BoolVar(&validateMCPUserScope, "mcp-user-scope", false, "Also check MCP servers against the configured harnesses' user and local scopes (reads files outside the project, for example ~/.claude.json); needs --strict")
	ValidateCmd.Flags().StringVar(&validateExplain, "explain", "", "Print what a rule (code or name, for example AR001) checks, why, examples and how to suppress it, then exit")
	addBaselineFlags(ValidateCmd)
	addTargetFlags(ValidateCmd)
	addChangedFlags(ValidateCmd)
	addFixFlags(ValidateCmd)
	specNoLocal.Bool(ValidateCmd.Flags(), &noLocal, "Ignore the machine-local config.local.* overlay and local/ content (the view a teammate without them sees)")
	ValidateCmd.Flags().StringVar(&validateRepoRoot, "repo-root", "", "Repository root that repo-relative paths and git-tracked globs resolve against (env AI_RULEZ_REPO_ROOT; default: the git toplevel, else the config's parent directory)")
}

// validatePreRun resolves --config-only and --strict into the internal state.
func validatePreRun() error {
	if validateOffline {
		cliLockPolicy.Offline = true
	}
	if validateWarnings && validateConfigOnly {
		return oops.Errorf("--strict (warnings fail) needs the content checks: drop --config-only")
	}
	validateStrict = !validateConfigOnly
	if validateWarnings {
		if validateFailOn != "" && validateFailOn != failOnWarning {
			return oops.Errorf("--strict means --fail-on warning and conflicts with --fail-on %s", validateFailOn)
		}
		validateFailOn = failOnWarning
	}
	return nil
}

// runRecursiveValidate validates every discovered config, reports all failures,
// and returns 1 if any config is invalid (0 otherwise). A tree with no config is
// linted as an implicit root by the content checks (see runImplicitRecursive).
func runRecursiveValidate() int { //nolint:gocyclo // linear per-root reporting; splitting it hides the flow
	// The same discovery `generate --recursive` counts, so the two summaries
	// agree on how many roots the tree holds.
	configFiles := recursiveConfigRoots()
	if len(configFiles) == 0 {
		if validateStrict {
			return runImplicitRecursive()
		}
		progress.PrintlnIfNotQuiet("No configuration files found")
		return 0
	}

	var failed []string
	loosened := 0 // failures that are a policy loosening, which exits 2 like a single root
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
			renderError(os.Stderr, err)
			return 1
		}
	}
	// The roots are checked concurrently over one shared view of the git index;
	// everything they print is reported here, in root order.
	ctx := gitutil.WithSharedIndex(cmdContext())
	forEachRootBounded(ctx, recursiveValidateWorkers(), configFiles, func(configPath string) validatedRoot { return validateRoot(ctx, configPath) },
		func(i int, configPath string, r validatedRoot, elapsed time.Duration) {
			defer reportRootProgress(i, len(configFiles), configPath, elapsed)
			r.notes.flush(logger.Std(), os.Stderr)
			if r.cfg != nil {
				warnLegacyLayout(r.cfg.ConfigDir)
			}
			if r.err != nil {
				fmt.Fprintf(os.Stderr, "❌ %s\n", configPath)
				renderError(os.Stderr, r.err)
				failed = append(failed, configPath)
				if errors.Is(r.err, config.ErrPolicyLoosens) {
					loosened++
				}
				return
			}
			warnWorktreeMarketplace(r.cfg)
			progress.PrintIfNotQuiet("✅ %s\n", configPath)
			if !validateStrict {
				warnUnpinned(r.cfg)
				warnFrontmatter(cmdContext(), r.cfg)
				return
			}
			if r.lintErr != nil {
				fmt.Fprintf(os.Stderr, "❌ %s\n", configPath)
				renderError(os.Stderr, r.lintErr)
				failed = append(failed, configPath)
				return
			}
			reports, cfgs = append(reports, r.report), append(cfgs, r.cfg)
		})
	if validateStrict && targetFlagsSet() {
		rep, cfg, err := lintOrphanPicks(ctx, rootBases(cfgs))
		if err != nil {
			renderError(os.Stderr, err)
			return 1
		}
		if rep != nil {
			reports, cfgs = append(reports, rep), append(cfgs, cfg)
		}
	}
	if err := ctx.Err(); err != nil {
		renderError(os.Stderr, oops.Wrapf(err, "validate interrupted before every root was checked"))
		return 1
	}
	strictCode := 0
	if validateStrict {
		strictCode = reportStrictCtx(ctx, reports, cfgs)
	}

	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ %d of %d config(s) invalid\n", len(failed), len(configFiles))
		if loosened == len(failed) {
			return exitCodeFor(config.ErrPolicyLoosens)
		}
		return 1
	}
	if strictCode != 0 {
		// The run exits non-zero on findings, so "all valid" would contradict it.
		// The report above already listed them; this names the count checked.
		fmt.Fprintf(os.Stderr, "\n❌ %d config(s) checked; findings were reported\n", len(configFiles))
		return strictCode
	}
	progress.PrintIfNotQuiet("\nAll %d config(s) are valid\n", len(configFiles))
	return strictCode
}

// runImplicitRecursive is what a recursive run does when the tree holds no
// configuration: it lints the implicit root exactly as a run without --recursive
// would, so a CI that adds --recursive never passes a repository unchecked.
func runImplicitRecursive() int {
	cfg, err := loadLintConfig(cmdContext(), config.WithFrontmatterErrors())
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	cfg.DeferMalformedFrontmatter = true
	if err := cfg.Validate(); err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	return runStrictSingle(cfg)
}

// rootBases returns the directory of each loaded root.
func rootBases(cfgs []*config.Config) []string {
	bases := make([]string, 0, len(cfgs))
	for _, cfg := range cfgs {
		bases = append(bases, cfg.BaseDir)
	}
	return bases
}

// lintOrphanPicks lints the files named with --path or --glob that no root of a
// recursive run covers, each root keeping only the picks below its own directory.
// They are linted once at the repository root with the implicit configuration
// (no root's configuration applies to them) and named in the notices. The report
// is nil when every pick belongs to a root.
func lintOrphanPicks(ctx context.Context, bases []string) (*lint.Report, *config.Config, error) {
	cfg, err := loadLintConfig(ctx, config.WithFrontmatterErrors())
	if err != nil {
		return nil, nil, err
	}
	if !cfg.Implicit {
		return nil, nil, nil // a configuration at the top is a root: it lints every pick
	}
	cfg.DeferMalformedFrontmatter = true
	if err := cfg.Validate(); err != nil {
		return nil, nil, err //nolint:wrapcheck // already contextual
	}
	cfg.Content = &config.ContentTree{} // the named files only
	report, err := strictLint(withUnmanagedLint(withOrphanRoots(ctx, bases)), cfg)
	if err != nil || report == nil {
		return nil, nil, err
	}
	return report, cfg, nil
}

// recursiveValidateWorkers is how many roots a recursive validate checks at once.
// The external scanners share the terminal (a dry run prints their plan to
// stdout) and one baseline file (--write-baseline), neither of which tolerates
// concurrent writers, so --external checks one root at a time.
func recursiveValidateWorkers() int {
	if validateExtern {
		return 1
	}
	return runtime.GOMAXPROCS(0)
}

// warnNestedRootsNotChecked tells a non-recursive run how many other
// configuration roots the working tree holds, so a CI that omits --recursive
// does not silently pass over them. The notice goes through the progress stream
// (PrintIfNotQuiet), so --quiet and a structured --format suppress it exactly as
// they do the other notices, and the exit code is unchanged. --recursive never
// calls it: that run checks every root.
func warnNestedRootsNotChecked(cfg *config.Config) {
	current := filepath.Clean(cfg.ConfigPath())
	nested := 0
	for _, path := range findConfigFilesRecursively() {
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if filepath.Clean(abs) == current {
			continue
		}
		nested++
	}
	if nested == 0 {
		return
	}
	progress.PrintIfNotQuiet("%d nested root(s) not checked; run with --recursive\n", nested)
}

// validateConfigFile applies the same checks as single-root validate (schema,
// then structural validation) to one config file.
func validateConfigFile(configPath string) (*config.Config, error) {
	cfg, err := checkConfigFile(cmdContext(), configPath)
	if cfg != nil {
		warnLegacyLayout(cfg.ConfigDir)
	}
	if err != nil {
		return nil, err
	}
	warnWorktreeMarketplace(cfg)
	return cfg, nil
}

// validatedRoot is what validating one root of a recursive run produced. It is
// built concurrently with the other roots and reported afterwards, in root order.
type validatedRoot struct {
	cfg     *config.Config // set once the config loaded, even when err is set
	err     error
	report  *lint.Report // strict runs only
	lintErr error
	notes   *rootNotes // what the root's work logged, printed in root order
}

// validateRoot checks one root, and under --strict lints it.
func validateRoot(ctx context.Context, configPath string) validatedRoot {
	var r validatedRoot
	ctx, r.notes = withRootNotes(ctx)
	r.cfg, r.err = checkConfigFile(ctx, configPath)
	if r.err == nil && validateStrict {
		r.report, r.lintErr = strictLint(withUnmanagedLint(ctx), r.cfg)
	}
	return r
}

// checkConfigFile is validateConfigFile without the notices: it prints nothing.
// cfg is set as soon as the config loaded, so the caller can warn about its
// layout even when a later check fails.
func checkConfigFile(ctx context.Context, configPath string) (*config.Config, error) {
	cfg, err := loadProjectFile(ctx, configPath, pluginLoadOptions(false)...)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateFile(configPath); err != nil {
		return cfg, schemaFailure(cfg, err)
	}
	if err := validateLocalOverlay(cfg); err != nil {
		return cfg, schemaFailure(cfg, err)
	}
	cfg.DeferMalformedFrontmatter = validateStrict
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	if err := checkLocalIncludes(cfg); err != nil {
		return cfg, err
	}
	if !validateStrict { // a strict run reports the attempts as AR74x findings
		if err := policyGate(ctx, cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

// schemaFailure rewords a schema validation error as the findings `generate`
// prints for the same problems: one line per unknown key with the closest known
// key, instead of the raw "- - additionalProperties: ..." list. An error the
// findings do not explain is returned unchanged.
func schemaFailure(cfg *config.Config, err error) error {
	if perr := config.UnknownPresetError(cfg); perr != nil {
		return perr // a preset typo: one line and a suggestion, not the schema's alternatives
	}
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
	Use:   "scan",
	Short: "Scan skills, rules and scripts for secrets, hidden text, injection and risky shell",
	Long: `Run the deterministic security checks of "validate" on their own:
secret patterns, hidden or bidirectional characters, prompt-injection phrases,
HTML comments that carry instructions, curl-pipe-shell, eval and base64 payloads,
credential access, unrestricted allowed-tools, outbound hosts outside an
allow-list, unpinned remote sources, anti-refusal and context-exfiltration
prose, persistence, unsafe deserialization, requests to internal endpoints,
malware-shaped content, shipped executables, dependency manifests and MCP
metadata. By default nothing is fetched or
executed. With --external the scanners configured in [[lint.external]] are run
as programs (those that declare egress = true also need --allow-egress).

Configure it in [lint] and [lint.security]. Exit codes: 0 clean, 1 the
configuration could not be loaded, 2 findings at or above --fail-on.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		validateStrict, strictSecurityOnly = true, true
		return ValidateCmd.RunE(cmd, args)
	},
}

func init() {
	specRecursive.Bool(ScanCmd.Flags(), &validateRecursive, "Scan every configuration file found recursively")
	ScanCmd.Flags().BoolVar(&validateExtern, "external", false, "Also run the scanners configured in [[lint.external]] and merge their findings")
	ScanCmd.Flags().StringSliceVar(&validateAllowEgress, "allow-egress", nil, "With --external, allow the named [[lint.external]] scanners that declare egress = true to run (repeatable)")
	addFormatFlag(ScanCmd.Flags(), &validateFormat, "", formatText, lint.Formats()...)
	ScanCmd.Flags().StringVar(&validateLintProfile, "lint-profile", "", "Lint preset: default, strict or permissive (overrides [lint] profile)")
	specOutput.String(ScanCmd.Flags(), &validateOutput, "Write the report to this file instead of stdout")
	ScanCmd.Flags().StringVar(&validateFailOn, "fail-on", "", "Lowest severity that exits 2: error (default), warning, info or none")
	addBaselineFlags(ScanCmd)
	addTargetFlags(ScanCmd)
	addChangedFlags(ScanCmd)
	specNoLocal.Bool(ScanCmd.Flags(), &noLocal, "Ignore the machine-local config.local.* overlay and local/ content")
	ScanCmd.Flags().StringVar(&validateRepoRoot, "repo-root", "", "Repository root that repo-relative paths and git-tracked globs resolve against (env AI_RULEZ_REPO_ROOT; default: the git toplevel, else the config's parent directory)")
}
