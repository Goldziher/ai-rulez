package commands

import (
	"context"
	"path/filepath"
	"slices"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// exposureLintSource is the session exposure validate hands the exposure rules (lint.ExposureSource): the
// inventory of the project root classified with the [exposure] area map. areas, areasFile and plugins replace
// what the table configures (`exposure leakage --areas`, `--plugins`); cache, when set, keeps the area map and
// the inventories the rules share in one run.
type exposureLintSource struct {
	ctx       context.Context
	cfg       *config.Config
	areas     *areas.Map
	areasFile string
	plugins   exposure.PluginMode
	cache     *exposureCache
}

// exposureCache holds what an exposureLintSource computed, for the rules of one run.
type exposureCache struct {
	mu   sync.Mutex
	invs map[exposure.PluginMode]*exposure.Inventory
}

// exposureLintOptions hands the exposure rules the project's session exposure when it has an [exposure] table.
// The plugin analyzer reads it too: AR964 claude-skill-listing-total is one of its rows.
func exposureLintOptions(ctx context.Context, cfg *config.Config, sel []string) []lint.Option {
	if cfg == nil || cfg.Exposure == nil || !lint.AnalyzerSelected(sel, lint.AnalyzerExposure, lint.AnalyzerPlugin) {
		return nil
	}
	return []lint.Option{lint.WithExposure(exposureLintSource{ctx: ctx, cfg: cfg, cache: &exposureCache{}})}
}

// root is the absolute project root.
func (s exposureLintSource) root() (string, error) {
	root, err := filepath.Abs(s.cfg.BaseDir)
	return root, err //nolint:wrapcheck // the rules stay silent on an error
}

// areaMap is the area map and the absolute path of its file ("" for the inline form); nil when there is none or
// it has problems, which AR9W5 reports.
func (s exposureLintSource) areaMap(root string) (m *areas.Map, file string) {
	if s.areas != nil {
		return s.areas, s.areasFile
	}
	exp := s.cfg.Exposure
	m, problems := areas.Load(exp, root)
	if m == nil || len(problems) > 0 {
		return nil, ""
	}
	if exp.AreasFile != "" {
		file = filepath.Join(root, filepath.FromSlash(exp.AreasFile))
	}
	return m, file
}

// mode is the plugin mode of the inventory: --plugins, else [exposure] plugins, else enabled.
func (s exposureLintSource) mode() exposure.PluginMode {
	if s.plugins != "" {
		return s.plugins
	}
	return exposure.PluginMode(s.cfg.Exposure.EffectivePlugins())
}

// inventory builds the inventory of root in mode, once per mode when the source has a cache.
func (s exposureLintSource) inventory(root string, mode exposure.PluginMode) (*exposure.Inventory, error) {
	if s.cache == nil {
		return exposure.Build(s.ctx, exposure.Options{Root: root, Plugins: mode}) //nolint:wrapcheck // already contextual
	}
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	if inv, ok := s.cache.invs[mode]; ok {
		return inv, nil
	}
	inv, err := exposure.Build(s.ctx, exposure.Options{Root: root, Plugins: mode})
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	if s.cache.invs == nil {
		s.cache.invs = map[exposure.PluginMode]*exposure.Inventory{}
	}
	s.cache.invs[mode] = inv
	return inv, nil
}

// Leakage implements lint.ExposureSource: the leaks of the start-of-session artifacts (exposure.Leaks over
// Inventory.Start, which fails closed) and the skill_areas keys that name no skill. A key that misses under the
// plugin mode is looked up again among the skills of every marketplace plugin, so a plugin that is not enabled
// still places its skills.
func (s exposureLintSource) Leakage() (lint.ExposureLeakage, bool) {
	root, err := s.root()
	if err != nil {
		return lint.ExposureLeakage{}, false
	}
	m, file := s.areaMap(root)
	if m == nil {
		return lint.ExposureLeakage{}, false
	}
	mode := s.mode()
	inv, err := s.inventory(root, mode)
	if err != nil {
		return lint.ExposureLeakage{}, false
	}
	start := inv.Start()
	out := lint.ExposureLeakage{Checked: len(start), AreasFile: file, Inputs: s.leakageInputs(root, file)}
	for _, l := range exposure.Leaks(start, m) {
		out.Leaks = append(out.Leaks, lint.ExposureLeak{
			Ambiguous: l.Severity == exposure.LeakWarning, Area: l.Area, Term: l.Term, Kind: string(l.Kind), Name: l.Name,
			Path: l.Path, Scope: l.Scope, Line: l.Line, Excerpt: l.Excerpt, Message: l.Message(),
		})
	}
	skillAreas := m.SkillAreas()
	unknown := exposure.UnknownSkillAreas(skillAreas, inv)
	if len(unknown) > 0 && mode != exposure.PluginsAll {
		if all, err := s.inventory(root, exposure.PluginsAll); err == nil {
			unknown = exposure.UnknownSkillAreas(skillAreas, all)
		}
	}
	for _, key := range unknown {
		out.UnknownSkills = append(out.UnknownSkills, lint.ExposureSkillArea{Skill: key, Area: skillAreas[key]})
	}
	return out, true
}

// leakageInputs are the absolute paths of the files besides the artifacts that decide the leaks: the areas
// file, the configuration file, and the root settings and marketplace.
func (s exposureLintSource) leakageInputs(root, areasFile string) []string {
	var out []string
	if areasFile != "" {
		out = append(out, areasFile)
	}
	if s.cfg.ConfigFile != "" {
		if p, err := filepath.Abs(s.cfg.ConfigPath()); err == nil {
			out = append(out, p)
		}
	}
	for _, rel := range exposure.StartInputs() {
		out = append(out, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return out
}

// StartExposure implements lint.ExposureSource: the start-of-session report of `ai-rulez exposure` with the
// [exposure] context_window; nil when the inventory cannot be built.
func (s exposureLintSource) StartExposure() *lint.ExposureStart {
	root, err := s.root()
	if err != nil {
		return nil
	}
	inv, err := s.inventory(root, s.mode())
	if err != nil {
		logger.Warn("Skipped the session exposure checks", "error", err)
		return nil
	}
	rep := exposure.NewReport(inv, exposure.ReportOptions{ContextWindow: s.cfg.Exposure.EffectiveContextWindow(), StartBudget: s.cfg.Exposure.StartBudget})
	return &lint.ExposureStart{
		ContextWindow: rep.Listing.ContextWindow, StartArtifacts: rep.StartArtifacts(), StartTokens: rep.StartTokens,
		ListingSkills: rep.Listing.Skills, ListingChars: rep.Listing.Chars, ListingBudgetChars: rep.Listing.BudgetChars,
	}
}

// Collisions implements lint.ExposureSource. A map with problems, or an inventory that cannot be built, gives
// none: AR9W5 reports the map. The shared inventory is classified on a copy.
func (s exposureLintSource) Collisions(minScore float64) []lint.ExposureCollision {
	root, err := s.root()
	if err != nil {
		return nil
	}
	m, _ := s.areaMap(root)
	if m == nil {
		return nil
	}
	inv, err := s.inventory(root, s.mode())
	if err != nil {
		return nil
	}
	classified := *inv
	classified.Artifacts = slices.Clone(inv.Artifacts)
	classified.Classify(m)
	_, found := exposure.FindCollisions(classified.Artifacts, minScore)
	out := make([]lint.ExposureCollision, 0, len(found))
	for i := range found {
		c := &found[i]
		out = append(out, lint.ExposureCollision{
			Score: c.Score, Shared: c.Shared,
			First: c.First, FirstKind: string(c.FirstKind), FirstName: c.FirstName, FirstArea: c.FirstArea,
			Second: c.Second, SecondKind: string(c.SecondKind), SecondName: c.SecondName, SecondArea: c.SecondArea,
		})
	}
	return out
}
