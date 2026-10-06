package commands

import (
	"context"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/samber/oops"
)

var (
	lockOutdated       bool
	lockFailOnOutdated bool
	lockOffline        bool
)

// versionSrc is one configured source that asks for a version range.
type versionSrc struct {
	kind, name string
	want       lockfile.Want
	// key identifies the repository, so one `ls-remote` serves every source of it.
	key  string
	list func(context.Context) ([]tagresolve.RawTag, error)
	// treeDir is the cached tree of the source at commit ("" when not cached),
	// which `update` compares before and after a move.
	treeDir func(commit string) string
}

// versionSources lists the sources of cfg that use a version constraint (git
// only), limited to kind and names when given.
func versionSources(cfg *config.Config, kind string, names map[string]bool) []versionSrc {
	var out []versionSrc
	keep := func(k, n string) bool { return (kind == "" || kind == k) && (len(names) == 0 || names[n]) }
	for _, vs := range includes.VersionSources(cfg) {
		if !keep(vs.Want.Kind, vs.Want.Name) {
			continue
		}
		url, w := vs.URL, vs.Want
		out = append(out, versionSrc{kind: w.Kind, name: w.Name, want: w, key: url,
			list: func(ctx context.Context) ([]tagresolve.RawTag, error) {
				return includes.ListRemoteTags(ctx, url, GetGitToken())
			},
			treeDir: func(string) string { return includes.CachedTreeDir(cfg, w) }})
	}
	for i := range cfg.SkillSources {
		spec := skillsource.FromConfig(&cfg.SkillSources[i])
		w := spec.Want()
		if !spec.IsGit() || w.Constraint == "" || !keep(lockfile.KindSource, spec.Name) {
			continue
		}
		out = append(out, versionSrc{kind: lockfile.KindSource, name: spec.Name, want: w, key: spec.URL,
			list:    func(ctx context.Context) ([]tagresolve.RawTag, error) { return skillsource.ListTags(ctx, spec) },
			treeDir: func(commit string) string { return skillsource.CachedTreeDir(spec, commit, "") }})
	}
	return out
}

// tagLister memoizes tag lists per repository for one run.
type tagLister struct {
	tags map[string][]tagresolve.RawTag
}

func (l *tagLister) of(ctx context.Context, s versionSrc) ([]tagresolve.RawTag, error) {
	if t, ok := l.tags[s.key]; ok {
		return t, nil
	}
	t, err := s.list(ctx)
	if err != nil {
		return nil, oops.With("source", s.name).Wrapf(err, "list the tags of %s %q", s.kind, s.name)
	}
	if l.tags == nil {
		l.tags = map[string][]tagresolve.RawTag{}
	}
	l.tags[s.key] = t
	return t, nil
}

// requireOnline is the --offline refusal of the commands that read remote tags.
func requireOnline(command string) error {
	if lockOffline || includes.SkipFetch {
		return oops.Hint("`ai-rulez lock --check` verifies the lock offline").
			Errorf("%s reads the remote's tags and needs the network; it cannot run with --offline", command)
	}
	return nil
}

// evaluateSources compares every source with its remote tags.
func evaluateSources(ctx context.Context, srcs []versionSrc, lock *lockfile.File) ([]tagresolve.Row, error) {
	var lister tagLister
	rows := make([]tagresolve.Row, 0, len(srcs))
	for _, s := range srcs {
		tags, err := lister.of(ctx, s)
		if err != nil {
			return nil, err
		}
		rows = append(rows, tagresolve.Evaluate(s.kind, s.name, s.want, lock.Find(s.kind, s.name), tags))
	}
	return rows, nil
}

// outdatedAt prints which sources have newer tags than their pins.
func outdatedAt(path string, kind string, names []string) int {
	if err := requireOnline("lock --outdated"); err != nil {
		fmtError(err)
		return 1
	}
	cfg, err := loadForLock(path, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		fmtError(err)
		return 1
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	rows, err := evaluateSources(context.Background(), versionSources(cfg, kind, wanted), lock)
	if err != nil {
		fmtError(err)
		return 1
	}
	report := tagresolve.NewReport(rows)
	if lockFormat == formatJSON {
		err = report.WriteJSON(os.Stdout)
	} else {
		err = report.WriteText(os.Stdout)
	}
	if err != nil {
		fmtError(oops.Wrapf(err, "write the report"))
		return 1
	}
	return outdatedExit(report)
}

// outdatedExit: 2 for an error finding (a moved tag, an unresolvable constraint)
// and, with --fail-on-outdated, for any allowed update; else 0.
func outdatedExit(r *tagresolve.Report) int {
	if r.Failing() || (lockFailOnOutdated && r.Summary.Updatable > 0) {
		return exitDrift
	}
	return 0
}
