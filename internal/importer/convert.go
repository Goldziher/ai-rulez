package importer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/utils"
	"github.com/samber/oops"
)

// DefaultConfigDir is the config directory convert writes to by default.
const DefaultConfigDir = ".ai-rulez"

const autoFrom = "auto"

// ErrConflicts is returned by Convert when a write would replace existing
// content and --force was not given. Nothing is written.
var ErrConflicts = oops.Errorf("existing files differ from the converted content")

// ConvertOptions configure one conversion.
type ConvertOptions struct {
	// Source is the project directory to read (default ".").
	Source string
	// Into is the config directory. A relative path is resolved against Source;
	// an absolute path is used as given. Nothing is ever written through a
	// symlink at or below it.
	Into string
	// From lists importer names; empty or "auto" detects.
	From   []string
	Domain string
	// Write turns the plan into files; without it nothing is written.
	Write         bool
	Force         bool
	SplitHeadings bool
	BestEffort    bool
}

// Detection is what one importer recognises in a source directory.
type Detection struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Files       []string `json:"files"`
}

// Detect runs every importer's detector.
func Detect(source string) ([]Detection, error) {
	abs, err := absDir(source)
	if err != nil {
		return nil, err
	}
	fsys := os.DirFS(abs)
	var out []Detection
	for _, imp := range Registry() {
		files := imp.Detect(fsys)
		if files == nil {
			files = []string{}
		}
		out = append(out, Detection{Name: imp.Name(), Description: imp.Description(), Files: files})
	}
	return out, nil
}

func absDir(source string) (string, error) {
	if source == "" {
		source = "."
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", oops.With("source", source).Wrapf(err, "resolve source directory")
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", oops.With("source", source).Errorf("source %q is not a directory", source)
	}
	return abs, nil
}

// Convert reads the source in the selected formats, builds the report and, with
// opts.Write, writes the converted tree. It never modifies the source files.
func Convert(ctx context.Context, opts ConvertOptions) (*Report, error) {
	abs, err := absDir(opts.Source)
	if err != nil {
		return nil, err
	}
	into := opts.Into
	if into == "" {
		into = DefaultConfigDir
	}
	intoAbs := into
	if !filepath.IsAbs(intoAbs) {
		intoAbs = filepath.Join(abs, intoAbs)
	}
	if opts.Domain != "" && utils.SanitizeName(opts.Domain) != opts.Domain {
		return nil, oops.Hint("Use lowercase letters, digits and hyphens").Errorf("invalid domain name %q", opts.Domain)
	}

	importers, err := pickImporters(abs, opts.From)
	if err != nil {
		return nil, err
	}
	importers, autoSkippedNative := preferRulesync(importers, opts.From)

	plan, err := runImporters(abs, importers, Options{SplitHeadings: opts.SplitHeadings, BestEffort: opts.BestEffort})
	if err != nil {
		return nil, err
	}
	if autoSkippedNative {
		plan.add(newFinding(StatusDropped, "(native files)", "", "",
			"CLAUDE.md, AGENTS.md, .cursor/rules and the other tool files are generated from .rulesync/ and were not imported; use --from native,rulesync to import both"))
		sortFindings(plan)
	}
	names := make([]string, 0, len(importers))
	for _, imp := range importers {
		names = append(names, imp.Name())
	}
	if len(plan.Items) == 0 && len(plan.MCPServers) == 0 && len(plan.InstalledSkills) == 0 {
		return nil, oops.Hint("Run `ai-rulez convert --list` to see what each importer detects").
			Errorf("nothing to convert: the selected importers found no importable content in %s", abs)
	}

	cfg := buildConfig(plan, filepath.Base(abs))
	report := &Report{
		SchemaVersion: ReportSchemaVersion,
		Importer:      strings.Join(names, ","),
		Source:        displayPath(opts.Source),
		Into:          filepath.ToSlash(into),
		Findings:      plan.Findings,
		plan:          planSummary(plan),
	}

	files, err := buildFiles(plan, cfg, opts.Domain)
	if err != nil {
		return nil, err
	}
	if err := checkTargets(abs, intoAbs, files); err != nil {
		return report, err
	}
	cfgAction, err := resolveConfig(intoAbs, cfg, plan, report, files)
	if err != nil {
		return report, err
	}
	classify(report, files, intoAbs, opts.Force, cfgAction)
	report.count()

	if err := checkStaged(ctx, report, files, cfg); err != nil {
		return nil, err
	}
	if report.Security.Blocked || report.Validation.Errors > 0 || !opts.Write {
		return report, nil
	}
	if report.Conflicts() > 0 && !opts.Force {
		return report, oops.Hint("Rerun with --force to replace the existing files, or --domain NAME to import beside them").
			Wrap(ErrConflicts)
	}
	if err := writeFiles(report, files, intoAbs); err != nil {
		return report, err
	}
	report.Written = true
	return report, nil
}

func displayPath(p string) string {
	if p == "" {
		return "."
	}
	return filepath.ToSlash(p)
}

func pickImporters(abs string, from []string) ([]Format, error) {
	var names []string
	for _, f := range from {
		for _, n := range strings.Split(f, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
	}
	if len(names) == 0 || (len(names) == 1 && names[0] == autoFrom) {
		return autoImporters(abs)
	}
	var out []Format
	for _, n := range names {
		imp, ok := Lookup(n)
		if !ok {
			var known []string
			for _, r := range Registry() {
				known = append(known, r.Name())
			}
			return nil, oops.Hint("Known importers: "+strings.Join(known, ", ")).Errorf("unknown importer %q", n)
		}
		out = append(out, imp)
	}
	return out, nil
}

// preferRulesync drops the native importer from an automatic run that also
// detects a rulesync project: the tool files next to .rulesync/ are its
// generated output, so importing them as well would duplicate every rule.
// An explicit --from keeps what was asked for.
func preferRulesync(importers []Format, from []string) ([]Format, bool) {
	for _, f := range from {
		if n := strings.TrimSpace(f); n != "" && n != autoFrom {
			return importers, false
		}
	}
	hasRulesync, hasNative := false, false
	for _, imp := range importers {
		hasRulesync = hasRulesync || imp.Name() == rulesyncName
		hasNative = hasNative || imp.Name() == nativeName
	}
	if !hasRulesync || !hasNative {
		return importers, false
	}
	var out []Format
	for _, imp := range importers {
		if imp.Name() != nativeName {
			out = append(out, imp)
		}
	}
	return out, true
}

// autoImporters returns every importer that detects something. They all run;
// runImporters puts skills-lock first so native skips the skills it tracks.
func autoImporters(abs string) ([]Format, error) {
	fsys := os.DirFS(abs)
	var found []Format
	for _, imp := range Registry() {
		if len(imp.Detect(fsys)) > 0 {
			found = append(found, imp)
		}
	}
	if len(found) == 0 {
		return nil, oops.Hint("Run `ai-rulez convert --list` to see what each importer looks for").
			Errorf("no importable files found in %s", abs)
	}
	return found, nil
}

// runImporters plans every importer and merges the plans. The skills-lock
// importer runs first so the native importer skips the skills it tracks.
func runImporters(abs string, importers []Format, opt Options) (*Plan, error) {
	fsys := os.DirFS(abs)
	sort.SliceStable(importers, func(i, j int) bool {
		return importers[i].Name() == skillsLockName && importers[j].Name() != skillsLockName
	})
	merged := &Plan{}
	for _, imp := range importers {
		p, err := imp.Plan(fsys, opt)
		if err != nil {
			return nil, oops.With("importer", imp.Name()).Wrapf(err, "plan %s import", imp.Name())
		}
		if imp.Name() == skillsLockName && len(p.InstalledSkills) > 0 {
			opt.SkipSkills = map[string]bool{}
			for _, s := range p.InstalledSkills {
				opt.SkipSkills[s.Name] = true
			}
		}
		merged.merge(p)
	}
	merged.Finalize()
	return merged, nil
}

func buildConfig(plan *Plan, project string) *config.Config {
	if project == "" || project == "." || project == string(filepath.Separator) {
		project = "imported-project"
	}
	cfg := &config.Config{
		Version:         config.ConfigVersionV4,
		Name:            project,
		MCPServersRaw:   plan.MCPServers,
		InstalledSkills: plan.InstalledSkills,
	}
	for _, p := range plan.Presets {
		cfg.Presets = append(cfg.Presets, config.Preset{BuiltIn: p})
	}
	if len(cfg.Presets) == 0 {
		plan.presetDefaulted = true
		cfg.Presets = []config.Preset{{BuiltIn: string(config.PresetClaude)}}
		plan.add(newFinding(StatusApproximated, "(project)", "presets", "presets",
			"no tool-specific file identifies a preset (AGENTS.md and .agents/skills are shared by many tools); defaulting to claude, adjust `presets` in config.toml"))
		sortFindings(plan)
	}
	return cfg
}

func sortFindings(p *Plan) {
	sort.SliceStable(p.Findings, func(i, j int) bool {
		a, b := p.Findings[i], p.Findings[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return a.Status < b.Status
	})
}

func planSummary(p *Plan) string {
	counts := map[Kind]int{}
	for _, it := range p.Items {
		counts[it.Kind]++
	}
	var parts []string
	for _, k := range []struct {
		kind  Kind
		label string
	}{{KindRule, "rules"}, {KindContext, "context"}, {KindSkill, "skills"}, {KindAgent, "agents"}, {KindCommand, "commands"}, {KindCheck, "checks"}} {
		if n := counts[k.kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k.label))
		}
	}
	if n := len(p.MCPServers); n > 0 {
		parts = append(parts, fmt.Sprintf("%d mcp servers", n))
	}
	if n := len(p.InstalledSkills); n > 0 {
		parts = append(parts, fmt.Sprintf("%d installed skills", n))
	}
	return strings.Join(parts, ", ")
}

// buildFiles renders every file, keyed by path relative to the config directory.
func buildFiles(plan *Plan, cfg *config.Config, domain string) (map[string][]byte, error) {
	files := map[string][]byte{}
	prefix := ""
	if domain != "" {
		prefix = "domains/" + domain + "/"
	}
	for i := range plan.Items {
		for _, f := range plan.Items[i].Files() {
			files[prefix+f.Path] = f.Data
		}
	}
	data, err := config.MarshalTOML(cfg)
	if err != nil {
		return nil, oops.Wrapf(err, "render config.toml")
	}
	files["config.toml"] = data
	return files, nil
}

const (
	configTOML         = "config.toml"
	presetsFindingPath = "(project)"
)

// otherConfigNames are the config formats the loader also reads.
var otherConfigNames = []string{"config.yaml", "config.yml", "config.json"}

// resolveConfig decides what happens to config.toml. An existing one is never
// replaced, with or without --force: the new presets, [[mcp_servers]] and
// [[installed_skills]] are merged into it (existing entries win), and a config
// in another format or one that cannot be parsed stops the run. The returned
// action is the config.toml action, or "" when it is not written.
func resolveConfig(intoAbs string, cfg *config.Config, plan *Plan, report *Report, files map[string][]byte) (string, error) {
	for _, name := range otherConfigNames {
		if _, err := os.Lstat(filepath.Join(intoAbs, name)); err != nil {
			continue
		}
		if _, err := os.Lstat(filepath.Join(intoAbs, configTOML)); err == nil {
			continue
		}
		delete(files, configTOML)
		report.Findings = append(report.Findings, newFinding(StatusNeedsAction, name, "", "",
			fmt.Sprintf("%s already exists in another format and was not changed; add by hand: %s", name, configSummary(plan))))
		report.Files = append(report.Files, FileAction{Path: name, Action: ActionManual})
		sortFindings(&Plan{Findings: report.Findings})
		return "", nil
	}

	path := filepath.Join(intoAbs, configTOML)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ActionCreate, nil
	}
	if err != nil {
		return "", oops.With("path", path).Wrapf(err, "read existing %s", configTOML)
	}
	existing, err := config.DecodeTOMLConfig(data, path)
	if err != nil {
		return "", oops.With("path", path).
			Hint("Fix or move the existing config.toml; convert never replaces it").
			Wrapf(err, "existing %s cannot be parsed", configTOML)
	}

	merged, added, notes := mergeConfig(existing, cfg, plan.presetDefaulted)
	for _, n := range notes {
		report.Findings = append(report.Findings, newFinding(StatusNeedsAction, configTOML, n.field, "", n.reason))
	}
	if plan.presetDefaulted && len(existing.Presets) > 0 {
		kept := report.Findings[:0:0]
		for _, f := range report.Findings {
			if f.Source == presetsFindingPath && f.Field == "presets" {
				continue
			}
			kept = append(kept, f)
		}
		report.Findings = kept
	}
	sortFindings(&Plan{Findings: report.Findings})
	if added == 0 {
		files[configTOML] = data
		return ActionUnchanged, nil
	}
	rendered, err := config.MarshalTOML(merged)
	if err != nil {
		return "", oops.Wrapf(err, "render merged %s", configTOML)
	}
	files[configTOML] = rendered
	if bytes.Equal(rendered, data) {
		return ActionUnchanged, nil
	}
	return ActionMerge, nil
}

type mergeNote struct{ field, reason string }

// mergeConfig adds the imported presets, MCP servers and installed skills to an
// existing config. Entries the config already has by name are kept as they are;
// a differing imported one is reported, not applied. added counts what was appended.
func mergeConfig(existing, add *config.Config, defaultedPreset bool) (merged *config.Config, added int, notes []mergeNote) {
	merged = existing
	hasPreset := func(name string) bool {
		for i := range merged.Presets {
			if merged.Presets[i].BuiltIn == name || merged.Presets[i].GetName() == name {
				return true
			}
		}
		return false
	}
	if !(defaultedPreset && len(existing.Presets) > 0) {
		for _, p := range add.Presets {
			if !hasPreset(p.BuiltIn) {
				merged.Presets = append(merged.Presets, p)
				added++
			}
		}
	}
	servers := map[string]config.MCPServer{}
	for _, s := range merged.MCPServersRaw {
		servers[s.Name] = s
	}
	for _, s := range add.MCPServersRaw {
		prev, ok := servers[s.Name]
		switch {
		case !ok:
			merged.MCPServersRaw = append(merged.MCPServersRaw, s)
			added++
		case !reflect.DeepEqual(prev, s):
			notes = append(notes, mergeNote{"mcp_servers." + s.Name, "an mcp_servers entry named " + s.Name + " already exists and was kept; the imported definition differs"})
		}
	}
	skills := map[string]config.InstalledSkillConfig{}
	for _, s := range merged.InstalledSkills {
		skills[s.Name] = s
	}
	for _, s := range add.InstalledSkills {
		prev, ok := skills[s.Name]
		switch {
		case !ok:
			merged.InstalledSkills = append(merged.InstalledSkills, s)
			added++
		case !reflect.DeepEqual(prev, s):
			notes = append(notes, mergeNote{"installed_skills." + s.Name, "an installed_skills entry named " + s.Name + " already exists and was kept; the imported one differs"})
		}
	}
	return merged, added, notes
}

// classify compares the planned files with the disk and fills report.Files.
// cfgAction is the already decided action of config.toml ("" when it is not written).
func classify(report *Report, files map[string][]byte, intoAbs string, force bool, cfgAction string) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		if rel == configTOML {
			if cfgAction != "" {
				report.Files = append(report.Files, FileAction{Path: rel, Action: cfgAction})
			}
			continue
		}
		existing, err := os.ReadFile(filepath.Join(intoAbs, filepath.FromSlash(rel)))
		action := ActionCreate
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err == nil && bytes.Equal(existing, files[rel]):
			action = ActionUnchanged
		case force:
			action = ActionOverwrite
		default:
			action = ActionConflict
		}
		report.Files = append(report.Files, FileAction{Path: rel, Action: action})
	}
	sort.SliceStable(report.Files, func(i, j int) bool { return report.Files[i].Path < report.Files[j].Path })
}

func configSummary(p *Plan) string {
	var parts []string
	if len(p.Presets) > 0 {
		parts = append(parts, "presets = ["+strings.Join(quoteAll(p.Presets), ", ")+"]")
	}
	if n := len(p.MCPServers); n > 0 {
		parts = append(parts, fmt.Sprintf("%d [[mcp_servers]]", n))
	}
	if n := len(p.InstalledSkills); n > 0 {
		parts = append(parts, fmt.Sprintf("%d [[installed_skills]]", n))
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

func quoteAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// checkStaged writes the planned tree to a scratch project and loads and scans
// it, so a tree that does not validate or carries a secret is never written to
// the real project. installed_skills are validated field by field instead of
// loaded: loading them would fetch from the network. The security scan always
// runs, also when validation fails, and also covers config.toml, which the
// project scan does not read, with inline suppression comments ignored.
func checkStaged(ctx context.Context, report *Report, files map[string][]byte, cfg *config.Config) error {
	tmp, err := os.MkdirTemp("", "ai-rulez-convert-*")
	if err != nil {
		return oops.Wrapf(err, "create scratch directory")
	}
	defer os.RemoveAll(tmp)

	root := filepath.Join(tmp, DefaultConfigDir)
	for rel, data := range files {
		if err := writeFileAtomic(filepath.Join(root, filepath.FromSlash(rel)), data, 0o644); err != nil {
			return oops.Wrapf(err, "stage %s", rel)
		}
	}
	staged := *cfg
	staged.InstalledSkills = nil
	data, err := config.MarshalTOML(&staged)
	if err != nil {
		return oops.Wrapf(err, "render staged config")
	}
	if err := writeFileAtomic(filepath.Join(root, configTOML), data, 0o644); err != nil {
		return oops.Wrapf(err, "stage %s", configTOML)
	}

	invalid := func(err error) {
		report.Validation.Errors++
		report.Validation.Messages = append(report.Validation.Messages, firstLine(err.Error()))
	}
	if err := config.ValidateInstalledSkills(cfg.InstalledSkills); err != nil {
		invalid(err)
	}
	loaded, err := config.LoadConfigFromDir(ctx, tmp, DefaultConfigDir, config.WithoutLocal(), config.WithoutRemote())
	if err == nil {
		err = loaded.Validate()
	}
	if err != nil {
		invalid(err)
	}

	var found []lint.Finding
	if loaded != nil {
		var loader lint.Loader
		tree, lerr := loader.Load(loaded.BaseDir)
		if lerr != nil {
			return oops.Wrapf(lerr, "index scratch project")
		}
		lr, rerr := lint.RunWith(loaded, tree, lint.Options{SecurityOnly: true})
		if rerr != nil {
			return oops.Wrapf(rerr, "security scan")
		}
		found = append(found, lr.Findings...)
	}
	// The project scan honours inline ignore comments and skips config.toml;
	// converted text is not trusted to silence itself, so scan every staged text again.
	rels := make([]string, 0, len(files)+1)
	for rel := range files {
		if rel != configTOML {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if isText(files[rel]) {
			found = append(found, lint.ScanText(DefaultConfigDir+"/"+rel, string(files[rel]))...)
		}
	}
	found = append(found, lint.ScanText(DefaultConfigDir+"/"+configTOML, string(data))...)

	report.Security.Findings = []SecurityFinding{}
	seen := map[string]bool{}
	for _, f := range found {
		sf := SecurityFinding{Code: f.Code, Severity: string(f.Severity), File: stagedRel(f.File), Line: f.Line, Message: f.Message}
		key := fmt.Sprintf("%s|%s|%d|%s", sf.Code, sf.File, sf.Line, sf.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		report.Security.Findings = append(report.Security.Findings, sf)
		if f.Severity == lint.SeverityError {
			report.Security.Blocked = true
		}
	}
	sort.SliceStable(report.Security.Findings, func(i, j int) bool {
		a, b := report.Security.Findings[i], report.Security.Findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	if report.Security.Blocked {
		report.Security.Code = CodeBlockedScan
	}
	return nil
}

func isText(data []byte) bool {
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}

func stagedRel(file string) string {
	file = filepath.ToSlash(file)
	if i := strings.Index(file, "/"+DefaultConfigDir+"/"); i >= 0 {
		return file[i+1:]
	}
	return file
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// checkTargets refuses to write through a symlink: every component below the
// source directory (or the --into directory itself when it lies elsewhere) that
// exists must be a real directory or file, for every path convert would write.
func checkTargets(abs, intoAbs string, files map[string][]byte) error {
	base := abs
	if rel, err := filepath.Rel(abs, intoAbs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		base = filepath.Dir(intoAbs)
	}
	rels := make([]string, 0, len(files)+1)
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	rels = append(rels, ".")
	for _, rel := range rels {
		full := filepath.Join(intoAbs, filepath.FromSlash(rel))
		sub, err := filepath.Rel(base, full)
		if err != nil {
			return oops.With("path", full).Wrapf(err, "resolve target path")
		}
		cur := base
		for _, comp := range strings.Split(sub, string(filepath.Separator)) {
			if comp == "." || comp == "" {
				continue
			}
			cur = filepath.Join(cur, comp)
			info, err := os.Lstat(cur)
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			if err != nil {
				return oops.With("path", cur).Wrapf(err, "inspect target path")
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return oops.With("path", cur).
					Hint("Remove the symlink, or choose another --into").
					Errorf("refusing to write through the symlink %s", cur)
			}
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".convert-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// mkdirAllTracked creates dir and returns the directories it had to create,
// outermost first, so a rollback can remove them again.
func mkdirAllTracked(dir string) ([]string, error) {
	var missing []string
	for d := dir; ; {
		if _, err := os.Lstat(d); err == nil {
			break
		}
		missing = append(missing, d)
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
		missing[i], missing[j] = missing[j], missing[i]
	}
	return missing, os.MkdirAll(dir, 0o755)
}

// writeFiles writes the create, overwrite and merge entries. On failure it
// restores what it changed and removes the directories it created, so a failed
// run leaves the project as it was; anything it could not undo is reported.
func writeFiles(report *Report, files map[string][]byte, intoAbs string) error {
	type undo struct {
		path string
		old  []byte
		had  bool
	}
	var done []undo
	var dirs []string
	rollback := func() error {
		var errs []error
		for i := len(done) - 1; i >= 0; i-- {
			u := done[i]
			if u.had {
				if err := writeFileAtomic(u.path, u.old, 0o644); err != nil {
					errs = append(errs, err)
				}
			} else if err := os.Remove(u.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		}
		for i := len(dirs) - 1; i >= 0; i-- {
			if err := os.Remove(dirs[i]); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	for _, f := range report.Files {
		if f.Action != ActionCreate && f.Action != ActionOverwrite && f.Action != ActionMerge {
			continue
		}
		target := filepath.Join(intoAbs, filepath.FromSlash(f.Path))
		created, err := mkdirAllTracked(filepath.Dir(target))
		dirs = append(dirs, created...)
		if err == nil {
			old, readErr := os.ReadFile(target)
			if err = writeFileAtomic(target, files[f.Path], 0o644); err == nil {
				done = append(done, undo{path: target, old: old, had: readErr == nil})
				continue
			}
		}
		if rbErr := rollback(); rbErr != nil {
			return oops.With("path", target).Wrapf(errors.Join(err, rbErr), "write %s failed and the rollback was incomplete", f.Path)
		}
		return oops.With("path", target).Wrapf(err, "write %s", f.Path)
	}
	return nil
}
