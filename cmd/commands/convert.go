package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
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
  native        CLAUDE.md, AGENTS.md, GEMINI.md, .cursor/rules, .github/instructions,
                .kiro/steering, .windsurf, .roo, .clinerules, .qwen, .junie,
                .agents/skills, skills, agents and commands of every supported preset,
                and MCP files (.mcp.json, .cursor/mcp.json, .vscode/mcp.json, ...)
  rulesync      rulesync.jsonc and .rulesync/ (rules, commands, subagents, skills, checks,
                mcp.jsonc, ignore); hooks and permissions are reported, not yet imported
  skills-lock   skills-lock.json of the Vercel skills CLI, as [[installed_skills]]

When a rulesync project is detected, auto skips native: the tool files next to
.rulesync/ are its generated output. Use --from native,rulesync to read both.

Nothing is written unless --write is given. Without --write or --dry-run, a
terminal gets a dry run and a script is asked to choose. Existing .ai-rulez/
files are never overwritten without --force; --domain NAME imports beside them.
An existing config.toml is never replaced, not even with --force: new presets,
[[mcp_servers]] and [[installed_skills]] are merged into it (existing entries
win). --into is relative to --source unless absolute, and nothing is written
through a symlink at or below it. Source files are never modified, and nothing
is fetched or executed. The converted tree is validated and security-scanned in
a scratch directory first, and a blocked scan writes nothing.

Exit codes: 0 done, 1 could not run or would overwrite, 2 blocked by the scan or
validation, or --fail-on matched.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if code := runConvert(watchParentContext(cmd), os.Stdout, stdoutIsTerminal()); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	f := ConvertCmd.Flags()
	f.StringSliceVar(&convertFrom, "from", []string{"auto"}, "Importers to run: native, rulesync, skills-lock or auto (every detected importer)")
	f.StringVar(&convertSource, "source", ".", "Directory to read")
	f.StringVar(&convertInto, "into", importer.DefaultConfigDir, "Config directory to write: relative to --source unless absolute; never written through a symlink")
	f.StringVar(&convertDomain, "domain", "", "Put the imported content in this domain (safe next to an existing tree)")
	f.BoolVar(&convertDryRun, "dry-run", false, "Print the plan and report; write nothing")
	f.BoolVar(&convertWrite, "write", false, "Write the converted tree")
	f.BoolVar(&convertForce, "force", false, "Overwrite existing content files that differ (config.toml is always merged, never replaced)")
	f.StringVar(&convertReport, "report", "", "Also write the report (in --format) to this file")
	f.StringVar(&convertFormat, "format", formatText, "Report format: text or json")
	f.StringSliceVar(&convertFailOn, "fail-on", nil, "Exit 2 when a finding has one of these statuses: approximated, dropped, needs-action, unsupported")
	f.BoolVar(&convertBestEffort, "best-effort", false, "Import the known fields of an unrecognised format version")
	f.BoolVar(&convertSplitHeadings, "split-headings", false, "Split root files such as CLAUDE.md into one context per H2 heading")
	f.BoolVar(&convertList, "list", false, "List the importers and what each detects in --source")
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// runConvert runs the command and returns the process exit code.
func runConvert(ctx context.Context, out io.Writer, interactive bool) int {
	progress.SetQuiet(true)
	defer progress.SetQuiet(false)

	if err := checkFormatFlag(convertFormat); err != nil {
		fmtError(err)
		return exitConvertCannotRun
	}
	if convertList {
		return listImporters(out)
	}
	if convertWrite && convertDryRun {
		fmtError(fmt.Errorf("--write and --dry-run cannot be combined"))
		return exitConvertCannotRun
	}
	write := convertWrite
	if !convertWrite && !convertDryRun && !interactive {
		fmtError(fmt.Errorf("pass --write to convert or --dry-run to preview; a script never converts by surprise"))
		return exitConvertCannotRun
	}
	for _, s := range convertFailOn {
		switch strings.ReplaceAll(s, "_", "-") {
		case "approximated", "dropped", "needs-action", "unsupported":
		default:
			fmtError(fmt.Errorf("unknown --fail-on status %q", s))
			return exitConvertCannotRun
		}
	}

	report, err := importer.Convert(ctx, importer.ConvertOptions{
		Source: convertSource, Into: convertInto, From: convertFrom, Domain: convertDomain,
		Write: write, Force: convertForce, SplitHeadings: convertSplitHeadings, BestEffort: convertBestEffort,
	})
	if report != nil {
		if werr := printConvertReport(out, report); werr != nil {
			fmtError(werr)
			return exitConvertCannotRun
		}
	}
	if err != nil {
		fmtError(err)
		return exitConvertCannotRun
	}
	if report.Security.Blocked || report.Validation.Errors > 0 {
		return exitConvertBlocked
	}
	if len(convertFailOn) > 0 && report.Matches(normalizeStatuses(convertFailOn)) {
		return exitConvertBlocked
	}
	if !write && convertFormat == formatText {
		fmt.Fprintln(out, "\nDry run: nothing was written. Rerun with --write to create the files.")
	}
	return 0
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
		report.WriteText(w)
		return nil
	}
	if convertReport != "" {
		file, err := os.Create(convertReport)
		if err != nil {
			return fmt.Errorf("create report file: %w", err)
		}
		defer file.Close()
		if err := render(file); err != nil {
			return err
		}
	}
	return render(out)
}

func listImporters(out io.Writer) int {
	detections, err := importer.Detect(convertSource)
	if err != nil {
		fmtError(err)
		return exitConvertCannotRun
	}
	if convertFormat == formatJSON {
		return writeJSONList(out, detections)
	}
	for _, d := range detections {
		fmt.Fprintf(out, "%s\n  %s\n", d.Name, d.Description)
		if len(d.Files) == 0 {
			fmt.Fprintln(out, "  detected: nothing")
			continue
		}
		fmt.Fprintf(out, "  detected: %s\n", strings.Join(d.Files, ", "))
	}
	return 0
}

func writeJSONList(out io.Writer, detections []importer.Detection) int {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(detections); err != nil {
		fmtError(err)
		return exitConvertCannotRun
	}
	return 0
}
