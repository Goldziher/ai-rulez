package commands

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// generatedOutputFiles returns the generated files of a configured project for
// --outputs: every file the last run recorded (generator.GeneratedPaths) and
// every file the next one would write, with the bytes a run would write where the
// plan has them (it does not matter whether generate has run) and the file on
// disk for a recorded file the plan no longer writes. A path outside the project
// root, and a file that is not a regular file, is left out. An implicit
// configuration has none: the files that carry the generated header are found
// among its targets.
func generatedOutputFiles(ctx context.Context, cfg *config.Config) []lint.OutputFile {
	if cfg.Implicit {
		return nil
	}
	gen := generator.NewGenerator(cfg)
	gen.SetContext(ctx)
	planned := generator.NewPlannedFiles(cfg)
	seen := map[string]bool{}
	var rels []string
	add := func(rel string) {
		if rel = filepath.ToSlash(rel); rel != "" && rel != "." && !seen[rel] && filepath.IsLocal(filepath.FromSlash(rel)) {
			seen[rel] = true
			rels = append(rels, rel)
		}
	}
	for _, abs := range gen.GeneratedPaths() {
		if rel, err := filepath.Rel(cfg.BaseDir, abs); err == nil {
			add(rel)
		}
	}
	for _, rel := range planned.PlannedPaths() {
		add(rel)
	}
	sort.Strings(rels)
	out := make([]lint.OutputFile, 0, len(rels))
	for _, rel := range rels {
		abs := filepath.Join(cfg.BaseDir, filepath.FromSlash(rel))
		data, ok := planned.Planned(rel)
		if !ok {
			var err error
			if data, _, err = safefs.ReadRegularKeepMode(abs); err != nil {
				continue
			}
		}
		out = append(out, lint.OutputFile{Abs: abs, Data: data})
	}
	return out
}
