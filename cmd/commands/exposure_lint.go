package commands

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// exposureLintSource is the session exposure validate hands the exposure rules
// (lint.ExposureSource): the inventory of the project root with the [exposure]
// plugins, built once and shared by every rule that reads it.
type exposureLintSource struct {
	ctx context.Context
	cfg *config.Config

	once sync.Once
	inv  *exposure.Inventory
}

// exposureLintOptions hands the exposure rules the project's session exposure
// when it has an [exposure] table. The plugin analyzer reads it too: AR964
// claude-skill-listing-total is one of its rows.
func exposureLintOptions(ctx context.Context, cfg *config.Config, sel []string) []lint.Option {
	if cfg == nil || cfg.Exposure == nil || !lint.AnalyzerSelected(sel, lint.AnalyzerExposure, lint.AnalyzerPlugin) {
		return nil
	}
	return []lint.Option{lint.WithExposure(&exposureLintSource{ctx: ctx, cfg: cfg})}
}

// inventory is the project's inventory, nil when it cannot be built.
func (s *exposureLintSource) inventory() *exposure.Inventory {
	s.once.Do(func() {
		root, err := filepath.Abs(s.cfg.BaseDir)
		if err == nil {
			s.inv, err = exposure.Build(s.ctx, exposure.Options{Root: root, Plugins: exposure.PluginMode(s.cfg.Exposure.EffectivePlugins())})
		}
		if err != nil {
			logger.Warn("Skipped the session exposure checks", "error", err)
			s.inv = nil
		}
	})
	return s.inv
}

// StartExposure implements lint.ExposureSource: the start-of-session report of
// `ai-rulez exposure` with the [exposure] context_window.
func (s *exposureLintSource) StartExposure() *lint.ExposureStart {
	inv := s.inventory()
	if inv == nil {
		return nil
	}
	rep := exposure.NewReport(inv, exposure.ReportOptions{ContextWindow: s.cfg.Exposure.EffectiveContextWindow(), StartBudget: s.cfg.Exposure.StartBudget})
	return &lint.ExposureStart{
		ContextWindow: rep.Listing.ContextWindow, StartArtifacts: rep.StartArtifacts(), StartTokens: rep.StartTokens,
		ListingSkills: rep.Listing.Skills, ListingChars: rep.Listing.Chars, ListingBudgetChars: rep.Listing.BudgetChars,
	}
}
