package commands

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"

	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// Exit codes of convert: 1 the conversion could not run (or would overwrite
// existing files), 2 the scan or validation blocked it or --fail-on matched.
const (
	exitConvertCannotRun = 1
	exitConvertBlocked   = 2
)

var (
	convertFrom          []string
	convertSource        string
	convertInto          string
	convertDomain        string
	convertDryRun        bool
	convertWrite         bool
	convertForce         bool
	convertReport        string
	convertFormat        string
	convertFailOn        []string
	convertBestEffort    bool
	convertSplitHeadings bool
	convertList          bool
	convertAllowFindings []string
	convertEnableHooks   bool
	convertEnablePerms   bool
	convertMerge         bool
	convertKeepNames     bool
	convertLock          bool
	convertDelivery      string
	convertFetch         bool
)

// ConvertCmd converts another tool's configuration into an .ai-rulez/ tree.
var ConvertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert existing AI tool files into an .ai-rulez/ tree",
	Long: `Read another tool's configuration and produce an equivalent .ai-rulez/ tree,
with a report that lists every construct as mapped, approximated, dropped,
needs-action or unsupported.

Importers (--from, comma separated, default auto, which runs every importer that
detects something, skills-lock first):
  agent-plugins an Agent Plugins directory (agent-plugins.org): plugin.json becomes the
                [plugin] block, skills/ the skills, mcp.json the [[mcp_servers]], and the
                Claude Code and ai-rulez extension namespaces agents, commands and rules
  native        CLAUDE.md, AGENTS.md, GEMINI.md, .cursor/rules, .github/instructions,
                .kiro/steering, .windsurf, .roo, .clinerules, .qwen, .junie,
                .agents/skills, skills, agents and commands of every supported preset,
                MCP files (.mcp.json, .cursor/mcp.json, .vscode/mcp.json, ...), and the
                hooks and permissions of .claude/settings.json, .codex/hooks.json,
                .gemini/settings.json, .cursor/hooks.json, .cursor/cli.json and .github/hooks
  rulesync      rulesync.jsonc and .rulesync/ (rules, commands, subagents, skills, checks,
                mcp.jsonc, hooks.jsonc, permissions.jsonc, ignore)
  apm           Microsoft APM: apm.yml (dependencies, MCP servers, target), .apm/
                primitives (instructions, agents, chatmodes, prompts, skills, context,
                hooks), installed apm_modules/ and apm.lock.yaml
  okf           an OKF bundle (index.md naming okf_version) at the source root or in
                docs/okf: the mapping of ` + "`ai-rulez import okf`" + `, with convert's report,
                scan and write. --domain places the bundle in a domain
  skills-lock   skills-lock.json of the Vercel skills CLI, as [[installed_skills]]
  tessl         tessl.json and the vendored .tessl/plugins/<workspace>/<plugin>/
                skills and rules; each eval scenario (task.md and criteria.json) becomes
                a *.eval.yaml case of the plugin's skill. A plugin that is not on disk is
                reported: nothing is fetched from the registry

When a rulesync, APM or Tessl project is detected, auto skips native: the tool files next
to their inputs are generated output. Use --from native,rulesync to read both.

Nothing is written unless --write is given. Without --write or --dry-run, a
terminal gets a dry run and a script is asked to choose. Existing .ai-rulez/
files are never overwritten without --force; --domain NAME imports beside them.
An existing config.toml is never replaced, not even with --force: new presets,
[[mcp_servers]] and [[installed_skills]] are merged into it (existing entries
win). --into is relative to --source unless absolute, and nothing is written
through a symlink at or below it. Source files are never modified, and nothing
is fetched (unless --fetch is given) or executed. Imported hooks never run on their own: they are written
to config.toml as a commented block, enabled by --enable-hooks (an imported allow
rule likewise needs --enable-permissions; ask and deny rules are live because they
only narrow). The converted tree is validated and security-scanned in
a scratch directory first, and a blocked scan writes nothing. Scan findings name
the source file and line they came from (the planned .ai-rulez path is added in
parentheses); --allow-findings CODE lets a code through.

--merge adds beside an existing tree without touching any of its files: an item
whose file exists with other content is imported as NAME-imported. --keep-names
never renames to settle a collision; it is reported instead. --delivery
static|served|both sets how the imported skills reach the agent ([skills]
delivery, or the domain's with --domain). --fetch reads the remote git sources of
the input (https only, through the skill-source fetcher): rulesync sources become
[[installed_skills]] pinned to the commit that was read, their rules and APM
packages are copied, and everything fetched is scanned before the write; a
failed fetch writes nothing. --lock runs ` + "`ai-rulez lock`" + ` after the write
to pin remote sources, authored content and outputs; convert never imports a
foreign lock hash.

Exit codes: 0 done, 1 could not run or would overwrite, 2 blocked by the scan or
validation, or --fail-on matched.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return exitStatus(runConvert(watchParentContext(cmd), os.Stdout, stdoutIsTerminal()))
	},
}

func init() {
	f := ConvertCmd.Flags()
	f.StringSliceVar(&convertFrom, "from", []string{"auto"}, "Importers to run: native, rulesync, apm, tessl, okf, skills-lock, agent-plugins or auto (every detected importer)")
	f.StringVar(&convertSource, "source", ".", "Directory to read")
	f.StringVar(&convertInto, "into", importer.DefaultConfigDir, "Config directory to write: relative to --source unless absolute; never written through a symlink")
	f.StringVar(&convertDomain, "domain", "", "Put the imported content in this domain (safe next to an existing tree)")
	f.BoolVar(&convertDryRun, "dry-run", false, "Print the plan and report; write nothing")
	f.BoolVar(&convertWrite, "write", false, "Write the converted tree")
	f.BoolVar(&convertForce, "force", false, "Overwrite existing content files that differ (config.toml is always merged, never replaced)")
	f.StringVar(&convertReport, "report", "", "Also write the report (in --format) to this file")
	addFormatFlag(f, &convertFormat, formatText, formatText, formatText, formatJSON)
	f.StringSliceVar(&convertFailOn, "fail-on", nil, "Exit 2 when a finding has one of these statuses: approximated, dropped, needs-action, unsupported")
	f.BoolVar(&convertBestEffort, "best-effort", false, "Import the known fields of an unrecognized format version")
	f.BoolVar(&convertSplitHeadings, "split-headings", false, "Split root files such as CLAUDE.md into one context per H2 heading")
	f.StringSliceVar(&convertAllowFindings, "allow-findings", nil, "Write despite security findings of these codes (for example AR001, a secret in the source); they stay in the report. Discouraged")
	f.BoolVar(&convertMerge, "merge", false, "Add beside an existing tree without touching a file of it: an item whose file exists with other content is imported as NAME-imported (excludes --force)")
	f.BoolVar(&convertKeepNames, "keep-names", false, "Never rename to resolve a name collision (between imported items, or with an existing file under --merge): report it instead")
	f.StringVar(&convertDelivery, "delivery", "", "How the imported skills reach the agent: static, served or both ([skills] delivery, or the domain's with --domain)")
	f.BoolVar(&convertFetch, "fetch", false, "Read the remote git sources the input names (rulesync sources, APM dependencies that are not installed) over the network: https only, scanned before anything is written, skills pinned to the commit that was read. Without it nothing is fetched and each source is reported")
	f.BoolVar(&convertLock, "lock", false, "After writing, run ai-rulez lock on the converted config to pin remote sources, authored content and outputs (needs --write)")
	f.BoolVar(&convertEnableHooks, "enable-hooks", false, "Write imported hooks as live [[hooks]]; without it they are a commented block you review first (a hook runs a command on your machine)")
	f.BoolVar(&convertEnablePerms, "enable-permissions", false, "Write imported allow rules as live [permissions]; without it they are commented (an allow applies to every harness). Ask and deny rules are always live")
	f.BoolVar(&convertList, "list", false, "List the importers and what each detects in --source")
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var allowCodeRe = regexp.MustCompile(`(?i)^AR[0-9A-Z]{3,4}$`)

// checkConvertFlags rejects flag combinations convert cannot honor.
func checkConvertFlags(interactive bool) error {
	switch {
	case convertWrite && convertDryRun:
		return oops.Errorf("--write and --dry-run cannot be combined")
	case !convertWrite && !convertDryRun && !interactive:
		return oops.Errorf("pass --write to convert or --dry-run to preview; a script never converts by surprise")
	case convertMerge && convertForce:
		return oops.Errorf("--merge and --force cannot be combined: --merge keeps every existing file, --force replaces them")
	case convertLock && !convertWrite:
		return oops.Errorf("--lock pins what was written: it needs --write")
	}
	for _, c := range convertAllowFindings {
		if !allowCodeRe.MatchString(strings.TrimSpace(c)) {
			return oops.Errorf("invalid --allow-findings code %q (expected a rule code such as AR001)", c)
		}
	}
	for _, s := range convertFailOn {
		switch strings.ReplaceAll(s, "_", "-") {
		case "approximated", "dropped", "needs-action", "unsupported":
		default:
			return oops.Hint("Use approximated, dropped, needs-action or unsupported").Errorf("unknown --fail-on status %q", s)
		}
	}
	return nil
}

func convertOptions(write bool) importer.ConvertOptions {
	return importer.ConvertOptions{
		Source: convertSource, Into: convertInto, From: convertFrom, Domain: convertDomain,
		Write: write, Force: convertForce, SplitHeadings: convertSplitHeadings, BestEffort: convertBestEffort,
		AllowFindings: convertAllowFindings, EnableHooks: convertEnableHooks, EnablePermissions: convertEnablePerms,
		Merge: convertMerge, KeepNames: convertKeepNames, Delivery: convertDelivery, Fetch: convertFetch,
	}
}

// printConvertNext tells a text-format user what to do after a dry run or a write.
func printConvertNext(out io.Writer, report *importer.Report, write bool) {
	if convertFormat != formatText {
		return
	}
	switch {
	case !write:
		fprintf(out, "\nDry run: nothing was written. Rerun with --write to create the files.\n")
	case report.NeedsLock() && !convertLock:
		fprintf(out, "\nNext: run `ai-rulez lock` to pin the imported remote skills (or rerun with --lock).\n")
	}
}

// runConvert runs the command and returns the process exit code.
func runConvert(ctx context.Context, out io.Writer, interactive bool) int {
	progress.SetQuiet(true)
	defer progress.SetQuiet(false)

	if err := checkFormatFlag(convertFormat); err != nil {
		renderStderr(err)
		return exitConvertCannotRun
	}
	if convertList {
		return listImporters(out)
	}
	if err := checkConvertFlags(interactive); err != nil {
		renderStderr(err)
		return exitConvertCannotRun
	}
	write := convertWrite

	report, err := importer.Convert(ctx, convertOptions(write))
	if report != nil {
		if werr := printConvertReport(out, report); werr != nil {
			renderStderr(werr)
			return exitConvertCannotRun
		}
	}
	if errors.Is(err, importer.ErrNothingToConvert) {
		reportNothingToConvert(out, err)
		return 0
	}
	if err != nil {
		renderStderr(err)
		return exitConvertCannotRun
	}
	if report.Security.Blocked || report.Validation.Errors > 0 {
		return exitConvertBlocked
	}
	if write && convertLock {
		if code := lockConverted(ctx); code != 0 {
			return exitConvertCannotRun
		}
	}
	if len(convertFailOn) > 0 && report.Matches(normalizeStatuses(convertFailOn)) {
		return exitConvertBlocked
	}
	printConvertNext(out, report, write)
	return 0
}

// reportNothingToConvert says that the selected importers found nothing to
// import. It is not an error: nothing is written and the exit code is 0. With
// --format json the report stream stays empty and the message goes to stderr.
func reportNothingToConvert(out io.Writer, err error) {
	msg := "Nothing to convert: " + strings.TrimSuffix(err.Error(), ": "+importer.ErrNothingToConvert.Error())
	if oe, ok := oops.AsOops(err); ok && oe.Hint() != "" {
		msg += "\n" + oe.Hint()
	}
	if convertFormat == formatJSON {
		fprintf(os.Stderr, "%s\n", msg)
		return
	}
	fprintf(out, "%s\n", msg)
}

// lockConverted runs `ai-rulez lock` on the config convert wrote. The lock prints
// to stdout; with --format json that would corrupt the report, so it goes to stderr.
func lockConverted(ctx context.Context) int {
	abs, err := filepath.Abs(convertSource)
	if err != nil {
		renderStderr(err)
		return exitConvertCannotRun
	}
	into := convertInto
	if !filepath.IsAbs(into) {
		into = filepath.Join(abs, into)
	}
	if convertFormat == formatJSON {
		saved := os.Stdout
		os.Stdout = os.Stderr
		defer func() { os.Stdout = saved }()
	}
	return writeLockAtContext(ctx, filepath.Join(into, "config.toml"), "", nil)
}

func normalizeStatuses(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ReplaceAll(s, "_", "-")
	}
	return out
}

func printConvertReport(out io.Writer, report *importer.Report) error {
	render := func(w io.Writer) error {
		if convertFormat == formatJSON {
			return report.WriteJSON(w)
		}
		return report.WriteText(w)
	}
	if convertReport != "" {
		var buf bytes.Buffer
		if err := render(&buf); err != nil {
			return err
		}
		// WriteFile reports the close error a deferred Close would drop: a full disk
		// shows up there.
		if err := os.WriteFile(convertReport, buf.Bytes(), 0o644); err != nil { //nolint:gosec // a report the user asked for
			return oops.With("path", convertReport).Wrapf(err, "write report file")
		}
	}
	return render(out)
}

func listImporters(out io.Writer) int {
	detections, err := importer.Detect(convertSource)
	if err != nil {
		renderStderr(err)
		return exitConvertCannotRun
	}
	if convertFormat == formatJSON {
		return writeJSONList(out, detections)
	}
	for _, d := range detections {
		fprintf(out, "%s\n  %s\n", d.Name, d.Description)
		if len(d.Files) == 0 {
			fprintf(out, "  detected: nothing\n")
			continue
		}
		fprintf(out, "  detected: %s\n", strings.Join(d.Files, ", "))
	}
	return 0
}

func writeJSONList(out io.Writer, detections []importer.Detection) int {
	if err := jsondoc.Write(out, detections); err != nil {
		renderStderr(err)
		return exitConvertCannotRun
	}
	return 0
}
