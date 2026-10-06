package commands

import (
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

// reviewConfigView returns the configuration whose content tree is the slice --role or --profile
// selects (the same slice `generate` renders), or cfg itself when neither is set. The lint
// evidence is always collected over the whole configuration.
func reviewConfigView(cfg *config.Config) (*config.Config, error) {
	if reviewFlags.role == "" && reviewFlags.profile == "" {
		return cfg, nil
	}
	gen := generator.NewGenerator(cfg)
	var tree *config.ContentTree
	var err error
	if reviewFlags.role != "" {
		tree, err = gen.ContentForRole(reviewFlags.role)
	} else {
		tree, err = gen.ContentForProfile(reviewFlags.profile)
	}
	if err != nil {
		return nil, oops.Wrapf(err, "select the content to review")
	}
	view := *cfg
	view.Content = tree
	return &view, nil
}

// reviewSince lists the items changed since --since (nil when the flag is off).
func reviewSince(cfg *config.Config, items []rv.Item) (map[string]bool, error) {
	if reviewFlags.since == "" {
		return nil, nil
	}
	changed, err := gitutil.ChangedSince(cfg.BaseDir, reviewFlags.since)
	if err != nil {
		return nil, oops.Wrapf(err, "list the files changed since %s", reviewFlags.since)
	}
	baseRel := ""
	if top := gitutil.TopLevel(cfg.BaseDir); top != "" {
		base := cfg.BaseDir
		if resolved, err := filepath.EvalSymlinks(base); err == nil {
			base = resolved
		}
		if resolvedTop, err := filepath.EvalSymlinks(top); err == nil {
			top = resolvedTop
		}
		if r, rerr := filepath.Rel(top, base); rerr == nil && r != "." {
			baseRel = filepath.ToSlash(r)
		}
	}
	return rv.ChangedItems(items, changed, baseRel), nil
}
