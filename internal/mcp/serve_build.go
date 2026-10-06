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

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
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
	// MaxCloneBytes is the clone size limit of git skill sources that set no
	// max_clone_bytes (0 selects AI_RULEZ_MAX_CLONE_BYTES, then 256 MiB).
	MaxCloneBytes int64
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
	// view is the serve view this build is of (ServeSetup.ViewKey).
	view string
	// empty says why nothing is served, when nothing is.
	empty string
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
	record, closeSink := st.telemetry(first.cfg)
	opts := ServeOptions{
		Role:         st.Role,
		Roles:        RolesFromConfig(holder.get),
		BudgetBytes:  st.BudgetBytes,
		Telemetry:    record,
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
		logs := st.usageFiles(first.cfg)
		opts.Fingerprint = func() (string, error) { return fingerprint(roots, logs...) }
		if baselineErr == nil {
			opts.Baseline = baseline
		}
	}
	if first.empty != "" {
		logger.Warn(first.empty)
	}
	srv := NewSkillServerWith(st.Version, first.catalog, opts)
	srv.closers = append(srv.closers, func() { closeSink(usageSinkFlushWait) })
	return srv, nil
}

// initialFingerprint fingerprints the configuration directory and the local
// --source directories as they are before the first build, without loading the
// configuration twice. When the configuration itself names local sources the
// watcher's first comparison differs once, which only costs one extra rebuild.
func (st *ServeSetup) initialFingerprint() (string, error) {
	if st.NoWatch {
		return "", nil
	}
	wd := st.workDir()
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
	// The same exclusions the watcher applies later, including the session salt
	// beside the log: a baseline that counts it differs from the first poll.
	return fingerprint(roots, absUsageFiles(st.UsageLog)...)
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
	// reuse, when not nil, are the sources an earlier build resolved; they are
	// used instead of resolving the configured sources again, so every view of one
	// lock run sees the same commits.
	reuse []*skillsource.Resolved
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
	roleKeeps, err := st.selectRole(cfg, gen)
	if err != nil {
		return nil, err
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

	specs, err := st.sourceSpecs(cfg)
	if err != nil {
		return nil, err
	}
	b := &built{cfg: cfg, lock: lock, view: st.ViewKey()}
	taken := map[string]bool{}
	for i := range served {
		taken[catalogName(&served[i])] = true
	}
	resolved, err := st.resolveSources(ctx, cfg, lock, specs, bo)
	if err != nil {
		return nil, err
	}
	for _, res := range resolved {
		spec := res.Spec
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

	if err := st.checkDomains(cfg); err != nil {
		return nil, err
	}
	catalog, err := BuildCatalog(profile, preset, served, st.Filter)
	if err != nil {
		return nil, oops.Wrapf(err, "build skill catalog")
	}
	if len(catalog.Skills()) == 0 {
		b.empty = noSkillsMessage(served, st.Filter)
	}
	if bo.admit {
		adm := Admission{Config: cfg, Enforce: cfg.LockEnforced() && !bo.ignoreLock, Pinning: bo.ignoreLock, View: b.view, DefaultTrust: defaultTrust(cfg)}
		if !bo.ignoreLock {
			adm.Lock = lock
		}
		catalog = catalog.Admit(adm)
	}
	b.catalog = catalog
	return b, nil
}

// resolveSources resolves the skill sources of a build, or hands back the ones an
// earlier build of the same run resolved.
func (st *ServeSetup) resolveSources(ctx context.Context, cfg *config.Config, lock *lockfile.File, specs []skillsource.Spec, bo buildOptions) ([]*skillsource.Resolved, error) {
	if bo.reuse != nil {
		return bo.reuse, nil
	}
	out := make([]*skillsource.Resolved, 0, len(specs))
	for _, spec := range specs {
		res, err := skillsource.Resolve(ctx, spec, skillsource.Options{
			CacheDir: st.CacheDir, Lock: lock, Offline: st.Offline, Frozen: st.Frozen, Refresh: bo.refresh,
			ProjectRoot: cfg.BaseDir, MaxCloneBytes: st.MaxCloneBytes,
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // already names the source
		}
		out = append(out, res)
	}
	return out, nil
}

// selectRole narrows the generator to the setup's role and returns the test a
// source skill must pass to be kept by it; nil when the setup has no role.
func (st *ServeSetup) selectRole(cfg *config.Config, gen *generator.Generator) (func(name string) bool, error) {
	if st.Role == "" {
		return nil, nil
	}
	if st.Profile != "" {
		return nil, oops.Errorf("--role and --profile are mutually exclusive")
	}
	if cfg.Content == nil {
		return nil, oops.Hint("A role selects project content; run in a project with [[roles]]").Errorf("role %q needs a project", st.Role)
	}
	if err := gen.SetRole(st.Role); err != nil {
		return nil, oops.Wrapf(err, "resolve role %q", st.Role)
	}
	resolved, err := cfg.ResolveRole(st.Role)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return func(name string) bool { return resolved.Flat().Keeps(config.RoleKindSkill, "", name) }, nil
}

// workDir is the project directory to serve: WorkDir, or the current directory
// (".") when the caller named none. The CLI sets WorkDir from its own working
// directory; the library never reads it.
func (st *ServeSetup) workDir() string {
	if st.WorkDir == "" {
		return "."
	}
	return st.WorkDir
}

func (st *ServeSetup) loadConfig(ctx context.Context) (*config.Config, error) {
	wd := st.workDir()
	if config.ResolveConfigDirName(wd) == "" && len(st.Sources) > 0 {
		// Serving only --source skills needs no project.
		abs, _ := filepath.Abs(wd) //nolint:errcheck // falls back to the given dir
		return &config.Config{BaseDir: abs}, nil
	}
	cfg, err := config.LoadConfig(ctx, wd)
	if err != nil {
		return nil, oops.Wrapf(err, "load configuration")
	}
	if err := config.CheckPolicy(cfg); err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	return cfg, nil
}

func (st *ServeSetup) sourceSpecs(cfg *config.Config) ([]skillsource.Spec, error) {
	var specs []skillsource.Spec
	seen := map[string]bool{}
	for i := range cfg.SkillSources {
		spec := skillsource.FromConfig(&cfg.SkillSources[i])
		// Only the user's own config may name a directory outside its project.
		spec.AllowOutside = cfg.UserScope
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
		ID: sk.Name, Source: source, Ref: res.Spec.Want().Ref, Pinned: res.Pinned, Commit: res.Commit,
		Trust: res.Spec.TrustLevel(), Delivery: config.DeliveryServed, Verbatim: true, Files: files,
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
		if s.Imported || s.Ref != "" || s.Commit != "" || includes.IsGitURL(s.Source) {
			return imports
		}
		return config.TrustWarn
	}
}

// usageLogPath is the file load_skill appends usage to, or "".
func (st *ServeSetup) usageLogPath(cfg *config.Config) string {
	logPath := st.UsageLog
	if logPath == "" && st.UsageSink == "" && cfg.Usage != nil && cfg.Usage.SkillsIndex && cfg.ConfigDir != "" {
		logPath = filepath.Join(cfg.ConfigDir, "local", "usage.jsonl")
	}
	return logPath
}

// usageFiles lists, as absolute paths, the files a load_skill writes: the usage
// log and the session salt beside it. Writing them must not trigger a reload.
func (st *ServeSetup) usageFiles(cfg *config.Config) []string {
	return absUsageFiles(st.usageLogPath(cfg))
}

// absUsageFiles is logPath and the salt file beside it, absolute; nil for no log.
func absUsageFiles(logPath string) []string {
	if logPath == "" {
		return nil
	}
	var out []string
	for _, p := range []string{logPath, usage.DefaultSaltPath(logPath)} {
		if abs, err := filepath.Abs(p); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

func (st *ServeSetup) telemetry(cfg *config.Config) (record func(SessionTelemetry), closeSink func(time.Duration)) {
	logPath := st.usageLogPath(cfg)
	options := usage.RecordOptions{LogPath: logPath, SinkCommand: st.UsageSink}
	if cfg.ConfigDir != "" {
		options.IndexPath = filepath.Join(cfg.ConfigDir, usage.IndexFileName)
		if logPath == "" {
			// A sink without a log has no salt file beside it; use the project's
			// so a sink record carries the same salted session as a log line.
			options.SaltPath = filepath.Join(cfg.ConfigDir, "local", "usage.salt")
		}
	}
	closeSink = func(time.Duration) {}
	if st.UsageSink != "" {
		sink := usage.NewAsyncSink(st.UsageSink, usageSinkQueue, func(err error) {
			logger.Warn("Usage sink failed", "error", err.Error())
		})
		options.AsyncSink = sink
		closeSink = sink.Close
	}
	return func(t SessionTelemetry) {
		_, err := usage.RecordServed(usage.ServedLoad{
			Skill: t.Skill, Digest: t.Digest, Session: t.Session, Harness: t.Client, Role: t.Role, Resource: t.Resource,
		}, options)
		if err != nil {
			logger.Warn("Could not record the skill load", "skill", t.Skill, "error", err.Error())
		}
		if sink := options.AsyncSink; sink != nil && sink.Dropped() > 0 {
			logger.Warn("Usage sink queue is full; dropping records", "dropped", sink.Dropped())
		}
	}, closeSink
}

// usageSinkQueue bounds the records waiting for a slow --usage-sink command.
const usageSinkQueue = 256

// usageSinkFlushWait is how long shutdown waits for queued sink records.
const usageSinkFlushWait = 3 * time.Second

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
// roots, leaving out VCS metadata and the usage logs a load_skill appends to
// (which must not trigger a reload): .jsonl files directly inside a `local`
// directory, and the files named in logs. A .jsonl file anywhere else is skill
// content and counts.
func fingerprint(roots []string, logs ...string) (string, error) {
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
			if isUsageLog(p, logs) || isCacheBookkeeping(root, p) {
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

// isCacheBookkeeping reports whether p is one of the two bookkeeping files of a
// cached clone, at the root of a watched directory. The lock digest
// (contentlock.DigestDir) leaves out exactly these names there, so anything else
// that merely starts with the name is skill content and must trigger a reload.
func isCacheBookkeeping(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && (rel == ".cache_meta.json" || rel == ".cache_meta.json.tmp")
}

func isUsageLog(p string, logs []string) bool {
	if filepath.Base(filepath.Dir(p)) == "local" && (strings.HasSuffix(p, ".jsonl") || filepath.Base(p) == "usage.salt") {
		return true
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	for _, l := range logs {
		if l == "" {
			continue
		}
		if la, err := filepath.Abs(l); err == nil && la == abs {
			return true
		}
	}
	return false
}

// buildAll builds the views a lock covers: the default view, the view of each
// role of the project, every other view the lock already records (a profile or
// --include-static view that an earlier `lock --profile ...` pinned), and the
// extra views the caller names. A skill a role delivers as served is pinned and
// checked even when the unscoped view does not serve it. The configured skill
// sources are resolved once and shared by every view that adds no source of its own.
func (st *ServeSetup) buildAll(ctx context.Context, bo buildOptions, extras []ServeSetup) ([]*built, error) {
	base := *st
	base.Role, base.Profile, base.IncludeStatic, base.Sources = "", "", false, nil
	first, err := base.build(ctx, bo)
	if err != nil {
		return nil, err
	}
	views := []*built{first}
	refresh := bo.refresh
	bo.reuse, bo.refresh = first.sources, false
	if bo.reuse == nil {
		bo.reuse = []*skillsource.Resolved{}
	}
	seen := map[string]bool{"": true}
	build := func(view ServeSetup, strict bool) error {
		key := view.ViewKey()
		if seen[key] {
			return nil
		}
		seen[key] = true
		opts := bo
		if len(view.Sources) > 0 {
			opts.reuse, opts.refresh = nil, refresh
		}
		b, err := view.build(ctx, opts)
		if err != nil {
			if strict {
				return err
			}
			logger.Warn("Left a view out of the served-skill lock", "view", key, "error", err.Error())
			return nil
		}
		views = append(views, b)
		return nil
	}
	for _, name := range first.cfg.RoleNames() {
		view := base
		view.Role = name
		if err := build(view, false); err != nil {
			return nil, err
		}
	}
	for _, key := range recordedViews(first.lock) {
		if view, ok := base.withView(key); ok && !namesRemovedRole(first.cfg.RoleNames(), view) {
			if err := build(view, false); err != nil {
				return nil, err
			}
		}
	}
	for _, extra := range extras {
		view := base
		view.Role, view.Profile, view.IncludeStatic, view.Sources = extra.Role, extra.Profile, extra.IncludeStatic, extra.Sources
		if err := build(view, true); err != nil {
			return nil, err
		}
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

// LockResult is what a `lock` run records for the skills the server would serve.
type LockResult struct {
	// Sources has one entry per skill source.
	Sources []lockfile.Entry
	// Served has one entry per skill that passes the security scan, in each view.
	Served []lockfile.Entry
	// Views lists every view that was evaluated, including those that serve
	// nothing, so the caller can drop pins of a view that no longer serves a skill.
	Views []string
	// Refused are the skills the security scan refuses; they are not pinned.
	Refused []Refusal
}

// LockRecords resolves everything the server would serve, in every view,
// re-resolving remote sources (unless offline), and returns the lock entries to
// record: one per skill source, and one per skill that passes the security scan
// in each view. Skills the scan refuses are returned as refusals and not pinned.
func (st *ServeSetup) LockRecords(ctx context.Context) (sources, served []lockfile.Entry, refused []Refusal, err error) {
	res, err := st.LockViews(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	return res.Sources, res.Served, res.Refused, nil
}

// LockViews is LockRecords with the evaluated views, and with extra views to
// pin besides the default one, the roles and the views the lock already records.
func (st *ServeSetup) LockViews(ctx context.Context, extras []ServeSetup) (*LockResult, error) {
	views, err := st.buildAll(ctx, buildOptions{admit: true, refresh: !st.Offline && !st.Frozen, ignoreLock: true}, extras)
	if err != nil {
		return nil, err
	}
	res := &LockResult{}
	haveSource := map[string]bool{}
	refused := map[string]bool{}
	for _, v := range views {
		for _, r := range v.sources {
			if !haveSource[r.Spec.Name] {
				haveSource[r.Spec.Name] = true
				res.Sources = append(res.Sources, r.Entry())
			}
		}
		res.Views = append(res.Views, v.view)
		for _, s := range v.catalog.Skills() {
			res.Served = append(res.Served, lockfile.Entry{Name: s.Name, Source: s.Source, Ref: s.Ref, Commit: s.Commit, Digest: s.LockDigest, View: v.view})
		}
		for _, r := range v.catalog.Refusals() {
			if key := r.Name + "\x00" + r.Code + "\x00" + r.Reason; !refused[key] {
				refused[key] = true
				res.Refused = append(res.Refused, r)
			}
		}
	}
	return res, nil
}

// ServedProblems compares the skills the server would serve with the lock's
// served pins, offline, and lists every disagreement. It is `lock --check` for
// served skills. It evaluates the default view, every role, every view the lock
// records and the extra views named (--role, --profile, --include-static, --source).
func (st *ServeSetup) ServedProblems(ctx context.Context, extras ...ServeSetup) ([]string, error) {
	off := *st
	off.Offline = true
	views, err := off.buildAll(config.WithOfflineIncludes(ctx), buildOptions{admit: true, ignoreLock: true}, extras)
	if err != nil {
		return nil, err
	}
	return servedProblems(views, views[0].lock), nil
}

func servedProblems(views []*built, lock *lockfile.File) []string {
	var problems []string
	servedAnywhere := map[string]bool{}
	checked := map[string]bool{}
	label := func(name, view string) string {
		if view == "" {
			return "served " + name
		}
		return fmt.Sprintf("served %s (view %s)", name, view)
	}
	for _, v := range views {
		checked[v.view] = true
		for _, s := range v.catalog.Skills() {
			servedAnywhere[s.Name] = true
			switch e := servedPin(lock, v.view, s.Name); {
			case e == nil:
				problems = append(problems, fmt.Sprintf("%s: not pinned in %s", label(s.Name, v.view), lockfile.FileName))
			case e.Digest != s.LockDigest:
				problems = append(problems, fmt.Sprintf("%s: digest %s differs from the lock's %s", label(s.Name, v.view), s.LockDigest, e.Digest))
			}
		}
	}
	if lock != nil {
		serves := func(view, name string) bool {
			for _, v := range views {
				if v.view == view && v.catalog.byName[name] != nil {
					return true
				}
			}
			return false
		}
		for _, e := range lock.Served {
			switch {
			case !checked[e.View]:
				// The lock records a view this check was not asked to evaluate.
			case e.View == "" && servedAnywhere[e.Name]:
				// An entry from before views existed covers every view.
			case !serves(e.View, e.Name):
				problems = append(problems, fmt.Sprintf("%s: in the lock but no longer served", label(e.Name, e.View)))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

// ServedRefusals lists the skills the security scan refuses to serve in the
// views of the setup, offline.
func (st *ServeSetup) ServedRefusals(ctx context.Context, extras ...ServeSetup) ([]Refusal, error) {
	off := *st
	off.Offline = true
	views, err := off.buildAll(config.WithOfflineIncludes(ctx), buildOptions{admit: true, ignoreLock: true}, extras)
	if err != nil {
		return nil, err
	}
	var out []Refusal
	seen := map[string]bool{}
	for _, v := range views {
		for _, r := range v.catalog.Refusals() {
			if key := r.Name + "\x00" + r.Code + "\x00" + r.Reason; !seen[key] {
				seen[key] = true
				out = append(out, r)
			}
		}
	}
	return out, nil
}

// ServedScanReports lists, offline, the served files the security scan cannot
// read (AR989) in the views of the setup: for authored and source skills alike,
// refused or not. It is what `validate --strict` reports.
func (st *ServeSetup) ServedScanReports(ctx context.Context, extras ...ServeSetup) ([]ScanReport, error) {
	off := *st
	off.Offline = true
	views, err := off.buildAll(config.WithOfflineIncludes(ctx), buildOptions{admit: true, ignoreLock: true}, extras)
	if err != nil {
		return nil, err
	}
	var out []ScanReport
	seen := map[string]bool{}
	for _, v := range views {
		for _, r := range v.catalog.ScanReports() {
			if key := r.Skill + "\x00" + r.Level + "\x00" + fmt.Sprint(r.Findings); !seen[key] {
				seen[key] = true
				out = append(out, r)
			}
		}
	}
	return out, nil
}

// checkDomains refuses a --domain that names no domain of the project, as an
// unknown role or profile is refused: a typo would otherwise serve nothing.
// "root" names the skills that belong to no domain.
func (st *ServeSetup) checkDomains(cfg *config.Config) error {
	for _, d := range st.Filter.Domains {
		d = strings.TrimSpace(d)
		if d == "root" {
			continue
		}
		if cfg.Content != nil {
			if _, ok := cfg.Content.Domains[d]; ok {
				continue
			}
		}
		return oops.Hint("Use `ai-rulez domain list` for the domains, or `root` for skills in no domain").Errorf("unknown domain %q", d)
	}
	return nil
}

// noSkillsMessage says why a server serves nothing: no skill has delivery served
// and no source is configured, or the --domain, --allow and --deny filters
// removed what was served (each filter with how many skills it leaves alone).
func noSkillsMessage(served []generator.ServedSkill, f SkillFilter) string {
	if len(served) == 0 {
		return "No skills are served: nothing has delivery served or both, and no skill source is configured (pass --include-static to serve every skill)"
	}
	count := func(only SkillFilter) int {
		n := 0
		for i := range served {
			if only.allows(&served[i]) {
				n++
			}
		}
		return n
	}
	var parts []string
	if len(f.Domains) > 0 {
		parts = append(parts, fmt.Sprintf("--domain %s keeps %d", strings.Join(f.Domains, ","), count(SkillFilter{Domains: f.Domains})))
	}
	if len(f.Allow) > 0 {
		parts = append(parts, fmt.Sprintf("--allow %s keeps %d", strings.Join(f.Allow, ","), count(SkillFilter{Allow: f.Allow})))
	}
	if len(f.Deny) > 0 {
		parts = append(parts, fmt.Sprintf("--deny %s leaves %d", strings.Join(f.Deny, ","), count(SkillFilter{Deny: f.Deny})))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("No skills are served: %d skill(s) were rendered but none could be represented (see the warnings above)", len(served))
	}
	return fmt.Sprintf("No skills are served: %d skill(s) have delivery served, but the filters removed all of them (%s)", len(served), strings.Join(parts, "; "))
}
