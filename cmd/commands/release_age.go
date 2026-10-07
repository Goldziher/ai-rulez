package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// newForgeClient builds the forge client of a run; tests replace it. The real
// one reads its token from the environment only and contacts only allowlisted hosts.
var newForgeClient = lockrun.DefaultForge

// ageGates is the minimum release age gate of every source of one run (see
// lockrun.AgeGates).
type ageGates struct{ *lockrun.AgeGates }

func newAgeGates(cfg *config.Config) ageGates {
	forgeFor := func(c *config.Config, offline bool) forge.Client { return newForgeClient(c, offline) }
	return ageGates{lockrun.NewAgeGates(cfg, forgeFor, lockOffline || cliLockPolicy.Offline, GetGitToken())}
}

// gateFor returns the gate of w, or nil when w has no minimum age.
func (g ageGates) gateFor(w lockfile.Want) *tagresolve.AgeGate { return g.GateFor(w) }

// installReleaseGateFor makes `update` and `lock --outdated` hold back tags
// younger than the configured minimum age of cfg while they resolve ranges: the
// gate goes into the command line's lock policy, so every later load of the run
// gets it. The returned function removes it.
func installReleaseGateFor(cfg *config.Config) (restore func()) {
	gates := newAgeGates(cfg)
	cliLockPolicy.ReleaseGate = gates.gateFor // handed to every later load of the run (withCLILockPolicy)
	return func() { cliLockPolicy.ReleaseGate = nil }
}
