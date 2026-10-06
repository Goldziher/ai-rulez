package generator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// gitignoreProbeNames stand in for the unknown file under a directory (or the
// text a glob star matches). Two dissimilar names are used so that a user rule
// that happens to match one stand-in does not make a whole directory look
// covered: "x.*", "*.tmp" or "generated-*" each catch only one of them. A name
// must not start with "ai-rulez": a Go project's own "/ai-rulez-*" or
// "**/ai-rulez-*" binary rules would match it and make git report every
// directory as already ignored, so nothing under it reached the managed block.
var gitignoreProbeNames = [...]string{"generated-probe", "x7q-probe.tmp"}

// gitignoreProbes maps an ignore pattern to the representative paths git is
// asked about: the path itself for a file (one probe), and for a directory or a
// glob one probe per stand-in name, a child of the directory or the name in
// place of each glob star. The pattern counts as covered only when every probe
// is ignored.
func gitignoreProbes(pattern string) []string {
	p := strings.TrimPrefix(pattern, "/")
	isDir := strings.HasSuffix(p, "/")
	if !isDir && !strings.Contains(p, "*") {
		return []string{p}
	}
	probes := make([]string, 0, len(gitignoreProbeNames))
	for _, name := range gitignoreProbeNames {
		q := p
		if isDir {
			q += name
		}
		probes = append(probes, strings.ReplaceAll(q, "*", name))
	}
	return probes
}

// flattenProbes concatenates the probes of every pattern and returns, per
// pattern, its half-open range in the flat list.
func flattenProbes(patterns []string) (flat []string, ranges [][2]int) {
	ranges = make([][2]int, len(patterns))
	for i, pattern := range patterns {
		start := len(flat)
		flat = append(flat, gitignoreProbes(pattern)...)
		ranges[i] = [2]int{start, len(flat)}
	}
	return flat, ranges
}

// withoutManagedBlock returns content with the ai-rulez managed block removed.
func withoutManagedBlock(content string) string {
	switch {
	case strings.Contains(content, gitignore.BeginMarker):
		return gitignore.ReplaceFencedBlock(content, "")
	case strings.Contains(content, gitignore.OldHeader):
		var kept []string
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) == gitignore.OldHeader {
				break
			}
			kept = append(kept, line)
		}
		return strings.Join(kept, "\n")
	}
	return content
}

// userIgnoreRules asks git for the last ignore rule matching each probe, as if
// ai-rulez's own entries were not there: a block entry would otherwise hide the
// user's rule and make our entries look user-made on the next run. When the root
// .gitignore or the exclude file holds one of our blocks, git is pointed at a
// throwaway copy of the ignore files without them; the user's files are never
// modified. It returns nil when there is no answer (not a repository, git
// missing or failing), so callers fall back to adding everything.
func (g *Generator) userIgnoreRules(probes []string) map[string]gitutil.IgnoreMatch {
	rootRel := ".gitignore"
	if prefix := gitutil.RepoRelative(g.git().TopLevel(g.config.BaseDir), g.config.BaseDir); prefix != "" && prefix != "." {
		rootRel = prefix + "/.gitignore"
	}
	begin, end := g.excludeMarkers()
	fallbackBegin, fallbackEnd := gitignore.FallbackMarkers(g.config.BaseDir)
	rewrite := func(rel, content string) string {
		switch rel {
		case rootRel:
			return withoutManagedBlock(content)
		case "info/exclude":
			content = gitignore.ReplaceMarkedBlock(content, begin, end, "")
			return gitignore.ReplaceMarkedBlock(content, fallbackBegin, fallbackEnd, "")
		}
		return content
	}
	var rules map[string]gitutil.IgnoreMatch
	var err error
	if g.hasOwnIgnoreBlock(begin) {
		rules, err = g.git().IgnoreRulesMirrored(g.config.BaseDir, probes, rewrite)
	} else {
		rules, err = g.git().IgnoreRules(g.config.BaseDir, probes)
	}
	if err != nil {
		g.log().Debug("Could not ask git about ignore rules; adding every entry", "error", err)
		return nil
	}
	return rules
}

// hasOwnIgnoreBlock reports whether the root .gitignore or the repository
// exclude file holds an ai-rulez block that must be left out of the evaluation.
func (g *Generator) hasOwnIgnoreBlock(excludeBegin string) bool {
	if data, err := gitutil.ReadIgnoreFileOrEmpty(filepath.Join(g.config.BaseDir, ".gitignore")); err == nil {
		if withoutManagedBlock(string(data)) != string(data) {
			return true
		}
	}
	if exclude := g.git().InfoExcludePath(g.config.BaseDir); exclude != "" {
		fallbackBegin, _ := gitignore.FallbackMarkers(g.config.BaseDir)
		if data, err := gitutil.ReadIgnoreFileOrEmpty(exclude); err == nil &&
			(strings.Contains(string(data), excludeBegin) || strings.Contains(string(data), fallbackBegin)) {
			return true
		}
	}
	return false
}

// protectedGitignorePatterns are the patterns that keep machine-local or secret
// content out of git. A user rule un-ignoring one of them is honored (it is not
// re-ignored) but warned about.
func (g *Generator) protectedGitignorePatterns(outputs []config.OutputFile) map[string]bool {
	protected := map[string]bool{}
	for _, output := range outputs {
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		switch {
		case output.LocalOnly:
			protected[localGitignorePattern(relPath)] = true
		case output.Sensitive:
			if pattern := gitignorePatternForOutput(relPath, output.IsDir); pattern != "" {
				protected[pattern] = true
			}
		}
	}
	return protected
}

func (g *Generator) isProtectedPattern(pattern string, protected map[string]bool) bool {
	localManifest := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath()))
	return protected[pattern] ||
		strings.Contains(pattern, ".local.") ||
		strings.HasSuffix(pattern, "/"+localSourceDirName+"/") ||
		pattern == localManifest
}

// unignoredProtected is a protected pattern a user rule deliberately un-ignores.
type unignoredProtected struct {
	Pattern string
	Rule    string
	Source  string
}

// neededGitignorePatterns returns, sorted, the collected patterns git does not
// already cover: those no user rule ignores and no user rule deliberately
// un-ignores. When git cannot answer, every pattern is needed. Protected
// patterns the user un-ignored are returned too: they stay out of the block, but
// the caller should warn.
func (g *Generator) neededGitignorePatterns(outputs []config.OutputFile) (needed []string, overridden []unignoredProtected) {
	var patterns []string
	for pattern := range g.collectGitignorePaths(outputs) {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	if len(patterns) == 0 {
		return nil, nil
	}
	probes, ranges := flattenProbes(patterns)
	rules := g.userIgnoreRules(probes)
	if rules == nil {
		return dropCoveredPatterns(patterns), nil
	}
	protected := g.protectedGitignorePatterns(outputs)
	needed = make([]string, 0, len(patterns))
	for i, pattern := range patterns {
		match, covered := coverage(rules, probes[ranges[i][0]:ranges[i][1]])
		switch {
		case covered:
			g.log().Debug("Already ignored by git", "pattern", pattern, "rule", match.Pattern, "source", match.Source)
		case match.Negated():
			if g.isProtectedPattern(pattern, protected) {
				overridden = append(overridden, unignoredProtected{Pattern: pattern, Rule: match.Pattern, Source: match.Source})
			}
		default:
			needed = append(needed, pattern)
		}
	}
	return dropCoveredPatterns(needed), overridden
}

// coverage combines the answers for one pattern's probes. The pattern is
// covered only if every probe is ignored. When it is not, the returned match is
// the first negating rule if any probe was un-ignored (a deliberate user
// choice), else a zero match.
func coverage(rules map[string]gitutil.IgnoreMatch, probes []string) (gitutil.IgnoreMatch, bool) {
	all := true
	var negated gitutil.IgnoreMatch
	for _, probe := range probes {
		m := rules[probe]
		if !m.Ignored() {
			all = false
		}
		if m.Negated() && !negated.Negated() {
			negated = m
		}
	}
	if all {
		return rules[probes[0]], true
	}
	return negated, false
}

// dropCoveredPatterns removes, from a sorted list, the patterns a directory
// pattern of the same list already covers (".xum/" covers ".xum/mcp.jsonc").
func dropCoveredPatterns(patterns []string) []string {
	var dirs []string
	for _, p := range patterns {
		if strings.HasSuffix(p, "/") && !strings.HasPrefix(p, "/") && !strings.ContainsAny(p, "*?[") {
			dirs = append(dirs, p)
		}
	}
	if len(dirs) == 0 {
		return patterns
	}
	kept := make([]string, 0, len(patterns))
	for _, p := range patterns {
		covered := false
		for _, d := range dirs {
			if p != d && strings.HasPrefix(p, d) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, p)
		}
	}
	return kept
}

// dropManagedBlock removes the ai-rulez block from the .gitignore at path,
// leaving every other line as it is.
func dropManagedBlock(path, content string) error {
	trimmed := strings.TrimRight(withoutManagedBlock(content), "\n")
	if trimmed != "" {
		trimmed += "\n"
	}
	if err := os.WriteFile(path, []byte(trimmed), 0o644); err != nil { //nolint:gosec // .gitignore is meant to be world-readable
		return oops.With("path", path).Wrapf(err, "write .gitignore")
	}
	return nil
}
