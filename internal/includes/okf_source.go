package includes

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/samber/oops"
)

// OKFSource reads an Open Knowledge Format bundle in a local directory as an
// include: the bundle is converted to .ai-rulez sources in a temporary
// directory, loaded like any other include, and discarded. A bundle in a git
// repository is a GitSource in OKF mode instead, so it is cached and pinned in
// ai-rulez.lock like every other remote include.
type OKFSource struct {
	name    string
	dir     string
	include []string
	// scan runs the security scan over the converted text before it is used.
	// The resolver supplies it per load; nil skips the scan (tests only).
	scan okfbridge.Scanner
}

func (r *Resolver) createOKFSource(ctx context.Context, c *config.IncludeConfig) (Source, error) {
	source := c.Source
	if c.LocalOverride != "" && !refreshing(r.cfg, lockfile.KindInclude, c.Name) {
		if err := checkLocalOverride(r.cfg, "includes", c.Name); err != nil {
			return nil, err
		}
		p := r.resolveLocalOverride(c)
		if p == "" {
			logger.FromContext(ctx).Info("Skipping include (local_override path not found)", "name", c.Name, "local_override", c.LocalOverride)
			return nil, nil
		}
		return &OKFSource{name: c.Name, dir: p, include: c.Include, scan: r.okfScan}, nil
	}
	if DetectSourceType(source) == SourceTypeGit {
		w := withVersion(lockfile.Want{
			Kind: lockfile.KindInclude, Name: c.Name, Source: lockSource(r.baseDir, source), Path: c.Path, Ref: c.Ref,
		}, c.VersionSpec())
		p, err := pinFor(r.cfg, r.lock, w)
		if err != nil {
			return nil, err
		}
		ref, err := versionRef(withHardenedGit(ctx), r.cfg, r.lock, w, p, source, r.accessToken, r.baseDir)
		if err != nil {
			return nil, err
		}
		src, err := NewOKFGitSourceIn(r.host(), c.Name, source, c.Path, ref, r.baseDir, c.Include, r.accessToken)
		if err != nil {
			return nil, oops.Wrapf(err, "failed to create git source for OKF include '%s'", c.Name)
		}
		src.pin = p
		src.okfScan = r.okfScan
		src.state = stateFor(r.cfg)
		return src, nil
	}
	dir := source
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.baseDir, dir)
	}
	if c.Path != "" {
		dir = filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(c.Path, "/")))
	}
	return &OKFSource{name: c.Name, dir: dir, include: c.Include, scan: r.okfScan}, nil
}

// GetType returns the source type.
func (s *OKFSource) GetType() SourceType { return SourceTypeLocal }

// GetName returns the include name.
func (s *OKFSource) GetName() string { return s.name }

// Fetch converts the bundle and loads the result.
func (s *OKFSource) Fetch(ctx context.Context) (*config.ContentTree, error) {
	return convertOKFBundle(ctx, s.dir, s.name, s.include, s.scan)
}

// convertOKFBundle reads the OKF bundle in dir and converts it to a content
// tree through a temporary .ai-rulez directory, applying the include filter.
func convertOKFBundle(ctx context.Context, dir, name string, include []string, scan okfbridge.Scanner) (*config.ContentTree, error) {
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		return nil, oops.With("include", name).Wrapf(err, "read OKF bundle")
	}
	tmp, err := os.MkdirTemp("", "ai-rulez-okf-include-*")
	if err != nil {
		return nil, oops.Wrapf(err, "create temp directory")
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // best-effort cleanup
	target := filepath.Join(tmp, ".ai-rulez")
	if err := os.MkdirAll(target, 0o755); err != nil { //nolint:gosec // temp dir
		return nil, oops.Wrapf(err, "create temp directory")
	}
	_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: target, Scan: scan})
	if err != nil {
		return nil, oops.With("include", name).Wrapf(err, "convert OKF bundle")
	}
	for _, p := range b.Problems {
		logger.FromContext(ctx).Warn("OKF include skipped an unsafe bundle path", "include", name, "path", p.Path, "reason", p.Message)
	}
	tree, err := config.ScanContentTreeContext(ctx, target)
	if err != nil {
		return nil, oops.With("include", name).Wrapf(err, "scan converted OKF bundle")
	}
	if len(include) > 0 {
		tree = (&LocalSource{include: include}).filterContent(tree)
	}
	return tree, nil
}
