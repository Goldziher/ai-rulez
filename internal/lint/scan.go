package lint

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

var (
	// "`name` skill", "`name` agent", "`name` rule": the kind word is singular,
	// because "`term` skills" is a compound noun ("disable-model-invocation
	// skills"), not a pointer to a skill named after the backticked term.
	nameAfterRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile("`([a-z][a-z0-9]*(?:-[a-z0-9]+)*(?::[a-z][a-z0-9]*(?:-[a-z0-9]+)*)?)`\\s+(skill|subagent|agent|rule)\\b")
	})
	nameBeforeRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile("\\b(skill|subagent|agent)\\s+`([a-z][a-z0-9_-]*(?::[a-z][a-z0-9_-]*)?)`")
	})
	slashRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|[\s(])/([a-z][a-z0-9]*(?:-[a-z0-9]+)+)(?:[\s,;:)]|\.(?:\s|$)|$)`)
	})
	tickSlashRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^/([a-z][a-z0-9]*(?:-[a-z0-9]+)+)(?:\s|$)`) })
	skillCallRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\bSkill\(\s*["']?([a-z][a-z0-9-]*)["']?\s*\)`) })
	subagentRe  = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\bsubagent_type["']?\s*[:=]\s*["']?([a-z][a-z0-9-]*(?::[a-z][a-z0-9-]*)?)`)
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
	// possessiveRe matches a possessive right before a token ("eslint's "), and
	// toolBeforeRe a linter or tool name ("the eslint "): "eslint's `camelcase`
	// rule" names a linter rule, not a rule item of the project (#416).
	possessiveRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile("(?i)[\\p{L}\\p{N}_-]['\u2019]s\\s+$") })
	// toolBeforeRe lists the linters and tools whose own "rules" prose names:
	// "the eslint `no-console` rule" is not a pointer to a rule item (#416).
	toolBeforeRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:eslint|tslint|stylelint|biome|oxlint|prettier|ruff|pylint|flake8|mypy|pyright|` +
			`clippy|rubocop|golangci-lint|staticcheck|shellcheck|markdownlint|rumdl|semgrep|checkstyle|sonarqube|sonar|` +
			`lint|linter|linters|linting)\s+$`)
	})
	// skillDirRe matches the file-path pointer forms "skills/x" and
	// ".claude/skills/x/SKILL.md", capturing the skill directory name. The
	// "skills/" component must start the path or sit under a harness dot
	// directory (".claude/skills/..."): a nested "skills/" segment of some other
	// tree ("src/mastra/public/skills/shared/<name>/SKILL.md") is not a pointer
	// to a skill of this project (#352).
	skillDirRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|[^\w/-])(?:\./)?(?:[.][a-z0-9_-]+/(?:domains/[a-z0-9_-]+/)?)?skills/([a-z0-9]+(?:-[a-z0-9]+)*)`)
	})
	// seeSkillRe matches a prose pointer by bare name: "see skill dev-cycle",
	// "load the skill dev-cycle". Only a hyphenated name counts, so "see skill
	// authoring" and "see skill-based design" stay prose (#363).
	seeSkillRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i:\b(?:see|load|read|use|follow|invoke|run))\s+(?:the\s+)?skill\s+([a-z0-9]+(?:-[a-z0-9]+)+)\b`)
	})
	skillWordAfterRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile("^`?\\s+skill\\b") })
	skillRelRe       = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(references|scripts|assets)/`) })
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
		labels := linkLabelSpans(l.Text)
		for _, m := range backtickMatches(l.Text) {
			if inSpans(labels, m[0]) {
				continue // the label of a link is display text; the link target is checked instead
			}
			tok := l.Text[m[2]:m[3]]
			if tickSlashRe().MatchString(strings.TrimSpace(tok)) && !slashInvocation(l.Text[:m[0]]) {
				continue // `/api-docs` in prose is a route, not a command
			}
			r.checkToken(it, l.No, tok, l.Text, m[3], func() bool { return r.pathNotAClaim(l.Text, prev, m[2], m[3]) })
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
func (r *runner) checkToken(it *item, line int, tok, text string, end int, notAClaim func() bool) { //nolint:gocyclo // linear token checks
	tok = strings.TrimSpace(tok)
	if m := tickSlashRe().FindStringSubmatch(tok); m != nil {
		if mentionNotAClaim(text, tok) {
			return
		}
		r.requireNameAs(it, line, m[1], kindCommand, userSkillWording(text[end:]), r.commands, r.skills)
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
		if r.checkSkillRelative(it, line, tok, m, text) {
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
	if !r.existsRepo(tok) && !r.existsScope(tok) && !r.numberedPrefixExists(tok) && !r.existsNearFile(it, tok) {
		msg := fmt.Sprintf("path %q does not exist in the repository", path.Clean(tok))
		r.add(CodePathMissing, it.abs, line, "%s", msg)
		r.noteMissing(it.abs, line, msg, tok)
	}
}

// missingPath is an AR401 finding that dropIgnoredMissing may still withdraw.
type missingPath struct {
	file    string
	line    int
	message string
	probes  []string // the repository paths git is asked about
}

// noteMissing remembers a reported missing path so a path git ignores can be withdrawn once the
// run has them all (dropIgnoredMissing): one git call for the run, not one per path.
func (r *runner) noteMissing(abs string, line int, msg, tok string) {
	rel := path.Clean(tok)
	probes := []string{rel}
	if r.baseRel != "" {
		probes = append(probes, r.baseRel+"/"+rel)
	}
	r.missingMu.Lock()
	r.missing = append(r.missing, missingPath{file: r.display(abs), line: max(line, 1), message: msg, probes: probes})
	r.missingMu.Unlock()
}

// dropIgnoredMissing withdraws the AR401 findings for paths that .gitignore, .git/info/exclude or
// the global excludes file hides: a file the repository documents but keeps local
// (`.claude/settings.local.json`), which its own checkout may not have. git is asked once, for
// the paths already found missing. Outside a repository, or when git fails, nothing is withdrawn.
func (r *runner) dropIgnoredMissing() {
	if len(r.missing) == 0 {
		return
	}
	seen := map[string]bool{}
	var probes []string
	for _, m := range r.missing {
		for _, p := range m.probes {
			if !seen[p] {
				seen[p] = true
				probes = append(probes, p)
			}
		}
	}
	sort.Strings(probes)
	ignored, err := gitutil.New(r.host.Runner).IgnoredAmongContext(r.ctx, r.tree.Top, probes)
	if err != nil || len(ignored) == 0 {
		return
	}
	drop := map[string]bool{}
	for _, m := range r.missing {
		if slices.ContainsFunc(m.probes, func(p string) bool { return ignored[p] }) {
			drop[fmt.Sprintf("%s\x00%d\x00%s", m.file, m.line, m.message)] = true
		}
	}
	kept := r.findings[:0:0]
	for i := range r.findings {
		f := &r.findings[i]
		if f.Code == CodePathMissing && drop[fmt.Sprintf("%s\x00%d\x00%s", f.File, f.Line, f.Message)] {
			continue
		}
		kept = append(kept, *f)
	}
	r.findings = kept
}

// existsNearFile reports whether tok exists relative to the directory of the file that names it, or
// to its project directory (the one that holds the nearest .claude directory): a path in a nested
// project's content is written from that project, not from the repository root.
func (r *runner) existsNearFile(it *item, tok string) bool {
	dirs := []string{filepath.Dir(it.abs)}
	if it.itemDir != "" {
		dirs = append(dirs, it.itemDir)
	}
	dirs = append(dirs, r.projectDirs(it.abs)...)
	for _, d := range dirs {
		abs := filepath.Join(d, filepath.FromSlash(tok))
		r.dep(it.abs, abs)
		if r.existsAbs(abs) {
			return true
		}
	}
	return false
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
func (r *runner) checkSkillRelative(it *item, line int, tok, prefix, text string) bool {
	if tok == prefix || tok == strings.TrimSuffix(prefix, "/") {
		return true // the bare directory name, used as prose
	}
	if it.itemDir == "" || placeholderSegment(tok) {
		return it.itemDir != ""
	}
	others, external := r.namedSkillDirs(text, it.itemDir)
	dirs := append([]string{it.itemDir}, others...)
	dirs = append(dirs, r.projectDirs(it.abs)...)
	for _, d := range dirs {
		abs := filepath.Join(d, filepath.FromSlash(tok))
		r.dep(it.abs, abs)
		if r.existsAbs(abs) {
			return true
		}
	}
	if r.existsRepo(tok) || r.allowed(tok) || external {
		return true
	}
	r.add(CodeSkillResourceMissing, it.abs, line, "%q was not found relative to this %s's directory (%s) or relative to the repo root (%s)",
		tok, it.kind, r.display(it.itemDir), r.display(r.tree.Top))
	return true
}

// harnessDirNames are the dot directories that hold a project's agent content.
const dirClaude = ".claude"

func harnessDirNames() []string { return []string{dirClaude, ".agents", ".codex"} }

// projectDirs lists the directories at or above the one holding abs that have a .claude directory
// (or .agents or .codex), nearest first: a nested project's content is written from there, so
// products/em/backend for products/em/backend/.claude/skills/x or products/em/backend/src/CLAUDE.md.
func (r *runner) projectDirs(abs string) []string {
	start := filepath.Dir(abs)
	if v, ok := r.projDirs.Load(start); ok {
		if dirs, isDirs := v.([]string); isDirs {
			return dirs
		}
	}
	var out []string
	for d := start; ; d = filepath.Dir(d) {
		rel := r.rel(d)
		if rel == "" && d != filepath.Clean(r.tree.Top) {
			break // left the repository
		}
		for _, h := range harnessDirNames() {
			if r.existsAbs(filepath.Join(d, h)) {
				out = append(out, d)
				break
			}
		}
		if d == filepath.Clean(r.tree.Top) || filepath.Dir(d) == d {
			break
		}
	}
	r.projDirs.Store(start, out)
	return out
}

// namedSkillDirs lists the directories of the other skills the text names ("see skill `x`",
// "the `x` skill"), apart from the skill in self. external is set when it names a skill no local
// file defines (an installed plugin's): its files are not ours to check.
func (r *runner) namedSkillDirs(text, self string) (dirs []string, external bool) {
	if !strings.Contains(text, "skill") {
		return nil, false
	}
	var names []string
	for _, m := range nameAfterRe().FindAllStringSubmatch(text, -1) {
		if kindForWord(m[2]) == kindSkill {
			names = append(names, m[1])
		}
	}
	for _, m := range nameBeforeRe().FindAllStringSubmatch(text, -1) {
		if kindForWord(m[1]) == kindSkill {
			names = append(names, m[2])
		}
	}
	for _, n := range names {
		var found []string
		for _, abs := range r.names[strings.ToLower(n)] {
			if d := filepath.Dir(abs); strings.EqualFold(filepath.Base(abs), "SKILL.md") && d != self {
				found = append(found, d)
			}
		}
		switch {
		case len(found) > 0:
			dirs = append(dirs, found...)
		case !r.skills[strings.ToLower(n)] || strings.Contains(n, ":"):
			external = true
		}
	}
	return dirs, external
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
	r.requireNameAs(it, line, name, kind, false, sets...)
}

// userSkillWording reports whether the text after a backticked "/name" calls it a skill ("`/name`
// skill"). Skills a user installs for themselves are not in the repository, so one that no local
// file defines is a hint, not an error.
func userSkillWording(after string) bool {
	return skillWordAfterRe().MatchString(after)
}

// requireNameAs is requireName; userLevel marks a name the text calls a skill, which may live in
// the user's own skill directory and is then reported as information.
func (r *runner) requireNameAs(it *item, line int, name, kind string, userLevel bool, sets ...map[string]bool) {
	key := strings.ToLower(name)
	r.depName(it.abs, key)
	if looksPlaceholder(key) || strings.Contains(key, ":") {
		return
	}
	if kind == kindRule && !r.hasRuleItems() {
		return // no rule item to point at: "the `no-console` rule" is prose (#416)
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
	if kind != kindRule && r.cfg != nil && r.cfg.Implicit {
		r.addWithSeverity(CodeReferenceUnknown, SeverityInfo, it.abs, line,
			"references %s %q, which no file of the repository defines; it may be a user-level or plugin %s", kind, name, kind)
		return
	}
	if kind != "rule" && r.pluginProvided(it) {
		r.addWithSeverity(CodeReferenceUnknown, SeverityInfo, it.abs, line,
			"references %s %q, which no local file defines; it may come from an installed plugin", kind, name)
		return
	}
	if userLevel {
		r.addWithSeverity(CodeReferenceUnknown, SeverityInfo, it.abs, line,
			"references skill %q, which no local file defines; it may be a user-level or plugin skill", name)
		return
	}
	r.add(CodeReferenceUnknown, it.abs, line, "references %s %q, which does not exist", kind, name)
}

// hasRuleItems reports whether the tree defines any rule, apart from the names
// lint.known_names declares.
func (r *runner) hasRuleItems() bool {
	known := make(map[string]bool, len(r.lc.KnownNames))
	for _, n := range r.lc.KnownNames {
		known[strings.ToLower(n)] = true
	}
	for n := range r.rules {
		if !known[n] {
			return true
		}
	}
	return false
}

var (
	listMarkerRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(?:[-*+>]|\d+[.)])\s+`) })
	invokeWords  = map[string]bool{
		"run": true, "runs": true, "invoke": true, "invokes": true, keyType: true, "use": true,
		"call": true, hookTypeCommand: true, keyCommands: true, "slash": true, "execute": true, "try": true, "via": true, "or": true, "then": true,
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

// linkLabelSpans lists the [start, end) byte ranges of the text of the inline links
// on a line ("[label](target)"); a code span inside one is the label, not a reference (#456).
func linkLabelSpans(text string) [][2]int {
	if !strings.Contains(text, "](") {
		return nil
	}
	var out [][2]int
	for _, m := range inlineLinkRe().FindAllStringIndex(text, -1) {
		open := strings.IndexByte(text[m[0]:m[1]], '[')
		closing := strings.Index(text[m[0]:m[1]], "](")
		if open >= 0 && closing > open {
			out = append(out, [2]int{m[0] + open, m[0] + closing})
		}
	}
	return out
}

// inSpans reports whether pos lies inside one of the spans.
func inSpans(spans [][2]int, pos int) bool {
	for _, sp := range spans {
		if pos >= sp[0] && pos < sp[1] {
			return true
		}
	}
	return false
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
			kind := kindForWord(l.Text[m[4]:m[5]])
			if kind == kindRule && toolRuleContext(l.Text[:m[0]]) {
				continue // "eslint's `camelcase` rule" is a linter rule (#416)
			}
			if kind == kindRule && !ruleReferenceContext(l.Text[m[2]:m[3]], l.Text[m[5]:]) {
				continue // "a `display` rule" is CSS or prose, not a pointer to a rule file
			}
			add(l.Text[m[2]:m[3]], kind)
		}
	}
	if agentWord {
		for _, m := range nameBeforeRe().FindAllStringSubmatch(l.Text, -1) {
			add(m[2], kindForWord(m[1]))
		}
	}
}

// ruleReferenceContext reports whether "`name` rule" points at a rule file: the name is
// hyphenated or underscored the way rule file names are, or the phrase goes on to say "rule
// file". A single plain word next to "rule" ("the `display` rule") is prose.
func ruleReferenceContext(name, after string) bool {
	return strings.ContainsAny(name, "-_") || strings.HasPrefix(after, " file")
}

// toolRuleContext reports whether the text before a "`x` rule" token ends in a
// possessive or a linter or tool name, so the rule belongs to a tool.
func toolRuleContext(before string) bool {
	return possessiveRe().MatchString(before) || toolBeforeRe().MatchString(before)
}

// plainNames adds the non-backticked references: Skill(...), subagent_type, a
// slash command, the labeled form ("reference: skill x") and a "skills/x" path.
func plainNames(l bodyLine, add func(name, kind string)) { //nolint:gocyclo // one scan over the alternative pointer forms; splitting it scatters the matchers
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
	if strings.Contains(l.Text, "skill ") || strings.Contains(l.Text, "Skill ") {
		for _, m := range seeSkillRe().FindAllStringSubmatch(l.Text, -1) {
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
		if mentionNotAClaim(l.Text, ref.name) {
			continue
		}
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
