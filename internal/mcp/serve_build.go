package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/skillsource"
	"github.com/Goldziher/ai-rulez/internal/usage"
	"github.com/samber/oops"
)

// ServeSetup is everything `ai-rulez mcp --serve-skills` needs to build and keep
// a catalog: where the project is, which skills to keep, which extra sources to
// add, and how strictly to treat the network, the lock and usage logging.
type ServeSetup struct {
	Version string
	WorkDir string
	Profile string
	Preset  string
	Filter  SkillFilter
	// Role is a role of the project's [[roles]]. The server then serves only the
	// skills the role keeps, with the delivery the role gives them (skills the
	// role delivers static are not served unless IncludeStatic is set), and it is
	// the default role of find_skill. Role and Profile are mutually exclusive.
	Role string
	// Sources are `--source` arguments (see skillsource.ParseArg), added to the
	// [[skill_sources]] of the configuration.
	Sources []string
	// IncludeStatic also serves skills whose delivery is static. By default a
	// project that uses delivery serves only served/both skills; a project that
	// sets no delivery anywhere serves every skill, as before.
	IncludeStatic bool
	// Frozen requires ai-rulez.lock to cover every remote source and never uses
	// the network. Offline never uses the network but does not require the lock.
	Frozen  bool
	Offline bool
	// CacheDir overrides the skill-source cache (tests).
	CacheDir string
	// BudgetBytes is the per-session cap of load_skill (0 default, -1 unlimited).
	BudgetBytes int
	// UsageLog and UsageSink receive one identifier-only line per load_skill.
	// With neither set the log is <config dir>/local/usage.jsonl when the
	// project enabled [usage] skills_index, and nothing is written otherwise.
	UsageLog  string
	UsageSink string
	// PollInterval is the live-reload check interval (0 selects the default).
	PollInterval time.Duration
	// NoWatch disables live reload.
	NoWatch bool
}

// built is one build of the catalog with what produced it.
type built struct {
	cfg     *config.Config
	catalog *Catalog
	lock    *lockfile.File
	// sources are what each source resolved to, for `ai-rulez lock`.
	sources []*skillsource.Resolved
}

// NewServer builds the catalog and the skills server around it. The caller runs
// Server.Watch (when NoWatch is false) and the MCP transport.
func (st *ServeSetup) NewServer(ctx context.Context) (*Server, error) {
	// Fingerprint before building: an edit made while the catalog is built is
	// then seen as a change by the watcher instead of being missed.
	baseline, baselineErr := st.initialFingerprint()
	first, err := st.build(ctx, buildOptions{admit: true})
	if err != nil {
		return nil, err
	}
	holder := &configHolder{}
	holder.set(first.cfg)
	opts := ServeOptions{
		Role:         st.Role,
		Roles:        RolesFromConfig(holder.get),
		BudgetBytes:  st.BudgetBytes,
		Telemetry:    st.telemetry(first.cfg),
		PollInterval: st.PollInterval,
	}
	if !st.NoWatch {
		roots := st.watchRoots(first)
		opts.Rebuild = func() (*Catalog, error) {
			b, err := st.build(ctx, buildOptions{admit: true})
			if err != nil {
				return nil, err
			}
			holder.set(b.cfg)
			return b.catalog, nil
		}
		opts.Fingerprint = func() (string, error) { return fingerprint(roots) }
		if baselineErr == nil {
			opts.Baseline = baseline
		}
	}
	if len(first.catalog.Skills()) == 0 {
		logger.Warn("No skills are served: nothing has delivery served or both, and no skill source is configured (pass --include-static to serve every skill)")
	}
	return NewSkillServerWith(st.Version, first.catalog, opts), nil
}

// initialFingerprint fingerprints the configuration directory and the local
// --source directories as they are before the first build, without loading the
// configuration twice. When the configuration itself names local sources the
// watcher's first comparison differs once, which only costs one extra rebuild.
func (st *ServeSetup) initialFingerprint() (string, error) {
	if st.NoWatch {
		return "", nil
	}
	wd := st.WorkDir
	if wd == "" {
		wd, _ = os.Getwd() //nolint:errcheck // an empty root fingerprints nothing
	}
	var roots []string
	if name := config.ResolveConfigDirName(wd); name != "" {
		abs, _ := filepath.Abs(filepath.Join(wd, filepath.FromSlash(name))) //nolint:errcheck // falls back to the given dir
		roots = append(roots, abs)
	}
	for _, arg := range st.Sources {
		if spec, err := skillsource.ParseArg(arg); err == nil && !spec.IsGit() {
			abs, _ := filepath.Abs(filepath.Join(spec.URL, filepath.FromSlash(spec.Path))) //nolint:errcheck // falls back to the given dir
			roots = append(roots, abs)
		}
	}
	return fingerprint(roots)
}

type buildOptions struct {
	// admit applies the security scan and lock enforcement. The lock command
	// builds without the lock (nothing to enforce yet) but still scans.
	admit bool
	// refresh re-resolves sources ignoring their pins (`ai-rulez lock`).
	refresh bool
	// ignoreLock neither reads nor enforces the lock's served pins (the lock
	// command is writing them).
	ignoreLock bool
	// noSources leaves the skill sources out: a role view can only narrow what
	// the full view serves, so the lock command builds sources once.
	noSources bool
}

func (st *ServeSetup) build(ctx context.Context, bo buildOptions) (*built, error) {
	cfg, err := st.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	var lock *lockfile.File
	if cfg.ConfigDir != "" {
		if lock, err = lockfile.Load(cfg.ConfigDir); err != nil {
			return nil, oops.Wrapf(err, "read %s", lockfile.FileName)
		}
	}
	profile, preset := st.Profile, st.Preset
	gen := generator.NewGenerator(cfg)
	var roleKeeps func(name string) bool
	if st.Role != "" {
		if st.Profile != "" {
			return nil, oops.Errorf("--role and --profile are mutually exclusive")
		}
		if cfg.Content == nil {
			return nil, oops.Hint("A role selects project content; run in a project with [[roles]]").Errorf("role %q needs a project", st.Role)
		}
		if err = gen.SetRole(st.Role); err != nil {
			return nil, oops.Wrapf(err, "resolve role %q", st.Role)
		}
		resolved, rerr := cfg.ResolveRole(st.Role)
		if rerr != nil {
			return nil, oops.Wrap(rerr)
		}
		roleKeeps = func(name string) bool { return resolved.Flat().Keeps(config.RoleKindSkill, "", name) }
	}
	var served []generator.ServedSkill
	if cfg.Content != nil {
		if preset, served, err = gen.ServedSkills(profile, preset); err != nil {
			return nil, oops.Wrapf(err, "render skills")
		}
		switch {
		case st.Role != "":
			profile = "role:" + st.Role
		case profile == "":
			profile = cfg.Default
		}
		served = selectByDelivery(served, st.IncludeStatic)
	}

	var specs []skillsource.Spec
	if !bo.noSources {
		if specs, err = st.sourceSpecs(cfg); err != nil {
			return nil, err
		}
	}
	b := &built{cfg: cfg, lock: lock}
	taken := map[string]bool{}
	for i := range served {
		taken[catalogName(&served[i])] = true
	}
	for _, spec := range specs {
		res, err := skillsource.Resolve(ctx, spec, skillsource.Options{
			CacheDir: st.CacheDir, Lock: lock, Offline: st.Offline, Frozen: st.Frozen, Refresh: bo.refresh,
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // already names the source
		}
		b.sources = append(b.sources, res)
		for _, sk := range res.Skills {
			if roleKeeps != nil && !roleKeeps(sk.Name) {
				continue
			}
			if taken[sk.Name] {
				logger.Warn("A skill source skill has the name of a skill that is already served; skipping it (set name_prefix on the source)", "source", spec.Name, "skill", sk.Name)
				continue
			}
			taken[sk.Name] = true
			served = append(served, servedFromSource(res, sk))
		}
	}

	catalog, err := BuildCatalog(profile, preset, served, st.Filter)
	if err != nil {
		return nil, oops.Wrapf(err, "build skill catalog")
	}
	if bo.admit {
		adm := Admission{Config: cfg, Enforce: cfg.LockEnforced() && !bo.ignoreLock, DefaultTrust: defaultTrust(cfg)}
		if !bo.ignoreLock {
			adm.Lock = lock
		}
		catalog = catalog.Admit(adm)
	}
	b.catalog = catalog
	return b, nil
}

func (st *ServeSetup) loadConfig(ctx context.Context) (*config.Config, error) {
	wd := st.WorkDir
	if wd == "" {
		var err error
		if wd, err = os.Getwd(); err != nil {
			return nil, oops.Wrapf(err, "working directory")
		}
	}
	if config.ResolveConfigDirName(wd) == "" && len(st.Sources) > 0 {
		// Serving only --source skills needs no project.
		abs, _ := filepath.Abs(wd) //nolint:errcheck // falls back to the given dir
		return &config.Config{BaseDir: abs}, nil
	}
	cfg, err := config.LoadConfig(ctx, wd)
	if err != nil {
		return nil, oops.Wrapf(err, "load configuration")
	}
	return cfg, nil
}

func (st *ServeSetup) sourceSpecs(cfg *config.Config) ([]skillsource.Spec, error) {
	var specs []skillsource.Spec
	seen := map[string]bool{}
	for i := range cfg.SkillSources {
		spec := skillsource.FromConfig(&cfg.SkillSources[i])
		specs = append(specs, spec)
		seen[spec.Name] = true
	}
	for _, arg := range st.Sources {
		spec, err := skillsource.ParseArg(arg)
		if err != nil {
			return nil, oops.Wrapf(err, "--source %q", arg)
		}
		if seen[spec.Name] {
			return nil, oops.Errorf("--source %q has the name %q, which another skill source already uses", arg, spec.Name)
		}
		seen[spec.Name] = true
		specs = append(specs, spec)
	}
	return specs, nil
}

// selectByDelivery keeps the skills a server should serve: those with delivery
// served or both; or every skill when includeStatic is set or when no skill
// anywhere opted into dynamic delivery (the behavior before delivery existed).
func selectByDelivery(all []generator.ServedSkill, includeStatic bool) []generator.ServedSkill {
	if includeStatic {
		return all
	}
	any := false
	for i := range all {
		if all[i].Delivery != config.DeliveryStatic && all[i].Delivery != "" {
			any = true
		}
	}
	if !any {
		return all
	}
	out := all[:0:0]
	for i := range all {
		if all[i].Delivery == config.DeliveryServed || all[i].Delivery == config.DeliveryBoth {
			out = append(out, all[i])
		}
	}
	return out
}

// catalogName is the name a served skill gets in the catalog: the frontmatter
// name, or the ID when the frontmatter names none.
func catalogName(s *generator.ServedSkill) string {
	if len(s.Files) > 0 && s.Files[0].RelPath == skillMarkdown {
		if front, err := parseFrontmatter(s.Files[0].Content); err == nil {
			if name := stringField(front, "name"); name != "" {
				return name
			}
		}
	}
	return s.ID
}

func servedFromSource(res *skillsource.Resolved, sk skillsource.Skill) generator.ServedSkill {
	files := make([]generator.ServedSkillFile, 0, len(sk.Files))
	for _, f := range sk.Files {
		files = append(files, generator.ServedSkillFile{RelPath: f.Path, Content: f.Content})
	}
	source := res.Spec.Redacted()
	if res.Spec.Path != "" {
		source += "#" + res.Spec.Path
	}
	return generator.ServedSkill{
		ID: sk.Name, Source: source, Ref: res.Spec.Ref, Pinned: res.Pinned, Commit: res.Commit,
		Trust: res.Spec.TrustLevel(), Delivery: config.DeliveryServed, Files: files,
	}
}

// defaultTrust is the scan level of a skill whose origin names none: skills
// authored in the project are scanned at their own severities, installed and
// included skills (anything with a ref) at the strict level unless
// lint.security.scan_imports says warn.
func defaultTrust(cfg *config.Config) func(*CatalogSkill) string {
	imports := config.TrustError
	if cfg != nil && cfg.Lint != nil && cfg.Lint.Security != nil &&
		strings.EqualFold(strings.TrimSpace(cfg.Lint.Security.ScanImports), "warn") {
		imports = config.TrustWarn
	}
	return func(s *CatalogSkill) string {
		if s.Ref != "" || s.Commit != "" || includes.IsGitURL(s.Source) {
			return imports
		}
		return config.TrustWarn
	}
}

func (st *ServeSetup) telemetry(cfg *config.Config) func(SessionTelemetry) {
	logPath := st.UsageLog
	if logPath == "" && st.UsageSink == "" && cfg.Usage != nil && cfg.Usage.SkillsIndex && cfg.ConfigDir != "" {
		logPath = filepath.Join(cfg.ConfigDir, "local", "usage.jsonl")
	}
	options := usage.RecordOptions{LogPath: logPath, SinkCommand: st.UsageSink}
	if cfg.ConfigDir != "" {
		options.IndexPath = filepath.Join(cfg.ConfigDir, usage.IndexFileName)
	}
	return func(t SessionTelemetry) {
		_, err := usage.RecordServed(usage.ServedLoad{
			Skill: t.Skill, Digest: t.Digest, Session: t.Session, Harness: t.Client, Role: t.Role, Resource: t.Resource,
		}, options)
		if err != nil {
			logger.Warn("Could not record the skill load", "skill", t.Skill, "error", err.Error())
		}
	}
}

// watchRoots lists the directories whose contents the catalog depends on: the
// configuration directory and every local skill source.
func (st *ServeSetup) watchRoots(b *built) []string {
	var roots []string
	if b.cfg.ConfigDir != "" {
		roots = append(roots, b.cfg.ConfigDir)
	}
	for _, res := range b.sources {
		if res.Commit == "" { // a local directory; git trees are immutable per commit
			roots = append(roots, res.Dir)
		}
	}
	return roots
}

// fingerprint hashes the path, size and modification time of every file below
// roots, leaving out VCS metadata and usage logs (a load_skill appends to one,
// which must not trigger a reload).
func fingerprint(roots []string) (string, error) {
	h := sha256.New()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), ".jsonl") || strings.HasPrefix(d.Name(), ".cache_meta") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err //nolint:wrapcheck // wrapped below
			}
			fmt.Fprintf(h, "%s\x00%d\x00%d\n", p, info.Size(), info.ModTime().UnixNano()) //nolint:errcheck // hash writes never fail
			return nil
		})
		if err != nil {
			return "", oops.With("root", root).Wrapf(err, "fingerprint skill files")
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// buildAll builds the full view and, for each role of the project, the role's
// view. A skill a role delivers as served is pinned and checked even when the
// unscoped view does not serve it, so the lock covers every view a server can be
// started with.
func (st *ServeSetup) buildAll(ctx context.Context, bo buildOptions) ([]*built, error) {
	base := *st
	base.Role = ""
	first, err := base.build(ctx, bo)
	if err != nil {
		return nil, err
	}
	views := []*built{first}
	bo.noSources, bo.refresh = true, false
	for _, name := range first.cfg.RoleNames() {
		view := base
		view.Role = name
		b, err := view.build(ctx, bo)
		if err != nil {
			logger.Warn("Left a role out of the served-skill lock", "role", name, "error", err.Error())
			continue
		}
		views = append(views, b)
	}
	return views, nil
}

// servedUnion lists the skills served by any view, once each, by name.
func servedUnion(views []*built) []*CatalogSkill {
	seen := map[string]bool{}
	var out []*CatalogSkill
	for _, v := range views {
		for _, s := range v.catalog.Skills() {
			if !seen[s.Name] {
				seen[s.Name] = true
				out = append(out, s)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LockRecords resolves everything the server would serve, in every role view,
// re-resolving remote sources (unless offline), and returns the lock entries to
// record: one per skill source, and one per skill that passes the security scan.
// Skills the scan refuses are returned as refusals and are not pinned.
func (st *ServeSetup) LockRecords(ctx context.Context) (sources, served []lockfile.Entry, refused []Refusal, err error) {
	views, err := st.buildAll(ctx, buildOptions{admit: true, refresh: !st.Offline && !st.Frozen, ignoreLock: true})
	if err != nil {
		return nil, nil, nil, err
	}
	for _, res := range views[0].sources {
		sources = append(sources, res.Entry())
	}
	for _, s := range servedUnion(views) {
		served = append(served, lockfile.Entry{Name: s.Name, Source: s.Source, Ref: s.Ref, Commit: s.Commit, Digest: s.LockDigest})
	}
	seen := map[string]bool{}
	for _, v := range views {
		for _, r := range v.catalog.Refusals() {
			if !seen[r.Name] {
				seen[r.Name] = true
				refused = append(refused, r)
			}
		}
	}
	return sources, served, refused, nil
}

// ServedProblems compares the skills the server would serve (in every role view)
// with the lock's served pins, offline, and lists every disagreement. It is
// `lock --check` for served skills.
func (st *ServeSetup) ServedProblems(ctx context.Context) ([]string, error) {
	off := *st
	off.Offline = true
	views, err := off.buildAll(config.WithOfflineIncludes(ctx), buildOptions{admit: true, ignoreLock: true})
	if err != nil {
		return nil, err
	}
	var problems []string
	have := map[string]bool{}
	for _, s := range servedUnion(views) {
		have[s.Name] = true
		switch e := views[0].lock.Find(lockfile.KindServed, s.Name); {
		case e == nil:
			problems = append(problems, fmt.Sprintf("served %s: not pinned in %s", s.Name, lockfile.FileName))
		case e.Digest != s.LockDigest:
			problems = append(problems, fmt.Sprintf("served %s: digest %s differs from the lock's %s", s.Name, s.LockDigest, e.Digest))
		}
	}
	if views[0].lock != nil {
		for _, e := range views[0].lock.Served {
			if !have[e.Name] {
				problems = append(problems, fmt.Sprintf("served %s: in the lock but no longer served", e.Name))
			}
		}
	}
	sort.Strings(problems)
	return problems, nil
}
