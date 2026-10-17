package commands

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/Goldziher/ai-rulez/v5/internal/agentssource"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/watch"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var generateWatch bool

// checkGenerateWatchFlags rejects flags that make no sense with --watch: it is a
// long-running, single-root write loop.
func checkGenerateWatchFlags() error {
	switch {
	case dryRun:
		return oops.Errorf("--watch and --dry-run are mutually exclusive: watch mode writes files on every change")
	case generateCheck:
		return oops.Errorf("--watch and --check are mutually exclusive: --check is a one-shot CI verification")
	case userScope:
		return oops.Errorf("--watch cannot be combined with --user; it watches the project's configuration directory")
	case pluginMode:
		return oops.Errorf("--watch cannot be combined with --plugin")
	case recursive:
		return oops.Errorf("--watch cannot be combined with --recursive; it watches a single configuration")
	}
	return nil
}

// runGenerateWatch generates once, then again whenever the configuration or its
// sources change, until interrupted.
func runGenerateWatch(parent context.Context) error {
	if err := checkGenerateWatchFlags(); err != nil {
		return err
	}
	ctx, stop := interruptContext(parent)
	defer stop()

	// Every cycle reloads the project: one collector for the whole watch says a
	// warning about the project once, not on every change.
	collector := diag.New(nil)
	var last *config.Config // the most recent successfully loaded configuration
	outputs := newGeneratedOutputFilter(initialConfigDir())
	run := func(ctx context.Context, triggers []string) error {
		if changed := changedPaths(triggers); len(changed) > 0 {
			logger.Info("Change detected, regenerating", "changed", describeTriggers(changed))
		}
		cfg, err := generateOnce(ctx, config.WithCollector(collector))
		if cfg != nil {
			last = cfg
			outputs.refresh(cfg)
		}
		if err == nil {
			logger.Success("Generated; watching for changes (Ctrl-C to stop)")
		}
		return err
	}
	onError := func(err error, _ []string) {
		// A bad config is the normal state while editing; keep watching.
		renderError(os.Stderr, err)
		logger.Warn("Generation failed; still watching for changes")
	}
	return watch.Watch(ctx, watch.Options{
		Targets: func() []watch.Target { return watchTargets(last) },
		Ignore:  func(path string) bool { return watchIgnore(path) || outputs.ignore(path) },
		Run:     run,
		OnError: onError,
		Log:     func(msg string, kv ...any) { logger.Debug(msg, kv...) },
		Warn:    func(msg string, kv ...any) { logger.Warn(msg, kv...) },
	})
}

// interruptContext is a context canceled by the first SIGINT or SIGTERM. The
// signal handling is released right after, so a second Ctrl-C takes the default
// action and kills the process even while a run is stuck.
func interruptContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx, stop
}

// changedPaths drops the "initial" pseudo-trigger, leaving the paths that
// changed. It does not rely on where "initial" sorts among them.
func changedPaths(triggers []string) []string {
	var out []string
	for _, t := range triggers {
		if t != "initial" {
			out = append(out, t)
		}
	}
	return out
}

// generatedOutputFilter recognizes the files the previous run recorded as
// generated. An include source can sit on a tree that also holds outputs; their
// rewrite must not count as a change, or every run would trigger the next.
type generatedOutputFilter struct {
	paths     atomic.Pointer[map[string]bool]
	configDir atomic.Pointer[string]
}

// newGeneratedOutputFilter starts with the configuration directory the watch will
// use before any run has loaded it, so the files the first run creates inside it
// (roles.json) are already known to be outputs.
func newGeneratedOutputFilter(configDir string) *generatedOutputFilter {
	f := &generatedOutputFilter{}
	f.setConfigDir(configDir)
	return f
}

func (f *generatedOutputFilter) setConfigDir(dir string) {
	if dir == "" {
		return
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	f.configDir.Store(&dir)
}

// refresh reads the manifests the finished run left behind.
func (f *generatedOutputFilter) refresh(cfg *config.Config) {
	f.setConfigDir(cfg.ConfigDir)
	set := map[string]bool{}
	for _, p := range generator.NewGenerator(cfg).GeneratedPaths() {
		set[filepath.Clean(p)] = true
	}
	f.paths.Store(&set)
}

func (f *generatedOutputFilter) ignore(path string) bool {
	if dir := f.configDir.Load(); dir != nil && slices.Contains(generator.ConfigDirOutputNames(), filepath.Base(path)) {
		if abs, err := filepath.Abs(filepath.Dir(path)); err == nil && abs == *dir {
			return true
		}
	}
	set := f.paths.Load()
	return set != nil && (*set)[filepath.Clean(path)]
}

// initialConfigDir is the directory a watch started with these arguments will
// watch, before any configuration is loaded: the directory of the targets that
// are not a single file.
func initialConfigDir() string {
	for _, t := range fallbackTargets() {
		if !t.File {
			return t.Path
		}
	}
	return ""
}

// generateOnce is the single-root path of `generate`, returning errors rather
// than exiting. The configuration is returned (when it loaded) so the caller
// can derive what to watch.
func generateOnce(ctx context.Context, loadOpts ...config.LoadOption) (*config.Config, error) {
	cfg, err := loadConfigForCommand(ctx, loadOpts...)
	if err != nil {
		return nil, err
	}
	warnLegacyLayout(cfg.ConfigDir)
	if err := cfg.Validate(); err != nil {
		return cfg, err //nolint:wrapcheck // already contextual
	}
	if err := applyGenerateOverrides(ctx, cfg); err != nil {
		return cfg, err
	}
	if err := importGate(ctx, cfg); err != nil {
		return cfg, err
	}
	gen := generator.NewGenerator(cfg)
	gen.SetAllowLocalDrift(allowLocalDrift)
	gen.SetOverwriteUnowned(generateForce)
	gen.SetContext(ctx)
	if err := applyRole(gen); err != nil {
		return cfg, err
	}
	// Every regeneration runs the same preflight as a one-shot generate: a pulled
	// config change that adds a hook or MCP command is announced, an unchanged one
	// stays silent, and a role's skillOverrides are reconciled before the write.
	if err := generatePreflight(cfg, gen); err != nil {
		return cfg, err
	}
	return cfg, gen.Generate(profile) //nolint:wrapcheck // already contextual
}

// describeTriggers shortens a trigger list for the log line.
func describeTriggers(triggers []string) string {
	const maxShown = 3
	shown := append([]string(nil), triggers...)
	if len(shown) > maxShown {
		shown = shown[:maxShown]
	}
	for i, t := range shown {
		if rel, err := filepath.Rel(".", t); err == nil && !safefs.RelEscapes(rel) {
			shown[i] = rel
		}
	}
	out := strings.Join(shown, ", ")
	if extra := len(triggers) - len(shown); extra > 0 {
		out += " (+" + strconv.Itoa(extra) + " more)"
	}
	return out
}

// watchIgnore drops files the generator itself writes inside the configuration
// directory, which would otherwise retrigger every run.
func watchIgnore(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, ".generated-manifest") || strings.HasPrefix(base, ".ai-rulez-manifest") || base == ".gitignore"
}

// watchTargets lists what to watch: the configuration directory and the local-path include sources. With no
// loaded configuration it falls back to discovery so a config that is broken at
// start still gets watched.
func watchTargets(cfg *config.Config) []watch.Target {
	var targets []watch.Target
	if cfg != nil && cfg.Contentless() {
		// The project root is the config directory: watch the config files, not the tree.
		targets = append(targets, rootConfigTargets(cfg.BaseDir)...)
		return append(targets, includeTargets(cfg)...)
	}
	if cfg != nil && cfg.ConfigDir != "" {
		if cfg.UsesAgentsSource() {
			// The content is AGENTS.md and .agents; a config in .agents/ai-rulez.toml
			// (and its state directory) is inside the watched .agents already.
			targets = append(targets, watch.Target{Path: filepath.Join(cfg.BaseDir, config.AgentsDirName)},
				watch.Target{Path: filepath.Join(cfg.BaseDir, config.AgentsMDFilename), File: true})
			// Nested AGENTS.md files are context too. The walk is the loader's, so a file added
			// later is picked up when the targets are listed again after the next run.
			for _, nested := range agentssource.NestedPaths(cfg.BaseDir) {
				targets = append(targets, watch.Target{Path: nested, File: true})
			}
		}
		if !cfg.AgentsLayout {
			targets = append(targets, watch.Target{Path: cfg.ConfigDir})
		}
		targets = append(targets, includeTargets(cfg)...)
		return targets
	}
	return fallbackTargets()
}

// fallbackTargets resolves the config location without loading it.
func fallbackTargets() []watch.Target {
	if cfgFile != "" {
		return []watch.Target{{Path: cfgFile, File: true}}
	}
	name := configDir
	if name == "" {
		name = defaultConfigDirName
	}
	if found, err := config.FindConfigFileInDirName(".", configDir); err == nil {
		if filepath.Base(found) == config.RootConfigFilename && config.IsRootConfigDir(filepath.Dir(found)) {
			return rootConfigTargets(filepath.Dir(found))
		}
		return []watch.Target{{Path: filepath.Dir(found)}}
	}
	return []watch.Target{{Path: name}}
}

// rootConfigTargets are the files of the single-file layout: ai-rulez.toml and,
// when it exists, its machine-local overlay.
func rootConfigTargets(baseDir string) []watch.Target {
	targets := []watch.Target{{Path: filepath.Join(baseDir, config.RootConfigFilename), File: true}}
	local := filepath.Join(baseDir, config.RootLocalConfigFilename)
	if info, err := os.Stat(local); err == nil && !info.IsDir() {
		targets = append(targets, watch.Target{Path: local, File: true})
	}
	return targets
}

// includeTargets are the local-path sources the configuration reads from: includes, installed skills,
// skill sources, installed marketplaces and plugins (their source and local override).
func includeTargets(cfg *config.Config) []watch.Target {
	var out []watch.Target
	// existing skips a path that is not on disk: an installed source such as owner/repo reads as a
	// relative path, and watching a directory that does not exist only adds noise.
	add := func(p string, existing bool) {
		p = strings.TrimPrefix(p, "file://")
		if p == "" || !includes.IsLocalPath(p) {
			return
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(cfg.BaseDir, p)
		}
		// A marketplace index or a single SKILL.md can be a file; watch it as one.
		info, err := os.Stat(p)
		if existing && err != nil {
			return
		}
		out = append(out, watch.Target{Path: p, File: err == nil && !info.IsDir()})
	}
	for i := range cfg.Includes {
		add(cfg.Includes[i].Source, false)
		add(cfg.Includes[i].LocalOverride, false)
	}
	for i := range cfg.InstalledSkills {
		add(cfg.InstalledSkills[i].Source, true)
		add(cfg.InstalledSkills[i].LocalOverride, false)
	}
	for i := range cfg.SkillSources {
		add(cfg.SkillSources[i].URL, true)
	}
	for i := range cfg.InstalledMarketplaces {
		add(cfg.InstalledMarketplaces[i].Source, true)
	}
	for i := range cfg.InstalledPlugins {
		add(cfg.InstalledPlugins[i].Source, true)
	}
	return out
}

func watchParentContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return cmdContext()
}
