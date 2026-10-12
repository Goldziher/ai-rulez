package lint

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	// "`name` skill", "`name` agent", "`name` rule": the kind word is singular,
	// because "`term` skills" is a compound noun ("disable-model-invocation
	// skills"), not a pointer to a skill named after the backticked term.
	nameAfterRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile("`([a-z][a-z0-9]*(?:-[a-z0-9]+)*)`\\s+(skill|subagent|agent|rule)\\b")
	})
	nameBeforeRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile("\\b(skill|subagent|agent)\\s+`([a-z][a-z0-9_-]*)`") })
	slashRe      = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|[\s(])/([a-z][a-z0-9]*(?:-[a-z0-9]+)+)(?:[\s,;:)]|\.(?:\s|$)|$)`)
	})
	tickSlashRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^/([a-z][a-z0-9]*(?:-[a-z0-9]+)+)(?:\s|$)`) })
	skillCallRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\bSkill\(\s*["']?([a-z][a-z0-9-]*)["']?\s*\)`) })
	subagentRe  = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\bsubagent_type["']?\s*[:=]\s*["']?([a-z][a-z0-9-]*)`)
	})
	// labeledSkillRe matches a labeled, non-backticked pointer such as
	// "Procedure/reference: skill x": a label separator, the word skill, then
	// the name. The separator keeps prose like "the skill system" out.
	labeledSkillRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`[:=]\s*skill\s+([a-z0-9]+(?:-[a-z0-9]+)*)`)
	})
	// indefiniteArticleRe matches a bare "a" or "an" right before a token:
	// "a `both` skill" describes a kind of skill (the backticked word is an
	// adjective), while "the `deploy` skill" names one (#352).
	indefiniteArticleRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?i)\ban?\s+$`) })
	// skillDirRe matches the file-path pointer forms "skills/x" and
	// ".claude/skills/x/SKILL.md", capturing the skill directory name. The
	// "skills/" component must start the path or sit under a harness dot
	// directory (".claude/skills/..."): a nested "skills/" segment of some other
	// tree ("src/mastra/public/skills/shared/<name>/SKILL.md") is not a pointer
	// to a skill of this project (#352).
	skillDirRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|[^\w/-])(?:\./)?(?:[.][a-z0-9_-]+/)?skills/([a-z0-9]+(?:-[a-z0-9]+)*)`)
	})
	skillRelRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(references|scripts|assets)/`) })
	// siblingSkillResourceRe matches a resource path that names a sibling skill
	// explicitly: "skills/<other>/references/x.md" or
	// ".claude/skills/<other>/references/x.md" (any "<prefix>/skills/").
	siblingSkillResourceRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|/)skills/([a-z0-9]+(?:-[a-z0-9]+)*)/((?:references|scripts|assets)/.+)$`)
	})
	extRe      = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^\.[A-Za-z0-9]{1,6}$`) })
	fragLineRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^L\d+`) })
	// mcpCallRe matches a skills MCP call that names a skill: "load_skill x"
	// inside a code span, "load_skill(x)", "load_skill(name: \"x\")" and the bare
	// command form "load_skill dev-cycle-procedure". A bare "load_skill word" is
	// not read: the argument may be prose ("load_skill returns"), and only a
	// hyphenated name or one quoted/code-spanned is unambiguously a skill.
	// find_skill is left out: its argument is a task description, not a name.
	mcpCallRe = sync.OnceValue(func() *regexp.Regexp {
		const name = `([a-z][a-z0-9]*(?:-[a-z0-9]+)*)`
		const hyphen = `([a-z][a-z0-9]*(?:-[a-z0-9]+)+)`
		const quote = `["'` + "`" + `]`
		return regexp.MustCompile(`\b(?:load_skill|get_skill|read_skill_file|list_skill_resources)(?:` +
			`\s*\(\s*name\s*[:=]\s*` + quote + `?` + name + // load_skill(name: "x")
			`|\s*\(\s*` + quote + `?` + name + // load_skill(x)
			`|\s+` + quote + name + // load_skill "x"
			`|\s+` + name + quote + // `load_skill x`
			`|\s+` + hyphen + // load_skill dev-cycle-procedure
			`)`)
	})
)

func looksPlaceholder(name string) bool {
	for _, p := range []string{"my-", "new-", "your-", "some-"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return name == "foo" || strings.HasPrefix(name, "foo-") || strings.Contains(name, "example")
}

func (r *runner) scanBody(it *item, d doc) {
	lines := d.body()
	for i, l := range lines {
		prev := ""
		if i > 0 && lines[i-1].No == l.No-1 {
			prev = lines[i-1].Text
		}
		for _, target := range linkTargets(l.Plain) {
			r.checkLink(it, l.No, target)
		}
		for _, m := range backtickMatches(l.Text) {
			tok := l.Text[m[2]:m[3]]
			if tickSlashRe().MatchString(strings.TrimSpace(tok)) && !slashInvocation(l.Text[:m[0]]) {
				continue // `/api-docs` in prose is a route, not a command
			}
			r.checkToken(it, l.No, tok, func() bool { return r.pathNotAClaim(l.Text, prev, m[2], m[3]) })
		}
		r.checkNames(it, l)
	}
}

func (r *runner) existsAbs(abs string) bool {
	if rel := r.rel(abs); rel != "" {
		return r.tree.Exists(rel)
	}
	_, err := os.Stat(abs)
	return err == nil
}

func (r *runner) checkLink(it *item, line int, target string) {
	if target == "" || schemeRe().MatchString(target) || strings.HasPrefix(target, "//") {
		return
	}
	pathPart, frag, _ := strings.Cut(target, "#")
	pathPart, _, _ = strings.Cut(pathPart, "?")
	if dec, err := url.PathUnescape(pathPart); err == nil {
		pathPart = dec
	}
	if dec, err := url.PathUnescape(frag); err == nil {
		frag = dec
	}
	if strings.ContainsAny(pathPart, "<>{}$*") {
		return
	}
	if pathPart == "" {
		r.checkAnchor(it, line, it.abs, frag, target)
		return
	}
	var cands []string
	if strings.HasPrefix(pathPart, "/") {
		cands = append(cands, filepath.Join(r.tree.Top, filepath.FromSlash(pathPart)))
	} else {
		cands = append(cands, filepath.Join(filepath.Dir(it.abs), filepath.FromSlash(pathPart)))
		if it.itemDir != "" {
			cands = append(cands, filepath.Join(it.itemDir, filepath.FromSlash(pathPart)))
		}
		cands = append(cands, filepath.Join(r.rootAbs(), filepath.FromSlash(pathPart)))
		// The content is emitted into each scope's root file, so a link written
		// relative to the emitted file resolves from the scope directory.
		for _, dir := range r.scopeDirs() {
			cands = append(cands, filepath.Join(dir, filepath.FromSlash(pathPart)))
		}
		if r.tree.Explicit {
			cands = append(cands, filepath.Join(r.tree.Top, filepath.FromSlash(pathPart)))
		}
	}
	for _, c := range cands {
		if r.existsAbs(c) {
			r.dep(it.abs, c)
			r.checkAnchor(it, line, c, frag, target)
			return
		}
	}
	for _, c := range cands {
		r.dep(it.abs, c) // a file that reappears, or is deleted, affects this finding
	}
	r.add(CodeLinkUnresolved, it.abs, line, "link target %q does not exist", target)
}

func (r *runner) checkAnchor(it *item, line int, file, frag, target string) {
	if frag == "" || fragLineRe().MatchString(frag) || !strings.HasSuffix(strings.ToLower(file), ".md") {
		return
	}
	var raw string
	if file == it.abs && it.isDoc {
		raw = it.cf.Content
	} else if data, ok := r.fileContent(file); ok {
		raw = data
	} else {
		return
	}
	if _, ok := headingSlugs(raw)[strings.ToLower(frag)]; !ok {
		r.add(CodeAnchorUnresolved, it.abs, line, "link %q: no heading produces the anchor #%s", target, frag)
	}
}

// checkToken handles one backticked token: a slash command, a skill-relative
// file, or a repo path.
//
// notAClaim is set when the surrounding text does not assert the path exists
// (negated, an example, another repository...); see pathNotAClaim.
// checkToken checks one backtick token. notAClaim says whether the sentence around
// it makes it prose rather than a claim about the repository; it costs a dozen
// regular expressions, so it is only asked for a token that looks like a path.
func (r *runner) checkToken(it *item, line int, tok string, notAClaim func() bool) { //nolint:gocyclo // linear token checks
	tok = strings.TrimSpace(tok)
	if m := tickSlashRe().FindStringSubmatch(tok); m != nil {
		r.requireName(it, line, m[1], kindCommand, r.commands, r.skills)
		return
	}
	tok = lineSuffixRe().ReplaceAllString(strings.TrimRight(tok, ".,;:"), "")
	if !pathLike(tok) {
		return
	}
	tok = strings.TrimPrefix(tok, "./")
	if notAClaim() {
		return
	}
	if m := skillRelRe().FindString(tok); m != "" {
		if r.checkSkillRelative(it, line, tok, m) {
			return
		}
	}
	if r.checkSiblingSkillResource(it, line, tok) {
		return
	}
	first, _, hasSlash := strings.Cut(tok, "/")
	if !hasSlash || first == "" || r.allowed(tok) {
		return
	}
	if !r.tree.IsTopLevel(first) && (r.baseRel == "" || !r.tree.Exists(r.baseRel+"/"+first)) {
		return
	}
	r.dep(it.abs, filepath.Join(r.tree.Top, filepath.FromSlash(tok)))
	if r.baseRel != "" {
		r.dep(it.abs, filepath.Join(r.tree.Top, filepath.FromSlash(r.baseRel), filepath.FromSlash(tok)))
	}
	if !r.existsRepo(tok) && !r.existsScope(tok) && !r.numberedPrefixExists(tok) {
		r.add(CodePathMissing, it.abs, line, "path %q does not exist in the repository", path.Clean(tok))
	}
}

// pathLike filters out tokens that are prose, placeholders, URLs, bazel labels,
// absolute or parent-relative paths, or dotted symbols such as helpers.run_async.
func pathLike(tok string) bool {
	if tok == "" || strings.Contains(tok, ":") || placeholder().MatchString(tok) {
		return false
	}
	if strings.HasPrefix(tok, "~") || strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "../") {
		return false
	}
	ext := path.Ext(tok)
	return ext == "" || extRe().MatchString(ext)
}

func (r *runner) allowed(tok string) bool {
	for _, g := range r.allow {
		if g.match(tok) {
			return true
		}
	}
	return false
}

// checkSkillRelative handles references/, scripts/ and assets/ tokens. It
// returns true when the token is fully handled (resolved, reported, or prose).
func (r *runner) checkSkillRelative(it *item, line int, tok, prefix string) bool {
	if tok == prefix || tok == strings.TrimSuffix(prefix, "/") {
		return true // the bare directory name, used as prose
	}
	if it.itemDir == "" {
		return false
	}
	r.dep(it.abs, filepath.Join(it.itemDir, filepath.FromSlash(tok)))
	if r.existsAbs(filepath.Join(it.itemDir, filepath.FromSlash(tok))) || r.existsRepo(tok) || r.allowed(tok) {
		return true
	}
	r.add(CodeSkillResourceMissing, it.abs, line, "%q was not found relative to this %s's directory (%s) or relative to the repo root (%s)",
		tok, it.kind, r.display(it.itemDir), r.display(r.tree.Top))
	return true
}

// checkSiblingSkillResource handles a resource path that names a sibling skill
// explicitly ("skills/<other>/references/x.md", ".claude/skills/<other>/...").
// A SKILL.md may point at another skill's file this way (#346): AR402 accepts it
// when the file is tracked at the written path or under the sibling's source
// directory, and reports it otherwise. It reports whether the token was such a
// path (a rule has no skill directory, so it never is).
func (r *runner) checkSiblingSkillResource(it *item, line int, tok string) bool {
	if it.itemDir == "" {
		return false
	}
	m := siblingSkillResourceRe().FindStringSubmatch(tok)
	if m == nil {
		return false
	}
	r.dep(it.abs, filepath.Join(r.tree.Top, filepath.FromSlash(tok)))
	if r.existsRepo(tok) || r.allowed(tok) || r.siblingSkillSource(it, m[1], m[2]) {
		return true
	}
	r.add(CodeSkillResourceMissing, it.abs, line, "%q names the %s skill but was not found relative to the repo root (%s) or under that skill's directory",
		tok, m[1], r.display(r.tree.Top))
	return true
}

// siblingSkillSource reports whether the sibling skill next to it ships tail: a
// sibling's source directory shares it.itemDir's parent.
func (r *runner) siblingSkillSource(it *item, sibling, tail string) bool {
	if it.itemDir == "" {
		return false
	}
	return r.existsAbs(filepath.Join(filepath.Dir(it.itemDir), sibling, filepath.FromSlash(tail)))
}

// existsRepo resolves a repo-relative path from the repository top or, for a
// nested root, from that root's directory.
func (r *runner) existsRepo(rel string) bool {
	return r.tree.Exists(rel) || (r.baseRel != "" && r.tree.Exists(r.baseRel+"/"+rel))
}

// scopeDirs lists the directories the configuration's [[scopes]] emit into, as
// absolute paths. A scope renders the root content into its own root file under
// BaseDir/<scope.Path>, so a relative link in that content is written relative
// to the scope directory, not to the source file. Empty without scopes.
func (r *runner) scopeDirs() []string {
	if len(r.cfg.Scopes) == 0 {
		return nil
	}
	base := r.rootAbs()
	out := make([]string, 0, len(r.cfg.Scopes))
	for _, s := range r.cfg.Scopes {
		if s.Path != "" {
			out = append(out, filepath.Join(base, filepath.FromSlash(s.Path)))
		}
	}
	return out
}

// existsScope reports whether rel exists under a scope's emitted directory,
// which is where the root content lands when it is rendered into that scope.
func (r *runner) existsScope(rel string) bool {
	for _, dir := range r.cfg.Scopes {
		if dir.Path == "" {
			continue
		}
		c := path.Join(r.baseRel, filepath.ToSlash(dir.Path), rel)
		if r.tree.Exists(c) {
			return true
		}
	}
	return false
}

func (r *runner) requireName(it *item, line int, name, kind string, sets ...map[string]bool) {
	key := strings.ToLower(name)
	r.depName(it.abs, key)
	if looksPlaceholder(key) || strings.Contains(key, ":") {
		return
	}
	for _, s := range sets {
		if s[key] {
			return
		}
	}
	// "the `test-writer` agent" or "`test-writer` rules" is prose about a name
	// that exists as another kind: the kind word is a loose description, not a
	// lookup, so only a name no namespace defines is unknown.
	for _, s := range []map[string]bool{r.rules, r.skills, r.agents, r.commands, r.contexts} {
		if s[key] {
			return
		}
	}
	if kind != "rule" && r.pluginProvided(it) {
		r.addWithSeverity(CodeReferenceUnknown, SeverityInfo, it.abs, line,
			"references %s %q, which no local file defines; it may come from an installed plugin", kind, name)
		return
	}
	r.add(CodeReferenceUnknown, it.abs, line, "references %s %q, which does not exist", kind, name)
}

var (
	listMarkerRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(?:[-*+>]|\d+[.)])\s+`) })
	invokeWords  = map[string]bool{
		"run": true, "runs": true, "invoke": true, "invokes": true, keyType: true, "use": true,
		"call": true, hookTypeCommand: true, "commands": true, "slash": true, "execute": true, "try": true, "via": true, "or": true, "then": true,
	}
)

// slashInvocation reports whether a "/name" token following before reads as a
// slash command invocation: it starts the line or a list item, or follows a word
// like "run" or "command". Any other "/word-word" in prose is usually an HTTP
// route or a URL path, which must not be reported as a missing command.
func slashInvocation(before string) bool {
	before = strings.TrimSpace(before)
	for listMarkerRe().MatchString(before + " ") {
		before = strings.TrimSpace(listMarkerRe().ReplaceAllString(before+" ", ""))
	}
	if before == "" {
		return true
	}
	words := strings.Fields(before)
	last := strings.ToLower(strings.Trim(words[len(words)-1], "*_:,.;()\"'"))
	return invokeWords[last]
}

// backtickMatches is backtickRe's FindAllStringSubmatchIndex, which needs a
// backtick to match at all.
func backtickMatches(text string) [][]int {
	if strings.IndexByte(text, '`') < 0 {
		return nil
	}
	return backtickRe().FindAllStringSubmatchIndex(text, -1)
}

// nameRef is a name a prose line mentions and the kind its wording claims it is.
type nameRef struct {
	name string
	kind string
}

// referenceNames lists the name references a body line makes. It is the single
// matcher behind AR301 (an unknown reference) and AR990 (a served skill named
// statically), so the two rules agree on what a reference is. Each pattern runs
// only on a line holding a literal every one of its matches contains.
func referenceNames(l bodyLine) []nameRef {
	var out []nameRef
	seen := map[nameRef]bool{}
	add := func(name, kind string) {
		if name = strings.TrimSpace(name); name == "" {
			return
		}
		ref := nameRef{name: name, kind: kind}
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	backtickedNames(l, add)
	plainNames(l, add)
	addMCPCallNames(l.Text, add)
	return out
}

// backtickedNames adds the references written with backticks: "`x` skill",
// "skill `x`". Each pattern runs only on a line holding a literal every one of
// its matches contains.
func backtickedNames(l bodyLine, add func(name, kind string)) {
	if strings.IndexByte(l.Text, '`') < 0 {
		return
	}
	agentWord := strings.Contains(l.Text, "agent") || strings.Contains(l.Text, "skill") // "subagent" holds "agent"
	if agentWord || strings.Contains(l.Text, "rule") {
		for _, m := range nameAfterRe().FindAllStringSubmatchIndex(l.Text, -1) {
			if indefiniteArticleRe().MatchString(l.Text[:m[0]]) {
				continue // "a `both` skill" is a kind of skill, not a name (#352)
			}
			add(l.Text[m[2]:m[3]], kindForWord(l.Text[m[4]:m[5]]))
		}
	}
	if agentWord {
		for _, m := range nameBeforeRe().FindAllStringSubmatch(l.Text, -1) {
			add(m[2], kindForWord(m[1]))
		}
	}
}

// plainNames adds the non-backticked references: Skill(...), subagent_type, a
// slash command, the labeled form ("reference: skill x") and a "skills/x" path.
func plainNames(l bodyLine, add func(name, kind string)) {
	if strings.Contains(l.Text, "Skill(") {
		for _, m := range skillCallRe().FindAllStringSubmatch(l.Text, -1) {
			add(m[1], kindSkill)
		}
	}
	if strings.Contains(l.Text, "subagent_type") {
		for _, m := range subagentRe().FindAllStringSubmatch(l.Text, -1) {
			add(m[1], kindAgent)
		}
	}
	if strings.IndexByte(l.Plain, '/') >= 0 && strings.IndexByte(l.Plain, '-') >= 0 {
		for _, m := range slashRe().FindAllStringSubmatchIndex(l.Plain, -1) {
			if slashInvocation(l.Plain[:m[2]-1]) {
				add(l.Plain[m[2]:m[3]], kindCommand)
			}
		}
	}
	if strings.Contains(l.Text, "skill") {
		for _, m := range labeledSkillRe().FindAllStringSubmatch(l.Text, -1) {
			add(m[1], kindSkill)
		}
	}
	if strings.Contains(l.Text, "skills/") {
		for _, m := range skillDirRe().FindAllStringSubmatch(l.Text, -1) {
			add(m[1], kindSkill)
		}
	}
}

// addMCPCallNames reads the skills MCP calls that name a skill ("load_skill x",
// "get_skill(x)", "load_skill(name: \"x\")") and adds each as a skill reference,
// so AR301 and AR990 read an MCP call like any other pointer (#343).
func addMCPCallNames(text string, add func(name, kind string)) {
	if !strings.Contains(text, "_skill") {
		return
	}
	for _, m := range mcpCallRe().FindAllStringSubmatch(text, -1) {
		for _, g := range m[1:] {
			if g != "" {
				add(g, kindSkill)
				break
			}
		}
	}
}

// kindForWord maps a kind word to the content kind it names. "subagent" and
// "agent" are both agent references.
func kindForWord(w string) string {
	switch w {
	case kindSkill:
		return kindSkill
	case kindRule:
		return kindRule
	default:
		return kindAgent
	}
}

// checkNames reads the skill, rule, agent and command names a line mentions and
// reports each that no namespace defines.
func (r *runner) checkNames(it *item, l bodyLine) {
	for _, ref := range referenceNames(l) {
		switch ref.kind {
		case kindSkill:
			r.requireName(it, l.No, ref.name, kindSkill, r.skills, r.commands)
		case kindRule:
			r.requireName(it, l.No, ref.name, kindRule, r.rules)
		case kindCommand:
			r.requireName(it, l.No, ref.name, kindCommand, r.commands, r.skills)
		default:
			r.requireName(it, l.No, ref.name, kindAgent, r.agents)
		}
	}
}
