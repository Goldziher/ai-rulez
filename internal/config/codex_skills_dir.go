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

// ValidateOutputSubdir rejects an output directory setting that is absolute or
// leaves the project root.
func ValidateOutputSubdir(key, dir string) error {
	if dir == "" || dir == "." || path.IsAbs(dir) || strings.HasPrefix(dir, "/") || dir == ".." ||
		strings.HasPrefix(dir, "../") || (len(dir) > 1 && dir[1] == ':') {
		return fmt.Errorf("%s %q must be a relative directory inside the project", key, dir)
	}
	clean := path.Clean(dir)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s %q must be a relative directory inside the project", key, dir)
	}
	for _, protected := range []string{aiRulezDirName, altConfigDirName, ".git"} {
		if clean == protected || strings.HasPrefix(clean, protected+"/") {
			return fmt.Errorf("%s %q must not be inside %s", key, dir, protected)
		}
	}
	return nil
}
