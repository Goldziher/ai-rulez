package lint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

var (
	skillNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	useWhenRe   = regexp.MustCompile(`(?i)\b(use|used|invoke|trigger)\b.*\b(when|before|after|for|whenever|if)\b|\bwhen\b|\bwhenever\b|\bload\s+(?:this\s+\w+\s+)?(?:for|before|when|whenever|after|if)\b|\buse\s+(?:this\s+\w+\s+|\w+\s+)?(?:as|any\s?time|anytime)\b`)
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
	if desc == "" && it.cf.MalformedFrontmatter {
		return // AR306 reports the unreadable block; the description is not missing
	}
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
	// A command is invoked by name, so it has no trigger to state.
	if it.kind != kindCommand && !useWhenRe.MatchString(desc) {
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
		r.addMetric(CodeSizeLines, it.abs, 1, lines, "%s is %d lines, over the budget of %d; move detail into references/ or split it", it.kind, lines, b.MaxLines)
	}
	if b.MaxTokens > 0 {
		if n := r.counter.Count(raw); n > b.MaxTokens {
			r.addMetric(CodeSizeTokens, it.abs, 1, n, "%s is about %d tokens, over the budget of %d", it.kind, n, b.MaxTokens)
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
	var served map[string]bool
	for _, s := range it.cf.Metadata.Skills {
		key := strings.ToLower(strings.TrimSpace(s))
		r.depName(it.abs, key)
		if served == nil {
			served = r.servedSkillNames()
		}
		if served[key] {
			continue // a served skill is not on disk; checkServedReferences reports the preload (AR990)
		}
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
		if rel := r.rel(abs); rel != "" {
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
			code, dup := compareDescriptions(entries[i], b, threshold)
			if !dup {
				continue
			}
			a := entries[i]
			line := r.docs[b.it.abs].lineOf("description", 1)
			r.dep(b.it.abs, a.it.abs) // the finding sits on b but exists because of a
			r.add(code, b.it.abs, line, "description is %s %s %q (%s)", map[string]string{CodeDescriptionDup: "identical to", CodeDescriptionNearDup: "near-identical to"}[code], a.it.kind, itemID(a.it.kind, a.it.cf), r.display(a.it.abs))
			break
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
