package config

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// ErrLegacyConfig reports a V2 or V3 configuration file. ai-rulez v5 reads only
// the directory config .ai-rulez/config.toml, so such a file is named and the
// caller is pointed at the 4.x migration instead of being told nothing was found.
var ErrLegacyConfig = errors.New("legacy V2/V3 config is no longer read")

// legacyMigrationCommand is the upgrade path for a V3 directory config.
const legacyMigrationCommand = MigrateCommandHint

// legacyFlatHint adds the V2 step: 4.x migrates only a directory config.
const legacyFlatHint = "A flat V2 file is not read by that command: move it to .ai-rulez/config.yaml first"

// legacyDirConfigNames are the V3 directory config names a v5 config.toml replaces.
var legacyDirConfigNames = []string{"config.yaml", "config.yml", "config.json"}

// legacyLocalNames are the V3 machine-local overlay names (config.local.toml is the only one read).
var legacyLocalNames = []string{"config.local.yaml", "config.local.yml", "config.local.json"}

// legacyFlatConfigNames are the V2 flat file names that sat beside the project.
var legacyFlatConfigNames = []string{
	"ai-rulez.yaml", "ai-rulez.yml", ".ai-rulez.yaml", ".ai-rulez.yml",
	"ai_rulez.yaml", "ai_rulez.yml", ".ai_rulez.yaml", ".ai_rulez.yml",
}

func firstExistingFile(v workspace.View, dir string, names []string) string {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if v.IsRegularFile(path) {
			return path
		}
	}
	return ""
}

// legacyConfigIn returns the V3 directory config (or overlay) inside a config
// directory, or "".
func legacyConfigIn(v workspace.View, configDir string) string {
	if path := firstExistingFile(v, configDir, legacyDirConfigNames); path != "" {
		return path
	}
	return firstExistingFile(v, configDir, legacyLocalNames)
}

// LegacyConfigIn returns the V2/V3 config file inside configDir (a config
// directory such as .ai-rulez/) when the directory has no config.toml, else "".
func LegacyConfigIn(configDir string) string {
	v := osView(configDir)
	if hasConfigFile(v, configDir) {
		return ""
	}
	return legacyConfigIn(v, configDir)
}

// FindLegacyConfig returns the V2/V3 config file that baseDir holds, or "": a
// config.yaml, config.yml or config.json in .ai-rulez/ (or .config/ai-rulez/),
// or a flat ai-rulez.yaml beside the project. A directory with a config.toml has
// none that matters, so it reports "" then.
func FindLegacyConfig(baseDir string) string {
	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return ""
	}
	return findLegacyConfig(osView(absDir), absDir)
}

// findLegacyConfig is FindLegacyConfig reading through v; baseDir is absolute.
func findLegacyConfig(v workspace.View, absDir string) string {
	for _, dirName := range configDirCandidates {
		configDir := filepath.Join(absDir, filepath.FromSlash(dirName))
		if hasConfigFile(v, configDir) {
			return ""
		}
		if path := firstExistingFile(v, configDir, legacyDirConfigNames); path != "" {
			return path
		}
	}
	return firstExistingFile(v, absDir, legacyFlatConfigNames)
}

// isLegacyConfigName reports whether base is the name of a V3 config or overlay.
func isLegacyConfigName(base string) bool {
	for _, names := range [][]string{legacyDirConfigNames, legacyLocalNames, legacyFlatConfigNames} {
		for _, n := range names {
			if base == n {
				return true
			}
		}
	}
	return false
}

// legacyConfigError is the message of a refused V2/V3 file; it matches ErrLegacyConfig.
type legacyConfigError struct{ path string }

func (e *legacyConfigError) Error() string {
	return fmt.Sprintf("found %s: YAML and JSON configs are no longer read; run `%s` to convert it to config.toml",
		e.path, legacyMigrationCommand)
}

func (e *legacyConfigError) Is(target error) bool { return target == ErrLegacyConfig }

// newLegacyConfigError names the legacy file and the way to migrate it.
func newLegacyConfigError(path string) error {
	err := oops.With("path", path)
	if isFlatName(filepath.Base(path)) {
		err = err.Hint(legacyFlatHint)
	}
	return err.Wrap(&legacyConfigError{path: path})
}

func isFlatName(base string) bool {
	for _, n := range legacyFlatConfigNames {
		if base == n {
			return true
		}
	}
	return false
}
