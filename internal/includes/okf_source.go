package includes

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/okf"
	"github.com/Goldziher/ai-rulez/internal/okfbridge"
	"github.com/samber/oops"
)

// OKFScan scans the text an OKF include converts to, before it is used. The
// caller wires it to the security scan (lint imports this package, so it cannot
// be called from here). nil skips the scan.
var OKFScan okfbridge.Scanner

// OKFSource reads an Open Knowledge Format bundle (a local directory or a git
// repository) as an include: the bundle is converted to .ai-rulez sources in a
// temporary directory, loaded like any other include, and discarded.
type OKFSource struct {
	name    string
	source  string
	subdir  string
	ref     string
	baseDir string
	include []string
}

func (r *Resolver) createOKFSource(c *config.IncludeConfig) *OKFSource {
	source := c.Source
	if c.LocalOverride != "" {
		if p := r.resolveLocalOverride(c); p != "" {
			source = p
		}
	}
	return &OKFSource{name: c.Name, source: source, subdir: c.Path, ref: c.Ref, baseDir: r.baseDir, include: c.Include}
}

// GetType returns the type of the underlying location.
func (s *OKFSource) GetType() SourceType { return DetectSourceType(s.source) }

// GetName returns the include name.
func (s *OKFSource) GetName() string { return s.name }

// Fetch converts the bundle and loads the result.
func (s *OKFSource) Fetch(ctx context.Context) (*config.ContentTree, error) {
	spec := s.source
	if DetectSourceType(spec) == SourceTypeLocal {
		if !filepath.IsAbs(spec) {
			spec = filepath.Join(s.baseDir, spec)
		}
		if s.subdir != "" {
			spec = filepath.Join(spec, filepath.FromSlash(strings.TrimPrefix(s.subdir, "/")))
		}
	} else {
		if s.ref != "" {
			spec += "@" + s.ref
		}
		if s.subdir != "" {
			spec += "#" + strings.TrimPrefix(s.subdir, "/")
		}
	}
	src, err := okfbridge.ParseSource(spec)
	if err != nil {
		return nil, oops.With("include", s.name).Wrapf(err, "parse OKF include source")
	}
	dir, cleanup, err := src.Fetch(ctx)
	if err != nil {
		return nil, oops.With("include", s.name).Wrapf(err, "fetch OKF bundle")
	}
	defer cleanup()
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		return nil, oops.With("include", s.name).Wrapf(err, "read OKF bundle")
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
	if _, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: target, Scan: OKFScan}); err != nil {
		return nil, oops.With("include", s.name).Wrapf(err, "convert OKF bundle")
	}
	tree, err := config.ScanContentTree(target)
	if err != nil {
		return nil, oops.With("include", s.name).Wrapf(err, "scan converted OKF bundle")
	}
	if len(s.include) > 0 {
		tree = (&LocalSource{include: s.include}).filterContent(tree)
	}
	return tree, nil
}
