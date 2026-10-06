package config

import (
	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// loadOptions are the resolved LoadOption settings.
type loadOptions struct {
	withoutLocal  bool
	includeMemo   any
	withoutRemote bool
	runner        runner.Runner
	host          ambient.Host
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

func applyLoadOptions(opts []LoadOption) loadOptions {
	var lo loadOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&lo)
		}
	}
	return lo
}
