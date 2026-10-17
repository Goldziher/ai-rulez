package doctor

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/agentssource"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

// contentDirs are the directories of a content tree that an unused source leaves behind.
func contentDirs() []string {
	return []string{"rules", "context", "skills", "agents", "commands", "checks", "domains"}
}

// checkSource: the configured source is the only one in use. A project that reads
// AGENTS.md and .agents (`source = "agents"`) ignores the .ai-rulez content tree,
// and a configuration in .agents/ai-rulez.toml makes .ai-rulez/config.toml dead;
// either leftover reads as live content and is not. A generated AGENTS.md or
// .agents file from before the switch is output, never source.
func checkSource(_ context.Context, s *state) []Finding {
	cfg := s.cfg
	if cfg == nil {
		return nil
	}
	var out []Finding
	out = append(out, shadowedConfig(cfg)...)
	if !cfg.UsesAgentsSource() {
		return out
	}
	out = append(out, ignoredTree(cfg)...)
	return append(out, generatedLeftovers(cfg)...)
}

// shadowedConfig reports a config.toml that .agents/ai-rulez.toml takes precedence over.
func shadowedConfig(cfg *config.Config) []Finding {
	if !cfg.AgentsLayout {
		return shadowedRootConfig(cfg)
	}
	var out []Finding
	for _, dir := range []string{".ai-rulez", ".config/ai-rulez"} {
		rel := dir + "/config.toml"
		if isFile(filepath.Join(cfg.BaseDir, filepath.FromSlash(rel))) {
			out = append(out, Finding{
				Check: CheckSource, Severity: SeverityWarning, Path: rel,
				Message: rel + " is ignored: " + config.AgentsDirName + "/" + config.AgentsConfigFilename + " takes precedence",
				Hint:    "keep one configuration; move settings you still need into " + config.AgentsDirName + "/" + config.AgentsConfigFilename,
			})
		}
	}
	return out
}

// shadowedRootConfig reports an ai-rulez.toml that the config directory in use
// takes precedence over.
func shadowedRootConfig(cfg *config.Config) []Finding {
	if cfg.Contentless() || cfg.BaseDir == "" || !isFile(filepath.Join(cfg.BaseDir, config.RootConfigFilename)) {
		return nil
	}
	dir := cfg.ConfigDirName
	return []Finding{{
		Check: CheckSource, Severity: SeverityWarning, Path: config.RootConfigFilename,
		Message: config.RootConfigFilename + " is ignored: " + dir + "/ takes precedence",
		Hint:    "keep one configuration; move settings you still need into " + dir + "/config.toml, or remove the directory to use " + config.RootConfigFilename,
	}}
}

// ignoredTree reports content of the .ai-rulez tree that source = "agents" does not read.
func ignoredTree(cfg *config.Config) []Finding {
	var out []Finding
	for _, dir := range []string{".ai-rulez", ".config/ai-rulez"} {
		root := filepath.Join(cfg.BaseDir, filepath.FromSlash(dir))
		if filepath.Clean(root) == filepath.Clean(cfg.ConfigDir) && cfg.AgentsLayout {
			continue
		}
		count := 0
		for _, sub := range contentDirs() {
			count += countFiles(filepath.Join(root, sub))
		}
		if count == 0 {
			continue
		}
		out = append(out, Finding{
			Check: CheckSource, Severity: SeverityWarning, Path: dir,
			Message: fmt.Sprintf("%d content file(s) under %s are not read: source = %q reads AGENTS.md and %s/", count, dir, config.SourceAgents, config.AgentsDirName),
			Hint:    "move them under " + config.AgentsDirName + "/ or remove them, or set source = \"ai-rulez\"",
		})
	}
	return out
}

// generatedLeftovers reports files in AGENTS.md and .agents that carry the generated header.
func generatedLeftovers(cfg *config.Config) []Finding {
	var found []string
	check := func(path string) {
		data, err := os.ReadFile(path) //nolint:gosec // a project file read-only
		if err != nil || !generator.HasGeneratedBanner(path, data) {
			return
		}
		if rel, relErr := filepath.Rel(cfg.BaseDir, path); relErr == nil {
			found = append(found, filepath.ToSlash(rel))
		}
	}
	check(filepath.Join(cfg.BaseDir, config.AgentsMDFilename))
	for _, nested := range agentssource.NestedPaths(cfg.BaseDir) {
		check(nested)
	}
	_ = filepath.WalkDir(filepath.Join(cfg.BaseDir, config.AgentsDirName), func(path string, d fs.DirEntry, err error) error { //nolint:errcheck // best effort, read-only
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil //nolint:nilerr // an unreadable entry is skipped
		}
		if strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(cfg.ConfigDir)+"/") {
			return nil // ai-rulez's own state
		}
		check(path)
		return nil
	})
	if len(found) == 0 {
		return nil
	}
	return []Finding{{
		Check: CheckSource, Severity: SeverityWarning, Path: found[0],
		Message: fmt.Sprintf("%d generated file(s) in the source are ignored: %s", len(found), strings.Join(found, ", ")),
		Hint:    "they are output of an earlier source = \"ai-rulez\" run; `ai-rulez generate` removes them",
	}}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// countFiles counts the regular files below dir.
func countFiles(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error { //nolint:errcheck // best effort, read-only
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}
