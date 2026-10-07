// Package migrate implements `ai-rulez migrate v5`: a deterministic rewrite of a
// 4.x project (config.toml, config.yaml/.yml, config.json, the config.local.*
// overlay, legacy mcp.* files and markdown frontmatter aliases) into the v5
// format. It only reads 4.x; anything older must go through ai-rulez 4.x first.
package migrate

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/walkutil"
)

// SchemaVersion is the version of the JSON report document.
const SchemaVersion = 1

// Status values of a project in the report.
const (
	StatusMigrated  = "migrated"
	StatusUnchanged = "unchanged"
	StatusError     = "error"
)

// Rule ids of the change list.
const (
	RuleVersion       = "version"
	RuleConvertFormat = "convert-format"
	RuleLintRatchet   = "lint-ratchet"
	RuleMCPMerge      = "mcp-merge"
	RulePinDefault    = "pin-default"
	RuleLocalOverlay  = "local-overlay"
	RuleFrontmatter   = "frontmatter-alias"
	RuleCommand       = "command-rename"
	RulePreset        = "preset-rename"
	RuleGitignore     = "gitignore-block"
)

// Options configures a migration run.
type Options struct {
	// Root is the project directory, or the directory to walk with Recursive.
	Root string
	// ConfigDirName restricts discovery to one config directory name; empty
	// means .ai-rulez with the .config/ai-rulez fallback.
	ConfigDirName string
	Recursive     bool
	// DryRun computes and reports the changes without writing anything. Check
	// does the same and lets the caller fail when changes are pending.
	DryRun, Check bool
	// AdoptDefaults skips pinning the 4.x defaults (agents_md = false, the
	// managed .gitignore block, Source-Hash headers), so the project takes the
	// new v5 defaults.
	AdoptDefaults bool
	// Write also rewrites frontmatter aliases in the markdown sources.
	Write bool
}

// Change is one entry of a project's change list.
type Change struct {
	File   string `json:"file"`
	Rule   string `json:"rule"`
	Detail string `json:"detail"`
}

// ProjectReport is the outcome for one config directory.
type ProjectReport struct {
	Path      string   `json:"path"`
	ConfigDir string   `json:"config_dir"`
	Status    string   `json:"status"`
	Changes   []Change `json:"changes"`
	Warnings  []string `json:"warnings"`
	Error     string   `json:"error,omitempty"`
}

// Summary counts projects by status.
type Summary struct {
	Migrated  int `json:"migrated"`
	Unchanged int `json:"unchanged"`
	Errors    int `json:"errors"`
}

// Report is the JSON document of a run.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	Target        string          `json:"target"`
	DryRun        bool            `json:"dry_run"`
	Check         bool            `json:"check"`
	Projects      []ProjectReport `json:"projects"`
	Summary       Summary         `json:"summary"`
}

// Pending reports whether any project needs changes (or failed).
func (r *Report) Pending() bool { return r.Summary.Migrated > 0 }

// Failed reports whether any project could not be migrated.
func (r *Report) Failed() bool { return r.Summary.Errors > 0 }

type fileWrite struct {
	path string
	data []byte
	perm os.FileMode
}

// plan is everything a project migration would do.
type plan struct {
	writes   []fileWrite
	removes  []string
	changes  []Change
	warnings []string
}

func (p *plan) change(file, rule, detail string) {
	p.changes = append(p.changes, Change{File: file, Rule: rule, Detail: detail})
}

// Run migrates every project found under opts.Root.
func Run(opts Options) (*Report, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, oops.With("path", opts.Root).Wrapf(err, "resolve project directory")
	}
	dirs, err := findConfigDirs(root, opts)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, oops.
			With("path", root).
			Hint("Run from the project directory, pass --recursive to search below it, or run 'ai-rulez init'").
			Errorf("no .ai-rulez configuration directory found in %s", root)
	}

	report := &Report{SchemaVersion: SchemaVersion, Command: "migrate", Target: "v5", DryRun: opts.DryRun, Check: opts.Check}
	for _, dir := range dirs {
		pr := ProjectReport{Path: relTo(root, projectDir(dir)), ConfigDir: relTo(root, dir), Changes: []Change{}, Warnings: []string{}}
		p, planErr := planProject(dir, opts)
		switch {
		case planErr != nil:
			pr.Status, pr.Error = StatusError, planErr.Error()
			report.Summary.Errors++
		case len(p.changes) == 0:
			pr.Status, pr.Warnings = StatusUnchanged, append(pr.Warnings, p.warnings...)
			report.Summary.Unchanged++
		default:
			pr.Changes, pr.Warnings = p.changes, append(pr.Warnings, p.warnings...)
			pr.Status = StatusMigrated
			report.Summary.Migrated++
			if !opts.DryRun && !opts.Check {
				if err := apply(p); err != nil {
					pr.Status, pr.Error = StatusError, err.Error()
					report.Summary.Migrated--
					report.Summary.Errors++
				}
			}
		}
		report.Projects = append(report.Projects, pr)
	}
	return report, nil
}

func relTo(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// projectDir is the directory that owns a config directory; the generic
// .config/ wrapper of .config/ai-rulez is skipped.
func projectDir(configDir string) string {
	parent := filepath.Dir(configDir)
	if filepath.Base(parent) == dirConfig {
		return filepath.Dir(parent)
	}
	return parent
}

const (
	dirAIRulez = ".ai-rulez"
	dirConfig  = ".config"
)

// probeConfigDirs returns the config directories below dir named by names that
// hold a config file. Without an explicit name a sibling .ai-rulez wins over
// .config/ai-rulez.
func probeConfigDirs(dir string, names []string, explicit bool) []string {
	var found []string
	for _, n := range names {
		cd := filepath.Join(dir, filepath.FromSlash(n))
		info, err := os.Stat(cd)
		if err != nil || !info.IsDir() || !hasAnyConfig(cd) {
			continue
		}
		found = append(found, cd)
		if !explicit && n == dirAIRulez {
			break
		}
	}
	return found
}

// skipDir reports whether a recursive search must not descend into path, which
// held found config directories.
func skipDir(root, path, name string, found []string) bool {
	if path == root {
		return false
	}
	if len(found) > 0 || name == dirAIRulez {
		return true
	}
	return name != dirConfig && walkutil.ShouldSkipDir(name)
}

// findConfigDirs returns the config directories to migrate, sorted.
func findConfigDirs(root string, opts Options) ([]string, error) {
	names := []string{dirAIRulez, dirConfig + "/ai-rulez"}
	explicit := opts.ConfigDirName != ""
	if explicit {
		names = []string{filepath.ToSlash(opts.ConfigDirName)}
	}
	if !opts.Recursive {
		return probeConfigDirs(root, names, explicit), nil
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		found := probeConfigDirs(path, names, explicit)
		out = append(out, found...)
		if skipDir(root, path, d.Name(), found) {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "search for configuration directories")
	}
	sort.Strings(out)
	return out, nil
}

var sourceNames = []string{"config.toml", "config.yaml", "config.yml", "config.json"}

func hasAnyConfig(dir string) bool {
	for _, n := range sourceNames {
		if info, err := os.Stat(filepath.Join(dir, n)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func apply(p *plan) error {
	for _, w := range p.writes {
		if err := gitutil.WriteFileAtomic(w.path, w.data, w.perm); err != nil {
			return oops.With("path", w.path).Wrapf(err, "write %s", filepath.Base(w.path))
		}
	}
	for _, r := range p.removes {
		if err := os.Remove(r); err != nil && !os.IsNotExist(err) {
			return oops.With("path", r).Wrapf(err, "remove %s", filepath.Base(r))
		}
	}
	return nil
}

// planProject computes the migration of one config directory.
func planProject(dir string, opts Options) (*plan, error) { //nolint:gocyclo // one pass: read the source, convert, rewrite, verify, then plan the side files
	p := &plan{}
	src, srcName := "", ""
	for _, n := range sourceNames {
		path := filepath.Join(dir, n)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if src == "" {
				src, srcName = path, n
			} else {
				p.warnings = append(p.warnings, n+" is ignored by 4.x (it reads "+srcName+" first) and was left in place; remove it once you have checked the result")
			}
		}
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, oops.With("path", src).Wrapf(err, "read %s", srcName)
	}

	text := string(raw)
	converted := false
	if srcName != "config.toml" {
		out, err := YAMLToTOML(raw)
		if err != nil {
			return nil, oops.With("path", src).Hint("Fix the file so that it parses, then run migrate again").Wrapf(err, "convert %s to TOML", srcName)
		}
		text, converted = string(out), true
	}
	doc := parseTOMLDoc(text)

	oldVersion, _ := currentVersion(doc)
	if !converted && oldVersion == config.ConfigVersionV5 {
		// The config is already v5, but the files beside it may not be: a rerun
		// with --write still has the frontmatter aliases and the overlay to do.
		p.warnings = append(p.warnings, legacyFileWarnings(dir)...)
		if err := planLocalOverlay(p, dir); err != nil {
			return nil, err
		}
		if err := planFrontmatter(p, dir, opts.Write); err != nil {
			return nil, err
		}
		return p, nil
	}
	if err := checkSourceVersion(oldVersion); err != nil {
		return nil, oops.With("path", src).Wrap(err)
	}
	if spellings := doc.ratchetSpellings(); len(spellings) > 1 {
		return nil, oops.
			With("path", src).
			Hint("Keep one table (merge the entries by hand into [lint.ratchet]) and delete the others, then run migrate again").
			Errorf("%s are set together, but v5 has one table for them, [lint.ratchet] (4.x preferred [lint.tolerate] over [lint.budget])", bracketed(spellings))
	}

	mainTarget := filepath.Join(dir, "config.toml")
	planMainConfig(p, dir, doc, oldVersion, srcName, converted, opts)
	if err := planMCPMerge(p, dir, doc); err != nil {
		return nil, err
	}

	final := doc.text()
	if _, err := config.DecodeTOML([]byte(final), mainTarget); err != nil {
		return nil, oops.
			With("path", mainTarget).
			Hint("The migrated configuration does not decode; fix the source file and run migrate again").
			Wrapf(err, "verify the migrated configuration")
	}
	p.writes = append(p.writes, fileWrite{path: mainTarget, data: []byte(final), perm: 0o644})
	if converted {
		p.removes = append(p.removes, src)
	}

	if err := planLocalOverlay(p, dir); err != nil {
		return nil, err
	}
	if err := planFrontmatter(p, dir, opts.Write); err != nil {
		return nil, err
	}
	return p, nil
}

// planStaleGitignoreBlock removes the managed .gitignore block of a 4.x project
// that takes the v5 default (gitignore off): the block would keep ignoring
// outputs that v5 expects to be committed.
func planStaleGitignoreBlock(p *plan, dir string, doc *tomlDoc) {
	if i := doc.rootKey("gitignore"); i >= 0 {
		_, val, _ := strings.Cut(stripComment(doc.lines[doc.stmts[i].start]), "=")
		if strings.TrimSpace(val) != "false" {
			return
		}
	}
	path := filepath.Join(projectDir(dir), ".gitignore")
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the project's own .gitignore in a one-shot rewrite
	if err != nil {
		return
	}
	text := string(raw)
	stripped := gitignore.WithoutManagedBlock(text)
	if stripped == text {
		return
	}
	stripped = strings.TrimRight(stripped, "\n")
	if stripped != "" {
		stripped += "\n"
	}
	p.writes = append(p.writes, fileWrite{path: path, data: []byte(stripped), perm: 0o644})
	p.change(relTo(projectDir(dir), path), RuleGitignore, "removed the ai-rulez managed block (v5 default: gitignore = false, generated files are committed)")
}

// currentVersion reads the root version of the document.
func currentVersion(doc *tomlDoc) (string, bool) {
	i := doc.rootKey("version")
	if i < 0 {
		return "", false
	}
	line := doc.lines[doc.stmts[i].start]
	_, val, ok := strings.Cut(stripComment(line), "=")
	if !ok {
		return "", false
	}
	return strings.Trim(strings.TrimSpace(val), `"'`), true
}

// planMainConfig applies the rewrite rules to the main document.
func planMainConfig(p *plan, dir string, doc *tomlDoc, oldVersion, srcName string, converted bool, opts Options) {
	file := relFile(dir, "config.toml")
	if converted {
		p.change(relFile(dir, srcName), RuleConvertFormat, "converted to config.toml with keys, order and comments kept; "+srcName+" is removed")
	}
	if _, changed := doc.setVersion(config.ConfigVersionV5); changed {
		from := oldVersion
		if from == "" {
			from = "missing"
		}
		p.change(file, RuleVersion, "version "+from+" -> "+config.ConfigVersionV5)
	}
	if n := doc.renameLintBudget(); n > 0 {
		p.change(file, RuleLintRatchet, "[lint.budget] / [lint.tolerate] renamed to [lint.ratchet] (per-rule finding counts); size limits stay in [lint.budgets]")
	}
	if n := doc.renamePreset("windsurf", "devin"); n > 0 {
		p.change(file, RulePreset, "preset windsurf renamed to devin (outputs move from .windsurf/ to .devin/; delete the old directory)")
	}
	if n := doc.dropPreset("continue-dev"); n > 0 {
		p.change(file, RulePreset, "preset continue-dev removed (it has no replacement; delete the old .continue/rules/ outputs)")
	}
	for _, rw := range commandRewrites {
		if n := doc.replaceAll(rw.old, rw.new); n > 0 {
			p.change(file, RuleCommand, "`"+rw.old+"` -> `"+rw.new+"` in "+plural(n, "line"))
		}
	}
	if !opts.AdoptDefaults {
		pinDefaults(p, doc, file)
	} else {
		planStaleGitignoreBlock(p, dir, doc)
	}
	p.warnings = append(p.warnings, deprecatedWarnings(doc)...)
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

func relFile(dir, name string) string {
	return filepath.ToSlash(filepath.Join(filepath.Base(dir), name))
}

const pinNote = "# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default."

// pinDefaults keeps the 4.x behavior of the three defaults v5 changes, unless
// the file already states a value.
func pinDefaults(p *plan, doc *tomlDoc, file string) {
	if doc.rootKey("agents_md") < 0 {
		doc.insertRootLine(pinNote, "agents_md = false")
		p.change(file, RulePinDefault, "agents_md = false (v5 default: true, AGENTS.md is the canonical instruction file)")
	}
	if doc.rootKey("gitignore") < 0 {
		doc.insertRootLine(pinNote, "gitignore = true")
		p.change(file, RulePinDefault, "gitignore = true (v5 default: false, no managed .gitignore block)")
	}
	hdr := []string{"header"}
	switch {
	case doc.hasTable(hdr):
		if !doc.tableHas(hdr, "hashes") && !headerInline(doc) {
			if doc.addToTable(hdr, `hashes = "full"`) {
				p.change(file, RulePinDefault, `[header] hashes = "full" (v5 default: "content", only the per-file Content-Hash)`)
			}
		}
	default:
		doc.appendBlock(pinNote + "\n[header]\nhashes = \"full\"")
		p.change(file, RulePinDefault, `[header] hashes = "full" (v5 default: "content", only the per-file Content-Hash)`)
	}
}

// headerInline reports a root-level inline `header = {...}` or dotted
// `header.x = ...` definition, which is left alone (the author set it).
func headerInline(doc *tomlDoc) bool {
	for _, s := range doc.stmts {
		if !s.header && len(s.table) == 0 && len(s.key) > 0 && s.key[0] == "header" {
			return true
		}
	}
	return false
}

func deprecatedWarnings(doc *tomlDoc) []string {
	var out []string
	if doc.hasTable([]string{"plugins"}) {
		out = append(out, "[[plugins]] no longer writes .claude/plugins.json or .codex/plugins.json (no tool reads them); remove the table or move the entries to the harness's own plugin settings")
	}
	return out
}

// legacyFileWarnings names leftovers v5 ignores.
func legacyFileWarnings(dir string) []string {
	var out []string
	for _, n := range []string{"mcp.yaml", "mcp.yml", "mcp.json", "mcp.toml"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			out = append(out, n+" is no longer read; its servers belong in config.toml under [[mcp_servers]]")
		}
	}
	return out
}

// bracketed renders ["budget", "tolerate"] as "[lint.budget] and [lint.tolerate]".
func bracketed(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = "[lint." + n + "]"
	}
	return strings.Join(parts, " and ")
}
