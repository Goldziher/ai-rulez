package okfbridge

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"gopkg.in/yaml.v3"
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
	res.Findings = projectFindings(cfg, b)
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
	exp, err := Export(tree, ExportOptions{Include: kinds, IndexStyle: cfg.OKFIndexStyle()})
	if err != nil {
		return nil, err
	}
	drift, err := okf.Compare(dir, exp.Files)
	if err != nil {
		return nil, err
	}
	var out []okf.Finding
	want := map[string][]byte{}
	for _, f := range exp.Files {
		want[f.Path] = f.Data
	}
	for _, p := range drift.Changed {
		have, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if readErr != nil {
			continue
		}
		if haveTitle, wantTitle, ok := titleOnlyDiffers(have, want[p]); ok {
			out = append(out, okf.NewFinding(okf.CodeExportDrift, p, 0,
				"the title was edited in the bundle (%q, the sources say %q); the sources win: run `ai-rulez generate` to restore it, or `ai-rulez import okf %s --force` to adopt the edit",
				haveTitle, wantTitle, cfg.OKFDir()))
		}
	}
	titled := map[string]bool{}
	for i := range out {
		titled[out[i].Path] = true
	}
	for _, g := range []struct {
		what  string
		paths []string
	}{{"is missing", drift.Missing}, {"differs from the sources", drift.Changed}, {"is not produced by the export", drift.Extra}} {
		for _, p := range g.paths {
			if titled[p] {
				continue
			}
			out = append(out, okf.NewFinding(okf.CodeExportDrift, p, 0,
				"%s %s; run `ai-rulez generate` (or `ai-rulez export okf`)", p, g.what))
		}
	}
	return out, nil
}

// projectFindings validates the bundle. The AR9B3 note that an index uses the
// frontmatter style is dropped when okf.index_style asks for it.
func projectFindings(cfg *config.Config, b *okf.Bundle) []okf.Finding {
	all := b.Validate()
	if cfg.OKFIndexStyle() != okf.StyleFrontmatter {
		return all
	}
	out := all[:0]
	for _, f := range all {
		if f.Code == okf.CodeVersionInvalid && f.Severity == okf.SeverityInfo && strings.Contains(f.Message, "frontmatter style") {
			continue
		}
		out = append(out, f)
	}
	return out
}

// titleOnlyDiffers reports whether two versions of a concept file differ in
// their title and nothing else (same other frontmatter keys, same body).
func titleOnlyDiffers(have, want []byte) (haveTitle, wantTitle string, ok bool) {
	h, hBody := okf.SplitFrontmatter(have)
	w, wBody := okf.SplitFrontmatter(want)
	if h.Root == nil || w.Root == nil || hBody != wBody || h.Scalar("title") == w.Scalar("title") {
		return "", "", false
	}
	if withoutTitle(h) != withoutTitle(w) {
		return "", "", false
	}
	return h.Scalar("title"), w.Scalar("title"), true
}

func withoutTitle(fm okf.Frontmatter) string {
	clone := &yaml.Node{Kind: yaml.MappingNode, Tag: yamlMapTag}
	for i := 0; i+1 < len(fm.Root.Content); i += 2 {
		if fm.Root.Content[i].Value != "title" {
			clone.Content = append(clone.Content, fm.Root.Content[i], fm.Root.Content[i+1])
		}
	}
	out, err := yaml.Marshal(clone)
	if err != nil {
		return ""
	}
	return string(out)
}
