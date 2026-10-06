package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// Default description bounds. 1024 is the Agent Skills specification limit.
const (
	defaultDescMin   = 20
	defaultDescMax   = 1024
	defaultNearDup   = 0.85
	maxSkillNameLen  = 64
	minNearDupTokens = 5
)

// Content kinds.
const (
	kindRule    = "rule"
	kindContext = "context"
	kindSkill   = "skill"
	kindAgent   = "agent"
	kindCommand = "command"
	kindCheck   = "check"
)

var defaultBudgets = map[string]config.LintBudget{
	kindRule:    {MaxLines: 200, MaxTokens: 2500},
	kindContext: {MaxLines: 300, MaxTokens: 3000},
	kindSkill:   {MaxLines: 500, MaxTokens: 5000},
	kindAgent:   {MaxLines: 300, MaxTokens: 3000},
	kindCommand: {MaxLines: 300, MaxTokens: 3000},
}

// Names Claude Code provides itself, so a reference to them is never "unknown".
var builtinSlash = []string{
	"add-dir", "terminal-setup", "release-notes", "security-review", "pr-comments",
	"output-style", "install-github-app", "permissions", "init", "review", "compact", "clear",
}

var builtinAgents = []string{"general-purpose", "explore", "plan", "statusline-setup", "output-style-setup"}

// Report is the result of linting one root.
type Report struct {
	Root     string    `json:"root"`
	Findings []Finding `json:"findings"`
	// Baseline is set when a baseline was applied to the report.
	Baseline *BaselineResult `json:"-"`
	// Deps maps a file to the files it refers to (repository-relative slash
	// paths), the input of changed-only reporting.
	Deps map[string][]string `json:"-"`
	// Scope is set when the report was narrowed to changed files.
	Scope *ChangedScope `json:"-"`
	// Profile is the non-default lint profile the run used.
	Profile string `json:"-"`
	// Risk is the advisory risk score, set by the caller after the baseline.
	Risk *RiskReport `json:"-"`
	// Analyzers is the analyzer selection the run used (--analyzer or
	// [lint] analyzers); nil when every analyzer ran. Baseline entries of the
	// analyzers that did not run are neither stale nor rewritten.
	Analyzers []string `json:"-"`
	// Protected holds the codes the organization policy protects from
	// suppression (required or floored codes and AR740-AR745): a baseline never
	// accepts them and [lint.tolerate] never tolerates them.
	Protected map[string]bool `json:"-"`
	// ConfigFile is the display path of the configuration file, where the
	// policy reports a suppression attempt.
	ConfigFile string `json:"-"`
	// Units counts the units (checks and scans) the run executed, by name: the
	// proof that an analyzer that was not selected did not run.
	Units map[string]int `json:"-"`
	// unitRuns keeps the analyzers each unit declared, for tests.
	unitRuns map[string]unitRun
}

// Counts returns findings per severity.
func (r *Report) Counts() map[Severity]int {
	out := map[Severity]int{}
	for _, f := range r.Findings {
		out[f.Severity]++
	}
	return out
}

// Failed reports whether any finding is at least as severe as failOn.
func Failed(findings []Finding, failOn string) bool {
	if failOn == "none" {
		return false
	}
	threshold, ok := ParseSeverity(failOn)
	if !ok || threshold == SeverityOff {
		threshold = SeverityError
	}
	for i := range findings {
		if findings[i].Severity.AtLeast(threshold) && !findings[i].IsAccepted() {
			return true
		}
	}
	return false
}

type item struct {
	kind    string
	abs     string
	domain  string
	cf      config.ContentFile
	owned   bool
	isDoc   bool // a markdown resource of a skill or command: body checks only
	itemDir string
}

type runner struct {
	cfg          *config.Config
	mcpOnce      sync.Once
	mcpEffective []config.MCPServer
	lc           config.LintConfig
	tree         *Tree
	baseRel      string
	cwd          string
	host         ambient.Host
	sev          map[string]Severity
	ignore       map[string]bool
	ignorePaths  []globMatcher
	allow        []globMatcher
	skills       map[string]bool
	commands     map[string]bool
	agents       map[string]bool
	rules        map[string]bool
	contexts     map[string]bool
	items        []item
	docs         map[string]doc
	counter      tokens.Counter
	findings     []Finding
	// protected holds the codes the organization policy protects from every
	// suppression route; attempts records the routes a repository tried on them.
	protected map[string]bool
	attempts  map[string]map[string]bool
	// forceSev replaces the severity of every finding while imported content is
	// scanned (lint.security.scan_imports).
	forceSev Severity
	// noInlineIgnore refuses `ai-rulez-lint-ignore` comments (served content).
	noInlineIgnore bool
	opts           Options
	drift          []PluginDrift
	delivery       []DeliveryFinding
	lockDrift      []LockDrift
	approvals      []ApprovalFinding
	okfDir         string
	okfFindings    []okf.Finding
	// deps records which file refers to which (both absolute): links, name
	// references, skill resources and hook scripts. --since uses it to report
	// files that refer to a changed file.
	deps  map[string]map[string]struct{}
	names map[string][]string
	// exampleGlobs are the lint.example_paths; exampleCache memoizes the
	// example-fence lines per file.
	exampleGlobs []globMatcher
	exampleCache map[string]map[int]bool
	// sel is the analyzer selection (nil: all); units counts what ran; cur is
	// the unit being executed.
	sel   map[string]bool
	units map[string]unitRun
	cur   *unitSpec
	// fmMCP caches the inline mcpServers of agent and skill frontmatter.
	fmMCP     []*mcpServer
	fmMCPDone bool
}

// Options selects what a run does beyond the default strict checks.
type Options struct {
	// SecurityOnly keeps only the security family (AR0xx).
	SecurityOnly bool
	// External also runs the scanners configured in lint.external.
	External bool
	// AllowEgress names the [[lint.external]] scanners with egress = true that
	// may run in this invocation (--allow-egress).
	AllowEgress []string
	// Scanner holds the scanner baseline and suppression settings of --external.
	Scanner ScannerOptions
	// Analyzers runs only these analyzers (--analyzer). Empty uses the
	// [lint] analyzers setting, and then every analyzer.
	Analyzers []string
	// Cwd is the directory finding paths are shown relative to (the caller's
	// working directory); empty shows them as given.
	Cwd string
	// NeedDeps keeps the checks that build the reference graph running although
	// their analyzer is not selected: changed-only reporting (--since) needs it.
	NeedDeps bool
}

// PluginDrift describes a generated plugin whose content changed against the
// baseline while its declared version stayed the same.
type PluginDrift struct {
	// Plugin is the plugin name.
	Plugin string
	// File is the absolute path of the manifest that carries the version.
	File string
	// Version is the unchanged version.
	Version string
	// Changed lists some of the bundle files whose content changed.
	Changed []string
}

// Option adds inputs the runner cannot compute from the repository tree alone.
type Option func(*runner)

// WithHost injects the environment, clock and process runner the run may use
// (nil fields keep the real ones).
func WithHost(h ambient.Host) Option { return func(r *runner) { r.host = h } }

// WithCwd sets the directory finding paths are shown relative to; without it
// they are shown as given.
func WithCwd(dir string) Option { return func(r *runner) { r.cwd = dir } }

// clock is the run's time: the injected clock, else the package clock.
func (r *runner) clock() time.Time {
	if r.host.Clock != nil {
		return r.host.Clock.Now()
	}
	return now()
}

// WithPluginDrift supplies the plugin version drift to report as AR961.
func WithPluginDrift(drift []PluginDrift) Option {
	return func(r *runner) { r.drift = drift }
}

// Run lints one loaded configuration against the repository tree.
func Run(cfg *config.Config, tree *Tree, opts ...Option) (*Report, error) {
	return RunWith(cfg, tree, Options{}, opts...)
}

// RunWith is Run with the security and external-scanner options.
func RunWith(cfg *config.Config, tree *Tree, so Options, opts ...Option) (*Report, error) {
	counter, err := tokens.New("")
	if err != nil {
		return nil, fmt.Errorf("token counter: %w", err)
	}
	r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}, counter: counter, opts: so, cwd: so.Cwd,
		deps: map[string]map[string]struct{}{}, names: map[string][]string{}}
	for _, opt := range opts {
		opt(r)
	}
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	baseAbs, _ := filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the given dir
	r.baseRel = tree.Rel(baseAbs)
	if r.baseRel == "." {
		r.baseRel = ""
	}
	r.resolveSettings()
	r.sel = parseSelection(so.Analyzers)
	if r.sel == nil {
		r.sel = parseSelection(r.lc.Analyzers)
	}
	if so.SecurityOnly && r.sel == nil {
		r.sel = map[string]bool{AnalyzerSecurity: true} // AR0xx is a subset of the security analyzer
	}
	r.collect()
	for i := range r.items {
		if r.items[i].owned {
			r.checkItem(&r.items[i])
		}
	}
	r.unit(unitOf("duplicates", AnalyzerDuplicates), r.checkDuplicates)
	r.unit(unitOf("mcp-command", AnalyzerMCP), r.checkMCP)
	r.unit(depUnitOf("settings-hooks", AnalyzerHooks), func() { r.checkHooks(baseAbs) })
	r.unit(unitOf("collapsed", AnalyzerDuplicates), r.checkCollapsed)
	r.unit(unitOf("unpinned", AnalyzerSecurity), r.checkUnpinned)
	r.unit(unitOf("delivery", AnalyzerDelivery, AnalyzerSecurity, AnalyzerLock), r.checkDelivery)
	r.unit(unitOf("imported", AnalyzerSecurity), r.scanImported)
	r.unit(unitOf("plugin-drift", AnalyzerPlugin), r.checkPluginDrift)
	r.unit(unitOf("eval-runner", AnalyzerEvals), r.checkEvalRunner)
	r.unit(unitOf("roles", AnalyzerRoles), r.checkRoles)
	r.unit(unitOf("lock-drift", AnalyzerLock), r.checkLockDrift)
	r.unit(unitOf("okf", AnalyzerOKF), r.checkOKF)
	r.unit(unitOf("telemetry", AnalyzerConfig, AnalyzerSecurity), r.checkTelemetry)
	r.unit(unitOf("external-config", AnalyzerSecurity), r.checkExternalConfig)
	r.unit(unitOf("traps", AnalyzerTraps), r.checkTraps)
	if so.External {
		r.unit(unitOf("external", AnalyzerSecurity), r.runExternal)
	}
	r.runRunChecks()
	if so.SecurityOnly {
		r.findings = securityOnly(r.findings)
	}
	r.unit(unitOf("settings-config", AnalyzerHooks, AnalyzerSecurity), r.checkSettingsConfig)
	r.unit(unitOf("llm-config", AnalyzerConfig, AnalyzerSecurity), r.checkLLMConfig)
	r.reportSuppressionAttempts()
	r.keepSelected()

	sort.SliceStable(r.findings, func(i, j int) bool {
		a, b := r.findings[i], r.findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	assignIdentity(r.findings, tree, r.cwd)
	rep := &Report{Root: r.display(baseAbs), Findings: r.findings, Protected: r.protected, ConfigFile: r.display(r.configFilePath()), Deps: r.exportDeps(), Analyzers: SelectedAnalyzers(keys(r.sel)),
		Units: map[string]int{}, unitRuns: r.units}
	for name, u := range r.units {
		rep.Units[name] = u.count
	}
	if p, ok := LookupProfile(r.lc.Profile); ok && p.Name != ProfileDefault {
		rep.Profile = p.Name
	}
	return rep, nil
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// keepSelected drops the findings of analyzers outside the selection: a unit
// may report rules of an analyzer that was not asked for.
func (r *runner) keepSelected() {
	if r.sel == nil {
		return
	}
	kept := r.findings[:0:0]
	for i := range r.findings {
		if r.sel[AnalyzerFor(r.findings[i].Code).Name] {
			kept = append(kept, r.findings[i])
		}
	}
	r.findings = kept
}

func (r *runner) resolveSettings() {
	r.sev = map[string]Severity{}
	for _, rule := range registry {
		r.sev[rule.Code] = rule.Default
	}
	r.applyProfile()
	if r.lc.Description != nil && r.lc.Description.RequireUseWhen {
		r.sev[CodeDescriptionStyle] = SeverityWarning
	}
	if r.lc.Evals != nil && r.lc.Evals.Require {
		r.sev[CodeEvalsMissing] = SeverityWarning
	}
	r.evalSettings()
	if r.cfg.LockEnforced() {
		// An unpinned remote or MCP package is a hole in an enforced lock, not a hint.
		r.sev[CodeUnpinnedRemote] = SeverityError
		r.sev[CodeMCPUnpinned] = SeverityError
	}
	for key, val := range r.lc.Severity {
		rule, ok := lookupRule(key)
		s, sok := ParseSeverity(val)
		if ok && sok {
			r.sev[rule.Code] = s
		}
	}
	r.ignore = map[string]bool{}
	for _, key := range r.lc.Ignore {
		if rule, ok := lookupRule(key); ok {
			r.ignore[rule.Code] = true
		}
	}
	r.applyPolicy()
	r.ignorePaths = compileGlobs(r.lc.IgnorePaths)
	r.exampleGlobs = compileGlobs(r.lc.ExamplePaths)
	r.allow = compileGlobs(r.lc.AllowPaths)
}

// compileGlobs compiles the valid patterns of a config list.
func compileGlobs(patterns []string) []globMatcher {
	var out []globMatcher
	for _, g := range patterns {
		if m, ok := newGlob(g); ok {
			out = append(out, m)
		}
	}
	return out
}

// ValidateSettings reports lint settings that name no known rule or severity,
// so a typo in config.toml does not silently disable a check.
func ValidateSettings(lc *config.LintConfig) []string {
	if lc == nil {
		return nil
	}
	var problems []string
	for _, a := range ValidateAnalyzerNames(lc.Analyzers) {
		problems = append(problems, fmt.Sprintf("lint.analyzers: unknown analyzer %q (use %s)", a, strings.Join(AnalyzerNames(), ", ")))
	}
	for _, key := range lc.Ignore {
		if _, ok := lookupRule(key); !ok {
			problems = append(problems, fmt.Sprintf("lint.ignore: unknown rule %q", key))
		}
	}
	for key, val := range lc.Severity {
		if _, ok := lookupRule(key); !ok {
			problems = append(problems, fmt.Sprintf("lint.severity: unknown rule %q", key))
		}
		if _, ok := ParseSeverity(val); !ok {
			problems = append(problems, fmt.Sprintf("lint.severity.%s: unknown severity %q", key, val))
		}
	}
	for kind := range lc.Budgets {
		if _, ok := defaultBudgets[kind]; !ok {
			problems = append(problems, fmt.Sprintf("lint.budgets: unknown content kind %q", kind))
		}
	}
	for kind := range lc.RequireMetadata {
		if _, ok := defaultBudgets[kind]; !ok {
			problems = append(problems, fmt.Sprintf("lint.require_metadata: unknown content kind %q", kind))
		}
	}
	problems = append(problems, validateBudgetAndRisk(lc)...)
	problems = append(problems, validateNewSettings(lc)...)
	problems = append(problems, validateEvalSettings(lc)...)
	problems = append(problems, validateTraps(lc.Traps)...)
	sort.Strings(problems)
	return problems
}

func (r *runner) display(abs string) string {
	if rel, err := filepath.Rel(r.cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

// add records a finding unless it is disabled, ignored in config, or ignored
// by an inline `ai-rulez-lint-ignore` comment on the line or the one above.
func (r *runner) add(code, abs string, line int, format string, args ...any) {
	r.addWithSeverity(code, "", abs, line, format, args...)
}

// addWithSeverity is add with a caller-chosen severity in place of the rule's
// configured one; "" keeps it. A rule switched off stays off.
func (r *runner) addWithSeverity(code string, override Severity, abs string, line int, format string, args ...any) {
	r.audit(code)
	sev := r.sev[code]
	if override != "" && sev != SeverityOff {
		sev = override
	}
	if sev == SeverityOff || r.ignore[code] {
		return
	}
	if r.suppressed(code, abs, line) {
		return
	}
	if exampleAware[code] && r.inExample(abs, line) {
		return
	}
	if r.forceSev != "" {
		sev = r.forceSev
	}
	rule, _ := lookupRule(code) //nolint:errcheck // every emitted code is registered
	r.findings = append(r.findings, Finding{
		Code: code, Name: rule.Name, Severity: sev,
		File: r.display(abs), Line: max(line, 1),
		Message: fmt.Sprintf(format, args...),
		Root:    r.display(r.rootAbs()),
	})
}

func (r *runner) rootAbs() string {
	abs, _ := filepath.Abs(r.cfg.BaseDir) //nolint:errcheck // display only
	return abs
}

// configRel returns abs as a slash path relative to the configuration
// directory, or "" when it lies elsewhere.
func (r *runner) configRel(abs string) string {
	cd, err := filepath.Abs(r.cfg.ConfigDir)
	if err != nil {
		return ""
	}
	rel, rerr := filepath.Rel(gitutil.Resolve(cd), gitutil.Resolve(abs))
	if rerr != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func (r *runner) pathIgnored(abs string) bool {
	if len(r.ignorePaths) == 0 {
		return false
	}
	cands := []string{r.tree.Rel(abs)}
	if rel := r.configRel(abs); rel != "" {
		cands = append(cands, rel)
	}
	for _, c := range cands {
		for _, g := range r.ignorePaths {
			if c != "" && g.match(c) {
				return true
			}
		}
	}
	return false
}

func (r *runner) inlineIgnored(abs string, line int, code string) bool {
	if r.forceSev != "" || r.noInlineIgnore {
		return false // imported text cannot silence its own findings
	}
	d, ok := r.docs[abs]
	if !ok {
		return false
	}
	for _, idx := range []int{line - 1, line - 2} {
		if idx < 0 || idx >= len(d.lines) {
			continue
		}
		codes, found := ignoreDirective(d.lines[idx])
		if !found {
			continue
		}
		if len(codes) == 0 {
			return true
		}
		for _, c := range codes {
			if rule, known := lookupRule(c); known && rule.Code == code {
				return true
			}
		}
	}
	return false
}

func itemID(kind string, cf config.ContentFile) string {
	if kind == kindSkill {
		return config.SkillID(cf)
	}
	base := cf.Name
	if strings.EqualFold(base, "COMMAND") || strings.EqualFold(base, "SKILL") {
		base = filepath.Base(filepath.Dir(cf.Path))
	}
	if kind == kindCommand {
		base = strings.ToLower(strings.NewReplacer(" ", "-", "_", "-").Replace(base))
	}
	return base
}

func (r *runner) collect() {
	r.skills, r.commands, r.agents, r.rules = map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	r.contexts = map[string]bool{}
	for _, n := range r.lc.KnownNames {
		n = strings.ToLower(n)
		r.skills[n], r.commands[n], r.agents[n], r.rules[n], r.contexts[n] = true, true, true, true, true
	}
	for _, n := range builtinSlash {
		r.commands[n] = true
	}
	for _, n := range builtinAgents {
		r.agents[n] = true
	}
	r.addInheritedNames()
	c := r.cfg.Content
	if c == nil {
		return
	}
	configDir, _ := filepath.Abs(r.cfg.ConfigDir) //nolint:errcheck // falls back to empty
	r.addTree(configDir, "", c.Rules, c.Context, c.Skills, c.Agents, c.Commands, c.Checks)
	names := make([]string, 0, len(c.Domains))
	for n := range c.Domains {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if d := c.Domains[n]; d != nil {
			r.addTree(configDir, n, d.Rules, d.Context, d.Skills, d.Agents, d.Commands, d.Checks)
		}
	}
}

func (r *runner) addTree(configDir, domain string, rules, context, skills, agents, commands, checks []config.ContentFile) {
	r.addItems(configDir, kindRule, domain, rules)
	r.addItems(configDir, kindContext, domain, context)
	r.addItems(configDir, kindSkill, domain, skills)
	r.addItems(configDir, kindAgent, domain, agents)
	r.addItems(configDir, kindCommand, domain, commands)
	r.addItems(configDir, kindCheck, domain, checks)
}

// contentNames lists the lower-case names an item answers to.
func contentNames(kind string, cf config.ContentFile) []string {
	names := []string{strings.ToLower(itemID(kind, cf))}
	if cf.Metadata == nil {
		return names
	}
	if n := strings.TrimSpace(cf.Metadata.Extra["name"]); n != "" {
		names = append(names, strings.ToLower(n))
	}
	for _, a := range cf.Metadata.Aliases {
		names = append(names, strings.ToLower(a))
	}
	return names
}

func (r *runner) addItems(configDir, kind, domain string, files []config.ContentFile) {
	set := map[string]map[string]bool{kindSkill: r.skills, kindCommand: r.commands, kindAgent: r.agents, kindRule: r.rules, kindContext: r.contexts}[kind]
	for _, cf := range files {
		abs, _ := filepath.Abs(cf.Path) //nolint:errcheck // keeps the raw path
		rel, err := filepath.Rel(configDir, abs)
		owned := err == nil && !strings.HasPrefix(rel, "..") && !strings.Contains(cf.Path, "://")
		for _, n := range contentNames(kind, cf) {
			if set != nil {
				set[n] = true
			}
			if r.names == nil {
				r.names = map[string][]string{}
			}
			r.names[n] = append(r.names[n], abs)
		}
		it := item{kind: kind, abs: abs, domain: domain, cf: cf, owned: owned}
		if base := strings.ToUpper(filepath.Base(abs)); owned && (base == "SKILL.MD" || base == "COMMAND.MD") {
			it.itemDir = filepath.Dir(abs)
		}
		r.items = append(r.items, it)
		if it.itemDir == "" {
			continue
		}
		for _, res := range cf.Resources {
			if strings.HasSuffix(strings.ToLower(res.RelPath), ".md") {
				r.items = append(r.items, item{
					kind: kind, abs: filepath.Join(it.itemDir, filepath.FromSlash(res.RelPath)), domain: domain,
					cf: config.ContentFile{Name: res.RelPath, Content: string(res.Content)}, owned: true, isDoc: true, itemDir: it.itemDir,
				})
			}
		}
	}
}

var (
	skillNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	useWhenRe   = regexp.MustCompile(`(?i)\b(use|used|invoke|trigger)\b.*\b(when|before|after|for|whenever|if)\b|\bwhen\b|\bwhenever\b`)
)

func (r *runner) checkItem(it *item) {
	raw := it.cf.Content
	if !it.isDoc {
		data, err := os.ReadFile(it.abs)
		if err != nil {
			return
		}
		raw = string(data)
	}
	d := parseDoc(raw)
	r.docs[it.abs] = d
	r.unit(unitOf("security-scan", AnalyzerSecurity), func() { r.securityScan(it.abs, raw) })
	r.unit(unitOf("markdown-shape", AnalyzerDescriptions), func() { r.checkMarkdownShape(it, d, raw) })
	if !it.isDoc {
		fm := parseFrontmatterDoc(d)
		r.unit(unitOf("frontmatter-keys", AnalyzerReferences), func() { r.checkFrontmatterKeys(it, fm) })
		r.unit(unitOf("typed-metadata", AnalyzerMetadata), func() { r.checkTypedMetadata(it, fm) })
		r.unit(unitOf("superseded", AnalyzerMetadata), func() { r.checkSuperseded(it, fm) })
		r.unit(unitOf("tool-breadth", AnalyzerSecurity), func() { r.checkToolBreadth(it, fm) })
		r.unit(unitOf("resources", AnalyzerSecurity), func() { r.scanResources(it) })
		r.unit(unitOf("globs", AnalyzerReferences), func() { r.checkGlobs(it, d) })
		r.unit(unitOf("description", AnalyzerDescriptions), func() { r.checkDescription(it, d) })
		r.unit(unitOf("budget", AnalyzerBudgets), func() { r.checkBudget(it, raw) })
		r.unit(unitOf("required-metadata", AnalyzerMetadata), func() { r.checkRequiredMetadata(it, d, fm) })
		r.unit(unitOf("skill-name", AnalyzerDescriptions), func() { r.checkSkillName(it, d) })
		r.unit(depUnitOf("frontmatter-skills", AnalyzerReferences), func() { r.checkFrontmatterSkills(it, d) })
		r.unit(unitOf("scripts", AnalyzerHooks), func() { r.checkScripts(it) })
		r.unit(unitOf("evals-missing", AnalyzerPlugin), func() { r.checkEvals(it, d) })
		r.runItemChecks(it, d, fm)
	}
	r.unit(depUnitOf("body-references", AnalyzerReferences), func() { r.scanBody(it, d) })
}

func (r *runner) checkGlobs(it *item, d doc) {
	if it.kind != kindRule && it.kind != kindContext {
		return
	}
	if it.cf.Metadata == nil || len(r.tree.files) == 0 {
		return
	}
	for _, pattern := range it.cf.Metadata.PathScope() {
		if strings.HasPrefix(pattern, "!") {
			continue
		}
		g, ok := newGlob(pattern)
		if !ok {
			r.add(CodeGlobNoMatch, it.abs, d.lineOf(pattern, 1), "glob %q is not a valid pattern", pattern)
			continue
		}
		if !r.tree.matchAny(g, r.baseRel) {
			if always, set := it.cf.Metadata.ExtraBool(keyAlwaysApply); set && always {
				r.add(CodeGlobNoMatch, it.abs, d.lineOf(pattern, 1), "glob %q matches no tracked file; this %s is alwaysApply so it still applies, but the glob is dead", pattern, it.kind)
				continue
			}
			r.add(CodeGlobNoMatch, it.abs, d.lineOf(pattern, 1), "glob %q matches no tracked file, so this %s never applies", pattern, it.kind)
		}
	}
}

func (r *runner) description(it *item) string { return config.SkillDescription(it.cf.Metadata) }

func (r *runner) checkDescription(it *item, d doc) {
	if it.kind != kindSkill && it.kind != kindAgent && it.kind != kindCommand {
		return
	}
	desc := r.description(it)
	line := d.lineOf("description", 1)
	if desc == "" {
		r.add(CodeDescriptionMissing, it.abs, 1, "%s %q has no description in its frontmatter", it.kind, itemID(it.kind, it.cf))
		return
	}
	minLen, maxLen := defaultDescMin, defaultDescMax
	if dc := r.lc.Description; dc != nil {
		if dc.MinLength > 0 {
			minLen = dc.MinLength
		}
		if dc.MaxLength > 0 {
			maxLen = dc.MaxLength
		}
	}
	if n := utf8.RuneCountInString(desc); n < minLen {
		r.add(CodeDescriptionLength, it.abs, line, "description is %d characters, below the minimum of %d", n, minLen)
	} else if n > maxLen {
		r.add(CodeDescriptionLength, it.abs, line, "description is %d characters, above the maximum of %d", n, maxLen)
	}
	if !useWhenRe.MatchString(desc) {
		r.add(CodeDescriptionStyle, it.abs, line, "description does not say when to use this %s (e.g. \"Use when ...\")", it.kind)
	}
}

func (r *runner) budgetFor(kind string) config.LintBudget {
	b := defaultBudgets[kind]
	if o, ok := r.lc.Budgets[kind]; ok {
		if o.MaxLines != 0 {
			b.MaxLines = o.MaxLines
		}
		if o.MaxTokens != 0 {
			b.MaxTokens = o.MaxTokens
		}
	}
	return b
}

func (r *runner) checkBudget(it *item, raw string) {
	b := r.budgetFor(it.kind)
	lines := len(strings.Split(strings.TrimRight(raw, "\n"), "\n"))
	if b.MaxLines > 0 && lines > b.MaxLines {
		r.add(CodeSizeLines, it.abs, 1, "%s is %d lines, over the budget of %d; move detail into references/ or split it", it.kind, lines, b.MaxLines)
	}
	if b.MaxTokens > 0 {
		if n := r.counter.Count(raw); n > b.MaxTokens {
			r.add(CodeSizeTokens, it.abs, 1, "%s is about %d tokens, over the budget of %d", it.kind, n, b.MaxTokens)
		}
	}
}

func metaValue(m *config.Metadata, key string) string {
	if m == nil {
		return ""
	}
	switch strings.ToLower(key) {
	case "priority":
		return m.Priority
	case "usage":
		return m.Usage
	case "category":
		return m.Category
	case "shortcut":
		return m.Shortcut
	case keyEffort:
		return m.Effort
	case "activation":
		return m.Activation
	case "targets":
		return strings.Join(m.Targets, ",")
	case keyTools:
		return strings.Join(m.Tools, ",")
	case keySkills:
		return strings.Join(m.Skills, ",")
	case "keywords":
		return strings.Join(m.Keywords, ",")
	case keyPaths:
		return strings.Join(m.Paths, ",")
	case keyGlobs:
		return strings.Join(m.Globs, ",")
	}
	return m.Extra[key]
}

func (r *runner) checkRequiredMetadata(it *item, _ doc, fm frontmatter) {
	for _, key := range r.lc.RequireMetadata[it.kind] {
		if strings.TrimSpace(metaValue(it.cf.Metadata, key)) != "" {
			continue
		}
		if k, ok := fm.lookup(key); ok && scalar(k.Value) != "" {
			continue // set inside the Agent Skills `metadata` map
		}
		r.add(CodeMetadataMissing, it.abs, 1, "%s %q is missing required frontmatter key %q", it.kind, itemID(it.kind, it.cf), key)
	}
}

func (r *runner) checkSkillName(it *item, d doc) {
	if it.kind != kindSkill || it.cf.Metadata == nil {
		return
	}
	name := strings.TrimSpace(it.cf.Metadata.Extra["name"])
	if name == "" {
		return
	}
	line := d.lineOf("name", 1)
	switch {
	case !skillNameRe.MatchString(name) || len(name) > maxSkillNameLen:
		r.addFix(r.renameSkillFix(it, d, line, normalizeSkillName(name), name), CodeSkillNameInvalid, it.abs, line,
			"skill name %q must be lowercase letters, digits and single hyphens, at most %d characters", name, maxSkillNameLen)
	case name != config.SkillID(it.cf):
		r.addFix(r.renameSkillFix(it, d, line, config.SkillID(it.cf), name), CodeSkillNameInvalid, it.abs, line,
			"skill name %q differs from its directory %q", name, config.SkillID(it.cf))
	}
}

func (r *runner) checkFrontmatterSkills(it *item, d doc) {
	if it.cf.Metadata == nil {
		return
	}
	for _, s := range it.cf.Metadata.Skills {
		key := strings.ToLower(strings.TrimSpace(s))
		r.depName(it.abs, key)
		if key == "" || strings.Contains(key, ":") || r.skills[key] || r.commands[key] {
			continue
		}
		r.add(CodeFrontmatterSkill, it.abs, d.lineOf(s, 1), "frontmatter skills: lists unknown skill %q", s)
	}
}

func (r *runner) checkScripts(it *item) {
	if it.itemDir == "" {
		return
	}
	for _, res := range it.cf.Resources {
		if res.Kind != config.SkillKindScripts || !strings.HasPrefix(string(res.Content), "#!") {
			continue
		}
		abs := filepath.Join(it.itemDir, filepath.FromSlash(res.RelPath))
		exe, known := res.Mode&0o111 != 0, true
		if rel := r.tree.Rel(abs); rel != "" {
			if e, k := r.tree.Executable(rel); k {
				exe, known = e, k
			}
		}
		if known && !exe {
			r.addFix(chmodFix(abs), CodeScriptNotExecutable, abs, 1, "script has a shebang but is not executable (chmod +x, and commit the mode)")
		}
	}
}

type descEntry struct {
	it     *item
	norm   string
	tokens map[string]struct{}
}

func (r *runner) descriptionEntries() []descEntry {
	var entries []descEntry
	for i := range r.items {
		it := &r.items[i]
		if it.isDoc || (it.kind != kindSkill && it.kind != kindAgent && it.kind != kindCommand) {
			continue
		}
		if desc := r.description(it); desc != "" {
			entries = append(entries, descEntry{it: it, norm: strings.Join(strings.Fields(strings.ToLower(desc)), " "), tokens: descTokens(desc)})
		}
	}
	return entries
}

// checkDuplicates compares descriptions across skills, agents and commands and
// reports each owned item once, against the first earlier item it duplicates.
func (r *runner) checkDuplicates() {
	threshold := defaultNearDup
	if r.lc.Description != nil && r.lc.Description.NearDuplicateThreshold > 0 {
		threshold = r.lc.Description.NearDuplicateThreshold
	}
	entries := r.descriptionEntries()
	for j := 1; j < len(entries); j++ {
		b := entries[j]
		if !b.it.owned {
			continue
		}
		for i := 0; i < j; i++ {
			if code, dup := compareDescriptions(entries[i], b, threshold); dup {
				a := entries[i]
				line := r.docs[b.it.abs].lineOf("description", 1)
				r.add(code, b.it.abs, line, "description is %s %s %q (%s)", map[string]string{CodeDescriptionDup: "identical to", CodeDescriptionNearDup: "near-identical to"}[code], a.it.kind, itemID(a.it.kind, a.it.cf), r.display(a.it.abs))
				break
			}
		}
	}
}

func compareDescriptions(a, b descEntry, threshold float64) (string, bool) {
	if a.it.abs == b.it.abs {
		return "", false
	}
	if a.norm == b.norm {
		return CodeDescriptionDup, true
	}
	if len(a.tokens) >= minNearDupTokens && len(b.tokens) >= minNearDupTokens && jaccard(a.tokens, b.tokens) >= threshold {
		return CodeDescriptionNearDup, true
	}
	return "", false
}

var stopwords = map[string]bool{"the": true, "and": true, "for": true, "with": true, "use": true, "when": true, "this": true, "that": true, "from": true, "are": true, "you": true, "your": true, "any": true, "into": true}

var wordRe = regexp.MustCompile(`[a-z0-9]{3,}`)

func descTokens(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		if !stopwords[w] {
			out[w] = struct{}{}
		}
	}
	return out
}

func jaccard(a, b map[string]struct{}) float64 {
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func (r *runner) configFilePath() string {
	if r.cfg.ConfigFile == "" {
		return ""
	}
	p, _ := filepath.Abs(filepath.Join(r.cfg.ConfigDir, r.cfg.ConfigFile)) //nolint:errcheck // display only
	return p
}

func (r *runner) checkMCP() {
	r.checkFrontmatterMCPCommands()
	path := r.configFilePath()
	if path == "" {
		return
	}
	var text []string
	if data, err := os.ReadFile(path); err == nil {
		text = strings.Split(string(data), "\n")
		r.docs[path] = doc{lines: text}
	}
	for i := range r.effectiveMCPServers() {
		s := &r.mcpEffective[i]
		n := s.Name
		if !s.IsEnabled() || s.GetTransport() != config.TransportStdio || s.Command == "" || strings.Contains(s.Command, "$") {
			continue
		}
		if r.commandResolves(s.Command) {
			continue
		}
		line := 1
		for i, l := range text {
			if strings.Contains(l, s.Command) {
				line = i + 1
				break
			}
		}
		r.add(CodeMCPCommandNotFound, path, line, "MCP server %q runs %q, which is not on PATH", n, s.Command)
	}
}

// checkFrontmatterMCPCommands reports an inline stdio server of an agent or
// skill whose command is not on PATH.
func (r *runner) checkFrontmatterMCPCommands() {
	for _, s := range r.frontmatterMCPServers() {
		if s.disabled || effectiveTransport(s) != "stdio" || s.command == "" || strings.Contains(s.command, "$") || r.commandResolves(s.command) {
			continue
		}
		r.add(CodeMCPCommandNotFound, s.file, s.line, "MCP server %q runs %q, which is not on PATH", s.name, s.command)
	}
}

func (r *runner) commandResolves(cmd string) bool {
	if strings.ContainsRune(cmd, filepath.Separator) || strings.HasPrefix(cmd, ".") {
		abs := cmd
		if !filepath.IsAbs(cmd) {
			abs = filepath.Join(r.rootAbs(), cmd)
			if _, err := os.Stat(abs); err != nil && r.tree.Explicit {
				abs = filepath.Join(r.tree.Top, cmd)
			}
		}
		info, err := os.Stat(abs)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(cmd)
	return err == nil
}

var projectVarRe = regexp.MustCompile(`(?:\$\{CLAUDE_PROJECT_DIR\}|\$CLAUDE_PROJECT_DIR)"?/([^\s"';&|)]+)`)

type hookFile struct {
	Hooks map[string][]struct {
		Hooks []struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func (r *runner) checkHooks(baseAbs string) {
	path := filepath.Join(baseAbs, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var hf hookFile
	if json.Unmarshal(data, &hf) != nil {
		return
	}
	r.docs[path] = doc{lines: strings.Split(string(data), "\n")}
	events := make([]string, 0, len(hf.Hooks))
	for e := range hf.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)
	for _, event := range events {
		for _, group := range hf.Hooks[event] {
			for _, h := range group.Hooks {
				if h.Type != "" && h.Type != "command" {
					continue
				}
				r.checkHookCommand(path, event, h.Command)
			}
		}
	}
}

func (r *runner) checkHookCommand(settings, event, command string) {
	trimmed := strings.TrimLeft(command, `"' `)
	for _, m := range projectVarRe.FindAllStringSubmatchIndex(command, -1) {
		rel := command[m[2]:m[3]]
		line := hookLine(r.docs[settings], rel)
		found := ""
		for _, cand := range []string{rel, joinRel(r.baseRel, rel)} {
			if r.tree.Exists(cand) {
				found = cand
				break
			}
		}
		if found == "" {
			r.add(CodeHookMissing, settings, line, "%s hook runs %q, which does not exist", event, rel)
			continue
		}
		r.dep(settings, filepath.Join(r.tree.Top, filepath.FromSlash(found)))
		direct := m[0] == len(command)-len(trimmed)
		if exe, known := r.tree.Executable(found); direct && known && !exe {
			r.addFix(chmodFix(filepath.Join(r.tree.Top, filepath.FromSlash(found))), CodeHookNotExecutable, settings, line, "%s hook runs %q, which is not executable", event, rel)
		}
	}
}

func joinRel(base, rel string) string {
	if base == "" {
		return rel
	}
	return base + "/" + rel
}

func hookLine(d doc, needle string) int {
	for i, l := range d.lines {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 1
}

// checkPluginDrift reports plugins whose content changed but whose version did
// not. A client that installed from a git-hosted marketplace keeps its cached
// copy until the version string changes (a plugin that declares no version is
// tracked by commit and is never reported).
func (r *runner) checkPluginDrift() {
	for _, d := range r.drift {
		changed := strings.Join(d.Changed, ", ")
		r.add(CodePluginVersionDrift, d.File, 1,
			"plugin %q changed since the baseline (%s) but its version is still %s; installs that cache the plugin keep the old copy until the version changes",
			d.Plugin, changed, d.Version)
	}
}

// checkEvals reports a skill that ships no eval cases. Cases live in the
// skill's own evals/ directory or in .ai-rulez/evals/<skill-name>/.
func (r *runner) checkEvals(it *item, d doc) {
	if r.sev[CodeEvalsMissing] == SeverityOff || it.kind != kindSkill || it.itemDir == "" {
		return
	}
	id := config.SkillID(it.cf)
	if r.evalsAllowed(id) {
		return
	}
	for _, dir := range []string{
		filepath.Join(it.itemDir, config.SkillKindEvals),
		filepath.Join(r.cfg.ConfigDir, config.EvalsDirName, id),
	} {
		if hasFiles(dir) {
			return
		}
	}
	r.add(CodeEvalsMissing, it.abs, d.lineOf("name", 1), "skill %q has no eval cases (add files under %s/ or %s/%s/)",
		id, config.SkillKindEvals, config.EvalsDirName, id)
}

func (r *runner) evalsAllowed(id string) bool {
	if r.lc.Evals == nil {
		return false
	}
	for _, pattern := range r.lc.Evals.Allow {
		if m, ok := newGlob(pattern); ok && m.match(id) {
			return true
		}
	}
	return false
}

// hasFiles reports whether dir holds at least one regular, non-hidden file.
func hasFiles(dir string) bool {
	found := false
	//nolint:errcheck // an unreadable or missing directory simply has no cases
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && !strings.HasPrefix(entry.Name(), ".") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// checkSettingsConfig checks the [[hooks]] and [permissions] blocks of
// config.toml against the tree. The generated settings files may be gitignored,
// so the declaration is checked at its source: a hook script that is missing or
// not executable fails the hook at runtime, and an allow rule for a whole tool
// defeats the point of listing rules.
func (r *runner) checkSettingsConfig() {
	if len(r.cfg.Hooks) == 0 && r.cfg.Permissions.IsEmpty() {
		return
	}
	path := r.configFilePath()
	if path == "" {
		return
	}
	var text []string
	if data, err := os.ReadFile(path); err == nil {
		text = strings.Split(string(data), "\n")
		r.docs[path] = doc{lines: text}
	}
	lineOf := func(needle string) int {
		for i, l := range text {
			if strings.Contains(l, needle) {
				return i + 1
			}
		}
		return 1
	}
	for _, group := range r.cfg.Hooks {
		for _, action := range group.Hooks {
			if action.Script == "" {
				continue
			}
			rel := joinRel(r.baseRel, filepath.ToSlash(filepath.Clean(action.Script)))
			if !r.tree.Exists(rel) {
				r.add(CodeHookSourceMissing, path, lineOf(action.Script), "%s hook runs %q, which does not exist", group.Event, action.Script)
				continue
			}
			if exe, known := r.tree.Executable(rel); known && !exe {
				r.addFix(chmodFix(filepath.Join(r.tree.Top, filepath.FromSlash(rel))), CodeHookSourceNotExec, path, lineOf(action.Script), "%s hook runs %q, which is not executable", group.Event, action.Script)
			}
		}
	}
	for _, rule := range r.cfg.Permissions.OverbroadAllowRules() {
		r.add(CodePermissionOverbroad, path, lineOf(rule), "permissions.allow %q permits every call of the tool; name the commands or paths it may run", rule)
	}
}

// validateBudgetAndRisk checks [lint.tolerate] (and its deprecated alias [lint.budget]) and [lint.risk].
func validateBudgetAndRisk(lc *config.LintConfig) []string {
	var problems []string
	if _, ok := LookupProfile(lc.Profile); !ok {
		problems = append(problems, fmt.Sprintf("lint.profile: unknown profile %q (use %s)", lc.Profile, strings.Join(ProfileNames(), ", ")))
	}
	for _, table := range []struct {
		name   string
		limits map[string]int
	}{{"tolerate", lc.Tolerate}, {"budget", lc.Budget}} {
		for key, limit := range table.limits {
			if _, ok := lookupRule(key); !ok {
				problems = append(problems, fmt.Sprintf("lint.%s: unknown rule %q", table.name, key))
			}
			if limit < 0 {
				problems = append(problems, fmt.Sprintf("lint.%s.%s: %d is negative", table.name, key, limit))
			}
		}
	}
	if lc.Risk != nil {
		for _, w := range []struct {
			name string
			v    *int
		}{{string(SeverityError), lc.Risk.Error}, {string(SeverityWarning), lc.Risk.Warning}, {string(SeverityInfo), lc.Risk.Info}} {
			if w.v != nil && *w.v < 0 {
				problems = append(problems, fmt.Sprintf("lint.risk.%s: %d is negative", w.name, *w.v))
			}
		}
	}
	return problems
}
