package config

import (
	"context"
	"os"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
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
	resolvers     Resolvers
	registry      *Registry
	policy        PolicyEnforcer
	policyDir     string
	collector     *diag.Collector
	lockPolicy    LockPolicy
	// frontmatterErrors: the caller fails on malformed frontmatter itself, so the
	// load does not warn about the same files first.
	frontmatterErrors bool
}

// ReloadOptions returns the options that load this project again the way c was
// loaded: through the same workspace, host (environment, clock, runner and
// logger), registry, resolvers, include fetch memo, organization policy and its
// directory, warning collector and lock policy. extra come last and win. A
// nested load of the same project (the shared view without the machine-local
// overlay, a baseline render) uses it, so it cannot fall back to the real disk,
// the process environment or the real git while the first load did not.
func (c *Config) ReloadOptions(extra ...LoadOption) []LoadOption {
	opts := []LoadOption{
		WithHost(c.Host),
		WithRegistry(c.Registry),
		WithResolvers(c.Resolve),
		WithIncludeMemo(c.IncludeMemo),
		WithPolicy(c.Policy()),
		WithPolicyDir(c.PolicyDir),
		WithLockPolicy(c.LockPolicy),
	}
	if c.Workspace != nil {
		opts = append(opts, WithWorkspace(c.Workspace))
	}
	if c.Diag != nil {
		opts = append(opts, WithCollector(c.Diag))
	}
	if c.frontmatterErrors {
		opts = append(opts, WithFrontmatterErrors())
	}
	return append(opts, extra...)
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

// WithFrontmatterErrors tells the load that the caller reports malformed
// frontmatter as an error (Config.Validate names the files), so the load does not
// print a warning for each file just before. Commands that do not validate keep
// the warning.
func WithFrontmatterErrors() LoadOption {
	return func(o *loadOptions) { o.frontmatterErrors = true }
}

// WithoutRemote skips resolving includes and installed skills: no remote is
// fetched and no cache is read. Use it for commands that only inspect the
// declared configuration, such as verifying ai-rulez.lock against it.
func WithoutRemote() LoadOption {
	return func(o *loadOptions) { o.withoutRemote = true }
}

// WithPolicyDir names the project directory the organization policy is
// discovered from, when the configuration is loaded from somewhere else, such as
// a snapshot of an earlier revision extracted to a temporary directory.
func WithPolicyDir(dir string) LoadOption {
	return func(o *loadOptions) { o.policyDir = dir }
}

// WithCollector makes the load keep its warning state in c instead of a fresh
// collector, and say each warning once for the life of c. A caller that loads the
// same project over and over (a watch loop, a live reload) passes one collector
// that outlives the loads, so a warning about the project is not repeated on every
// cycle. Loads that must not share state keep their own collector (the default).
func WithCollector(c *diag.Collector) LoadOption {
	return func(o *loadOptions) { o.collector = c }
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

// WithResolvers supplies the functions that resolve includes and installed skills
// (see Resolvers). A load without them fails when the configuration declares
// includes or installed skills, unless WithoutRemote is given.
func WithResolvers(r Resolvers) LoadOption {
	return func(o *loadOptions) { o.resolvers = r }
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
func (lo loadOptions) baseView(ctx context.Context, baseDir string) (workspace.View, string, error) {
	if lo.ws != nil {
		abs := filepath.Clean(baseDir)
		if !rooted(abs) {
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
	ws, err := workspace.AroundBelow(orBackground(ctx), gitutil.New(loadHost(lo).Runner), abs, lo.host.GetEnv("GIT_CEILING_DIRECTORIES"))
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
	if lo.collector != nil {
		lo.host.Log = diag.OnceLogger(lo.collector, lo.host.Log)
	}
	return lo
}

// fileView is baseView for a path that names a config file or directory: the
// workspace has to contain both the config directory and the project directory
// that owns it, which is found with one bootstrap read when no workspace is given.
func (lo loadOptions) fileView(ctx context.Context, absPath string) (workspace.View, string, error) {
	if lo.ws != nil {
		return workspace.NewView(lo.ws), absPath, nil
	}
	ws, err := workspace.AroundBelow(orBackground(ctx), gitutil.New(loadHost(lo).Runner), bootstrapBase(absPath), lo.host.GetEnv("GIT_CEILING_DIRECTORIES"))
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
		return ProjectBaseDir(filepath.Dir(absPath))
	}
	if st, err := os.Stat(filepath.Join(absPath, configTOMLFilename)); err == nil && !st.IsDir() {
		return ProjectBaseDir(absPath)
	}
	return absPath
}

// orBackground is ctx, or context.Background() for a caller that passed nil
// (the git questions a load asks need a context to run under).
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
