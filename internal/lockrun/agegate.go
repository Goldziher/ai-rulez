package lockrun

import (
	"context"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/releasetime"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// ForgeFunc builds the forge client a run asks for release times.
type ForgeFunc func(cfg *config.Config, offline bool) forge.Client

// DefaultForge is the real forge client: it reads its token from the
// configuration's environment only and contacts only allowlisted hosts.
func DefaultForge(cfg *config.Config, offline bool) forge.Client {
	return forge.NewClient(forge.Options{Host: cfg.Host, Offline: offline})
}

// AgeGates builds the minimum release age gate ([lock] min_release_age) of
// every source of one run. One Timer serves a repository, so a run asks the
// forge once per tag. GateFor is a config.VersionPolicy.ReleaseGate.
type AgeGates struct {
	cfg      *config.Config
	global   string
	newForge ForgeFunc
	offline  bool
	token    string
	client   forge.Client
	seen     *releasetime.SeenStore

	mu     sync.Mutex
	timers map[string]*releasetime.Timer
}

// NewAgeGates returns the gates of cfg's [lock] settings. newForge (nil:
// DefaultForge) is called once, when the first gate needs it; token
// authenticates the git lookup of a tag's commit date.
func NewAgeGates(cfg *config.Config, newForge ForgeFunc, offline bool, token string) *AgeGates {
	if newForge == nil {
		newForge = DefaultForge
	}
	g := &AgeGates{cfg: cfg, global: cfg.LockMinReleaseAge(), newForge: newForge, offline: offline, token: token, timers: map[string]*releasetime.Timer{}}
	if path, err := config.CacheDirIn(cfg.Host.Env, releasetime.FileName); err == nil {
		g.seen = releasetime.OpenSeenStore(path)
	} else {
		cfg.Log().Debug("no first-seen record: no cache directory", "error", err)
	}
	return g
}

// GateFor returns the gate of w, or nil when w has no minimum age.
func (g *AgeGates) GateFor(w lockfile.Want) *tagresolve.AgeGate {
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
			g.client = g.newForge(g.cfg, g.offline)
		}
		source, token := w.Source, g.token
		timer = releasetime.New(releasetime.Options{
			Mode:   g.cfg.LockMinReleaseAgeSource(),
			Source: source,
			Forge:  g.client,
			Seen:   g.seen,
			Commit: func(ctx context.Context, tag tagresolve.RawTag) (time.Time, error) {
				return includes.RemoteTagDate(ctx, source, token, tag.Name)
			},
			Clock: g.cfg.Host.Clock,
		})
		g.timers[w.Source] = timer
	}
	return &tagresolve.AgeGate{Min: minAge, Now: g.cfg.Host.Clock.Now(), Timer: timer}
}
