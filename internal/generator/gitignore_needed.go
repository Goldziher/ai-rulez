package generator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/Goldziher/ai-rulez/internal/logger"
)

// gitignoreProbeName stands in for the unknown file under a directory pattern.
const gitignoreProbeName = "ai-rulez-probe"

// gitignoreProbe maps an ignore pattern to a representative path git can be
// asked about: the path itself for a file, a child for a directory, and the probe
// name in place of each glob star. A short stand-in such as "x" would match a
// user rule like "x.*" that does not cover the pattern's real files.
func gitignoreProbe(pattern string) string {
	p := strings.TrimPrefix(pattern, "/")
	if strings.HasSuffix(p, "/") {
		p += gitignoreProbeName
	}
	return strings.ReplaceAll(p, "*", gitignoreProbeName)
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
	if prefix := gitutil.RepoRelative(gitutil.TopLevel(g.config.BaseDir), g.config.BaseDir); prefix != "" && prefix != "." {
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
		rules, err = gitutil.IgnoreRulesMirrored(g.config.BaseDir, probes, rewrite)
	} else {
		rules, err = gitutil.IgnoreRules(g.config.BaseDir, probes)
	}
	if err != nil {
		logger.Debug("Could not ask git about ignore rules; adding every entry", "error", err)
		return nil
	}
	return rules
}

// hasOwnIgnoreBlock reports whether the root .gitignore or the repository
// exclude file holds an ai-rulez block that must be left out of the evaluation.
func (g *Generator) hasOwnIgnoreBlock(excludeBegin string) bool {
	if data, err := os.ReadFile(filepath.Join(g.config.BaseDir, ".gitignore")); err == nil { //nolint:gosec // project .gitignore
		if withoutManagedBlock(string(data)) != string(data) {
			return true
		}
	}
	if exclude := gitutil.InfoExcludePath(g.config.BaseDir); exclude != "" {
		fallbackBegin, _ := gitignore.FallbackMarkers(g.config.BaseDir)
		if data, err := os.ReadFile(exclude); err == nil && //nolint:gosec // git exclude file
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
	probes := make([]string, len(patterns))
	for i, pattern := range patterns {
		probes[i] = gitignoreProbe(pattern)
	}
	rules := g.userIgnoreRules(probes)
	if rules == nil {
		return dropCoveredPatterns(patterns), nil
	}
	protected := g.protectedGitignorePatterns(outputs)
	needed = make([]string, 0, len(patterns))
	for i, pattern := range patterns {
		match := rules[probes[i]]
		switch {
		case match.Ignored():
			logger.Debug("Already ignored by git", "pattern", pattern, "rule", match.Pattern, "source", match.Source)
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
