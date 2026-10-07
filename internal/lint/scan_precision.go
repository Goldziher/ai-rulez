package lint

import (
	"path"
	"regexp"
	"strings"
)

// The checks below keep AR401 and AR402 quiet where a backticked path is not a
// claim that the path exists: it is negated, an example, a placeholder, a build
// artifact, an alternative, or it names a file of another repository.

var (
	sentenceEndRe = regexp.MustCompile(`[.!?;]\s`)
	// noBeforeRe is a bare "no" right before the token ("There is no `x`"); the
	// other markers negate the whole sentence.
	noBeforeRe    = regexp.MustCompile(`(?i)\bno\s+(?:\w+\s+){0,2}$`)
	negationRe    = regexp.MustCompile(`(?i)\b(?:never|removed|deleted|decommissioned|retired|dropped|renamed|obsolete|older|formerly|gone|empty)\b|\bno longer\b|\bnot (?:exist|present|tracked|committed|checked in)|n't exist\b|\bused to\b|\bmoved to\b`)
	ignoredRe     = regexp.MustCompile(`(?i)\bgit-?ignored?\b|\bignored by git\b|\bnot (?:committed|checked in)\b|\.gitignore\b|\bbuild artifacts?\b|\bgenerated at (?:build|run) ?time\b`)
	exampleRe     = regexp.MustCompile(`(?i)\be\.g\.|\bfor (?:example|instance)\b|\bsuch as\b`)
	alternativeRe = regexp.MustCompile(`(?i)\bor\s+(?:under|in|at|inside|within)?\s*$`)
	atRefRe       = regexp.MustCompile(`^\s*@\s`)
	dateHolderRe  = regexp.MustCompile(`\bYYYY\b|\bMM\b|\bDD\b|\bNNN+\b|\bX\.Y\.Z\b`)
	repoSlugRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	repoWordRe    = regexp.MustCompile(`(?i)\brepos?(?:sitory|sitories)?\b|github\.com/`)
	digitsRe      = regexp.MustCompile(`^\d+$`)
)

// buildDirs are directory names a build, an installer or a runtime creates and a
// repository rarely tracks.
var buildDirs = map[string]bool{
	"target": true, "node_modules": true, ".venv": true, "venv": true, "vendor": true,
	"dist": true, "build": true, "__pycache__": true, ".tox": true, ".pytest_cache": true,
	".gradle": true, ".next": true,
}

// pathNotAClaim reports whether the backticked token at text[start:end] is
// written in a way that does not assert the path exists in this repository.
func (r *runner) pathNotAClaim(text, prev string, start, end int) bool {
	tok := strings.TrimSpace(text[start:end])
	before, after := text[:start], text[end:]
	if m := sentenceEndRe.FindAllStringIndex(before, -1); len(m) > 0 {
		before = before[m[len(m)-1][1]:]
	}
	if m := sentenceEndRe.FindStringIndex(after); m != nil {
		after = after[:m[0]]
	}
	sentence := before + " " + after
	switch {
	case dateHolderRe.MatchString(tok):
		return true
	case noBeforeRe.MatchString(strings.TrimRight(before, "` ") + " "), negationRe.MatchString(sentence), ignoredRe.MatchString(sentence):
		return true
	case exampleRe.MatchString(text[:start]):
		return true
	case alternativeRe.MatchString(strings.TrimRight(before, "` ")):
		return true
	case strings.HasPrefix(strings.TrimPrefix(after, "`"), " @ ") || atRefRe.MatchString(strings.TrimPrefix(after, "`")):
		return true // "charts/pro @ development" names a ref of another repository
	case hasBuildDir(tok), r.otherRepoLine(text, prev, tok), globInParagraph(text, prev, tok):
		return true
	}
	return false
}

func hasBuildDir(tok string) bool {
	for _, seg := range strings.Split(strings.TrimSuffix(tok, "/"), "/")[1:] {
		if buildDirs[seg] {
			return true
		}
	}
	return false
}

// globInParagraph reports whether the line or the one before it carries a glob in
// code ("`e2e/**` also prunes `src/test/java/e2e/`"): the other paths there are
// illustrations of what the glob matches.
func globInParagraph(text, prev, tok string) bool {
	for _, line := range []string{text, prev} {
		for _, m := range backtickRe.FindAllStringSubmatch(line, -1) {
			if m[1] != tok && strings.ContainsAny(m[1], "*") && strings.Contains(m[1], "/") {
				return true
			}
		}
	}
	return false
}

// otherRepoLine reports whether the line names another repository, as a
// "owner/repo" slug that is no path of this one, in a table row or next to the
// word "repo": the other paths on that line live in that repository.
func (r *runner) otherRepoLine(text, _ string, tok string) bool {
	tableRow := strings.HasPrefix(strings.TrimSpace(text), "|")
	if !tableRow && !repoWordRe.MatchString(text) {
		return false
	}
	for _, m := range backtickRe.FindAllStringSubmatch(text, -1) {
		slug := strings.TrimSpace(m[1])
		if slug == tok || !repoSlugRe.MatchString(slug) || path.Ext(slug) != "" {
			continue
		}
		first, _, _ := strings.Cut(slug, "/")
		if !r.tree.IsTopLevel(first) && !r.existsRepo(slug) {
			return true
		}
	}
	return false
}

// numberedPrefixExists reports whether rel is a bare number ("adrs/0065") that
// prefixes a tracked entry of its directory ("adrs/0065-license.md").
func (r *runner) numberedPrefixExists(rel string) bool {
	dir, base := path.Split(strings.TrimSuffix(rel, "/"))
	if !digitsRe.MatchString(base) {
		return false
	}
	roots := []string{""}
	if r.baseRel != "" {
		roots = append(roots, r.baseRel+"/")
	}
	for _, p := range r.tree.Paths() {
		for _, root := range roots {
			if strings.HasPrefix(p, root+dir+base+"-") || strings.HasPrefix(p, root+dir+base+".") {
				return true
			}
		}
	}
	return false
}
