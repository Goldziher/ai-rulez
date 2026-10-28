package commands

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// newExposureLeakageCmd is `exposure leakage`, which shares the group's flags.
func newExposureLeakageCmd(f *exposureFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leakage",
		Short: "Report area terms in the text a session sees at its start (AR9W0, AR9W1)",
		Long: `Report every term of an area that is not shared in the text a Claude Code session sees at its start:
the root CLAUDE.md, unconditional root rules, the skill and agent listing lines, the MCP server names and
the enabled plugin names (AR9W0, a violation), outside repository path literals, and each listed skill or
agent that names only ambiguous words (AR9W1, a warning). An artifact whose phase is unknown counts as
start of session. A start-of-session artifact whose file could not be read (over the read limit, a
symlink to outside the root) is listed as unchecked: it is not a finding, and no leak in it can be found.

--root inspects that directory as a session started there sees it, with the configuration and area map
of the project; the paths are relative to --root.

The findings are those of ` + "`ai-rulez validate`" + `, accepted by the same [[lint.accept]] entries: an accepted
finding is listed with the entry's reason, and an entry for AR9W0 or AR9W1 that accepts nothing is a stale
acceptance (AR9I0). A [[lint.accept]] match is a substring of the message
` + "`<kind> \"<name>\" at session start names term \"<term>\" of area \"<area>\"`" + `.

--format json prints schema/exposure-leakage.v1.schema.json. Exit codes: 0 no violation and no stale
acceptance, 1 the command could not run (bad flags, an invalid configuration or area map, no area map),
2 a violation or a stale acceptance.`,
		Example: `  ai-rulez exposure leakage
  ai-rulez exposure leakage --root services/billing
  ai-rulez exposure leakage --areas tools/context/areas.toml --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := runExposureLeakage(cmd.Context(), f)
			if err != nil {
				return fail(err)
			}
			if f.asJSON {
				err = jsondoc.Write(cmd.OutOrStdout(), rep)
			} else {
				err = writeLeakageText(cmd.OutOrStdout(), rep)
			}
			if err != nil {
				return fail(err)
			}
			if !rep.OK() {
				return exitStatus(exitFindings)
			}
			return nil
		},
	}
	addJSONFormat(cmd.Flags(), &f.asJSON, "")
	return cmd
}

// runExposureLeakage runs the exposure rules of `validate` over the inventory
// of --root (the project root by default) with the area map and plugin mode
// of the flags, and returns their leakage view. The paths are relative to
// --root.
func runExposureLeakage(ctx context.Context, f *exposureFlags) (*exposure.LeakageReport, error) {
	if f.plugins != "" && !exposure.PluginMode(f.plugins).Valid() {
		return nil, oops.Errorf("unknown --plugins %q (use none, enabled or all)", f.plugins)
	}
	cfg, root, err := exposureProject(ctx, f.root)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		if cfg, err = loadProjectImplicit(ctx, root, config.WithoutRemote()); err != nil {
			return nil, err
		}
	}
	if problems := cfg.Exposure.Validate(); len(problems) > 0 {
		return nil, oops.Errorf("the [exposure] table is invalid (AR9W5): %s", strings.Join(problems, "; "))
	}
	if problems := lint.ValidateSettings(cfg.Lint); len(problems) > 0 {
		return nil, oops.Errorf("invalid [lint] settings: %v", problems)
	}
	m, file, err := exposureMap(cfg.Exposure, cfg.BaseDir, root, f.areasFile, configFileOf(cfg))
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, oops.Errorf("no area map: configure [exposure] areas or areas_file, or pass --areas")
	}
	base, err := filepath.Abs(cfg.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve the project root")
	}
	tree, err := lint.LoadTreeContext(ctx, gitutil.Git{}, leakageTreeRoot(base, root), "")
	if err != nil {
		return nil, oops.Wrapf(err, "index repository files")
	}
	src := exposureLintSource{
		ctx: ctx, cfg: cfg, dir: root, areas: m, areasFile: areasFilePath(root, file),
		plugins: exposure.PluginMode(f.plugins), cache: &exposureCache{},
	}
	rep, err := lint.RunWith(cfg, tree, lint.Options{Analyzers: []string{lint.AnalyzerExposure}, Cwd: root},
		lint.WithContext(ctx), lint.WithExposure(src))
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	res := rep.Leakage()
	if res == nil {
		return nil, oops.Errorf("the exposure rules did not run (is the exposure analyzer disabled?)")
	}
	out := leakageReport(res)
	out.Root, out.Plugins, out.AreasFile = filepath.ToSlash(root), src.mode(), file
	return out, nil
}

// leakageReport is the leakage view of a run as `exposure leakage` prints it: an accepted finding is
// LeakAccepted with the entry's reason, an error LeakViolation and anything less LeakWarning, so
// [lint.severity] moves a leak between the two.
func leakageReport(res *lint.LeakageResult) *exposure.LeakageReport {
	out := &exposure.LeakageReport{
		SchemaVersion:    exposure.SchemaLeakage,
		ArtifactsChecked: res.Checked,
		Findings:         make([]exposure.Leak, 0, len(res.Leaks)),
		Unchecked:        make([]exposure.Unchecked, 0, len(res.Unchecked)),
		StaleAcceptances: append([]string{}, res.Stale...),
	}
	for _, u := range res.Unchecked {
		out.Unchecked = append(out.Unchecked, exposure.Unchecked{Path: u.Path, Kind: exposure.Kind(u.Kind), Name: u.Name, Reason: u.Reason})
	}
	for i := range res.Leaks {
		l := &res.Leaks[i]
		sev := exposure.LeakWarning
		switch {
		case l.Accepted:
			sev = exposure.LeakAccepted
		case l.Severity == lint.SeverityError:
			sev = exposure.LeakViolation
		}
		out.Findings = append(out.Findings, exposure.Leak{
			Severity: sev, Code: l.Code, Area: l.Area, Term: l.Term, Kind: exposure.Kind(l.Kind), Name: l.Name,
			Path: l.Path, Scope: l.Scope, Line: l.Line, Excerpt: l.Excerpt, Reason: l.Reason,
		})
	}
	return out
}

// leakageTreeRoot is the directory whose files the leakage run indexes: the
// project root when root lies inside it, so the paths of [[lint.accept]] and
// [lint] ignore_paths stay relative to the project, as in validate; else root.
func leakageTreeRoot(base, root string) string {
	if filepath.IsAbs(filepath.FromSlash(rootRelative(base, root))) {
		return root
	}
	return base
}

// areasFilePath is the absolute path of the areas file exposureMap names
// (relative to root and slash separated, or absolute).
func areasFilePath(root, file string) string {
	if file == "" {
		return ""
	}
	path := filepath.FromSlash(file)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return path
}

// writeLeakageText prints the counts per area, kind and term of the
// violations, every finding, the artifacts that could not be checked, and the
// stale acceptances.
func writeLeakageText(w io.Writer, rep *exposure.LeakageReport) error {
	var violations []exposure.Leak
	counts := map[exposure.LeakSeverity]int{}
	for i := range rep.Findings {
		l := &rep.Findings[i]
		counts[l.Severity]++
		if l.Severity == exposure.LeakViolation {
			violations = append(violations, *l)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Leakage at session start: %d %s checked\n", rep.ArtifactsChecked, plural(rep.ArtifactsChecked, "artifact"))
	fmt.Fprintf(&b, "violations: %d  warnings: %d  accepted: %d\n", counts[exposure.LeakViolation], counts[exposure.LeakWarning], counts[exposure.LeakAccepted])
	for _, by := range []struct {
		label string
		key   func(l *exposure.Leak) string
	}{
		{"area", func(l *exposure.Leak) string { return l.Area }},
		{"kind", func(l *exposure.Leak) string { return string(l.Kind) }},
		{"term", func(l *exposure.Leak) string { return l.Area + "/" + l.Term }},
	} {
		if len(violations) == 0 {
			break
		}
		fmt.Fprintf(&b, "violations by %s:\n", by.label)
		for _, c := range tallyLeaks(violations, by.key) {
			fmt.Fprintf(&b, "  %s: %d\n", c.label, c.n)
		}
	}
	if len(rep.Findings) > 0 {
		b.WriteString("findings:\n")
	}
	for i := range rep.Findings {
		l := &rep.Findings[i]
		fmt.Fprintf(&b, "  %s %s %s/%s %s:%d: %s\n", l.Severity, l.Code, l.Area, l.Term, l.Path, l.Line, l.Excerpt)
		if l.Reason != "" {
			fmt.Fprintf(&b, "    accepted: %s\n", l.Reason)
		}
	}
	for _, u := range rep.Unchecked {
		fmt.Fprintf(&b, "unchecked: %s %s %s: %s\n", u.Kind, u.Name, u.Path, u.Reason)
	}
	for _, s := range rep.StaleAcceptances {
		fmt.Fprintf(&b, "stale acceptance: %s\n", s)
	}
	_, err := io.WriteString(w, b.String())
	return oops.Wrapf(err, "write the leakage report")
}

// labelCount is one row of a tally.
type labelCount struct {
	label string
	n     int
}

// tallyLeaks counts the leaks by key, most frequent first, ties in first-seen order.
func tallyLeaks(leaks []exposure.Leak, key func(l *exposure.Leak) string) []labelCount {
	index := map[string]int{}
	var out []labelCount
	for i := range leaks {
		k := key(&leaks[i])
		if at, ok := index[k]; ok {
			out[at].n++
			continue
		}
		index[k] = len(out)
		out = append(out, labelCount{label: k, n: 1})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].n > out[j].n })
	return out
}
