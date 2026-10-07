package presets

import (
	"path"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// OKF preset: keeps an Open Knowledge Format bundle (docs/okf by default) in
// sync with the sources. The bundle is committed documentation, so it is never
// gitignored, and every file is written verbatim: a generated-by banner would
// break the frontmatter that OKF requires at the start of each concept.
// See docs/okf.md.

// OKFPresetGenerator writes the OKF bundle.
type OKFPresetGenerator struct{}

func (g *OKFPresetGenerator) GetName() string { return config.PresetOKF }

func (g *OKFPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{filepath.Join(baseDir, filepath.FromSlash(config.DefaultOKFDir))}
}

func (g *OKFPresetGenerator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	// A monorepo scope run writes scoped instruction files only; the bundle
	// belongs to the project root.
	if rulefiles.InScope(cfg) {
		return nil, nil
	}
	// A role renders one person's slice for their machine; the committed bundle
	// documents the whole project and must not shrink to that slice.
	if cfg.RoleActive() {
		return nil, nil
	}
	kinds, err := okfbridge.ParseKinds(cfg.OKFInclude())
	if err != nil {
		return nil, err
	}
	res, err := okfbridge.Export(content, okfbridge.ExportOptions{Include: kinds, IndexStyle: cfg.OKFIndexStyle(), LocalDir: cfg.ConfigDir})
	if err != nil {
		return nil, err
	}
	rel := cfg.OKFDir()
	root := filepath.Join(baseDir, filepath.FromSlash(rel))

	dirs := map[string]bool{}
	for d := rel; d != "." && d != "/" && d != ""; d = path.Dir(d) {
		dirs[d] = true
	}
	for _, f := range res.Files {
		for d := path.Join(rel, path.Dir(f.Path)); d != "." && d != rel; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	names := make([]string, 0, len(dirs))
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)
	outputs := make([]config.OutputFile, 0, len(names)+len(res.Files))
	for _, d := range names {
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(baseDir, filepath.FromSlash(d)), IsDir: true})
	}
	for _, f := range res.Files {
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		outputs = append(outputs, config.OutputFile{
			Path:       filepath.Join(root, filepath.FromSlash(f.Path)),
			RawContent: append([]byte{}, f.Data...),
			Mode:       mode,
			Committed:  true,
		})
	}
	return outputs, nil
}
