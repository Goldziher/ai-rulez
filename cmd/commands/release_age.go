package commands

import (
	"context"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/releasetime"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// newForgeClient builds the forge client of a run; tests replace it. The real
// one reads its token from the environment only and contacts only allowlisted hosts.
var newForgeClient = func(cfg *config.Config, offline bool) forge.Client {
	return forge.NewClient(forge.Options{Host: cfg.Host, Offline: offline})
}

// ageGates builds the minimum release age gate of every source of one run. One
// Timer serves a repository, so a run asks the forge once per tag.
type ageGates struct {
	cfg    *config.Config
	global string
	client forge.Client
	seen   *releasetime.SeenStore

	mu     sync.Mutex
	timers map[string]*releasetime.Timer
}

func newAgeGates(cfg *config.Config) *ageGates {
	g := &ageGates{cfg: cfg, global: cfg.LockMinReleaseAge(), timers: map[string]*releasetime.Timer{}}
	if path, err := config.CacheDirIn(cfg.Host.Env, releasetime.FileName); err == nil {
		g.seen = releasetime.OpenSeenStore(path)
	} else {
		logger.Debug("no first-seen record: no cache directory", "error", err)
	}
	return g
}

// gateFor returns the gate of w, or nil when w has no minimum age.
func (g *ageGates) gateFor(w lockfile.Want) *tagresolve.AgeGate {
	raw := w.MinReleaseAge
	if raw == "" {
		raw = g.global
	}
	minAge, err := semver.ParseAge(raw)
	if err != nil || minAge <= 0 || w.Constraint == "" {
		return nil // an invalid age is reported by config validation
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	timer, ok := g.timers[w.Source]
	if !ok {
		if g.client == nil {
			g.client = newForgeClient(g.cfg, lockOffline || cliLockPolicy.Offline)
		}
		source := w.Source
		timer = releasetime.New(releasetime.Options{
			Mode:   g.cfg.LockMinReleaseAgeSource(),
			Source: source,
			Forge:  g.client,
			Seen:   g.seen,
			Commit: func(ctx context.Context, tag tagresolve.RawTag) (time.Time, error) {
				return includes.RemoteTagDate(ctx, source, GetGitToken(), tag.Name)
			},
			Clock: g.cfg.Host.Clock,
		})
		g.timers[w.Source] = timer
	}
	return &tagresolve.AgeGate{Min: minAge, Now: g.cfg.Host.Clock.Now(), Timer: timer}
}

// installReleaseGate makes `lock` and `update` hold back tags younger than the
// configured minimum age while they resolve ranges. It reads [lock] from a
// config load that fetches nothing, because the sources are resolved during the
// real load. The returned function removes the gate.
func installReleaseGate(path string) (restore func()) {
	cfg, err := loadForLock(path, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return func() {} // the real load reports the error
	}
	return installReleaseGateFor(cfg)
}

func installReleaseGateFor(cfg *config.Config) (restore func()) {
	gates := newAgeGates(cfg)
	cliLockPolicy.ReleaseGate = gates.gateFor // handed to every later load of the run (withCLILockPolicy)
	return func() { cliLockPolicy.ReleaseGate = nil }
}
