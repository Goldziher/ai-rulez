package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

var (
	publishTo         string
	publishDist       string
	publishTag        string
	publishRepo       string
	publishFormat     string
	publishDryRun     bool
	publishExecute    bool
	publishYes        bool
	publishForce      bool
	publishAllowDirty bool
	publishTemplates  []string
	// publishRunner starts gh and git; nil means a real process. Tests set it.
	publishRunner runner.Runner
)

// ghEnvPass are the variables gh needs to authenticate and reach GitHub. They
// are passed through to gh untouched; ai-rulez never reads their values.
var ghEnvPass = []string{
	"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_HOST",
	"GH_CONFIG_DIR", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR",
}

// PublishCmd packages the verified plugin bundle into release artifacts.
var PublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Package the plugin bundle into deterministic, checksummed release artifacts",
	Long: `Turn the generated plugin bundle into release artifacts in a local dist directory:
a reproducible tar.gz, <name>-<version>.manifest.json, SHA256SUMS, a copy of
ai-rulez.lock, RELEASE_NOTES.md and publish-plan.json.

Preflight runs first and stops before anything is written: validate --strict,
lock --check, verify --plugin and a secret scan of the bundle. The archive is
byte-identical for the same bundle and commit (sorted entries, fixed mtime,
uid/gid 0, normalised modes, no gzip name or time). The mtime is
SOURCE_DATE_EPOCH, else the committer time of HEAD, else 0.

Nothing leaves the machine unless --execute --yes is given with --to
github-release: ai-rulez then runs the argv printed in publish-plan.json
through the gh CLI. Credentials are gh's own (GH_TOKEN or gh auth login);
ai-rulez never reads them. --dry-run prints the artifacts and commands and
writes nothing.

--template FILE renders a text/template (fields: Name, Version, Tag, Repo,
Commit, AIRulezVersion, Runtimes, BundleFile, BundleDigest, BundleSize,
LockTree, LockDigest, Files; function: json) into <dist>/emit/ for an operator
to upload to a channel ai-rulez has no native emitter for.

Exit codes: 0 done, 1 the run could not complete, 2 a gate failed.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		exitPublish(runPublish(ctx, cmd.OutOrStdout()))
	},
}

var publishVerifyCmd = &cobra.Command{
	Use:   "verify <dir>",
	Short: "Recompute the checksums, manifest and archive of a dist directory",
	Long: `Verify a dist directory (or a downloaded release) offline: every SHA256SUMS entry,
the manifest against the archive's files, the lock copy against the manifest, and
the archive's determinism rules (sorted entries, uid/gid 0, normalised modes).

Exit codes: 0 verified, 1 the directory cannot be read, 2 a mismatch.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		exitPublish(runPublishVerify(cmd.OutOrStdout(), args[0]))
	},
}

func init() {
	f := PublishCmd.Flags()
	f.StringVar(&publishTo, "to", "", "Upload target: github-release (default: build only)")
	f.StringVar(&publishDist, "dist", "dist", "Directory the artifacts are written to")
	f.StringVar(&publishTag, "tag", "", "Release tag (default: v<[plugin] version>); it must already exist on the remote")
	f.StringVar(&publishRepo, "repo", "", "OWNER/REPO of the release (default: [plugin] repository, else the origin remote)")
	f.BoolVar(&publishDryRun, "dry-run", false, "Run preflight and print the artifacts and commands without writing or running anything")
	f.BoolVar(&publishExecute, "execute", false, "Run the upload through gh (needs --to and --yes)")
	f.BoolVar(&publishYes, "yes", false, "Confirm --execute without a prompt")
	f.BoolVar(&publishForce, "force", false, "With --execute, replace the assets of an existing release instead of refusing")
	f.BoolVar(&publishAllowDirty, "allow-dirty", false, "Publish from a tree with uncommitted changes or no commit")
	f.StringArrayVar(&publishTemplates, "template", nil, "Render this text/template into <dist>/emit/ (repeatable)")
	f.StringVarP(&profile, "profile", "p", "", "Profile used to generate the plugin bundle")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	addFormatFlag(f, &publishFormat, formatText, formatText, formatText, formatJSON)
	addFormatFlag(publishVerifyCmd.Flags(), &publishFormat, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(f)
	addJSONFlagAlias(publishVerifyCmd.Flags())
	PublishCmd.AddCommand(publishVerifyCmd)
}

// exitPublish prints err and exits with the status it carries.
func exitPublish(err error) {
	if err == nil {
		return
	}
	var pe *publish.Error
	if errors.As(err, &pe) {
		fmt.Fprintf(os.Stderr, "Error: %s\n", pe.Error())
		if pe.Hint != "" {
			fmt.Fprintf(os.Stderr, "\nHint: %s\n", pe.Hint)
		}
		os.Exit(pe.Exit)
	}
	fmtError(err)
	os.Exit(1)
}

func checkPublishFlags() error {
	if err := checkFormatFlag(publishFormat); err != nil {
		return err
	}
	switch {
	case publishTo != "" && publishTo != publish.TargetGitHubRelease:
		return oops.Errorf("unknown --to %q (use %s)", publishTo, publish.TargetGitHubRelease)
	case publishExecute && publishDryRun:
		return oops.Errorf("--execute and --dry-run cannot be combined")
	case publishExecute && publishTo == "":
		return oops.Errorf("--execute needs --to github-release")
	case publishExecute && !publishYes:
		return oops.Hint("review the commands with --dry-run, then pass --yes").Errorf("--execute needs --yes")
	case publishForce && !publishExecute:
		return oops.Errorf("--force only applies with --execute")
	case (publishTag != "" || publishRepo != "") && publishTo == "":
		return oops.Errorf("--tag and --repo need --to github-release")
	}
	return nil
}

func runPublish(ctx context.Context, out io.Writer) error {
	if err := checkPublishFlags(); err != nil {
		return err
	}
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutLocal())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if cfg.Plugin == nil {
		return publish.Errorf(publish.CodeSource, publish.ExitGate, "add a [plugin] block with name and version; marketplace-only roots are not published yet", "no [plugin] block is configured")
	}
	if err := publish.ValidateName(cfg.Plugin.Name, cfg.Plugin.Version); err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	distAbs, err := filepath.Abs(publishDist)
	if err != nil {
		return oops.Wrapf(err, "resolve --dist")
	}
	files, err := publishPreflight(cfg)
	if err != nil {
		return err
	}
	in, err := publishInput(ctx, cfg, files, distAbs)
	if err != nil {
		return err
	}
	dist, err := publish.Build(*in)
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	if !publishDryRun {
		if err := dist.Write(distAbs); err != nil {
			return err //nolint:wrapcheck // a publish.Error carries the exit status
		}
	}
	if err := printPublish(out, dist, distAbs); err != nil {
		return err
	}
	if !publishExecute {
		return nil
	}
	url, err := publish.Execute(ctx, publishRunner, dist.Plan, publish.ExecuteOptions{
		Dir: distAbs, Env: runner.ScrubEnv(os.Environ(), ghEnvPass, nil), Force: publishForce,
	})
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	if url != "" {
		logger.Success("Published", "tag", dist.Plan.Tag, "repo", dist.Plan.Repo, "result", url)
	} else {
		logger.Success("Published", "tag", dist.Plan.Tag, "repo", dist.Plan.Repo)
	}
	return nil
}

// publishPreflight runs the four gates and returns the verified bundle files.
func publishPreflight(cfg *config.Config) ([]generator.PluginFile, error) {
	if err := strictGate(cfg); err != nil {
		return nil, err
	}
	logger.Info("preflight: validate --strict ok")
	if code := checkLockAt(""); code != 0 {
		exit := publish.ExitFailed
		if code == exitDrift {
			exit = publish.ExitGate
		}
		return nil, publish.Errorf(publish.CodePreflight, exit, "run `ai-rulez lock` after reviewing `ai-rulez lock --diff`", "lock --check failed")
	}
	gen := generator.NewGenerator(cfg)
	if err := gen.VerifyPlugin(profile); err != nil {
		return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "run `ai-rulez generate --plugin` and commit the result", "verify --plugin failed: %v", err)
	}
	logger.Info("preflight: verify --plugin ok")
	files, err := gen.PluginFiles(profile)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	if err := publish.CheckTree(cfg.BaseDir, paths); err != nil {
		return nil, err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	if err := secretGate(files); err != nil {
		return nil, err
	}
	logger.Info("preflight: secret scan ok", "files", len(files))
	return files, nil
}

// strictGate is `validate --strict` as a gate: the same lint, baseline and
// budget handling, failing at the configured threshold but never above error.
func strictGate(cfg *config.Config) error {
	report, err := strictLint(cfg)
	if err != nil {
		return publish.Errorf(publish.CodePreflight, publish.ExitFailed, "", "validate --strict could not run: %v", err)
	}
	reports, cfgs := []*lint.Report{report}, []*config.Config{cfg}
	excess, _, done := prepareReports(reports, cfgs)
	if done {
		return publish.Errorf(publish.CodePreflight, publish.ExitFailed, "", "validate --strict could not run")
	}
	threshold := failOnFor(cfg)
	if threshold == "none" {
		threshold = "error"
	}
	if lint.FailedWithExcess(report.Findings, threshold, budgetsFor(cfg), excess[0]) || baselineBlocks(reports) {
		if werr := lint.Write(os.Stderr, lint.FormatText, lint.Combine(reports), lint.WriteOptions{Version: Version, FailOn: threshold}); werr != nil {
			logger.Warn("Could not print the findings", "error", werr)
		}
		return publish.Errorf(publish.CodePreflight, publish.ExitGate, "fix the findings above (see `ai-rulez validate --strict`)", "validate --strict reported findings")
	}
	return nil
}

// secretGate scans every bundle file with the security scan's secret patterns.
// The report names the file and the pattern, never the value.
func secretGate(files []generator.PluginFile) error {
	var hits []string
	for _, f := range files {
		if name, ok := lint.DetectSecret(string(f.Data)); ok {
			hits = append(hits, f.Path+" ("+name+")")
		}
	}
	if len(hits) == 0 {
		return nil
	}
	return publish.Errorf(publish.CodeSecret, publish.ExitGate, "remove the value, rotate the credential and regenerate the bundle",
		"the bundle contains credentials: %s", strings.Join(hits, ", "))
}

func publishInput(ctx context.Context, cfg *config.Config, files []generator.PluginFile, distAbs string) (*publish.Input, error) {
	lockPath := lockfile.Path(cfg.ConfigDir)
	if err := publish.CheckTree(filepath.Dir(lockPath), []string{filepath.Base(lockPath)}); err != nil {
		return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "run `ai-rulez lock`", "no usable %s: %v", lockfile.FileName, err)
	}
	lockBytes, err := os.ReadFile(lockPath) //nolint:gosec // the project's own lock file
	if err != nil {
		return nil, oops.With("path", lockPath).Wrapf(err, "read lock file")
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil || lock == nil {
		return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "run `ai-rulez lock`", "cannot read %s", lockfile.FileName)
	}
	lockBytes, err = shippedLock(lockBytes, lock)
	if err != nil {
		return nil, err
	}
	distRel := ""
	if top := gitutil.New(publishRunner).TopLevel(cfg.BaseDir); top != "" {
		distRel = gitutil.RepoRelative(top, distAbs)
	}
	src := publish.ReadSource(ctx, publishRunner, cfg.BaseDir, distRel)
	if src.Source.Dirty && !publishAllowDirty {
		return nil, publish.Errorf(publish.CodeSource, publish.ExitGate, "commit the changes (including the generated bundle), or pass --allow-dirty for a throwaway build",
			"the source tree is dirty or has no commit")
	}
	src.Source.Repo = publish.StripCredentials(cfg.Plugin.Repository)
	if src.Source.Repo == "" {
		src.Source.Repo = src.Remote
	}
	mtime, err := sourceDateEpoch(src.Mtime)
	if err != nil {
		return nil, err
	}
	in := &publish.Input{
		Name: cfg.Plugin.Name, Version: cfg.Plugin.Version, AIRulezVersion: Version,
		Runtimes: cfg.Plugin.ResolvedRuntimes(), Lock: lockBytes, LockVersion: lock.Version, LockTree: lock.Tree,
		Source: src.Source, Mtime: mtime, Target: publishTo,
	}
	for _, f := range files {
		in.Files = append(in.Files, publish.File{Path: f.Path, Data: f.Data, Executable: f.Executable})
	}
	if publishTo != "" {
		in.Tag = publishTag
		if in.Tag == "" {
			in.Tag = "v" + cfg.Plugin.Version
		}
		in.Repo = publishRepo
		if in.Repo == "" {
			in.Repo = publish.RepoFromURL(cfg.Plugin.Repository)
		}
		if in.Repo == "" {
			in.Repo = publish.RepoFromURL(src.Remote)
		}
	}
	for _, path := range publishTemplates {
		body, rerr := os.ReadFile(path) //nolint:gosec // an explicit --template chosen by the user
		if rerr != nil {
			return nil, oops.With("path", path).Wrapf(rerr, "read template")
		}
		in.Templates = append(in.Templates, publish.Template{Name: filepath.Base(path), Body: string(body)})
	}
	return in, nil
}

// shippedLock returns the lock bytes that go into the dist directory. Reviewer
// approvals (emails, notes) are repository-internal and outside the tree
// digest, so the shipped copy is the same lock without its [[approval]]
// records: its tree, content pins and output pins are unchanged and
// `lock --check` against it behaves as against the original. A lock without
// approvals ships byte for byte.
func shippedLock(raw []byte, lock *lockfile.File) ([]byte, error) {
	if len(lock.Approval) == 0 {
		return raw, nil
	}
	tmp, err := os.MkdirTemp("", "ai-rulez-lock-*")
	if err != nil {
		return nil, oops.Wrapf(err, "create temporary directory")
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // best effort cleanup of our own directory
	stripped := *lock
	stripped.Approval = nil
	if err := lockfile.Save(tmp, &stripped); err != nil {
		return nil, oops.Wrapf(err, "render the shipped lock copy")
	}
	out, err := os.ReadFile(lockfile.Path(tmp)) //nolint:gosec // the file Save just wrote in our temp directory
	if err != nil {
		return nil, oops.Wrapf(err, "read the shipped lock copy")
	}
	return out, nil
}

// sourceDateEpoch resolves the archive mtime: SOURCE_DATE_EPOCH, else commitTime.
func sourceDateEpoch(commitTime int64) (int64, error) {
	raw := os.Getenv("SOURCE_DATE_EPOCH")
	if raw == "" {
		return commitTime, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, oops.Errorf("SOURCE_DATE_EPOCH must be a non-negative integer, got %q", raw)
	}
	return n, nil
}

func printPublish(out io.Writer, d *publish.Dist, dir string) error {
	if publishFormat == formatJSON {
		_, err := out.Write(d.Files[publish.PlanFile])
		return oops.Wrapf(err, "write plan")
	}
	verb := "wrote"
	if publishDryRun {
		verb = "would write"
	}
	fmt.Fprintf(out, "preflight   validate --strict ok | lock ok | verify --plugin ok | secrets 0\n")
	fmt.Fprintf(out, "artifacts   %s %d files to %s\n", verb, len(d.Files), dir)
	for _, a := range d.Plan.Artifacts {
		fmt.Fprintf(out, "            %-40s %s  %d bytes\n", a.Path, a.Digest, a.Size)
	}
	fmt.Fprintf(out, "            %-40s %s  %d bytes\n", publish.PlanFile, publish.Digest(d.Files[publish.PlanFile]), len(d.Files[publish.PlanFile]))
	for _, c := range d.Plan.Commands {
		verb := "would run"
		if publishExecute {
			verb = "running"
		}
		fmt.Fprintf(out, "%-11s %s\n", verb, shellJoin(c.Argv))
	}
	if d.Plan.Credentials != "" {
		fmt.Fprintf(out, "credentials %s\n", d.Plan.Credentials)
	}
	return nil
}

// shellJoin renders argv so it can be pasted into a POSIX shell: arguments with
// characters outside a safe set are single-quoted. The argv itself never goes
// through a shell.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./_-") == "" {
			parts[i] = a
			continue
		}
		parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}

func runPublishVerify(out io.Writer, dir string) error {
	if err := checkFormatFlag(publishFormat); err != nil {
		return err
	}
	res, err := publish.Verify(dir)
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	if publishFormat == formatJSON {
		data, merr := json.MarshalIndent(res, "", "  ")
		if merr != nil {
			return oops.Wrapf(merr, "encode result")
		}
		if _, werr := out.Write(append(data, '\n')); werr != nil {
			return oops.Wrapf(werr, "write result")
		}
	} else if res.OK() {
		fmt.Fprintf(out, "verified %s %s: %d files match SHA256SUMS, the manifest and the archive\n", res.Name, res.Version, res.Files)
	}
	if !res.OK() {
		if publishFormat != formatJSON {
			for _, p := range res.Problems {
				fmt.Fprintf(os.Stderr, "%s: %s\n", p.Path, p.Message)
			}
		}
		return publish.Errorf(publish.CodeVerify, publish.ExitGate, "", "%d mismatch(es) in %s", len(res.Problems), dir)
	}
	return nil
}
