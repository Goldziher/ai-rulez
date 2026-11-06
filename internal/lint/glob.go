package lint

import "github.com/Goldziher/ai-rulez/v5/internal/pathglob"

// globMatcher matches slash-separated repo-relative paths against one glob (see package pathglob).
type globMatcher struct{ m pathglob.Matcher }

func newGlob(pattern string) (globMatcher, bool) {
	m, ok := pathglob.New(pattern)
	return globMatcher{m: m}, ok
}

func malformedGlob(pattern string) bool { return pathglob.Malformed(pattern) }

func (g globMatcher) match(path string) bool { return g.m.Match(path) }

func expandBraces(p string) []string { return pathglob.ExpandBraces(p) }
