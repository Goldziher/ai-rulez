package config

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// The root layout keeps the whole configuration in one file, ai-rulez.toml at
// the project root, with no .ai-rulez/ directory. It is for a repository that
// hand-writes its own CLAUDE.md and AGENTS.md and only wants ai-rulez for what a
// config file declares: installed skills, skill sources, includes, MCP servers,
// hooks, permissions, lint settings and the lock. The configuration is
// "contentless": the content tree (rules, context, skills, agents, commands,
// checks, domains) is never scanned, so ConfigDir is the project root and nothing
// below it is read as authored content.
const (
	// RootConfigFilename is the configuration file of the root layout.
	RootConfigFilename = AgentsConfigFilename
	// RootLocalConfigFilename is its machine-local overlay.
	RootLocalConfigFilename = "ai-rulez.local.toml"
	// RootConfigDirName is the config directory name of the root layout, relative to
	// the project: the project itself.
	RootConfigDirName = "."

	// rootManifestName and rootLocalManifestName replace .generated-manifest.json
	// and its .local variant, which would otherwise sit loose in the project root.
	rootManifestName      = ".ai-rulez-manifest.json"
	rootLocalManifestName = ".ai-rulez-manifest.local.json"
	// rootLocalLockName is the advisory lock guarding overlay edits in the root layout.
	rootLocalLockName = ".ai-rulez-local.lock"
)

// IsRootConfigDir reports whether dir is the project root of a configuration kept
// in ai-rulez.toml: a directory that holds that file and is neither the .agents
// directory (whose ai-rulez.toml is the "agents" source) nor a config directory.
func IsRootConfigDir(dir string) bool {
	return isRootConfigDirIn(nil, dir)
}

// isRootConfigDirIn is IsRootConfigDir reading through v (the disk when v is nil).
func isRootConfigDirIn(v *workspace.View, dir string) bool {
	dir = filepath.Clean(dir)
	if IsAgentsConfigDir(dir) || filepath.Base(dir) == aiRulezDirName {
		return false
	}
	file := filepath.Join(dir, RootConfigFilename)
	if v != nil {
		return v.IsRegularFile(file)
	}
	return safefs.IsFile(file)
}

// localOverlayName is the file name of the machine-local overlay of the config
// directory configDir.
func localOverlayName(v workspace.View, configDir string) string {
	if isRootConfigDirIn(&v, configDir) {
		return RootLocalConfigFilename
	}
	return localVariantName(configTOMLFilename)
}

// isRootLayoutMain reports whether mainPath is the ai-rulez.toml of a root
// layout whose config directory is configDir.
func isRootLayoutMain(mainPath, configDir string) bool {
	return filepath.Base(mainPath) == RootConfigFilename &&
		filepath.Clean(filepath.Dir(mainPath)) == filepath.Clean(configDir) &&
		!IsAgentsConfigDir(configDir) && filepath.Base(filepath.Clean(configDir)) != aiRulezDirName
}

// Contentless reports whether the configuration has no content tree: it was read
// from ai-rulez.toml at the project root, so no rules, context, skills, agents,
// commands, checks, domains or local content exist, and the generator writes no
// instruction or root file (see SkipsRootFiles).
func (c *Config) Contentless() bool { return c != nil && c.contentless }

// SkipsRootFiles reports whether the generator must leave the instruction and
// root files (AGENTS.md, CLAUDE.md, rule files, ...) alone because the project
// keeps them by hand. It is the one place that decides this, so that it can be
// unified with a general skip_outputs setting later.
func (c *Config) SkipsRootFiles() bool { return c.Contentless() }

// ManifestName is the file name of the generated manifest, which lists every path
// a run wrote. It lives in ConfigDir.
func (c *Config) ManifestName() string {
	if c.Contentless() {
		return rootManifestName
	}
	return ".generated-manifest.json"
}

// LocalManifestName is the file name of the gitignored manifest of machine-local
// outputs.
func (c *Config) LocalManifestName() string {
	if c.Contentless() {
		return rootLocalManifestName
	}
	return ".generated-manifest.local.json"
}

// LocalDir is the directory for machine-local state (usage logs, the role
// ledger): <ConfigDir>/local, or in the root layout a hidden directory matched by
// the managed ignore patterns, so it cannot be mistaken for a folder of the project.
func (c *Config) LocalDir() string {
	if c.Contentless() {
		return filepath.Join(c.ConfigDir, ".ai-rulez-local.d")
	}
	return filepath.Join(c.ConfigDir, "local")
}

// RootManifestNames are the manifest file names of the root layout.
func RootManifestNames() []string { return []string{rootManifestName, rootLocalManifestName} }

// rootOverlayGitignorePatterns keep the root layout's overlay, its lock and its
// temp files out of version control.
func rootOverlayGitignorePatterns() []string {
	return []string{"ai-rulez.local.*", ".ai-rulez-local.*"}
}

// loadRootLayout loads the project at baseDir whose configuration is the
// ai-rulez.toml beside it. ConfigDir is baseDir and the content scan is skipped.
func loadRootLayout(ctx context.Context, v workspace.View, baseDir string, lo loadOptions) (*Config, error) {
	mainPath := filepath.Join(baseDir, RootConfigFilename)
	if !v.IsRegularFile(mainPath) {
		return nil, noConfig{oops.
			With("path", mainPath).
			Hint(fmt.Sprintf("Create %s\nRun 'ai-rulez init --minimal' to create it", RootConfigFilename)).
			Errorf("no config file found (tried %s)", RootConfigFilename)}
	}
	cfg, err := loadConfigTOML(v, mainPath)
	if err != nil {
		return nil, err
	}
	cfg.ConfigFile = RootConfigFilename
	if len(cfg.DefaultLayers) > 0 {
		return nil, oops.
			With("path", mainPath).
			Hint("Layers are config.<name>.toml files in a .ai-rulez/ directory; drop default_layers or use the directory layout.").
			Errorf("default_layers is not supported in %s", RootConfigFilename)
	}
	cfg, err = withLocalOverlay(v, cfg, mainPath, baseDir, lo) //nolint:contextcheck // the overlay tracking probe is a bounded local git call with its own timeout
	if err != nil {
		return nil, err
	}
	cfg.ConfigFilePath = mainPath
	cfg.contentless = true
	return finishLoadConfig(ctx, v, cfg, baseDir, baseDir, lo)
}

// warnShadowedRootConfig tells the user that an ai-rulez.toml beside a config
// directory is ignored: the directory wins.
func warnShadowedRootConfig(cfg *Config, v workspace.View, baseDir string) {
	if cfg == nil || cfg.Contentless() || !v.IsRegularFile(filepath.Join(baseDir, RootConfigFilename)) {
		return
	}
	cfg.Warn(RootConfigFilename+" is ignored: the "+cfg.ConfigDirName+"/ directory takes precedence",
		"hint", "keep one configuration; move settings you still need into "+cfg.ConfigDirName+"/config.toml")
}
