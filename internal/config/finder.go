package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

func FindConfigFile(startDir string) (string, error) {
	return FindConfigFileInDirName(startDir, aiRulezDirName)
}

func FindConfigFileInDirName(startDir, configDirName string) (string, error) {
	if configDirName == "" {
		configDirName = aiRulezDirName
	}
	// When discovering the default layout, also accept the project-level
	// .config/ai-rulez/ convention as a fallback to .ai-rulez/. An explicitly
	// supplied --config-dir is honored exactly and never expanded.
	dirNames := []string{configDirName}
	if configDirName == aiRulezDirName {
		dirNames = append(dirNames, altConfigDirName)
	}
	configNames := configNamesForDirNames(dirNames)

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", oops.
			With("path", startDir).
			With("operation", "resolve absolute path").
			Hint("Check if the directory exists and is accessible").
			Wrapf(err, "resolve absolute path")
	}

	visited := make(map[string]bool)

	for !visited[dir] {
		visited[dir] = true

		for _, name := range configNames {
			configPath := filepath.Join(dir, filepath.FromSlash(name))
			if _, err := os.Stat(configPath); err == nil {
				return configPath, nil
			}
		}
		// A V2/V3 file with no config.toml beside it: say so, never walk past it.
		if legacy := FindLegacyConfig(dir); legacy != "" {
			return "", newLegacyConfigError(legacy)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", oops.
		With("search_dir", startDir).
		With("supported_names", configNames).
		Hint(fmt.Sprintf("Run 'ai-rulez init' to create a new configuration file\nCreate %s/config.toml (or %s/config.toml)\nCheck if you're in the correct directory\nUse --config flag to specify the config file path explicitly", aiRulezDirName, altConfigDirName)).
		Errorf("no configuration file found")
}

// IsConfigDirAncestor reports whether path is a strict ancestor directory on
// the way to a possibly nested config directory target — for example ".config"
// (or "svc/.config") for target ".config/ai-rulez". Recursive walks use it to
// descend into a wrapper directory that would otherwise be pruned as hidden.
func IsConfigDirAncestor(path, configDirName string) bool {
	parts := strings.Split(filepath.ToSlash(configDirName), "/")
	if len(parts) < 2 {
		return false
	}
	p := filepath.ToSlash(path)
	for i := 1; i < len(parts); i++ {
		ancestor := strings.Join(parts[:i], "/")
		if p == ancestor || strings.HasSuffix(p, "/"+ancestor) {
			return true
		}
	}
	return false
}

// configNamesForDirNames returns, in priority order, the config file paths to
// probe: config.toml in each config directory (tool-specific first, then the
// .config/ convention). Returned paths are slash-separated.
func configNamesForDirNames(dirNames []string) []string {
	names := make([]string, 0, len(dirNames))
	for _, dirName := range dirNames {
		names = append(names, dirName+"/"+configTOMLFilename)
	}
	return names
}

// localVariantName inserts ".local" before the extension of a base filename
// (e.g. "CLAUDE.md" → "CLAUDE.local.md", "config.toml" → "config.local.toml").
func localVariantName(base string) string {
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext) + ".local" + ext
}

// LocalVariantPath returns the sibling path with ".local" inserted before the
// extension, but only for markdown files. It returns "" when p is not a ".md"
// file — used by preset generators to derive the machine-local root filename
// (CLAUDE.md → CLAUDE.local.md) while skipping presets whose root is not a
// single markdown file.
func LocalVariantPath(p string) string {
	if filepath.Ext(p) != markdownExt {
		return ""
	}
	dir := filepath.Dir(p)
	local := localVariantName(filepath.Base(p))
	if dir == "" || dir == "." {
		return local
	}
	return filepath.Join(dir, local)
}
