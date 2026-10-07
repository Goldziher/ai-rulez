package lint

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// "`name` skill", "`name` agent", "`name` rule": hyphenated names only, since
	// a bare word in backticks before "skill" is usually prose.
	nameAfterRe  = regexp.MustCompile("`([a-z][a-z0-9]*(?:-[a-z0-9]+)+)`\\s+(skill|subagent|agent|rule)s?\\b")
	nameBeforeRe = regexp.MustCompile("\\b(skill|subagent|agent)\\s+`([a-z][a-z0-9_-]*)`")
	slashRe      = regexp.MustCompile(`(?:^|[\s(])/([a-z][a-z0-9]*(?:-[a-z0-9]+)+)(?:[\s,;:)]|\.(?:\s|$)|$)`)
	tickSlashRe  = regexp.MustCompile(`^/([a-z][a-z0-9]*(?:-[a-z0-9]+)+)(?:\s|$)`)
	skillCallRe  = regexp.MustCompile(`\bSkill\(\s*["']?([a-z][a-z0-9-]*)["']?\s*\)`)
	subagentRe   = regexp.MustCompile(`\bsubagent_type["']?\s*[:=]\s*["']([a-z][a-z0-9-]*)["']`)
	skillRelRe   = regexp.MustCompile(`^(references|scripts|assets)/`)
	extRe        = regexp.MustCompile(`^\.[A-Za-z0-9]{1,6}$`)
	fragLineRe   = regexp.MustCompile(`^L\d+`)
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
		for _, m := range backtickRe.FindAllStringSubmatchIndex(l.Text, -1) {
			tok := l.Text[m[2]:m[3]]
			if tickSlashRe.MatchString(strings.TrimSpace(tok)) && !slashInvocation(l.Text[:m[0]]) {
				continue // `/api-docs` in prose is a route, not a command
			}
			r.checkToken(it, l.No, tok, r.pathNotAClaim(l.Text, prev, m[2], m[3]))
		}
		r.checkNames(it, l)
	}
}

func (r *runner) existsAbs(abs string) bool {
	if rel := r.tree.Rel(abs); rel != "" {
		return r.tree.Exists(rel)
	}
	_, err := os.Stat(abs)
	return err == nil
}

func (r *runner) checkLink(it *item, line int, target string) {
	if target == "" || schemeRe.MatchString(target) || strings.HasPrefix(target, "//") {
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
	if frag == "" || fragLineRe.MatchString(frag) || !strings.HasSuffix(strings.ToLower(file), ".md") {
		return
	}
	var raw string
	if file == it.abs && it.isDoc {
		raw = it.cf.Content
	} else if data, err := os.ReadFile(file); err == nil {
		raw = string(data)
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
func (r *runner) checkToken(it *item, line int, tok string, notAClaim bool) {
	tok = strings.TrimSpace(tok)
	if m := tickSlashRe.FindStringSubmatch(tok); m != nil {
		r.requireName(it, line, m[1], kindCommand, r.commands, r.skills)
		return
	}
	tok = lineSuffixRe.ReplaceAllString(strings.TrimRight(tok, ".,;:"), "")
	if !pathLike(tok) {
		return
	}
	tok = strings.TrimPrefix(tok, "./")
	if notAClaim {
		return
	}
	if m := skillRelRe.FindString(tok); m != "" {
		if r.checkSkillRelative(it, line, tok, m) {
			return
		}
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
	if !r.existsRepo(tok) && !r.numberedPrefixExists(tok) {
		r.add(CodePathMissing, it.abs, line, "path %q does not exist in the repository", path.Clean(tok))
	}
}

// pathLike filters out tokens that are prose, placeholders, URLs, bazel labels,
// absolute or parent-relative paths, or dotted symbols such as helpers.run_async.
func pathLike(tok string) bool {
	if tok == "" || strings.Contains(tok, ":") || placeholder.MatchString(tok) {
		return false
	}
	if strings.HasPrefix(tok, "~") || strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "../") {
		return false
	}
	ext := path.Ext(tok)
	return ext == "" || extRe.MatchString(ext)
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

// existsRepo resolves a repo-relative path from the repository top or, for a
// nested root, from that root's directory.
func (r *runner) existsRepo(rel string) bool {
	return r.tree.Exists(rel) || (r.baseRel != "" && r.tree.Exists(r.baseRel+"/"+rel))
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
	listMarkerRe = regexp.MustCompile(`^(?:[-*+>]|\d+[.)])\s+`)
	invokeWords  = map[string]bool{
		"run": true, "runs": true, "invoke": true, "invokes": true, "type": true, "use": true,
		"call": true, hookTypeCommand: true, "commands": true, "slash": true, "execute": true, "try": true, "via": true, "or": true, "then": true,
	}
)

// slashInvocation reports whether a "/name" token following before reads as a
// slash command invocation: it starts the line or a list item, or follows a word
// like "run" or "command". Any other "/word-word" in prose is usually an HTTP
// route or a URL path, which must not be reported as a missing command.
func slashInvocation(before string) bool {
	before = strings.TrimSpace(before)
	for listMarkerRe.MatchString(before + " ") {
		before = strings.TrimSpace(listMarkerRe.ReplaceAllString(before+" ", ""))
	}
	if before == "" {
		return true
	}
	words := strings.Fields(before)
	last := strings.ToLower(strings.Trim(words[len(words)-1], "*_:,.;()\"'"))
	return invokeWords[last]
}

func (r *runner) checkNames(it *item, l bodyLine) {
	for _, m := range nameAfterRe.FindAllStringSubmatch(l.Text, -1) {
		switch m[2] {
		case "skill":
			r.requireName(it, l.No, m[1], "skill", r.skills, r.commands)
		case "rule":
			r.requireName(it, l.No, m[1], "rule", r.rules)
		default:
			r.requireName(it, l.No, m[1], "agent", r.agents)
		}
	}
	for _, m := range nameBeforeRe.FindAllStringSubmatch(l.Text, -1) {
		if m[1] == "skill" {
			r.requireName(it, l.No, m[2], "skill", r.skills, r.commands)
		} else {
			r.requireName(it, l.No, m[2], "agent", r.agents)
		}
	}
	for _, m := range slashRe.FindAllStringSubmatchIndex(l.Plain, -1) {
		if !slashInvocation(l.Plain[:m[2]-1]) {
			continue // "GET /user-profile" is a route, not a command
		}
		r.requireName(it, l.No, l.Plain[m[2]:m[3]], kindCommand, r.commands, r.skills)
	}
	for _, m := range skillCallRe.FindAllStringSubmatch(l.Text, -1) {
		r.requireName(it, l.No, m[1], "skill", r.skills, r.commands)
	}
	for _, m := range subagentRe.FindAllStringSubmatch(l.Text, -1) {
		r.requireName(it, l.No, m[1], "agent", r.agents)
	}
}
