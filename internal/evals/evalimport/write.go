package evalimport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"gopkg.in/yaml.v3"
)

// ImporterVersion is the version stamped in the output header.
const ImporterVersion = 1

// Options configures Run.
type Options struct {
	Source Source
	// Paths are the scenario directories, criteria files or directories of
	// scenarios to import.
	Paths []string
	// OutDir is where the case files are written (created when missing).
	OutDir string
	// DryRun maps and reports, and writes nothing.
	DryRun bool
	// Force overwrites existing files.
	Force  bool
	Map    MapOptions
	Limits Limits
}

// Finding is a note of the import report; the importer reports AR9A5 (info) for
// scenarios with unmapped fields. `validate` never reports it.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Scenario string `json:"scenario"`
	Message  string `json:"message"`
}

// Result is the outcome of an import.
type Result struct {
	Importer int       `json:"importer"`
	Source   string    `json:"source"`
	DryRun   bool      `json:"dry_run"`
	Out      string    `json:"out"`
	Reports  []Report  `json:"scenarios"`
	Findings []Finding `json:"findings,omitempty"`
}

// Run imports every scenario under opts.Paths. Nothing is written unless every
// scenario maps and no target exists (without Force): an import is all or
// nothing.
func Run(opts *Options) (*Result, error) {
	if opts.Source == nil {
		return nil, errors.New("no import source")
	}
	if len(opts.Paths) == 0 {
		return nil, errors.New("give at least one scenario path")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, errors.New("no output directory")
	}
	var scenarios []Scenario
	for _, p := range opts.Paths {
		if !opts.Source.Detect(p) {
			return nil, fmt.Errorf("%s does not look like a %s scenario (expected a %s or a directory of scenario directories)", p, opts.Source.Name(), criteriaFile)
		}
		loaded, err := opts.Source.Load(p, opts.Limits)
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, loaded...)
	}
	limits := opts.Limits.withDefaults()
	if len(scenarios) > limits.MaxScenarios {
		return nil, fmt.Errorf("%d scenarios; the limit is %d", len(scenarios), limits.MaxScenarios)
	}
	mapped, err := mapAll(opts, scenarios)
	if err != nil {
		return nil, err
	}
	files, err := render(mapped)
	if err != nil {
		return nil, err
	}
	if err := checkTargets(opts, files); err != nil {
		return nil, err
	}
	res := buildResult(opts, mapped, files)
	if opts.DryRun {
		return res, nil
	}
	if err := writeAll(opts, files); err != nil {
		return res, err
	}
	return res, nil
}

// buildResult assembles the per-scenario reports and the findings for unmapped fields.
func buildResult(opts *Options, mapped []*Mapped, files [][]outFile) *Result {
	res := &Result{Importer: ImporterVersion, Source: opts.Source.Name(), DryRun: opts.DryRun, Out: opts.OutDir}
	for i, m := range mapped {
		for _, f := range files[i] {
			m.Report.Written = append(m.Report.Written, filepath.ToSlash(f.rel))
		}
		res.Reports = append(res.Reports, m.Report)
		if n := len(m.Report.Unmapped); n > 0 {
			res.Findings = append(res.Findings, Finding{Code: FindingCode, Severity: "info", Scenario: m.Report.Scenario,
				Message: fmt.Sprintf("%d field(s) of the input have no counterpart and were not imported", n)})
		}
	}
	return res
}

// writeAll writes every rendered file, rolling back the ones already written when a write fails.
func writeAll(opts *Options, files [][]outFile) error {
	var done []undo
	for _, group := range files {
		for _, f := range group {
			path := filepath.Join(opts.OutDir, f.rel)
			prev, hadPrev := readPrevious(path)
			if err := writeFile(path, f.data, opts.Force); err != nil {
				rollback(done)
				return err
			}
			done = append(done, undo{path: path, prev: prev, hadPrev: hadPrev})
		}
	}
	return nil
}

// undo reverses one written file: the content it replaced, or its removal.
type undo struct {
	path    string
	prev    []byte
	hadPrev bool
}

// readPrevious reads the regular file about to be replaced, if there is one.
func readPrevious(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // a case file of the import's own output directory
	return data, err == nil
}

// rollback undoes the files an import wrote before it failed, newest first, so a
// failed import leaves the output directory as it found it (best effort).
func rollback(done []undo) {
	for i := len(done) - 1; i >= 0; i-- {
		u := done[i]
		if u.hadPrev {
			_ = os.WriteFile(u.path, u.prev, 0o644) //nolint:gosec,errcheck // restoring a committed case file
			continue
		}
		_ = os.Remove(u.path) //nolint:errcheck // best effort
	}
}

// mapAll maps every scenario and gives each a unique case id (a repeated name gets
// -2, -3, ...), re-mapping with the new id so the file names, the prompt file and
// the fixture paths all carry it.
func mapAll(opts *Options, scenarios []Scenario) ([]*Mapped, error) {
	used := map[string]bool{}
	out := make([]*Mapped, 0, len(scenarios))
	for i := range scenarios {
		m, err := opts.Source.Map(&scenarios[i], opts.Map)
		if err != nil {
			return nil, err
		}
		if used[m.ID] {
			id := m.ID
			for n := 2; used[id] || id == m.ID; n++ {
				id = fmt.Sprintf("%s-%d", m.ID, n)
			}
			mo := opts.Map
			mo.ID = id
			if m, err = opts.Source.Map(&scenarios[i], mo); err != nil {
				return nil, err
			}
			m.Report.Assumed = append(m.Report.Assumed, fmt.Sprintf("case id %q (another scenario already has the name)", id))
		}
		used[m.ID] = true
		out = append(out, m)
	}
	return out, nil
}

// outFile is one file of the import.
type outFile struct {
	rel  string
	data []byte
}

// render produces the files of every mapped scenario and checks that the case
// files read back through the normal case parser.
func render(mapped []*Mapped) ([][]outFile, error) {
	all := make([][]outFile, len(mapped))
	for i, m := range mapped {
		doc, err := renderCase(m)
		if err != nil {
			return nil, err
		}
		name := m.ID + ".eval.yaml"
		if cases, problems := evals.ParseFile(name, doc); len(problems) > 0 || len(cases) != 1 {
			return nil, fmt.Errorf("scenario %q produced a case file the case parser rejects (%v); this is a bug in the importer", m.Report.Scenario, problems)
		}
		group := []outFile{{rel: name, data: doc}, {rel: m.TaskFile, data: []byte(ensureNewline(m.TaskContent))}}
		for _, f := range m.Fixtures {
			group = append(group, outFile{rel: f.Path, data: f.Content})
		}
		all[i] = group
	}
	return all, nil
}

func ensureNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// renderCase writes the case file: a provenance header, then the case. A lifted
// assertion carries a comment naming its criterion.
func renderCase(m *Mapped) ([]byte, error) {
	var node yaml.Node
	if err := node.Encode(evals.CaseFile{SchemaVersion: evals.CaseSchemaVersion, Cases: []evals.Case{m.Case}}); err != nil {
		return nil, fmt.Errorf("encode case: %w", err)
	}
	commentLifted(&node, m.Report.Lifted)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# imported by ai-rulez eval import (importer %d) from scenario %s\n", ImporterVersion, strconv.Quote(summarize(m.Report.Scenario)))
	fmt.Fprintf(&buf, "# source %s\n", m.Report.SourceSHA256)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return nil, fmt.Errorf("encode case: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode case: %w", err)
	}
	return buf.Bytes(), nil
}

// commentLifted marks each lifted assertion of the encoded document.
func commentLifted(doc *yaml.Node, lifts []Lift) {
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if len(lifts) == 0 {
		return
	}
	cases := mapValue(root, "cases")
	if cases == nil || len(cases.Content) == 0 {
		return
	}
	assertions := mapValue(cases.Content[0], "assertions")
	if assertions == nil {
		return
	}
	for i, item := range assertions.Content {
		if i < len(lifts) && len(item.Content) > 0 {
			// The comment goes on the item's first key, which is where yaml.v3 prints a
			// sequence item's head comment.
			item.Content[0].HeadComment = "lifted from criterion " + strconv.Quote(summarize(lifts[i].Criterion))
		}
	}
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// checkTargets refuses to overwrite without Force.
func checkTargets(opts *Options, files [][]outFile) error {
	if opts.Force {
		return nil
	}
	var existing []string
	for _, group := range files {
		for _, f := range group {
			if _, err := os.Lstat(filepath.Join(opts.OutDir, f.rel)); err == nil {
				existing = append(existing, filepath.ToSlash(f.rel))
			}
		}
	}
	if len(existing) > 0 {
		sort.Strings(existing)
		return fmt.Errorf("%s already exist in %s; use --force to overwrite", strings.Join(existing, ", "), opts.OutDir)
	}
	return nil
}

// writeFile writes data atomically to path (creating its directory). Without
// force an existing file is an error; a symlink at the path is replaced, never
// followed.
func writeFile(path string, data []byte, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if !force {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // a committed case file, world-readable like the others
		if err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close() //nolint:errcheck // the write error is the one to report
			return fmt.Errorf("write %s: %w", path, err)
		}
		return f.Close()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".import-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup after a rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() //nolint:errcheck // the write error is the one to report
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // a committed case file, world-readable like the others
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WriteText renders the result for people.
func (r *Result) WriteText(w io.Writer) error {
	var b strings.Builder
	for i := range r.Reports {
		rep := &r.Reports[i]
		verb := "wrote"
		if r.DryRun {
			verb = "would write"
		}
		fmt.Fprintf(&b, "%s %s (1 case)\n", verb, strings.Join(rep.Written, ", "))
		writeLines(&b, "mapped:  ", rep.Mapped)
		writeLines(&b, "assumed: ", rep.Assumed)
		for _, l := range rep.Lifted {
			fmt.Fprintf(&b, "lifted:   criterion %s -> %s\n", strconv.Quote(summarize(l.Criterion)), l.Assertion)
		}
		for _, wmsg := range rep.Warnings {
			fmt.Fprintf(&b, "warning:  %s\n", strconv.Quote(wmsg))
		}
		if n := len(rep.Unmapped); n > 0 {
			fmt.Fprintf(&b, "unmapped (%d), %s:\n", n, FindingCode)
			for _, u := range rep.Unmapped {
				fmt.Fprintf(&b, "  %-28s value %s", u.Path, strconv.Quote(u.Value))
				if u.Hint != "" {
					fmt.Fprintf(&b, "  -> %s", u.Hint)
				}
				b.WriteByte('\n')
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeLines(b *strings.Builder, label string, lines []string) {
	for _, l := range lines {
		fmt.Fprintf(b, "%s %s\n", label, l)
	}
}
