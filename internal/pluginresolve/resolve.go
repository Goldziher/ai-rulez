// Package pluginresolve resolves a configuration's [[installed_marketplaces]]
// and [[installed_plugins]]: it fetches each marketplace and plugin through the
// includes machinery (cache, lock pins, version constraints), reads the plugin
// layouts with internal/pluginimport and hands the content to the config loader.
//
// It sits above internal/includes because the plugin readers use
// internal/importer, which itself imports includes.
package pluginresolve

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// Resolvers is includes.Resolvers plus the marketplace and plugin resolver.
func Resolvers(gitToken string, scan okfbridge.Scanner) config.Resolvers {
	r := includes.Resolvers(gitToken, scan)
	r.Plugins = func(ctx context.Context, cfg *config.Config) (*config.PluginResolution, error) {
		return ResolveInstalledPlugins(ctx, cfg, gitToken)
	}
	return r
}

// ResolveInstalledPlugins resolves every marketplace and plugin of cfg.
func ResolveInstalledPlugins(ctx context.Context, cfg *config.Config, accessToken string) (*config.PluginResolution, error) {
	ctx = includes.PolicyContext(ctx, cfg)
	lock, err := includes.LoadLock(cfg)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	r := &resolver{cfg: cfg, lock: lock, token: accessToken, markets: map[string]*market{}, log: cfg.Log()}
	out := &config.PluginResolution{}
	var violations, failures []error
	classify := func(kind, name string, err error) {
		switch {
		case errors.Is(err, config.ErrLockViolation) || includes.StrictLock(cfg):
			violations = append(violations, oops.Wrapf(errors.Join(config.ErrLockViolation, err), "%s %q", kind, name))
		case !includes.Tolerated(ctx) && cfg.LockPolicy.Mode != includes.LockRefresh:
			failures = append(failures, oops.Wrapf(errors.Join(config.ErrPluginUnresolved, err), "%s %q", kind, name))
		default:
			cfg.Warn("Failed to resolve "+kind, "name", name, "error", err)
		}
	}
	for i := range cfg.InstalledMarketplaces {
		m := &cfg.InstalledMarketplaces[i]
		if _, err := r.marketplace(ctx, m.Name); err != nil {
			classify("installed marketplace", m.Name, err)
		}
	}
	for i := range cfg.InstalledPlugins {
		p := &cfg.InstalledPlugins[i]
		res, err := r.plugin(ctx, p)
		if err != nil {
			classify("installed plugin", p.Name, err)
			continue
		}
		r.merge(out, p, res)
	}
	if len(violations) > 0 {
		return nil, errors.Join(violations...)
	}
	if len(failures) > 0 {
		return nil, errors.Join(failures...)
	}
	sortScan(out.Scan)
	return out, nil
}

func sortScan(files []config.ImportedScanFile) {
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
}

type resolver struct {
	cfg     *config.Config
	lock    *lockfile.File
	token   string
	markets map[string]*market
	log     logger.Logger
}

// stamp carries the entry's scope onto the files it delivered.
func stamp(files []config.ContentFile, p *config.InstalledPluginConfig, trust string) {
	for i := range files {
		files[i].Profiles, files[i].Domain, files[i].Trust = p.Profiles, p.Domain, trust
	}
}

// merge adds one plugin's delivery to the resolution, applying the entry's scope.
func (r *resolver) merge(out *config.PluginResolution, p *config.InstalledPluginConfig, res *pluginDelivery) {
	for _, group := range []struct {
		dst *[]config.ContentFile
		src []config.ContentFile
	}{{&out.Skills, res.Skills}, {&out.Agents, res.Agents}, {&out.Commands, res.Commands}, {&out.Rules, res.Rules}} {
		stamp(group.src, p, r.cfg.PluginTrust(p))
		*group.dst = append(*group.dst, group.src...)
	}
	for i := range res.MCP {
		s := res.MCP[i]
		s.Domain, s.Profiles = p.Domain, p.Profiles
		out.MCP = append(out.MCP, s)
	}
	out.Hooks = append(out.Hooks, res.Hooks...)
	out.Scan = append(out.Scan, res.Scan...)
	for _, f := range res.Findings {
		r.log.Debug("Installed plugin import", "plugin", p.Name, "status", string(f.Status), "source", f.Source, "field", f.Field, "reason", f.Reason)
	}
	if res.HooksDisabled > 0 && r.cfg.OncePerRun("plugin-hooks-disabled\x00"+p.Name) {
		r.log.Info("Installed plugin hooks are not imported; set enable_hooks = true to import them", "plugin", p.Name, "hooks", res.HooksDisabled)
	}
}

// relFromProject is the project-relative path of dir ("./x/y"), or "" when dir
// is outside the project: the path the plugin root variables stand for.
func relFromProject(cfg *config.Config, dir string) string {
	rel, err := filepath.Rel(cfg.BaseDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	if rel == "." {
		return "."
	}
	return "./" + filepath.ToSlash(rel)
}
