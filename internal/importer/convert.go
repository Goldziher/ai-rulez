package importer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/utils"
	"github.com/samber/oops"
)

// DefaultConfigDir is the config directory convert writes to by default.
const DefaultConfigDir = ".ai-rulez"

const autoFrom = "auto"

// ErrNothingToConvert is returned by Convert when the selected importers found
// nothing to import. It is an outcome, not a failure: the command reports it and
// exits 0.
var ErrNothingToConvert = errors.New("nothing to convert")

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
	// AllowFindings lists scan codes (for example AR001) whose error findings do
	// not block the write. They stay in the report, marked allowed.
	AllowFindings []string
	// EnableHooks writes imported hooks as live [[hooks]]; without it they are a
	// commented block. EnablePermissions does the same for imported allow rules
	// (ask and deny rules are always live, they only narrow).
	EnableHooks       bool
	EnablePermissions bool
	// Merge adds beside an existing tree without touching a file of it: an item
	// whose file exists with other content is imported as NAME-imported. It
	// excludes Force.
	Merge bool
	// KeepNames never renames to resolve a collision (between imported items, or
	// with an existing file under Merge): the collision is an error.
	KeepNames bool
	// Delivery sets how the imported skills reach the agent: static, served or
	// both ([skills] delivery, or the domain's when Domain is set).
	Delivery string
	// Fetch lets convert read the remote git sources the input names (rulesync
	// sources, APM dependencies that are not installed) over the network. Without
	// it nothing is fetched and each source is reported.
	Fetch bool
	// Fetcher replaces the default git fetcher; for tests.
	Fetcher Fetcher
	// NativePaths limits the native importer to these project paths (a file or a
	// directory), the way `init --from .claude,CLAUDE.md` names its sources.
	NativePaths []string
}

// Detection is what one importer recognizes in a source directory.
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

// conversion is the state of one Convert run between planning and writing.
type conversion struct {
	abs, into, intoAbs string
	importers          []Format
	plan               *Plan
}

// Convert reads the source in the selected formats, builds the report and, with
// opts.Write, writes the converted tree. It never modifies the source files.
func Convert(ctx context.Context, opts ConvertOptions) (*Report, error) {
	c, err := planConversion(ctx, opts)
	if err != nil {
		return nil, err
	}
	p, report, err := c.prepare(ctx, opts)
	if err != nil || p == nil {
		return report, err
	}
	if report.Security.Blocked || report.Validation.Errors > 0 || !opts.Write {
		return report, nil
	}
	if report.Conflicts() > 0 && !opts.Force {
		return report, oops.Hint("Rerun with --force to replace the existing files, or --domain NAME to import beside them").
			Wrap(ErrConflicts)
	}
	if err := writeFiles(report, p.files, p.execs, c.intoAbs); err != nil {
		return report, err
	}
	report.Written = true
	if err := recordImported(c); err != nil {
		return report, err
	}
	if err := ignoreLocalTree(logger.FromContext(ctx), c.abs, c.intoAbs, p.files); err != nil {
		return report, err
	}
	return report, nil
}

// planConversion resolves the paths, runs the importers and makes room for the
// items of a --merge run. Nothing is written.
func planConversion(ctx context.Context, opts ConvertOptions) (*conversion, error) {
	abs, err := absDir(opts.Source)
	if err != nil {
		return nil, err
	}
	c := &conversion{abs: abs, into: opts.Into}
	if c.into == "" {
		c.into = DefaultConfigDir
	}
	c.intoAbs = c.into
	if !filepath.IsAbs(c.intoAbs) {
		c.intoAbs = filepath.Join(abs, c.intoAbs)
	}
	if opts.Domain != "" && utils.SanitizeName(opts.Domain) != opts.Domain {
		return nil, oops.Hint("Use lowercase letters, digits and hyphens").Errorf("invalid domain name %q", opts.Domain)
	}
	if opts.Merge && opts.Force {
		return nil, oops.Hint("--merge keeps every existing file; --force replaces them").Errorf("--merge and --force cannot be combined")
	}

	importers, err := pickImporters(abs, opts.From)
	if err != nil {
		return nil, err
	}
	importers, generatedFrom := preferSources(importers, opts.From)
	c.importers = importers
	c.plan, err = runImporters(ctx, abs, importers, Options{
		SplitHeadings: opts.SplitHeadings, BestEffort: opts.BestEffort, KeepNames: opts.KeepNames, Fetch: opts.Fetch,
		Fetcher: opts.Fetcher, Domain: opts.Domain, NativePaths: opts.NativePaths,
	})
	if err != nil {
		return nil, err
	}
	if len(c.plan.collisions) > 0 {
		return nil, oops.Hint("Rename one of the sources, or drop --keep-names to give the later one a stable suffix").
			Errorf("name collisions with --keep-names: %s", describeCollisions(c.plan.collisions))
	}
	if generatedFrom != "" {
		c.plan.add(newFinding(StatusDropped, "(native files)", "", "",
			"CLAUDE.md, AGENTS.md, .cursor/rules and the other tool files are generated from "+generatedFrom+" and were not imported; use --from native,"+strings.Join(importerNames(importers), ",")+" to import both"))
		sortFindings(c.plan)
	}
	if c.plan.empty() {
		hint := "Run `ai-rulez convert --list` to see what each importer detects"
		if len(c.plan.Remotes) > 0 {
			hint = fmt.Sprintf("The input names %d remote source(s) that are not on disk; rerun with --fetch to import them", len(c.plan.Remotes))
		}
		return nil, oops.Hint(hint).
			Wrapf(ErrNothingToConvert, "the selected importers found no importable content in %s", abs)
	}
	if opts.Merge {
		if err := mergeRename(c.plan, c.intoAbs, opts.Domain, opts.KeepNames); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// prepared is the planned tree: every file, keyed by path below the config directory.
type prepared struct {
	files map[string][]byte
	// execs are the paths of files written with the execute bits.
	execs map[string]bool
}

// prepare renders the config and the files, classifies them against the disk and
// runs the scan and validation of the planned tree. It writes nothing.
func (c *conversion) prepare(ctx context.Context, opts ConvertOptions) (*prepared, *Report, error) {
	plan := c.plan
	reportDisabled(plan, opts.EnableHooks, opts.EnablePermissions)
	live, off := splitEnabled(plan, opts.EnableHooks, opts.EnablePermissions)
	cfg := buildConfig(plan, filepath.Base(c.abs))
	cfg.Hooks, cfg.Permissions = live.Hooks, live.Permissions
	if err := applyDelivery(cfg, plan, opts.Delivery, opts.Domain); err != nil {
		return nil, nil, err
	}
	sortFindings(plan)
	// The staged copy carries every imported hook and rule, live or not, so
	// validation and the scan cover the commented block too.
	stage := *cfg
	stage.Hooks = plan.Hooks
	if !plan.Permissions.IsEmpty() {
		all := plan.Permissions
		stage.Permissions = &all
	}
	report := &Report{
		SchemaVersion: ReportSchemaVersion,
		Importer:      strings.Join(importerNames(c.importers), ","),
		Source:        displayPath(opts.Source),
		Into:          filepath.ToSlash(c.into),
		Findings:      plan.Findings,
		plan:          planSummary(plan),
		needsLock:     len(plan.InstalledSkills) > 0,
	}

	files, err := buildFiles(plan, cfg, opts.Domain)
	if err != nil {
		return nil, nil, err
	}
	if err := checkTargets(c.abs, c.intoAbs, files); err != nil {
		return nil, report, err
	}
	cfgAction, err := resolveConfig(c.intoAbs, cfg, plan, report, files, &off)
	if err != nil {
		return nil, report, err
	}
	block, err := off.render()
	if err != nil {
		return nil, report, err
	}
	cfgAction = appendDisabled(files, block, cfgAction)
	classify(report, files, c.intoAbs, opts.Force, cfgAction)
	report.count()

	sc := scanContext{srcDir: c.abs, origins: origins(plan, opts.Domain), allow: opts.AllowFindings, fetched: plan.fetchedText}
	if err := checkStaged(ctx, report, files, &stage, sc); err != nil {
		return nil, nil, err
	}
	return &prepared{files: files, execs: execFiles(plan, opts.Domain)}, report, nil
}

// ignoreLocalTree keeps the personal content convert wrote below local/ out of
// git until `generate` takes over that job: it adds the tree to the project's
// .gitignore. A config directory outside the project cannot be ignored from it.
func ignoreLocalTree(log logger.Logger, abs, intoAbs string, files map[string][]byte) error {
	wrote := false
	for rel := range files {
		if strings.HasPrefix(rel, localDir+"/") {
			wrote = true
			break
		}
	}
	if !wrote {
		return nil
	}
	rel, inside := relInside(abs, intoAbs)
	if !inside {
		return nil
	}
	pattern := path.Join(filepath.ToSlash(rel), localDir) + "/"
	if err := gitignore.EnsureEntries(log, abs, []string{pattern}); err != nil {
		return oops.Hint("Add "+pattern+" to .gitignore by hand: it holds personal content").Wrapf(err, "ignore the local tree")
	}
	return nil
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

func importerNames(importers []Format) []string {
	names := make([]string, 0, len(importers))
	for _, imp := range importers {
		names = append(names, imp.Name())
	}
	return names
}

// generatorRoots name the input trees whose tool files are generated output.
var generatorRoots = map[string]string{rulesyncName: ".rulesync/", apmName: ".apm/ and apm_modules/", tesslName: ".tessl/"}

// preferSources drops the native importer from an automatic run that also
// detects a project of a tool that generates the tool files (rulesync, APM): the
// files next to its inputs are its output, so importing them as well would
// duplicate every rule. It returns the sources that made it drop native, "" when
// it did not. An explicit --from keeps what was asked for.
func preferSources(importers []Format, from []string) ([]Format, string) {
	for _, f := range from {
		if n := strings.TrimSpace(f); n != "" && n != autoFrom {
			return importers, ""
		}
	}
	hasNative := false
	var roots []string
	for _, imp := range importers {
		hasNative = hasNative || imp.Name() == nativeName
		if root, ok := generatorRoots[imp.Name()]; ok {
			roots = append(roots, root)
		}
	}
	if !hasNative || len(roots) == 0 {
		return importers, ""
	}
	var out []Format
	for _, imp := range importers {
		if imp.Name() != nativeName {
			out = append(out, imp)
		}
	}
	return out, strings.Join(roots, " and ")
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
			Wrapf(ErrNothingToConvert, "no importable files found in %s", abs)
	}
	return found, nil
}

// runImporters plans every importer and merges the plans. The skills-lock
// importer runs first so the native importer skips the skills it tracks.
func runImporters(ctx context.Context, abs string, importers []Format, opt Options) (*Plan, error) {
	fsys := os.DirFS(abs)
	sort.SliceStable(importers, func(i, j int) bool {
		return importers[i].Name() == skillsLockName && importers[j].Name() != skillsLockName
	})
	merged := &Plan{keepNames: opt.KeepNames}
	for _, imp := range importers {
		source := fs.FS(fsys)
		if imp.Name() == nativeName {
			source = restrict(fsys, opt.NativePaths)
		}
		p, err := imp.Plan(source, opt)
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
	if err := merged.resolveRemotes(ctx, opt); err != nil {
		return nil, err
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
	if n := len(p.Raw); n > 0 {
		parts = append(parts, fmt.Sprintf("%d bundle files", n))
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
	for i := range plan.Items {
		for _, f := range plan.Items[i].Files() {
			files[placed(domain, f.Path)] = f.Data
		}
	}
	for _, f := range plan.Raw {
		files[f.Path] = f.Data // already below its domain
	}
	data, err := config.MarshalTOML(cfg)
	if err != nil {
		return nil, oops.Wrapf(err, "render config.toml")
	}
	files["config.toml"] = data
	return files, nil
}

// execFiles lists the planned files that keep an execute bit, keyed like buildFiles.
func execFiles(plan *Plan, domain string) map[string]bool {
	out := map[string]bool{}
	for i := range plan.Items {
		for _, f := range plan.Items[i].Files() {
			if f.Exec {
				out[placed(domain, f.Path)] = true
			}
		}
	}
	for _, f := range plan.Raw {
		if f.Exec {
			out[f.Path] = true
		}
	}
	return out
}

const (
	configTOML         = "config.toml"
	presetsFindingPath = "(project)"
)

// otherConfigNames are the V3 config formats ai-rulez no longer reads.
var otherConfigNames = []string{"config.yaml", "config.yml", "config.json"}

// resolveConfig decides what happens to config.toml. An existing one is never
// replaced, with or without --force: the new presets, [[mcp_servers]] and
// [[installed_skills]] are merged into it (existing entries win), and a config
// in a V3 format or one that cannot be parsed stops the run. The returned
// action is the config.toml action, or "" when it is not written.
func resolveConfig(intoAbs string, cfg *config.Config, plan *Plan, report *Report, files map[string][]byte, off *disabledSet) (string, error) {
	for _, name := range otherConfigNames {
		if _, err := os.Lstat(filepath.Join(intoAbs, name)); err != nil {
			continue
		}
		if _, err := os.Lstat(filepath.Join(intoAbs, configTOML)); err == nil {
			continue
		}
		delete(files, configTOML)
		report.Findings = append(report.Findings, newFinding(StatusNeedsAction, name, "", "",
			fmt.Sprintf("%s is a V3 config that ai-rulez no longer reads and was not changed; migrate it with ai-rulez 4.x (npx ai-rulez@4 migrate v4), or add by hand: %s", name, configSummary(plan))))
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

	off.withoutExisting(existing)
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
	if !defaultedPreset || len(existing.Presets) == 0 {
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
	added += mergeHooksAndPermissions(merged, add)
	n, deliveryNotes := mergeDelivery(merged, add)
	added += n
	notes = append(notes, deliveryNotes...)
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
	if n := len(p.Hooks); n > 0 {
		parts = append(parts, fmt.Sprintf("%d [[hooks]] (review each command first)", n))
	}
	if n := len(p.Permissions.Allow) + len(p.Permissions.Ask) + len(p.Permissions.Deny); n > 0 {
		parts = append(parts, fmt.Sprintf("%d [permissions] rule(s)", n))
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
		return discardTemp(tmp, name, err)
	}
	if err := tmp.Close(); err != nil {
		return discardTemp(nil, name, err)
	}
	if err := os.Chmod(name, perm); err != nil {
		return discardTemp(nil, name, err)
	}
	if err := os.Rename(name, path); err != nil {
		return discardTemp(nil, name, err)
	}
	return nil
}

// discardTemp closes (when open) and removes a temporary file after err, and
// returns err joined with any failure to clean up.
func discardTemp(open *os.File, name string, err error) error {
	if open != nil {
		err = errors.Join(err, open.Close())
	}
	if rerr := os.Remove(name); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		err = errors.Join(err, rerr)
	}
	return err
}

// relInside returns target relative to base, and false when it is not below base.
func relInside(base, target string) (string, bool) {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// removeScratch deletes a scratch directory; a failure only leaves temporary
// files behind, so it is logged rather than returned.
func removeScratch(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		logger.Debug("could not remove the scratch directory", "path", dir, "error", err)
	}
}

// writePerms returns the file and directory modes of a planned path. Machine-local
// content is personal, so it is owner-only like the files the CLI creates there.
func writePerms(rel string) (file, dir os.FileMode) {
	if rel == localDir || strings.HasPrefix(rel, localDir+"/") {
		return 0o600, 0o700
	}
	return 0o644, 0o755
}

// mkdirAllTracked creates dir and returns the directories it had to create,
// outermost first, so a rollback can remove them again.
func mkdirAllTracked(dir string, perm os.FileMode) ([]string, error) {
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
	return missing, os.MkdirAll(dir, perm)
}

// writeFiles writes the create, overwrite and merge entries. On failure it
// restores what it changed and removes the directories it created, so a failed
// run leaves the project as it was; anything it could not undo is reported.
func writeFiles(report *Report, files map[string][]byte, execs map[string]bool, intoAbs string) error {
	type undo struct {
		path string
		old  []byte
		had  bool
		perm os.FileMode
	}
	var done []undo
	var dirs []string
	rollback := func() error {
		var errs []error
		for i := len(done) - 1; i >= 0; i-- {
			u := done[i]
			if u.had {
				if err := writeFileAtomic(u.path, u.old, u.perm); err != nil {
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
		filePerm, dirPerm := writePerms(f.Path)
		if execs[f.Path] {
			filePerm |= (filePerm & 0o444) >> 2 // x wherever r is set: 0644 -> 0755, 0600 -> 0700
		}
		created, err := mkdirAllTracked(filepath.Dir(target), dirPerm)
		dirs = append(dirs, created...)
		if err == nil {
			old, readErr := os.ReadFile(target)
			if err = writeFileAtomic(target, files[f.Path], filePerm); err == nil {
				done = append(done, undo{path: target, old: old, had: readErr == nil, perm: filePerm})
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

// recordImported leaves the record generate reads (generator.ConvertRecordName):
// the native files whose content now lives in the config directory. The first
// generate replaces them without --force; a file nobody imported stays protected.
func recordImported(c *conversion) error {
	seen := map[string]bool{}
	var rels []string
	add := func(rel string) {
		rel = path.Clean(strings.TrimPrefix(filepath.ToSlash(rel), "./"))
		if rel != "." && !seen[rel] && fs.ValidPath(rel) {
			seen[rel] = true
			rels = append(rels, rel)
		}
	}
	for _, it := range c.plan.Items {
		for _, s := range it.Sources {
			add(s)
		}
	}
	for _, s := range c.plan.Pointers {
		add(s)
	}
	files, err := collectImported(c.abs, rels)
	if err != nil {
		return err
	}
	return generator.WriteConvertRecord(c.intoAbs, files)
}

// collectImported reads the regular files at rels (a directory: every regular
// file below it) through an os.Root on the project, so a path swapped for a
// symlink between the check and the read cannot reach outside the project.
func collectImported(projectDir string, rels []string) (map[string][]byte, error) {
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return nil, oops.With("path", projectDir).Wrapf(err, "open the project to record the imported files")
	}
	defer func() { _ = root.Close() }()
	files := map[string][]byte{}
	for _, rel := range rels {
		info, err := root.Lstat(filepath.FromSlash(rel))
		switch {
		case err != nil || info.Mode()&os.ModeSymlink != 0:
		case info.IsDir():
			collectDirFiles(root, rel, files)
		case info.Mode().IsRegular() && info.Size() <= maxFileBytes:
			if data, rerr := root.ReadFile(filepath.FromSlash(rel)); rerr == nil {
				files[rel] = data
			}
		}
	}
	return files, nil
}

// collectDirFiles adds every regular file below dir (slash-separated, relative
// to root) to files, keyed by its path below root.
func collectDirFiles(root *os.Root, dir string, files map[string][]byte) {
	walkErr := fs.WalkDir(root.FS(), dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil //nolint:nilerr // an unreadable entry is simply not recorded
		}
		if info, ierr := d.Info(); ierr != nil || info.Size() > maxFileBytes {
			return nil //nolint:nilerr // too large to have been imported
		}
		if data, rerr := root.ReadFile(filepath.FromSlash(p)); rerr == nil {
			files[p] = data
		}
		return nil
	})
	_ = walkErr // the callback never fails; an unreadable directory is simply not recorded
}
