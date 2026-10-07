package lint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// Frontmatter key and metadata type names used more than once.
const (
	keyEffort = "effort"
	keySkills = "skills"
	keyGlobs  = "globs"
	keyPaths  = "paths"
	keyTools  = "tools"
	typeDate  = "date"
)

// now is the clock of the freshness checks; tests replace it.
var now = ambient.Clock(nil).Now

// Frontmatter keys that are known without configuration. The sources are the
// Agent Skills specification, the Claude Code skill and subagent references,
// and the keys ai-rulez itself reads from frontmatter.
var (
	specKeys = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}
	// claudeSkillKeys are the documented Claude Code skill and command keys.
	claudeSkillKeys = []string{
		"when_to_use", "argument-hint", "arguments", "disable-model-invocation", "user-invocable",
		"disallowed-tools", "model", keyEffort, "context", "agent", "background", "hooks", keyPaths, "shell",
	}
	// claudeAgentKeys are the documented Claude Code subagent keys (camelCase).
	claudeAgentKeys = []string{
		keyTools, "disallowedTools", "model", "permissionMode", "maxTurns", keySkills, "mcpServers", "hooks", "memory",
		"background", "omitClaudeMd", keyEffort, "isolation", "color", "initialPrompt", "experimental", "permission_mode",
	}
	// ruleKeys are the keys rules and context files understand, including the
	// Cursor and Windsurf spellings ai-rulez maps.
	ruleKeys = []string{keyGlobs, keyPaths, "glob", "alwaysApply", "trigger", "activation", "description", "name"}
	// checkKeys are the frontmatter keys of a code-review check (docs/checks.md).
	checkKeys = []string{"description", "severity", "tools", "targets"}
	// ownKeys are the ai-rulez keys valid on every kind.
	ownKeys = []string{
		"priority", "targets", "aliases", "keywords", "usage", "shortcut", "category", "placement", "short-description",
		keyTools, keySkills, keyGlobs, keyPaths, keyEffort, "deprecated", "superseded_by", "delivery", "triggers",
	}
)

func (r *runner) knownKeys(kind string) map[string]bool {
	known := map[string]bool{}
	add := func(keys ...[]string) {
		for _, list := range keys {
			for _, k := range list {
				known[k] = true
			}
		}
	}
	add(specKeys, ownKeys, r.lc.AllowedKeys)
	switch kind {
	case kindSkill, kindCommand:
		add(claudeSkillKeys)
	case kindAgent:
		add(claudeAgentKeys)
	case kindCheck:
		add(checkKeys)
	case kindContext:
		// summary is rendered into the inline context section.
		add(ruleKeys, []string{"summary"})
	default:
		add(ruleKeys)
	}
	return known
}

// KnownContextKeys lists the top-level frontmatter keys AR303 accepts on a
// context file without a project-specific lint.allowed_keys, so an importer can
// keep an unknown key out of the top level.
func KnownContextKeys() map[string]bool {
	r := &runner{}
	known := r.knownKeys(kindContext)
	out := make(map[string]bool, len(known))
	for k := range known {
		out[k] = true
	}
	return out
}

// checkFrontmatterKeys reports a top-level key no tool reads; a typo such as
// allowed_tools is otherwise ignored without a message.
func (r *runner) checkFrontmatterKeys(it *item, fm frontmatter) {
	known := r.knownKeys(it.kind)
	for _, k := range fm.keys {
		if known[k.Name] {
			continue
		}
		hint := ""
		if near := nearestKey(k.Name, known); near != "" {
			hint = fmt.Sprintf("; did you mean %q?", near)
		}
		r.addFix(r.renameKeyFix(it, fm, k, nearestKey(k.Name, known)), CodeFrontmatterKey, it.abs, k.Line,
			"unknown frontmatter key %q for a %s%s (list it in lint.allowed_keys if intended)", k.Name, it.kind, hint)
	}
}

// nearestKey suggests a known key that differs only by case or separators, the
// usual typo (allowed_tools for allowed-tools).
func nearestKey(key string, known map[string]bool) string {
	norm := func(s string) string { return strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(s)) }
	target := norm(key)
	var matches []string
	for k := range known {
		if norm(k) == target {
			matches = append(matches, k)
		}
	}
	sort.Strings(matches)
	if len(matches) > 0 {
		return matches[0]
	}
	best, bestDist := "", 3
	for k := range known {
		if d := editDistance(target, norm(k)); d < bestDist || (d == bestDist && best != "" && k < best) {
			best, bestDist = k, d
		}
	}
	if len(target) < 5 {
		return ""
	}
	return best
}

// checkTypedMetadata applies the lint.metadata rules.
func (r *runner) checkTypedMetadata(it *item, fm frontmatter) {
	keys := make([]string, 0, len(r.lc.Metadata))
	for k := range r.lc.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rule := r.lc.Metadata[key]
		if len(rule.Kinds) > 0 && !containsFold(rule.Kinds, it.kind) {
			continue
		}
		k, ok := fm.lookup(key)
		if !ok || scalarOrList(k.Value) == "" {
			if rule.Required {
				r.add(CodeMetadataMissing, it.abs, 1, "%s %q is missing required metadata key %q", it.kind, itemID(it.kind, it.cf), key)
			}
			continue
		}
		r.checkMetadataValue(it, key, rule, k)
	}
}

func scalarOrList(v any) string {
	if list, ok := v.([]any); ok && len(list) > 0 {
		return "list"
	}
	return scalar(v)
}

func (r *runner) checkMetadataValue(it *item, key string, rule config.LintMetadataRule, k fmKey) {
	switch rule.Type {
	case typeDate:
		t, ok := parseDate(k.Value)
		if !ok {
			r.add(CodeMetadataInvalid, it.abs, k.Line, "metadata %q is %q, not a date (use YYYY-MM-DD)", key, scalar(k.Value))
			return
		}
		if rule.MaxAgeDays > 0 {
			age := int(r.clock().Sub(t).Hours() / 24)
			switch {
			case age < 0:
				r.add(CodeMetadataInvalid, it.abs, k.Line, "metadata %q is %s, a date in the future", key, t.Format("2006-01-02"))
			case age > rule.MaxAgeDays:
				r.add(CodeMetadataStale, it.abs, k.Line, "metadata %q is %s, %d days old; the limit is %d (re-verify the %s and update the date)", key, t.Format("2006-01-02"), age, rule.MaxAgeDays, it.kind)
			}
		}
	case "enum":
		if v := scalar(k.Value); !containsFold(rule.Values, v) {
			r.add(CodeMetadataInvalid, it.abs, k.Line, "metadata %q is %q, not one of %s", key, v, strings.Join(rule.Values, ", "))
		}
	default:
		if scalar(k.Value) == "" {
			r.add(CodeMetadataInvalid, it.abs, k.Line, "metadata %q must be a string value", key)
		}
	}
}

func containsFold(list []string, v string) bool {
	for _, e := range list {
		if strings.EqualFold(strings.TrimSpace(e), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

// checkSuperseded reports a deprecated item whose replacement does not exist.
func (r *runner) checkSuperseded(it *item, fm frontmatter) {
	k, ok := fm.lookup("superseded_by")
	if !ok {
		return
	}
	target := strings.ToLower(scalar(k.Value))
	if target == "" || strings.Contains(target, ":") {
		return
	}
	for _, set := range []map[string]bool{r.skills, r.commands, r.agents, r.rules} {
		if set[target] {
			return
		}
	}
	r.add(CodeSupersededMissing, it.abs, k.Line, "%s %q is superseded by %q, which does not exist", it.kind, itemID(it.kind, it.cf), scalar(k.Value))
}

// checkCollapsed reports names that more than one source defines, where
// generation keeps one copy and drops the rest without a word.
func (r *runner) checkCollapsed() {
	if r.cfg.Content == nil {
		return
	}
	configDir, _ := filepath.Abs(r.cfg.ConfigDir) //nolint:errcheck // falls back to empty
	allow := map[string]bool{}
	for _, a := range r.lc.AllowOverrides {
		allow[strings.ToLower(strings.TrimSpace(a))] = true
	}
	for _, dup := range presets.CollapsedDuplicates(r.cfg.Content) {
		name := strings.ToLower(dup.Name)
		paths := append([]string{dup.Winner}, dup.Losers...)
		if allow[name] {
			continue
		}
		allowed := false
		for _, p := range paths {
			scope := scopeOf(p, configDir)
			if scope != "" && allow[scope+"/"+name] {
				allowed = true
			}
		}
		if allowed {
			continue
		}
		anchor := dup.Winner
		if abs, err := filepath.Abs(anchor); err != nil || !filepath.IsAbs(dup.Winner) || !underDir(abs, configDir) {
			anchor = dup.Losers[0]
		}
		abs, _ := filepath.Abs(anchor) //nolint:errcheck // display only
		r.add(CodeDuplicateCollapsed, abs, 1, "%s %q is defined in %s (kept) and %s (dropped); rename one, or list %q in lint.allow_overrides if the shadowing is intended",
			dup.Kind, dup.Name, r.labelPath(dup.Winner), r.labelPaths(dup.Losers), dup.Name)
	}
}

func (r *runner) labelPath(p string) string {
	if filepath.IsAbs(p) {
		return r.display(p)
	}
	return p // an embedded builtin, already written as builtin://...
}

func (r *runner) labelPaths(paths []string) string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = r.labelPath(p)
	}
	return strings.Join(out, ", ")
}

func underDir(abs, dir string) bool {
	rel, err := filepath.Rel(dir, abs)
	return err == nil && !strings.HasPrefix(rel, "..")
}

// scopeOf names where a copy lives: its domain, "builtin" for an embedded file,
// "import" for content outside this config directory, "" for root content.
func scopeOf(p, configDir string) string {
	if !filepath.IsAbs(p) {
		return "builtin"
	}
	rel, err := filepath.Rel(configDir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "import"
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 2 && parts[0] == "domains" {
		return parts[1]
	}
	return ""
}

// checkUnpinned reports remote sources that follow a moving ref without a pin.
func (r *runner) checkUnpinned() {
	for _, w := range includes.Unpinned(r.cfg) {
		path := r.configFilePath()
		if path == "" {
			path = filepath.Join(r.rootAbs(), ".ai-rulez", "config.toml")
		}
		line := 1
		if data, err := os.ReadFile(path); err == nil {
			for i, l := range strings.Split(string(data), "\n") {
				if strings.Contains(l, `"`+w.Name+`"`) {
					line = i + 1
					break
				}
			}
			if _, ok := r.docs[path]; !ok {
				r.docs[path] = doc{lines: strings.Split(string(data), "\n")}
			}
		}
		ref := w.Ref
		if ref == "" {
			ref = "the default branch (HEAD)"
		}
		r.add(CodeUnpinnedRemote, path, line, "%s %q follows %s and is not pinned by %s; run `ai-rulez lock` (or pin ref to a full commit SHA)", w.Kind, w.Name, ref, lockfile.FileName)
	}
}

// scanResources scans the text files a skill or command ships (scripts, data).
// Markdown resources are items of their own and are scanned there.
func (r *runner) scanResources(it *item) {
	if it.itemDir == "" {
		return
	}
	for _, res := range it.cf.Resources {
		if strings.HasSuffix(strings.ToLower(res.RelPath), ".md") || len(res.Content) > 512*1024 ||
			!utf8.Valid(res.Content) || bytes.IndexByte(res.Content, 0) >= 0 {
			continue
		}
		r.securityScan(filepath.Join(it.itemDir, filepath.FromSlash(res.RelPath)), string(res.Content))
	}
}

// importLevel maps lint.security.scan_imports to how imported content is
// scanned. Unset scans it and keeps each finding's own severity, so an
// error-level finding blocks `generate`. "error" and "warn" force every finding
// to that severity. "off" is the opt-out.
func (r *runner) importLevel() (force Severity, on bool) {
	switch strings.ToLower(strings.TrimSpace(r.security().ScanImports)) {
	case freshOff:
		return "", false
	case "error":
		return SeverityError, true
	case "warn", "warning":
		return SeverityWarning, true
	}
	return "", true
}

// scanImported runs the security rules over content that came from includes and
// installed skills. Findings take the scan_imports level, and an inline ignore
// comment inside imported text is not honored: the author of the import must not
// be able to silence the check on their own content.
func (r *runner) scanImported() {
	force, on := r.importLevel()
	if !on {
		return
	}
	r.forceSev = force
	defer func() { r.forceSev = "" }()
	for i := range r.items {
		it := &r.items[i]
		if it.owned || !filepath.IsAbs(it.abs) {
			continue
		}
		data, err := os.ReadFile(it.abs)
		if err != nil {
			continue
		}
		raw := string(data)
		r.docs[it.abs] = parseDoc(raw)
		r.securityScan(it.abs, raw)
		if !it.isDoc {
			r.checkToolBreadth(it, parseFrontmatterDoc(r.docs[it.abs]))
			imp := *it
			imp.itemDir = filepath.Dir(it.abs)
			r.scanResources(&imp)
		}
	}
}

// ScanImports runs the security rules over imported content only. It is what
// `generate` calls before writing when lint.security.scan_imports is set, so
// nothing from a remote is written until it has been checked. The findings carry
// the scan_imports severity.
func ScanImports(cfg *config.Config, opts ...Option) ([]Finding, error) {
	top, _ := filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the given dir
	r := &runner{cfg: cfg, tree: &Tree{Top: top}, docs: map[string]doc{}}
	for _, opt := range opts {
		opt(r)
	}
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	if _, on := r.importLevel(); !on {
		return nil, nil
	}
	r.resolveSettings()
	r.collect()
	r.scanImported()
	sort.SliceStable(r.findings, func(i, j int) bool {
		if r.findings[i].File != r.findings[j].File {
			return r.findings[i].File < r.findings[j].File
		}
		return r.findings[i].Line < r.findings[j].Line
	})
	return r.findings, nil
}

func securityOnly(in []Finding) []Finding {
	out := in[:0:0]
	for _, f := range in {
		if strings.HasPrefix(f.Code, "AR0") || AnalyzerFor(f.Code).Name == AnalyzerSecurity {
			out = append(out, f)
		}
	}
	return out
}

// validateNewSettings checks the security, metadata and external settings.
func validateNewSettings(lc *config.LintConfig) []string {
	var problems []string
	problems = append(problems, validateMetadataRules(lc.Metadata)...)
	problems = append(problems, validateSecurity(lc.Security)...)
	problems = append(problems, validateCapabilityAndLoadBudgets(lc)...)
	problems = append(problems, validateExternal(lc.External)...)
	problems = append(problems, validateScannerPolicy(lc)...)
	return problems
}

func validateMetadataRules(rules map[string]config.LintMetadataRule) []string {
	var problems []string
	for key, rule := range rules {
		switch rule.Type {
		case "", "string", typeDate:
		case "enum":
			if len(rule.Values) == 0 {
				problems = append(problems, fmt.Sprintf("lint.metadata.%s: type enum needs values", key))
			}
		default:
			problems = append(problems, fmt.Sprintf("lint.metadata.%s: unknown type %q (use string, date or enum)", key, rule.Type))
		}
		if rule.MaxAgeDays < 0 || (rule.MaxAgeDays > 0 && rule.Type != typeDate) {
			problems = append(problems, fmt.Sprintf("lint.metadata.%s: max_age_days needs type date and a positive value", key))
		}
		for _, kind := range rule.Kinds {
			if _, ok := defaultBudgets[kind]; !ok {
				problems = append(problems, fmt.Sprintf("lint.metadata.%s: unknown content kind %q", key, kind))
			}
		}
	}
	return problems
}

func validateSecurity(sec *config.LintSecurity) []string {
	if sec == nil {
		return nil
	}
	var problems []string
	switch strings.ToLower(sec.ScanImports) {
	case "", "off", "warn", "warning", "error":
	default:
		problems = append(problems, fmt.Sprintf("lint.security.scan_imports: unknown level %q (use off, warn or error, or leave it unset)", sec.ScanImports))
	}
	for _, p := range sec.SecretPatterns {
		if strings.TrimSpace(p.Name) == "" {
			problems = append(problems, "lint.security.secret_patterns: a pattern needs a name")
		}
		if _, err := regexp.Compile(p.Regex); err != nil {
			problems = append(problems, fmt.Sprintf("lint.security.secret_patterns.%s: invalid regex: %v", p.Name, err))
		}
	}
	for _, tag := range sec.DirectiveTags {
		if !ValidDirectiveTag(tag) {
			problems = append(problems, fmt.Sprintf("lint.security.directive_tags: %q is not an element name (letters, digits, '-', '_', ':' and '.', starting with a letter)", tag))
		}
	}
	for _, org := range sec.TrustedOrgs {
		if strings.TrimSpace(org) == "" {
			problems = append(problems, "lint.security.trusted_orgs: an entry is empty")
		}
	}
	for _, h := range sec.AllowedHosts {
		if strings.Contains(h, "/") {
			problems = append(problems, fmt.Sprintf("lint.security.allowed_hosts: %q must be a bare host such as example.com or *.example.com", h))
		}
	}
	return problems
}

var directiveTagRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]*$`)

// ValidDirectiveTag reports whether tag can be a [lint.security] directive_tags entry.
func ValidDirectiveTag(tag string) bool { return directiveTagRe.MatchString(strings.TrimSpace(tag)) }

func validateCapabilityAndLoadBudgets(lc *config.LintConfig) []string {
	var problems []string
	if c := lc.Capability; c != nil && c.MaxNetworkCommands != nil && *c.MaxNetworkCommands < 0 {
		problems = append(problems, fmt.Sprintf("lint.capability.max_network_commands: %d must not be negative", *c.MaxNetworkCommands))
	}
	known := LoadBudgetIDs()
	for id, limit := range lc.LoadBudgets {
		switch {
		case !slices.Contains(known, id):
			problems = append(problems, fmt.Sprintf("lint.load_budgets: unknown limit %q (known: %s)", id, strings.Join(known, ", ")))
		case limit < 1:
			problems = append(problems, fmt.Sprintf("lint.load_budgets.%s: %d must be positive", id, limit))
		}
	}
	return problems
}

func validateExternal(list []config.LintExternal) []string {
	var problems []string
	for i, ex := range list {
		if strings.TrimSpace(ex.Name) == "" || (len(ex.Command) == 0 && ex.Profile == "") {
			problems = append(problems, fmt.Sprintf("lint.external[%d]: name and command (or profile) are required", i))
		}
		if !validFormat(ex.Format) {
			problems = append(problems, fmt.Sprintf("lint.external[%d]: unknown format %q (use sarif, json or adapter:<%s>)", i, ex.Format, strings.Join(knownAdapters, "|")))
		}
	}
	return problems
}

// renameKeyFix builds the safe fix for AR303: rename the key to the known key
// it differs from only by case or separators. It returns nil when there is no
// such key, the target key is already set, or the line is not a plain key line.
func (r *runner) renameKeyFix(it *item, fm frontmatter, k fmKey, near string) *Fix {
	if near == "" {
		return nil
	}
	for _, other := range fm.keys {
		if other.Name == near {
			return nil // renaming would create a duplicate key
		}
	}
	d, ok := r.docs[it.abs]
	if !ok || k.Line < 1 || k.Line > len(d.lines) {
		return nil
	}
	newLine, ok := renameKeyLine(d.lines[k.Line-1], k.Name, near)
	if !ok {
		return nil
	}
	return &Fix{
		Description: fmt.Sprintf("rename frontmatter key %q to %q", k.Name, near),
		Confidence:  FixSafe,
		Edits:       []Edit{{File: it.abs, Line: k.Line, Old: d.lines[k.Line-1], New: newLine}},
	}
}
