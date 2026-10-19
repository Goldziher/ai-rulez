package lint

import (
	"path"
	"regexp"
	"strings"
	"sync"
)

// The checks below keep AR401 and AR402 quiet where a backticked path is not a
// claim that the path exists: it is negated, an example, a placeholder, a build
// artifact, an alternative, or it names a file of another repository.

var (
	sentenceEndRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`[.!?;]\s`) })
	// noBeforeRe is a bare "no" right before the token ("There is no `x`"); the
	// other markers negate the whole sentence.
	noBeforeRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?i)\bno\s+(?:\w+\s+){0,2}$`) })
	negationRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:never|removed|deleted|decommissioned|retired|dropped|renamed|obsolete|older|formerly|gone|empty)\b|\bno longer\b|\bnot (?:exist|present|tracked|committed|checked in)|n't exist\b|\bused to\b|\bmoved to\b|` + historicalMarkers)
	})
	// prohibitionRe is a ban on creating the thing: "Don't add a `references/patterns/` tier".
	// A ban on using it ("Do not edit `x`") still asserts that it exists.
	prohibitionRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:don['’]t|do not|never|avoid|must not|should not)\s+(?:\w+\s+){0,2}?(?:add|create|introduce|make|write|invent)\b`)
	})
	// mentionHistoricalRe marks a sentence that talks about a name that used to exist, or
	// that is declared not to: such a mention is no pointer.
	mentionHistoricalRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:removed|deleted|decommissioned|retired|dropped|renamed|obsolete|formerly|no longer|used to)\b|n't exist\b|\bnot exist|` + historicalMarkers)
	})
	ignoredRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\bgit-?ignored?\b|\bignored by git\b|\bnot (?:committed|checked in)\b|\.gitignore\b|\bbuild artifacts?\b|\bgenerated at (?:build|run) ?time\b`)
	})
	exampleRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\be\.g\.|\bfor (?:example|instance)\b|\bsuch as\b`)
	})
	alternativeRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?i)\bor\s+(?:under|in|at|inside|within)?\s*$`) })
	atRefRe       = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^\s*@\s`) })
	dateHolderRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\bYYYY\b|\bMM\b|\bDD\b|\bNNN+\b|\bX\.Y\.Z\b`) })
	repoSlugRe    = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	})
	repoWordRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?i)\brepos?(?:sitory|sitories)?\b|github\.com/`) })
	digitsRe   = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^\d+$`) })
	// imageRefRe matches a container image reference without a registry host or
	// a tag/digest: a namespace and an image name, each a Docker image component
	// (lower-case alphanumerics joined by ".", "_" or "-").
	imageRefRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+$`)
	})
)

// historicalMarkers is the regexp alternation of the words that mark a mention as history or a
// denial: the former thing, a deprecated one, one that is not (or no longer) registered.
const historicalMarkers = `\bformer\b|\bdeprecated\b|\bsuperseded\b|\bde-?registered\b|\bnot (?:a )?(?:registered|real|valid|available)\b`

// sentenceAround is the sentence of text that holds text[start:end].
func sentenceAround(text string, start, end int) string {
	before, after := text[:start], text[end:]
	if m := sentenceEndRe().FindAllStringIndex(before, -1); len(m) > 0 {
		before = before[m[len(m)-1][1]:]
	}
	if m := sentenceEndRe().FindStringIndex(after); m != nil {
		after = after[:m[0]]
	}
	return before + " " + text[start:end] + " " + after
}

// mentionNotAClaim reports whether every mention of name in text sits in a sentence that
// denies it or puts it in the past ("the former `x` skill", "`/x` is NOT a registered command",
// "Don't add a `references/patterns/` tier"). A name that is not in text is not negated.
func mentionNotAClaim(text, name string) bool {
	found := false
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], name)
		if i < 0 {
			break
		}
		i += from
		from = i + len(name)
		found = true
		sentence := sentenceAround(text, i, from)
		if !mentionHistoricalRe().MatchString(sentence) && !prohibitionRe().MatchString(sentence) {
			return false
		}
	}
	return found
}

// placeholderSegment reports whether a path has a segment that stands for a name: a single capital
// letter ("references/services/X.md"), "..." or a glob.
func placeholderSegment(tok string) bool {
	for _, seg := range strings.Split(tok, "/") {
		base := strings.TrimSuffix(seg, path.Ext(seg))
		if len(base) == 1 && base[0] >= 'A' && base[0] <= 'Z' || strings.Contains(seg, "...") || strings.Contains(seg, "…") || strings.Contains(seg, "*") {
			return true
		}
	}
	return false
}

// imageRefToken reports whether tok is a container image reference rather than a
// repository path. It has no file extension and the "namespace/name" shape of an
// image, and its image name carries the underscore Docker names use, which a
// repository path of this shape rarely does. An image tag names nothing in the
// repository, so AR401 must not report it. A token with a file extension, or an
// extension-less path without the image-name underscore, stays a path claim.
func imageRefToken(tok string) bool {
	if path.Ext(tok) != "" || !imageRefRe().MatchString(tok) {
		return false
	}
	return strings.Contains(tok[strings.LastIndexByte(tok, '/')+1:], "_")
}

// buildDirs are directory names a build, an installer or a runtime creates and a
// repository rarely tracks.
var buildDirs = map[string]bool{
	"target": true, dirNodeModules: true, dirVenv: true, "venv": true, "vendor": true,
	"dist": true, "build": true, "__pycache__": true, ".tox": true, ".pytest_cache": true,
	".gradle": true, ".next": true,
}

// pathNotAClaim reports whether the backticked token at text[start:end] is
// written in a way that does not assert the path exists in this repository.
func (r *runner) pathNotAClaim(text, prev string, start, end int) bool {
	tok := strings.TrimSpace(text[start:end])
	before, after := text[:start], text[end:]
	if m := sentenceEndRe().FindAllStringIndex(before, -1); len(m) > 0 {
		before = before[m[len(m)-1][1]:]
	}
	if m := sentenceEndRe().FindStringIndex(after); m != nil {
		after = after[:m[0]]
	}
	sentence := before + " " + after
	switch {
	case dateHolderRe().MatchString(tok):
		return true
	case noBeforeRe().MatchString(strings.TrimRight(before, "` ") + " "), negationRe().MatchString(sentence), ignoredRe().MatchString(sentence), prohibitionRe().MatchString(sentence):
		return true
	case exampleRe().MatchString(text[:start]):
		return true
	case alternativeRe().MatchString(strings.TrimRight(before, "` ")):
		return true
	case strings.HasPrefix(strings.TrimPrefix(after, "`"), " @ ") || atRefRe().MatchString(strings.TrimPrefix(after, "`")):
		return true // "charts/pro @ development" names a ref of another repository
	case hasBuildDir(tok), r.otherRepoLine(text, prev, tok), globInParagraph(text, prev, tok), imageRefToken(tok):
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
		for _, m := range backtickRe().FindAllStringSubmatch(line, -1) {
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
	if !tableRow && !repoWordRe().MatchString(text) {
		return false
	}
	for _, m := range backtickRe().FindAllStringSubmatch(text, -1) {
		slug := strings.TrimSpace(m[1])
		if slug == tok || !repoSlugRe().MatchString(slug) || path.Ext(slug) != "" {
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
	if !digitsRe().MatchString(base) {
		return false
	}
	roots := []string{""}
	if r.baseRel != "" {
		roots = append(roots, r.baseRel+"/")
	}
	for _, root := range roots {
		if r.tree.HasPathPrefix(root+dir+base+"-") || r.tree.HasPathPrefix(root+dir+base+".") {
			return true
		}
	}
	return false
}
