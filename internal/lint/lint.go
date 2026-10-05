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
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/tokens"
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
	for _, f := range findings {
		if f.Severity.AtLeast(threshold) {
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
	cfg         *config.Config
	lc          config.LintConfig
	tree        *Tree
	baseRel     string
	cwd         string
	sev         map[string]Severity
	ignore      map[string]bool
	ignorePaths []globMatcher
	allow       []globMatcher
	skills      map[string]bool
	commands    map[string]bool
	agents      map[string]bool
	rules       map[string]bool
	items       []item
	docs        map[string]doc
	counter     tokens.Counter
	findings    []Finding
	// forceSev replaces the severity of every finding while imported content is
	// scanned (lint.security.scan_imports).
	forceSev Severity
	// noInlineIgnore refuses `ai-rulez-lint-ignore` comments (served content).
	noInlineIgnore bool
	opts           Options
	drift          []PluginDrift
	delivery       []DeliveryFinding
}

// Options selects what a run does beyond the default strict checks.
type Options struct {
	// SecurityOnly keeps only the security family (AR0xx).
	SecurityOnly bool
	// External also runs the scanners configured in lint.external.
	External bool
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
	r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}, counter: counter, opts: so}
	for _, opt := range opts {
		opt(r)
	}
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	r.cwd, _ = os.Getwd()                   //nolint:errcheck // display paths fall back to absolute
	baseAbs, _ := filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the given dir
	r.baseRel = tree.Rel(baseAbs)
	if r.baseRel == "." {
		r.baseRel = ""
	}
	r.resolveSettings()
	r.collect()
	for i := range r.items {
		if r.items[i].owned {
			r.checkItem(&r.items[i])
		}
	}
	r.checkDuplicates()
	r.checkMCP()
	r.checkHooks(baseAbs)
	r.checkCollapsed()
	r.checkUnpinned()
	r.checkDelivery()
	r.scanImported()
	r.checkPluginDrift()
	r.checkEvalRunner()
	if so.External {
		r.runExternal()
	}
	if so.SecurityOnly {
		r.findings = securityOnly(r.findings)
	}
	r.checkSettingsConfig()

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
	return &Report{Root: r.display(baseAbs), Findings: r.findings}, nil
}

func (r *runner) resolveSettings() {
	r.sev = map[string]Severity{}
	for _, rule := range registry {
		r.sev[rule.Code] = rule.Default
	}
	if r.lc.Description != nil && r.lc.Description.RequireUseWhen {
		r.sev[CodeDescriptionStyle] = SeverityWarning
	}
	if r.lc.Evals != nil && r.lc.Evals.Require {
		r.sev[CodeEvalsMissing] = SeverityWarning
	}
	r.evalSettings()
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
	for _, g := range r.lc.IgnorePaths {
		if m, ok := newGlob(g); ok {
			r.ignorePaths = append(r.ignorePaths, m)
		}
	}
	for _, g := range r.lc.AllowPaths {
		if m, ok := newGlob(g); ok {
			r.allow = append(r.allow, m)
		}
	}
}

// ValidateSettings reports lint settings that name no known rule or severity,
// so a typo in config.toml does not silently disable a check.
func ValidateSettings(lc *config.LintConfig) []string {
	if lc == nil {
		return nil
	}
	var problems []string
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
	problems = append(problems, validateNewSettings(lc)...)
	problems = append(problems, validateEvalSettings(lc)...)
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
	sev := r.sev[code]
	if sev == SeverityOff || r.ignore[code] {
		return
	}
	if r.pathIgnored(abs) || r.inlineIgnored(abs, line, code) {
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

func (r *runner) pathIgnored(abs string) bool {
	if len(r.ignorePaths) == 0 {
		return false
	}
	cands := []string{r.tree.Rel(abs)}
	if cd, err := filepath.Abs(r.cfg.ConfigDir); err == nil {
		if rel, rerr := filepath.Rel(cd, abs); rerr == nil {
			cands = append(cands, filepath.ToSlash(rel))
		}
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
	for _, n := range r.lc.KnownNames {
		n = strings.ToLower(n)
		r.skills[n], r.commands[n], r.agents[n], r.rules[n] = true, true, true, true
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
	r.addTree(configDir, "", c.Rules, c.Context, c.Skills, c.Agents, c.Commands)
	names := make([]string, 0, len(c.Domains))
	for n := range c.Domains {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if d := c.Domains[n]; d != nil {
			r.addTree(configDir, n, d.Rules, d.Context, d.Skills, d.Agents, d.Commands)
		}
	}
}

func (r *runner) addTree(configDir, domain string, rules, context, skills, agents, commands []config.ContentFile) {
	r.addItems(configDir, kindRule, domain, rules)
	r.addItems(configDir, kindContext, domain, context)
	r.addItems(configDir, kindSkill, domain, skills)
	r.addItems(configDir, kindAgent, domain, agents)
	r.addItems(configDir, kindCommand, domain, commands)
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
	set := map[string]map[string]bool{kindSkill: r.skills, kindCommand: r.commands, kindAgent: r.agents, kindRule: r.rules}[kind]
	for _, cf := range files {
		abs, _ := filepath.Abs(cf.Path) //nolint:errcheck // keeps the raw path
		rel, err := filepath.Rel(configDir, abs)
		owned := err == nil && !strings.HasPrefix(rel, "..") && !strings.Contains(cf.Path, "://")
		for _, n := range contentNames(kind, cf) {
			if set != nil {
				set[n] = true
			}
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
	r.securityScan(it.abs, raw)
	if !it.isDoc {
		fm := parseFrontmatterDoc(d)
		r.checkFrontmatterKeys(it, fm)
		r.checkTypedMetadata(it, fm)
		r.checkSuperseded(it, fm)
		r.checkToolBreadth(it, fm)
		r.scanResources(it)
		r.checkGlobs(it, d)
		r.checkDescription(it, d)
		r.checkBudget(it, raw)
		r.checkRequiredMetadata(it, d, fm)
		r.checkSkillName(it, d)
		r.checkFrontmatterSkills(it, d)
		r.checkScripts(it)
		r.checkEvals(it, d)
	}
	r.scanBody(it, d)
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
		r.add(CodeSkillNameInvalid, it.abs, line, "skill name %q must be lowercase letters, digits and single hyphens, at most %d characters", name, maxSkillNameLen)
	case name != config.SkillID(it.cf):
		r.add(CodeSkillNameInvalid, it.abs, line, "skill name %q differs from its directory %q", name, config.SkillID(it.cf))
	}
}

func (r *runner) checkFrontmatterSkills(it *item, d doc) {
	if it.cf.Metadata == nil {
		return
	}
	for _, s := range it.cf.Metadata.Skills {
		key := strings.ToLower(strings.TrimSpace(s))
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
			r.add(CodeScriptNotExecutable, abs, 1, "script has a shebang but is not executable (chmod +x, and commit the mode)")
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
	path := r.configFilePath()
	if path == "" {
		return
	}
	var text []string
	if data, err := os.ReadFile(path); err == nil {
		text = strings.Split(string(data), "\n")
		r.docs[path] = doc{lines: text}
	}
	names := make([]string, 0, len(r.cfg.MCPServers))
	for n := range r.cfg.MCPServers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := r.cfg.MCPServers[n]
		if s == nil || !s.IsEnabled() || s.GetTransport() != config.TransportStdio || s.Command == "" || strings.Contains(s.Command, "$") {
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

func (r *runner) commandResolves(cmd string) bool {
	if strings.ContainsRune(cmd, filepath.Separator) || strings.HasPrefix(cmd, ".") {
		abs := cmd
		if !filepath.IsAbs(cmd) {
			abs = filepath.Join(r.rootAbs(), cmd)
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
		direct := m[0] == len(command)-len(trimmed)
		if exe, known := r.tree.Executable(found); direct && known && !exe {
			r.add(CodeHookNotExecutable, settings, line, "%s hook runs %q, which is not executable", event, rel)
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
				r.add(CodeHookSourceNotExec, path, lineOf(action.Script), "%s hook runs %q, which is not executable", group.Event, action.Script)
			}
		}
	}
	for _, rule := range r.cfg.Permissions.OverbroadAllowRules() {
		r.add(CodePermissionOverbroad, path, lineOf(rule), "permissions.allow %q permits every call of the tool; name the commands or paths it may run", rule)
	}
}
