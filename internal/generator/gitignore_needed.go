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
func withoutManagedBlock(content string) string { return gitignore.WithoutManagedBlock(content) }

// userIgnoreRules asks git for the last ignore rule matching each probe, as if
// ai-rulez's own entries were not there: a block entry would otherwise hide the
// user's rule and make our entries look user-made on the next run. Git is
// pointed at a throwaway copy of the repository's ignore files, so the user's
// files are never modified and the answer holds only what the repository
// tracks: the machine's global excludes file and .git/info/exclude are left out,
// or two developers would generate different blocks (#518). It returns nil when
// there is no answer (not a repository, git missing or failing), so callers fall
// back to adding everything.
func (g *Generator) userIgnoreRules(probes []string) map[string]gitutil.IgnoreMatch {
	rootRel := gitignoreFileName
	if rel := gitutil.RepoRelative(g.git().TopLevelContext(g.ctx, g.config.BaseDir), g.managedGitignorePath()); rel != "" {
		rootRel = rel
	}
	rewrite := func(rel, content string) string {
		switch rel {
		case "info/exclude":
			// Machine-local, like the global excludes file: never part of the answer.
			return ""
		case rootRel:
			return withoutManagedBlock(content)
		}
		return content
	}
	rules, err := g.git().IgnoreRulesMirroredContext(g.ctx, g.config.BaseDir, probes, rewrite)
	if err != nil {
		g.log().Debug("Could not ask git about ignore rules; adding every entry", "error", err)
		return nil
	}
	return rules
}

// protectedGitignorePatterns are the patterns that keep machine-local or secret
// content out of git. A user rule un-ignoring one of them is honored (it is not
// re-ignored) but warned about.
func (g *Generator) protectedGitignorePatterns(outputs []config.OutputFile) map[string]bool {
	protected := map[string]bool{}
	for _, output := range outputs {
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		switch {
		case output.LocalOnly && output.IsDir:
			protected[strings.TrimSuffix(relPath, "/")+"/"] = true
		case output.LocalOnly:
			protected[localGitignorePattern(g.config.RulesDirs, relPath)] = true
		case output.Sensitive:
			if pattern := gitignorePatternForOutput(g.config.RulesDirs, relPath, output.IsDir); pattern != "" {
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
	Line    int
	// Protected: a machine-local or secret output. Any other is committed on
	// purpose, which gitignore_exclude says without a hand-written negation.
	Protected bool
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
		if !covered && !match.Negated() {
			// git reports no rule for a probe below a directory that "!dir/" re-includes,
			// so a directory entry needs the user's negation read from the file.
			if m, ok := g.userDirNegation(pattern); ok {
				match = m
			}
		}
		switch {
		case covered:
			g.log().Debug("Already ignored by git", "pattern", pattern, "rule", match.Pattern, "source", match.Source)
		case match.Negated():
			overridden = append(overridden, unignoredProtected{
				Pattern: pattern, Rule: match.Pattern, Source: match.Source, Line: match.Line,
				Protected: g.isProtectedPattern(pattern, protected),
			})
		default:
			needed = append(needed, pattern)
		}
	}
	return dropCoveredPatterns(needed), overridden
}

// userDirNegation finds the user's own "!dir/" (or "!/dir/") line for a directory
// pattern in the .gitignore this run manages, outside the managed block. git
// cannot be asked: the probes are children of the directory and no rule names
// them, although the directory itself is deliberately re-included.
func (g *Generator) userDirNegation(pattern string) (gitutil.IgnoreMatch, bool) {
	if !strings.HasSuffix(pattern, "/") {
		return gitutil.IgnoreMatch{}, false
	}
	data, err := gitutil.ReadIgnoreFileOrEmpty(g.log(), g.managedGitignorePath())
	if err != nil {
		return gitutil.IgnoreMatch{}, false
	}
	want := strings.TrimPrefix(joinGitignorePrefix(g.gitignorePrefix, pattern), "/")
	inBlock := false
	for i, line := range strings.Split(string(data), "\n") {
		switch trimmed := strings.TrimSpace(line); {
		case trimmed == gitignore.BeginMarker:
			inBlock = true
		case trimmed == gitignore.EndMarker:
			inBlock = false
		case inBlock || !strings.HasPrefix(trimmed, "!"):
		case strings.TrimPrefix(strings.TrimPrefix(trimmed, "!"), "/") == want:
			return gitutil.IgnoreMatch{Source: gitignoreFileName, Line: i + 1, Pattern: trimmed}, true
		}
	}
	return gitutil.IgnoreMatch{}, false
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
