package config

import (
	"fmt"
	"path"
	"strings"
)

// DefaultCodexSkillsDir is where Codex discovers repository skills
// (https://learn.chatgpt.com/docs/build-skills). Releases before 4.24 wrote
// ".codex/skills", which Codex does not read; codex_skills_dir = ".codex/skills"
// restores it.
const DefaultCodexSkillsDir = ".agents/skills"

// CodexSkillsDirOrDefault is the slash-separated skills directory of the codex
// preset, relative to the output base dir.
func (c *Config) CodexSkillsDirOrDefault() string {
	if c == nil || strings.TrimSpace(c.CodexSkillsDir) == "" {
		return DefaultCodexSkillsDir
	}
	return path.Clean(strings.ReplaceAll(strings.TrimSpace(c.CodexSkillsDir), "\\", "/"))
}

// ValidateOutputSubdir rejects an output directory setting that is absolute,
// leaves the project root, or points into a control directory (see
// ValidateOutputPath).
func ValidateOutputSubdir(key, dir string) error {
	return ValidateOutputPath(key, dir)
}

// ValidateOutputPath rejects a config-supplied output path (file or directory)
// that could write outside the project or over VCS or ai-rulez control data. A
// hostile repository controls these settings, so the check is an allowlist of
// shape: a non-empty relative slash path, no NUL, no ".." segment, no drive or
// UNC prefix, no segment naming a git directory (".git", case-insensitively,
// including the Windows spellings ".git." and "git~1"), and a first segment that
// is not .ai-rulez, .config/ai-rulez, .hg or .svn.
func ValidateOutputPath(key, dir string) error {
	bad := func() error {
		return fmt.Errorf("%s %q must be a relative path inside the project", key, dir)
	}
	norm := strings.ReplaceAll(dir, "\\", "/")
	if norm == "" || norm == "." || strings.ContainsRune(norm, 0) || strings.HasPrefix(norm, "/") ||
		path.IsAbs(norm) || (len(norm) > 1 && norm[1] == ':') {
		return bad()
	}
	segments := strings.Split(norm, "/")
	for _, seg := range segments {
		if seg == ".." {
			return bad()
		}
	}
	clean := path.Clean(norm)
	if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return bad()
	}
	cleanSegments := strings.Split(clean, "/")
	for _, seg := range cleanSegments {
		if IsGitDirName(seg) {
			return fmt.Errorf("%s %q must not be inside %s", key, dir, gitDirName)
		}
	}
	first := strings.ToLower(cleanSegments[0])
	for _, protected := range []string{aiRulezDirName, altConfigDirName, ".hg", ".svn"} {
		if clean == protected || strings.HasPrefix(strings.ToLower(clean), protected+"/") || first == protected {
			return fmt.Errorf("%s %q must not be inside %s", key, dir, protected)
		}
	}
	return nil
}

// IsGitDirName reports whether one path segment names a git directory under any
// spelling a filesystem folds onto ".git".
func IsGitDirName(seg string) bool {
	s := strings.ToLower(strings.TrimRight(seg, ". "))
	return s == gitDirName || s == "git~1"
}
