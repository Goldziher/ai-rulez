package config

import (
	"os"
	"path/filepath"
	"strings"
)

// okfRootIndex is the file whose presence marks a configuration directory as an
// OKF bundle (see "ai-rulez migrate okf").
const okfRootIndex = "index.md"

// LegacyLayoutMessage is the deprecation notice for a configuration directory
// that is not an OKF bundle yet.
const LegacyLayoutMessage = "This configuration directory uses the pre-OKF layout, which is deprecated; run 'ai-rulez migrate okf' to convert it to an OKF bundle"

// IsLegacyLayout reports whether configDir still uses the pre-OKF layout: it
// holds markdown content and has no root index.md. A directory without content
// has nothing to migrate.
func IsLegacyLayout(configDir string) bool {
	if info, err := os.Stat(filepath.Join(configDir, okfRootIndex)); err == nil && !info.IsDir() {
		return false
	}
	for _, dir := range []string{rulesDir, contextDir, skillsDir, agentsDir, commandsDir, checksDir, domainsDir} {
		if holdsContent(filepath.Join(configDir, dir)) {
			return true
		}
	}
	return false
}

// holdsContent reports whether dir contains a markdown file at any depth.
func holdsContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			if holdsContent(filepath.Join(dir, e.Name())) {
				return true
			}
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			return true
		}
	}
	return false
}
