package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/samber/oops"
	"github.com/spf13/viper"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// implicitNotice is the one line a run without a configuration prints on stderr.
const implicitNotice = "no ai-rulez config found; linting detected files\n"

// loadLintConfig loads the project for the read-only commands (validate, scan,
// doctor): its configuration when it has one, else an implicit configuration
// that lints what the project holds by convention (see config.LoadImplicit).
// An explicit -C/--config or --config-dir never falls back, and neither does a
// directory below a configured project: that project's configuration is found
// by walking up.
func loadLintConfig(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
	return lintLoader()(ctx, opts...)
}

// lintLoader is loadLintConfig for a command that loads more than once (doctor):
// the closure announces an implicit configuration a single time.
func lintLoader() func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
	var once sync.Once
	return func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
		return loadLintConfigOnce(ctx, &once, opts...)
	}
}

func loadLintConfigOnce(ctx context.Context, once *sync.Once, opts ...config.LoadOption) (*config.Config, error) {
	cfg, err := loadConfigOrNoConfig(ctx, opts...)
	if err == nil || !errors.Is(err, config.ErrNoConfig) || cfgFile != "" || configDir != "" {
		return cfg, err
	}
	cwd := workingDir()
	if _, found := config.FindConfigFile(cwd); found == nil || !errors.Is(found, config.ErrNoConfig) {
		return nil, err // a configuration above (or a legacy one): this directory is not a project without one
	}
	root := cwd
	if validateRepoRoot != "" {
		root = validateRepoRoot
	} else if top := (gitutil.Git{}).TopLevelContext(ctx, cwd); top != "" {
		root = top
	}
	if !(gitutil.Git{}).IsRepoContext(ctx, root) && broadRoot(root) {
		return nil, oops.
			Hint("Run it from inside a project directory").
			Errorf("refusing to lint everything below %s: it is not a project (no git repository, no ai-rulez config)", root)
	}
	implicit, ierr := loadProjectImplicit(ctx, root, opts...)
	if ierr != nil {
		return nil, ierr //nolint:wrapcheck // already contextual
	}
	if !viper.GetBool("quiet") {
		once.Do(func() { noteLine(ctx, os.Stderr, implicitNotice) })
	}
	return implicit, nil
}

// broadRoot reports whether dir is the file system root or the home directory:
// a walk of either is never what a lint without a configuration should do.
func broadRoot(dir string) bool {
	if dir == filepath.Dir(dir) {
		return true
	}
	home, err := os.UserHomeDir()
	return err == nil && sameDir(dir, home)
}

// withInitHint adds what to do about a missing configuration to the error of a
// command that cannot run without one (generate, clean, lock, ...).
func withInitHint(err error) error {
	if err == nil || !errors.Is(err, config.ErrNoConfig) {
		return err
	}
	return oops.
		Hint("Run `ai-rulez init` (or `ai-rulez init --source agents`) to create one; validate, scan and doctor work without a configuration").
		Wrapf(err, "no ai-rulez config found: run `ai-rulez init`")
}
