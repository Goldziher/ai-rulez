package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	pemit "github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
	"github.com/Goldziher/ai-rulez/v5/internal/publish/oci"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// publishContext is what a publish run resolves once and every plugin shares.
type publishContext struct {
	cfg     *config.Config
	opts    *publishOptions
	pre     *verifiedBundle
	distAbs string
	multi   bool

	lockBytes []byte
	lock      *lockfile.File
	src       publish.SourceInfo
	mtime     int64
	repo      string
	approval  *publish.ApprovalInfo
	sbom      []byte
	signer    signing.Signer
	prevLock  []byte
	prevLabel string
	top       string
	repoPath  string
	ctx       context.Context
}

// newPublishContext resolves the lock, the source, the policy gates, the SBOM,
// the signer and the previous release, in that order, so a failed gate stops
// before any signing or network step.
func newPublishContext(ctx context.Context, cfg *config.Config, opts *publishOptions, pre *verifiedBundle, distAbs string, multi bool) (*publishContext, error) {
	pc := &publishContext{cfg: cfg, opts: opts, pre: pre, distAbs: distAbs, multi: multi, ctx: ctx}
	lockPath := lockfile.Path(cfg.ConfigDir)
	if err := publish.CheckTree(filepath.Dir(lockPath), []string{filepath.Base(lockPath)}); err != nil {
		return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "run `ai-rulez lock`", "no usable %s: %v", lockfile.FileName, err)
	}
	raw, err := os.ReadFile(lockPath) //nolint:gosec // the project's own lock file
	if err != nil {
		return nil, oops.With("path", lockPath).Wrapf(err, "read lock file")
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil || lock == nil {
		return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "run `ai-rulez lock`", "cannot read %s", lockfile.FileName)
	}
	pc.lock = lock
	if pc.lockBytes, err = shippedLock(raw, lock); err != nil {
		return nil, err
	}
	distRel := ""
	top := gitutil.New(publishRunner).TopLevel(cfg.BaseDir)
	if top != "" {
		distRel = gitutil.RepoRelative(top, distAbs)
		pc.repoPath = gitutil.RepoRelative(top, cfg.BaseDir)
	}
	pc.src = publish.ReadSource(ctx, publishRunner, cfg.BaseDir, distRel)
	if pc.src.Source.Dirty && !opts.allowDirty {
		return nil, publish.Errorf(publish.CodeSource, publish.ExitGate, "commit the changes (including the generated bundle), or pass --allow-dirty for a throwaway build",
			"the source tree is dirty or has no commit")
	}
	if err := pc.checkBundleTracked(top); err != nil {
		return nil, err
	}
	pluginRepo := ""
	if cfg.Plugin != nil {
		pluginRepo = cfg.Plugin.Repository
	}
	pc.src.Source.Repo = publish.PublicRemote(pluginRepo)
	if pc.src.Source.Repo == "" {
		pc.src.Source.Repo = pc.src.Remote
	}
	if pc.mtime, err = sourceDateEpoch(pc.src.Mtime); err != nil {
		return nil, err
	}
	pc.repo = pc.resolveRepo(pluginRepo)

	if pc.approval, err = approvalGate(cfg, opts.requireApproved); err != nil {
		return nil, err
	}
	if opts.requireSignature && !publishSigning() {
		return nil, publish.Errorf(publish.CodeUnsigned, publish.ExitGate, "sign with --sign-key FILE or --sign-keyless",
			"require_signature is set and the bundle is not being signed")
	}
	if publishWithSBOM {
		if pc.sbom, err = publishSBOM(cfg); err != nil {
			return nil, err
		}
	}
	// A dry run signs nothing: keyless signing would reach Fulcio and write a
	// public Rekor entry, and a key would be read for no output.
	if !publishDryRun {
		if pc.signer, err = publishSigner(ctx, ambient.Env(nil)); err != nil {
			return nil, err
		}
	}
	pc.top = top
	pc.prevLock, pc.prevLabel, err = pc.previousLock(top)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

// checkBundleTracked fails when git does not track a bundle file: a gitignored
// bundle passes the clean-tree check, but the commit a pinned index names would
// not hold it. --allow-dirty turns the failure into a warning.
func (pc *publishContext) checkBundleTracked(top string) error {
	g := gitutil.New(publishRunner)
	if top == "" || !g.IsRepo(pc.cfg.BaseDir) {
		return nil // no repository: the dirty gate already decided
	}
	paths := make([]string, len(pc.pre.files))
	for i, f := range pc.pre.files {
		paths[i] = f.Path
	}
	tracked, err := g.TrackedAmong(pc.cfg.BaseDir, paths)
	if err != nil {
		return nil //nolint:nilerr // a failing query is reported by the source and tree checks
	}
	var missing []string
	for _, p := range paths {
		if !tracked[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if len(missing) > maxPublishListed {
		missing = append(missing[:maxPublishListed:maxPublishListed], fmt.Sprintf("and %d more", len(missing)-maxPublishListed))
	}
	if pc.opts.allowDirty {
		logger.Warn("bundle files are not tracked by git", "files", strings.Join(missing, ", "))
		return nil
	}
	return publish.Errorf(publish.CodeSource, publish.ExitGate, "commit the generated bundle (is it gitignored?), or pass --allow-dirty for a throwaway build",
		"git does not track these bundle files, so the published commit would lack them: %s", strings.Join(missing, ", "))
}

// resolveRepo is OWNER/REPO: --repo, else the github-release table, else the
// plugin's repository, else origin.
func (pc *publishContext) resolveRepo(pluginRepo string) string {
	switch {
	case publishRepo != "":
		return publishRepo
	case pc.opts.ghRepo != "":
		return pc.opts.ghRepo
	}
	if r := publish.RepoFromURL(pluginRepo); r != "" {
		return r
	}
	return publish.RepoFromURL(pc.src.Remote)
}

// previousLockFor is the previous lock of one plugin. A single plugin, and any
// run with --since, share the context's; a multi-plugin release diffs each
// plugin against the closest earlier tag of that plugin (<name>-v*), or has no
// changes section when it has none.
func (pc *publishContext) previousLockFor(spec *pluginSpec) (data []byte, label string) {
	if !pc.multi || publishSince != "" || pc.top == "" {
		return pc.prevLock, pc.prevLabel
	}
	tag := publish.PreviousTagMatching(pc.ctx, publishRunner, pc.cfg.BaseDir, spec.tag, spec.name+"-v*")
	if tag == "" {
		return nil, ""
	}
	lockRel := gitutil.RepoRelative(pc.top, lockfile.Path(pc.cfg.ConfigDir))
	data, found, err := workspace.ReadFileAt(pc.ctx, pc.cfg.BaseDir, tag, lockRel, publishRunner)
	if err != nil || !found {
		return nil, ""
	}
	return data, tag
}

// previousLock reads the lock the release notes diff against: the lock at
// --since (an error when it cannot be read), else at the previous tag (skipped
// quietly when there is none or it held no lock).
func (pc *publishContext) previousLock(top string) (data []byte, label string, err error) {
	lockRel := gitutil.RepoRelative(top, lockfile.Path(pc.cfg.ConfigDir))
	if top == "" || lockRel == "" {
		if publishSince != "" {
			return nil, "", publish.Errorf(publish.CodeConfig, publish.ExitFailed, "", "--since needs a git repository that holds %s", lockfile.FileName)
		}
		return nil, "", nil
	}
	tag := publishSince
	explicit := tag != ""
	if !explicit && !pc.multi {
		tag = publish.PreviousTag(pc.ctx, publishRunner, pc.cfg.BaseDir, pc.defaultTag())
	}
	if tag == "" {
		return nil, "", nil
	}
	data, found, readErr := workspace.ReadFileAt(pc.ctx, pc.cfg.BaseDir, tag, lockRel, publishRunner)
	if readErr != nil {
		if explicit {
			return nil, "", publish.Errorf(publish.CodeConfig, publish.ExitFailed, "pass a tag that contains "+lockRel,
				"cannot read %s at %s for the release notes: %v", lockfile.FileName, tag, readErr)
		}
		logger.Warn("Cannot read the previous lock; release notes will not diff against it", "tag", tag, "error", readErr)
		return nil, "", nil
	}
	if !found {
		if explicit {
			return nil, "", publish.Errorf(publish.CodeConfig, publish.ExitFailed, "pass a tag that contains "+lockRel,
				"cannot read %s at %s for the release notes", lockfile.FileName, tag)
		}
		return nil, "", nil
	}
	return data, tag, nil
}

func (pc *publishContext) defaultTag() string {
	if publishTag != "" {
		return publishTag
	}
	if pc.cfg.Plugin != nil {
		return "v" + pc.cfg.Plugin.Version
	}
	return ""
}

// pluginSpec is one plugin to build.
type pluginSpec struct {
	name, version, description string
	runtimes                   []string
	files                      []publish.File
	tag                        string
	category                   string
	keywords                   []string
}

// singleSpec is the spec of a project with one [plugin] block: its files are
// the bundle of the requested runtimes.
func (pc *publishContext) singleSpec() (*pluginSpec, error) {
	p := pc.cfg.Plugin
	runtimes := p.ResolvedRuntimes()
	files := pc.pre.files
	if len(pc.opts.runtimes) > 0 {
		for _, r := range pc.opts.runtimes {
			if !slices.Contains(runtimes, r) {
				return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "the plugin ships: "+strings.Join(runtimes, ", "),
					"runtime %q is not one of the [plugin] runtimes", r)
			}
		}
		cp, err := pc.filteredFiles(pc.opts.runtimes)
		if err != nil {
			return nil, err
		}
		runtimes, files = pc.opts.runtimes, cp
	}
	spec := &pluginSpec{
		name: p.Name, version: p.Version, description: p.Description, runtimes: runtimes,
		tag: publishTag, category: p.Category, keywords: p.Keywords,
	}
	if spec.tag == "" {
		spec.tag = "v" + p.Version
	}
	for _, f := range files {
		spec.files = append(spec.files, publish.File{Path: f.Path, Data: f.Data, Executable: f.Executable})
	}
	return spec, nil
}

// filteredFiles renders the bundle again with only the requested runtimes. The
// full set was verified against the files on disk; the subset is rendered by the
// same generator from a copy of the configuration.
func (pc *publishContext) filteredFiles(runtimes []string) ([]generator.PluginFile, error) {
	cp := *pc.cfg
	if pc.cfg.Plugin != nil {
		p := *pc.cfg.Plugin
		p.Runtimes = append([]string(nil), runtimes...)
		cp.Plugin = &p
	}
	if m := pc.cfg.Marketplace; m != nil {
		mc := *m
		if m.FromDomains != nil {
			fd := *m.FromDomains
			fd.Runtimes = append([]string(nil), runtimes...)
			mc.FromDomains = &fd
		}
		mc.Plugins = append([]config.MarketplacePlugin(nil), m.Plugins...)
		for i := range mc.Plugins {
			mc.Plugins[i].Runtimes = append([]string(nil), runtimes...)
		}
		cp.Marketplace = &mc
	}
	files, err := generator.NewGenerator(&cp).WithMemberRuntimes(runtimes).PluginFiles(profile)
	if err != nil {
		var memberErr *generator.MemberRuntimeError
		if errors.As(err, &memberErr) {
			return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "pass only runtimes every member ships, or publish the member on its own",
				"--runtime: %s", memberErr.Error())
		}
		return nil, err //nolint:wrapcheck // already contextual
	}
	return files, nil
}

// market is the marketplace identity emitters and pinned indexes use.
func (pc *publishContext) market() pemit.Market {
	m := pemit.Market{}
	if mk := pc.cfg.Marketplace; mk != nil {
		m.Name, m.Description = mk.Name, mk.Description
		if mk.Owner != nil {
			m.OwnerName, m.OwnerEmail = mk.Owner.Name, mk.Owner.Email
		}
	}
	if p := pc.cfg.Plugin; p != nil {
		if m.Name == "" {
			m.Name = p.Name
		}
		if m.Description == "" {
			m.Description = p.Description
		}
		if m.OwnerName == "" && p.Author != nil {
			m.OwnerName, m.OwnerEmail = p.Author.Name, p.Author.Email
		}
	}
	return m
}

// rules are the project's root rules and context files: the plugin bundle
// carries none, and Kiro steering is made of them.
func (pc *publishContext) rules() []pemit.Doc {
	var docs []pemit.Doc
	if pc.cfg.Content == nil {
		return nil
	}
	for _, group := range [][]config.ContentFile{pc.cfg.Content.Rules, pc.cfg.Content.Context} {
		for i := range group {
			f := &group[i]
			act := f.Metadata.ResolveActivation()
			docs = append(docs, pemit.Doc{
				Name: f.Name, Description: act.Description, Body: f.Content,
				Mode: string(act.Mode), Globs: act.Globs,
			})
		}
	}
	return docs
}

func (pc *publishContext) emitRequestFor(tag string) *publish.EmitRequest {
	if len(pc.opts.emitters) == 0 {
		return nil
	}
	base := pemit.Input{
		Version: pc.versionLabel(), Repo: pc.repo, Commit: pc.src.Source.Commit, Tag: tag, LockTree: pc.lock.Tree,
		Market: pc.market(), Rules: pc.rules(),
	}
	if p := pc.cfg.Plugin; p != nil {
		base.Plugins = []pemit.Plugin{{Name: p.Name, Category: p.Category, Keywords: p.Keywords}}
	}
	return &publish.EmitRequest{Names: pc.opts.emitters, Experimental: pc.opts.experimental, Base: base, Options: pc.opts.emitOptions}
}

func (pc *publishContext) versionLabel() string {
	if pc.cfg.Plugin != nil {
		return pc.cfg.Plugin.Version
	}
	return ""
}

// newInput assembles the Build input of one plugin.
func (pc *publishContext) newInput(spec *pluginSpec) (*publish.Input, error) {
	in := &publish.Input{
		Name: spec.name, Version: spec.version, Description: spec.description, AIRulezVersion: Version,
		Runtimes: spec.runtimes, Files: spec.files, Lock: pc.lockBytes, LockVersion: pc.lock.Version, LockTree: pc.lock.Tree,
		Source: pc.src.Source, Mtime: pc.mtime, Target: publishTo, Channel: publishChannel,
		Templates: pc.opts.templates, NPM: pc.opts.npm, SBOM: pc.sbom, Approval: pc.approval,
		PreviousExplicit: publishSince != "", RequireSignature: pc.opts.requireSignature,
		Sign: pc.signCallback(), Repo: pc.repo, Tag: spec.tag,
	}
	in.PreviousLock, in.PreviousLabel = pc.previousLockFor(spec)
	if !pc.multi {
		in.Emit = pc.emitRequestFor(spec.tag)
	}
	if publishTo == publish.TargetOCI {
		in.OCIRepository = pc.opts.ociRepo
		if pc.multi && in.OCIRepository != "" {
			in.OCIRepository += "/" + spec.name
		}
	}
	if publishTo == publish.TargetGitHubRelease && in.Repo == "" {
		return nil, publish.Errorf(publish.CodeTarget, publish.ExitFailed, "pass --repo OWNER/REPO or set [plugin] repository", "cannot tell which repository to release to")
	}
	return in, nil
}

// pinFor builds the pinned-marketplace input for a release ref (the tag), or
// nil without --marketplace. The index is the Claude index of the verified
// bundle, so the claude runtime must be configured.
func (pc *publishContext) pinFor(tag string) (*publish.Pin, error) {
	if !publishMarketplace {
		return nil, nil //nolint:nilnil // no pinned index requested
	}
	index, indexRoot, found := findClaudeIndex(pc.pre.files)
	if !found {
		return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "add the claude runtime to [plugin] runtimes",
			"--marketplace pins the Claude marketplace index, and the bundle has none")
	}
	if pc.repo == "" {
		return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "pass --repo OWNER/REPO or set [plugin] repository", "cannot tell which repository the marketplace is pinned to")
	}
	ref := tag
	if r, ok := pc.opts.channelRefs[publishChannel]; ok && publishChannel != "" {
		ref = r
	}
	if ref == "" {
		return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "pass --tag or set [publish.marketplace] channels", "a multi-plugin marketplace needs a ref to pin to")
	}
	repoPath := pc.repoPath
	if repoPath == "." {
		repoPath = ""
	}
	return &publish.Pin{Index: index, IndexRoot: indexRoot, RepoPath: repoPath, Repo: pc.repo, Ref: ref}, nil
}

// writeEmitOnly writes the files of one emitter to --out (default emit/<name>).
func (pc *publishContext) writeEmitOnly(out interface{ Write([]byte) (int, error) }, d *publish.Dist, name string) error {
	dir := publishEmitOut
	if dir == "" {
		dir = filepath.Join("emit", name)
	}
	prefix := publish.EmitDir + "/" + name + "/"
	var paths []string
	for p := range d.Files {
		if strings.HasPrefix(p, prefix) {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return publish.Errorf(publish.CodeConfig, publish.ExitFailed, "", "emitter %s produced no files", name)
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return publish.Errorf(publish.CodeBundleUnsafe, publish.ExitFailed, "choose an empty --out directory", "%s is not empty", dir)
	}
	warnAll(d.Warnings)
	slices.Sort(paths)
	for _, p := range paths {
		target := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(p, prefix)))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return oops.With("path", target).Wrapf(err, "create output directory")
		}
		if err := os.WriteFile(target, d.Files[p], 0o644); err != nil { //nolint:gosec // emitter output is meant to be shared
			return oops.With("path", target).Wrapf(err, "write emitter file")
		}
		if _, err := out.Write([]byte("wrote " + target + "\n")); err != nil {
			return oops.Wrapf(err, "write result")
		}
	}
	return nil
}

// resolveVerifyTarget turns the argument of `publish verify` into a directory:
// a directory as it is, an OCI reference pulled into a temporary one.
func resolveVerifyTarget(ctx context.Context, target string) (dir string, cleanup func(), err error) {
	noop := func() {}
	if info, serr := os.Stat(target); serr == nil && info.IsDir() {
		return target, noop, nil
	}
	if ref, perr := oci.ParseRepository(target); perr != nil || ref.Reference == "" || !strings.Contains(target, "/") {
		return "", noop, publish.Errorf(publish.CodeVerify, publish.ExitFailed, "pass a dist directory or an OCI reference such as ghcr.io/acme/skills/x@sha256:...",
			"%s is neither a directory nor an OCI reference", target)
	}
	tmp, err := os.MkdirTemp("", "ai-rulez-verify-*")
	if err != nil {
		return "", noop, oops.Wrapf(err, "create temporary directory")
	}
	cleanup = func() { os.RemoveAll(tmp) } //nolint:errcheck // best effort cleanup of our own directory
	digest, err := publish.PullOCI(ctx, oci.Target{Ref: target}, tmp)
	if err != nil {
		cleanup()
		return "", noop, err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	logger.Info("resolved the OCI reference", "ref", target, "digest", digest)
	if publish.OCIRefIsMutable(target) {
		logger.Warn("the reference is a tag, which its owner can move; verify checks the content it pointed at just now. Pin by digest and name a trusted signer (--key, --identity)",
			"pin", digest)
	}
	return tmp, cleanup, nil
}
