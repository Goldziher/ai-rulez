package config

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// okfRootIndex is the file whose presence marks a configuration directory as an
// OKF bundle (see "ai-rulez migrate okf").
const okfRootIndex = "index.md"

// warnLegacyLayout says, at info level, that the configuration directory still
// uses the pre-OKF layout. The loader reads that layout during the deprecation
// window; the reader is removed in v6. A directory without content has nothing
// to migrate and is not reported.
func warnLegacyLayout(log logger.Logger, v workspace.View, configDir string, tree *ContentTree) {
	if tree == nil || !hasOwnContent(tree) {
		return
	}
	if info, err := v.Stat(filepath.Join(configDir, okfRootIndex)); err == nil && !info.IsDir() {
		return
	}
	logger.Or(log).Info("This configuration directory uses the legacy layout, which is deprecated and stops loading in v6",
		"path", configDir, "hint", "run 'ai-rulez migrate okf' to convert it to an OKF bundle")
}

func hasOwnContent(tree *ContentTree) bool {
	if len(tree.Rules)+len(tree.Context)+len(tree.Skills)+len(tree.Agents)+len(tree.Commands)+len(tree.Checks) > 0 {
		return true
	}
	for _, d := range tree.Domains {
		if d != nil && !d.Builtin && !d.FromInclude {
			return true
		}
	}
	return false
}
