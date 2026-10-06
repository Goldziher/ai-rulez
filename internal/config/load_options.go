package config

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// loadOptions are the resolved LoadOption settings.
type loadOptions struct {
	withoutLocal  bool
	includeMemo   any
	withoutRemote bool
	runner        runner.Runner
	host          ambient.Host
	ws            workspace.Workspace
}

// LoadOption customizes how a configuration is loaded.
type LoadOption func(*loadOptions)

// WithoutLocal skips machine-local inputs: the config.local.* overlay and the
// .ai-rulez/local/ content tree. Use it for anything that reads the shared
// configuration in order to modify and save it, and to reproduce the view a
// teammate without local overrides sees.
func WithoutLocal() LoadOption {
	return func(o *loadOptions) { o.withoutLocal = true }
}

// WithoutRemote skips resolving includes and installed skills: no remote is
// fetched and no cache is read. Use it for commands that only inspect the
// declared configuration, such as verifying ai-rulez.lock against it.
func WithoutRemote() LoadOption {
	return func(o *loadOptions) { o.withoutRemote = true }
}

// WithIncludeMemo makes the loaded config share an include fetch cache created
// by an earlier load (Config.IncludeMemo), so sources are fetched once per run.
func WithIncludeMemo(memo any) LoadOption {
	return func(o *loadOptions) { o.includeMemo = memo }
}

// WithRunner makes every external command the load starts (git, for the
// .gitignore-aware skill bundling and for include fetches) go through r instead
// of running a real process. A nil r keeps the default (runner.Exec).
func WithRunner(r runner.Runner) LoadOption {
	return func(o *loadOptions) { o.runner = r }
}

// WithHost loads the configuration with the given environment, clock, process
// runner and logger instead of the real process's. The host is kept on the
// loaded Config (Config.Host), so generation and includes use it too. Fields left
// nil keep the real facility; a runner given by WithRunner is overridden by a
// non-nil host.Runner.
func WithHost(h ambient.Host) LoadOption {
	return func(o *loadOptions) {
		o.host = h
		if h.Runner != nil {
			o.runner = h.Runner
		}
	}
}

// WithWorkspace loads the project from ws instead of from the real directory
// baseDir names: every config, content and include read goes through it, so the
// project can live in memory or in a repository snapshot. baseDir may then be
// relative to ws's root, and the process working directory is never consulted.
func WithWorkspace(ws workspace.Workspace) LoadOption {
	return func(o *loadOptions) { o.ws = ws }
}

// baseView returns the view the project at baseDir is read through and baseDir as
// an absolute path. Without WithWorkspace it is the real file system, rooted at
// the repository that contains baseDir (see workspace.Around); a relative baseDir
// then meets the process working directory here and nowhere else.
func (lo loadOptions) baseView(baseDir string) (workspace.View, string, error) {
	if lo.ws != nil {
		abs := filepath.Clean(baseDir)
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(lo.ws.Root(), abs)
		}
		return workspace.NewView(lo.ws), abs, nil
	}
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return workspace.View{}, "", oops.
			With("path", baseDir).
			Hint("Check if the directory path is valid and accessible").
			Wrapf(err, "resolve absolute path")
	}
	ws, err := workspace.Around(abs)
	if err != nil {
		return workspace.View{}, "", err //nolint:wrapcheck // already contextual
	}
	return workspace.NewView(ws), abs, nil
}

func applyLoadOptions(opts []LoadOption) loadOptions {
	var lo loadOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&lo)
		}
	}
	return lo
}

// fileView is baseView for a path that names a config file or directory: the
// workspace has to contain both the config directory and the project directory
// that owns it, which is found with one bootstrap read when no workspace is given.
func (lo loadOptions) fileView(absPath string) (workspace.View, string, error) {
	if lo.ws != nil {
		return workspace.NewView(lo.ws), absPath, nil
	}
	ws, err := workspace.Around(bootstrapBase(absPath))
	if err != nil {
		return workspace.View{}, "", err //nolint:wrapcheck // already contextual
	}
	return workspace.NewView(ws), absPath, nil
}

// bootstrapBase guesses the project directory of a config path before any
// workspace exists to read through: a config file or a directory holding a
// config.toml belongs to the directory that contains the config directory.
func bootstrapBase(absPath string) string {
	info, err := os.Stat(absPath)
	if err != nil {
		return filepath.Dir(absPath)
	}
	if !info.IsDir() {
		return projectBaseDir(filepath.Dir(absPath))
	}
	if st, err := os.Stat(filepath.Join(absPath, configTOMLFilename)); err == nil && !st.IsDir() {
		return projectBaseDir(absPath)
	}
	return absPath
}
