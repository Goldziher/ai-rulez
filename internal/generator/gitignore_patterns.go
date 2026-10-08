package generator

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/hookplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers" // Register DSL-backed preset generators (overrides legacy registrations where they overlap)
)

// gitignorePatternForOutput maps one generated output path to the pattern that
// belongs in the managed .gitignore block, or "" when the path must not be ignored
// at all. Each family of generated output gets its own resolver so none of them
// has to be read through the others.
func gitignorePatternForOutput(rd *config.RulesDirSet, relPath string, isDir bool) string {
	relPath = strings.TrimPrefix(filepath.ToSlash(relPath), "./")
	if relPath == ".github" || strings.HasSuffix(relPath, "/.github") {
		return ""
	}
	if pattern, matched := rulesDirGitignorePattern(rd, relPath, isDir); matched {
		return pattern
	}
	// A plugin directory is shared with hand-written plugins: ignore the generated module only.
	if !isDir && hookplugins.IsModulePath(relPath) {
		return relPath
	}
	if pattern, matched := githubGitignorePattern(relPath); matched {
		return pattern
	}
	if pattern, matched := sharedDirGitignorePattern(relPath, isDir); matched {
		return pattern
	}
	if !isDir && isSpecSidecarFile(relPath) {
		return relPath
	}
	if pattern, matched := assistantDirGitignorePattern(relPath, isDir); matched {
		return pattern
	}
	if slices.Contains(gitignoreRootFiles(), relPath) {
		return relPath
	}
	if isDir {
		return strings.TrimSuffix(relPath, "/") + "/"
	}
	return relPath
}

// rulesDirGitignorePattern resolves relPath against the rules folders. They are
// shared with hand-written rules, so generated files are ignored one by one
// rather than the folder; matched is false when relPath is not under one.
func rulesDirGitignorePattern(rd *config.RulesDirSet, relPath string, isDir bool) (pattern string, matched bool) {
	rest, ok := rd.Remainder(relPath)
	if !ok {
		return "", false
	}
	if rest == "" {
		return "", true
	}
	// A subfolder of a rules folder (.clinerules/workflows/) is hand-authored
	// territory too, so it is never ignored as a whole either.
	if isDir {
		return "", true
	}
	return relPath, true
}

// sharedDirGitignorePattern resolves relPath against the directories users keep
// their own files in (providers.IsSharedDir), at the repo root or nested under a
// subproject. Such a directory and its direct children (.config/<tool>/) are
// never ignored as a whole; a generated file in them is ignored by exact path and
// only a deeper directory, which is the tool's own, becomes a directory pattern.
func sharedDirGitignorePattern(relPath string, isDir bool) (pattern string, matched bool) {
	segments := strings.Split(strings.TrimSuffix(relPath, "/"), "/")
	for i, segment := range segments {
		if !providers.IsSharedDir(segment) {
			continue
		}
		below := len(segments) - i - 1
		switch {
		case !isDir:
			return relPath, true
		case below <= 1:
			return "", true
		default:
			return strings.TrimSuffix(relPath, "/") + "/", true
		}
	}
	return "", false
}

// isSpecSidecarFile reports whether relPath, at the repo root or under a
// subproject, is a sidecar file a provider spec writes.
func isSpecSidecarFile(relPath string) bool {
	for _, p := range specSidecarPaths() {
		if relPath == p || strings.HasSuffix(relPath, "/"+p) {
			return true
		}
	}
	return false
}

// assistantDirGitignorePattern resolves relPath against the assistant directories
// ai-rulez shares with the user (.claude/, .gemini/, ...), whether at the repo root
// or nested under a subproject. matched is false when relPath is not inside one of
// them; a matched but empty pattern means the path is the shared directory itself,
// which must stay un-ignored because the user tracks their own files in it (#184).
func assistantDirGitignorePattern(relPath string, isDir bool) (pattern string, matched bool) {
	for _, dir := range gitignoreAssistantDirs() {
		trimmedDir := strings.TrimSuffix(dir, "/")
		if relPath == trimmedDir {
			return "", true
		}
		if strings.HasPrefix(relPath, dir) {
			return ownedAssistantSubPath("", dir, strings.TrimPrefix(relPath, dir), isDir), true
		}
		idx := strings.Index(relPath, "/"+trimmedDir)
		if idx < 0 {
			continue
		}
		remainder := relPath[idx+1+len(trimmedDir):]
		if remainder == "" {
			return "", true
		}
		// Anything else is a longer segment that merely starts with the directory
		// name (".clauderc"), so keep looking.
		if strings.HasPrefix(remainder, "/") {
			return ownedAssistantSubPath(relPath[:idx+1], dir, remainder[1:], isDir), true
		}
	}
	return "", false
}

// githubGitignorePattern resolves relPath against the .github/ content ai-rulez
// generates, at the repo root or nested under a subproject.
func githubGitignorePattern(relPath string) (pattern string, matched bool) {
	for _, candidate := range generatedGithubPatterns {
		if relPath == strings.TrimSuffix(candidate, "/") || strings.HasPrefix(relPath, candidate) {
			return candidate, true
		}
		if idx := strings.Index(relPath, "/"+candidate); idx >= 0 {
			return relPath[:idx+1] + candidate, true
		}
	}
	return "", false
}

// ownedAssistantSubPath narrows a gitignore pattern to the content ai-rulez
// actually writes inside an assistant directory.
//
// An assistant directory is shared territory: ai-rulez writes .claude/skills/
// and .claude/agents/, while the user hand-authors and tracks
// .claude/settings.json beside them. Ignoring the directory root makes git skip
// those tracked files with no diagnostic (issue #184), so the pattern names the
// first owned segment instead — .claude/skills/ rather than .claude/.
//
// remainder is the path relative to the assistant directory. A remainder with a
// separator identifies an owned subdirectory; a single segment is either a
// directory marker (isDir) or a file ai-rulez writes into the root, which is
// ignored by name so narrowing never stops covering generated content.
func ownedAssistantSubPath(nestedPrefix, assistantDir, remainder string, isDir bool) string {
	if remainder == "" {
		return ""
	}
	if idx := strings.Index(remainder, "/"); idx >= 0 {
		return nestedPrefix + assistantDir + remainder[:idx+1]
	}
	if isDir {
		return nestedPrefix + assistantDir + remainder + "/"
	}

	return nestedPrefix + assistantDir + remainder
}

// generatedRootFiles and generatedAssistantDirs are the static entries for the
// Go-implemented presets. Declarative provider specs add their own through
// providers.GitignoreHints, merged in by gitignoreRootFiles and
// gitignoreAssistantDirs, so a new builtin spec needs no entry here.
var generatedRootFiles = [...]string{
	"AGENTS.md",
	"CLAUDE.md",
	"GEMINI.md",
	".mcp.json",
}

var generatedAssistantDirs = [...]string{
	".agents/",
	".claude/",
	".codex/",
	".cursor/",
	".gemini/",
	".continue/",
	".cline/",
	".clinerules/",
	".devin/",
	".junie/",
	".opencode/",
	".amp/",
	".xum/",
}

var (
	gitignoreTablesOnce sync.Once
	gitignoreRoots      []string
	gitignoreDirs       []string
	mcpConfigPathsOnce  sync.Once
	mcpConfigPaths      []string
	sidecarPathsOnce    sync.Once
	sidecarPaths        []string
)

func gitignoreTables() {
	gitignoreTablesOnce.Do(func() {
		hintFiles, hintDirs := providers.GitignoreHints()
		gitignoreRoots = mergeUnique(generatedRootFiles[:], hintFiles)
		gitignoreDirs = mergeUnique(generatedAssistantDirs[:], hintDirs)
	})
}

// gitignoreRootFiles returns the static root files plus those the provider specs declare.
func gitignoreRootFiles() []string {
	gitignoreTables()
	return gitignoreRoots
}

// gitignoreAssistantDirs returns the static assistant directories plus those the
// provider specs write into.
func gitignoreAssistantDirs() []string {
	gitignoreTables()
	return gitignoreDirs
}

// specMCPConfigPaths returns the MCP config paths the provider specs declare.
func specMCPConfigPaths() []string {
	mcpConfigPathsOnce.Do(func() { mcpConfigPaths = providers.MCPConfigPaths() })
	return mcpConfigPaths
}

// specSidecarPaths returns the sidecar file paths the provider specs declare.
func specSidecarPaths() []string {
	sidecarPathsOnce.Do(func() { sidecarPaths = providers.SidecarPaths() })
	return sidecarPaths
}

// mergeUnique appends the entries of extra missing from base, keeping base order.
func mergeUnique(base, extra []string) []string {
	out := append([]string(nil), base...)
	for _, e := range extra {
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return out
}

var generatedGithubPatterns = [...]string{
	".github/copilot-instructions.md",
	".github/agents/",
	".github/commands/",
	".github/skills/",
}
