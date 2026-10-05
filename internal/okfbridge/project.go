package okfbridge

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

// ProjectResult is the outcome of linting a project's configured OKF bundle.
type ProjectResult struct {
	// Dir is the absolute bundle directory.
	Dir      string
	Findings []okf.Finding
}

// Configured reports whether the project asks for an OKF bundle: the okf
// preset is on, or an [okf] section names a directory.
func Configured(cfg *config.Config) bool {
	return cfg != nil && (cfg.OKFEnabled() || (cfg.OKF != nil && cfg.OKF.Dir != ""))
}

// CheckProject validates the project's bundle. When tree is non-nil and the okf
// preset manages the bundle, it also reports AR9B5 for every file that differs
// from what an export would write now. A bundle that does not exist yet is only
// a finding when the preset is expected to have written it.
func CheckProject(cfg *config.Config, tree *config.ContentTree) (*ProjectResult, error) {
	if !Configured(cfg) {
		return nil, nil
	}
	dir := filepath.Join(cfg.BaseDir, filepath.FromSlash(cfg.OKFDir()))
	res := &ProjectResult{Dir: dir}
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist) || (err == nil && !info.IsDir()):
		if cfg.OKFEnabled() && tree != nil {
			res.Findings = append(res.Findings, okf.NewFinding(okf.CodeExportDrift, "", 0,
				"the bundle directory %s does not exist; run `ai-rulez generate`", cfg.OKFDir()))
		}
		return res, nil
	case err != nil:
		return nil, err
	}
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		return nil, err
	}
	res.Findings = b.Validate()
	if tree == nil || !cfg.OKFEnabled() {
		return res, nil
	}
	drift, err := driftFindings(cfg, tree, dir)
	if err != nil {
		return nil, err
	}
	res.Findings = append(res.Findings, drift...)
	return res, nil
}

// driftFindings compares the bundle on disk with a fresh export.
func driftFindings(cfg *config.Config, tree *config.ContentTree, dir string) ([]okf.Finding, error) {
	kinds, err := ParseKinds(cfg.OKFInclude())
	if err != nil {
		return nil, err
	}
	exp, err := Export(tree, ExportOptions{Include: kinds})
	if err != nil {
		return nil, err
	}
	drift, err := okf.Compare(dir, exp.Files)
	if err != nil {
		return nil, err
	}
	var out []okf.Finding
	for _, g := range []struct {
		what  string
		paths []string
	}{{"is missing", drift.Missing}, {"differs from the sources", drift.Changed}, {"is not produced by the export", drift.Extra}} {
		for _, p := range g.paths {
			out = append(out, okf.NewFinding(okf.CodeExportDrift, p, 0,
				"%s %s; run `ai-rulez generate` (or `ai-rulez export okf`)", p, g.what))
		}
	}
	return out, nil
}
