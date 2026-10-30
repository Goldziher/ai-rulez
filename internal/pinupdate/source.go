// Package pinupdate moves the pins of sources that use a version constraint and
// reports which sources have newer tags. `ai-rulez lock --outdated`,
// `ai-rulez update` and the MCP tools lock_outdated, update_plan and update run
// this code, so the pre-pin security scan and the refusals (AR730, AR731, AR732,
// scan findings) are the same everywhere. The package holds no process state:
// the caller passes its loader, lock policy and git token in an Env.
package pinupdate

import (
	"context"
	"errors"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// ErrRefused marks a run that refused a source (a moved tag, an unsatisfiable
// constraint, or a pre-pin scan with error findings): findings, not a failure
// to run. Nothing was written. The Report says which sources were refused.
var ErrRefused = errors.New("a source was refused; nothing was written")

// Env is what a run needs from its caller.
type Env struct {
	// Version is the ai-rulez version recorded in the lock.
	Version string
	// Policy is the base lock policy of the run's loads.
	Policy config.LockPolicy
	// Load loads the project under a lock policy, with opts.
	Load func(ctx context.Context, policy config.LockPolicy, opts ...config.LoadOption) (*config.Config, error)
	// GitToken authenticates the remote lookups (empty: anonymous).
	GitToken string
	// NewForge builds the forge client of the release age gate (nil: lockrun.DefaultForge).
	NewForge lockrun.ForgeFunc
	// Collector keeps the warnings of the loads the run makes (nil: their own).
	Collector *diag.Collector
	// Cwd is the directory scan records show paths relative to.
	Cwd string
}

// Source is one configured source that asks for a version range.
type Source struct {
	Kind, Name string
	Want       lockfile.Want
	// Key identifies the repository, so one `ls-remote` serves every source of it.
	Key  string
	List func(context.Context) ([]tagresolve.RawTag, error)
	// TreeDir is the cached tree of the source at commit ("" when not cached),
	// which `update` compares before and after a move.
	TreeDir func(commit string) string
}

// VersionSources lists the sources of cfg that use a version constraint (git
// only), limited to kind and names when given. token authenticates the tag lookups.
func VersionSources(cfg *config.Config, kind string, names map[string]bool, token string) []Source {
	var out []Source
	keep := func(k, n string) bool { return (kind == "" || kind == k) && (len(names) == 0 || names[n]) }
	sources := includes.VersionSources(cfg)
	for i := range sources {
		vs := &sources[i]
		if !keep(vs.Want.Kind, vs.Want.Name) {
			continue
		}
		url, w := vs.URL, vs.Want
		out = append(out, Source{Kind: w.Kind, Name: w.Name, Want: w, Key: url,
			List: func(ctx context.Context) ([]tagresolve.RawTag, error) {
				return includes.ListRemoteTags(ctx, url, token)
			},
			TreeDir: func(commit string) string { return includes.CachedTreeDir(cfg, w, commit) }})
	}
	for i := range cfg.SkillSources {
		spec := skillsource.FromConfig(&cfg.SkillSources[i])
		w := spec.Want()
		if !spec.IsGit() || w.Constraint == "" || !keep(lockfile.KindSource, spec.Name) {
			continue
		}
		out = append(out, Source{Kind: lockfile.KindSource, Name: spec.Name, Want: w, Key: spec.URL,
			List:    func(ctx context.Context) ([]tagresolve.RawTag, error) { return skillsource.ListTags(ctx, spec) },
			TreeDir: func(commit string) string { return skillsource.CachedTreeDir(spec, commit, "") }})
	}
	return out
}

// TagLister memoizes tag lists per repository for one run.
type TagLister struct {
	tags map[string][]tagresolve.RawTag
}

// Of lists the tags of s's repository once per run.
func (l *TagLister) Of(ctx context.Context, s Source) ([]tagresolve.RawTag, error) {
	if t, ok := l.tags[s.Key]; ok {
		return t, nil
	}
	t, err := s.List(ctx)
	if err != nil {
		return nil, oops.With("source", s.Name).Wrapf(err, "list the tags of %s %q", s.Kind, s.Name)
	}
	if l.tags == nil {
		l.tags = map[string][]tagresolve.RawTag{}
	}
	l.tags[s.Key] = t
	return t, nil
}

// RequireOnline is the offline refusal of the commands that read remote tags.
func RequireOnline(command string, offline bool) error {
	if offline {
		return oops.Hint("`ai-rulez lock --check` verifies the lock offline").
			Errorf("%s reads the remote's tags and needs the network; it cannot run with --offline", command)
	}
	return nil
}

// Evaluate compares every source with its remote tags; gate (nil: none) holds
// back tags younger than the minimum release age.
func Evaluate(ctx context.Context, srcs []Source, lock *lockfile.File, gate func(lockfile.Want) *tagresolve.AgeGate) ([]tagresolve.Row, error) {
	var lister TagLister
	rows := make([]tagresolve.Row, 0, len(srcs))
	for i := range srcs {
		s := &srcs[i]
		tags, err := lister.Of(ctx, *s)
		if err != nil {
			return nil, err
		}
		var g *tagresolve.AgeGate
		if gate != nil {
			g = gate(s.Want)
		}
		rows = append(rows, tagresolve.EvaluateGated(ctx, s.Want, lock.Find(s.Kind, s.Name), tags, g))
	}
	return rows, nil
}

// CheckNames refuses names that match no source with a version constraint.
func CheckNames(srcs []Source, wanted map[string]bool) error {
	found := map[string]bool{}
	for i := range srcs {
		found[srcs[i].Name] = true
	}
	var missing []string
	for n := range wanted {
		if !found[n] {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return oops.Hint("`ai-rulez lock --outdated` lists the sources that use a version constraint").
			Errorf("%v: not a remote include, installed skill or skill source with a version constraint", missing)
	}
	return nil
}

// Prepared is a project loaded for a run: its configuration, its lock and the
// evaluated version sources.
type Prepared struct {
	Cfg     *config.Config
	Current *lockfile.File
	Srcs    []Source
	Rows    []tagresolve.Row
	// Gate holds back tags younger than the configured minimum release age.
	Gate func(lockfile.Want) *tagresolve.AgeGate
}

// Prepare loads the project without fetching remotes, reads the lock and
// evaluates the sources of kind and names against their remote tags.
func Prepare(ctx context.Context, env Env, kind string, names []string) (*Prepared, error) {
	cfg, err := env.Load(ctx, env.Policy, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return nil, err
	}
	current, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	p := &Prepared{Cfg: cfg, Current: current}
	p.Gate = lockrun.NewAgeGates(cfg, env.NewForge, env.Policy.Offline, env.GitToken).GateFor
	p.Srcs = VersionSources(cfg, kind, wanted, env.GitToken)
	if err := CheckNames(p.Srcs, wanted); err != nil {
		return nil, err
	}
	if p.Rows, err = Evaluate(ctx, p.Srcs, current, p.Gate); err != nil {
		return nil, err
	}
	return p, nil
}
