package commands

import (
	"context"
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// keepsOKFIndexes reports whether the configuration directory is an OKF bundle
// whose index.md files ai-rulez maintains (a generated root index.md, see
// "migrate okf"). A project on the agents source, or one that was never migrated,
// has none.
func keepsOKFIndexes(cfg *config.Config) bool {
	return cfg != nil && cfg.ConfigDir != "" && !cfg.UsesAgentsSource() && okfbridge.IsBundleDir(cfg.ConfigDir)
}

// refreshOKFIndexes rewrites the index.md files of an OKF configuration directory
// so the titles and descriptions they list follow the content, as the add and
// remove commands already do. It is part of every write of `generate`, so the
// edit, generate, commit loop leaves nothing for `migrate okf --check` to report.
// It returns the index files (absolute paths) the run owns, which the caller
// records in the generated manifest (#533).
func refreshOKFIndexes(ctx context.Context, cfg *config.Config) ([]string, error) {
	if !keepsOKFIndexes(cfg) {
		return nil, nil
	}
	rel, err := okfbridge.RefreshIndexesFilesFrom(ctx, cfg.ConfigDir, cfg.Content)
	if err != nil {
		return nil, oops.Hint("Run 'ai-rulez migrate okf' to rebuild the index.md files.").Wrapf(err, "refresh the OKF index.md files")
	}
	abs := make([]string, 0, len(rel))
	for _, p := range rel {
		abs = append(abs, filepath.Join(cfg.ConfigDir, filepath.FromSlash(p)))
	}
	return abs, nil
}

// okfIndexPaths lists the index files refreshOKFIndexes owns, without writing:
// the drift check marks them as current outputs so they are not reported stale.
func okfIndexPaths(ctx context.Context, cfg *config.Config) ([]string, error) {
	if !keepsOKFIndexes(cfg) {
		return nil, nil
	}
	rel, err := okfbridge.IndexPathsFrom(ctx, cfg.ConfigDir, cfg.Content)
	if err != nil {
		return nil, oops.Wrapf(err, "list the OKF index.md files")
	}
	abs := make([]string, 0, len(rel))
	for _, p := range rel {
		abs = append(abs, filepath.Join(cfg.ConfigDir, filepath.FromSlash(p)))
	}
	return abs, nil
}

// staleOKFIndexes lists the index.md files of an OKF configuration directory that
// no longer match its content, as paths relative to the configuration directory.
// It is the index check of `migrate okf --check`.
func staleOKFIndexes(ctx context.Context, cfg *config.Config) ([]string, error) {
	if !keepsOKFIndexes(cfg) {
		return nil, nil
	}
	stale, err := okfbridge.StaleIndexesFrom(ctx, cfg.ConfigDir, cfg.Content)
	if err != nil {
		return nil, oops.Wrapf(err, "check the OKF index.md files")
	}
	return stale, nil
}

// staleOKFIndexDrift reports the stale indexes of a configuration directory as
// `generate --check` drift items.
func staleOKFIndexDrift(ctx context.Context, cfg *config.Config) ([]driftItem, error) {
	stale, err := staleOKFIndexes(ctx, cfg)
	if err != nil {
		return nil, err
	}
	items := make([]driftItem, 0, len(stale))
	for _, rel := range stale {
		abs := filepath.Join(cfg.ConfigDir, filepath.FromSlash(rel))
		items = append(items, driftItem{
			Kind: string(generator.DriftStale),
			Path: displayDriftAbs(abs, abs),
			Note: "lists an old title or description; run `ai-rulez generate`",
		})
	}
	return items, nil
}
