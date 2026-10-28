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
// inventory of the project root, or of dir (`exposure leakage --root`), classified with the [exposure] area map.
// areas, areasFile and plugins replace what the table configures (`exposure leakage --areas`, `--plugins`);
// cache, when set, keeps the inventories the rules share in one run.
type exposureLintSource struct {
	ctx context.Context
	cfg *config.Config
	// dir is the absolute directory to inventory, "" for the project root (the configuration's base).
	dir       string
	areas     *areas.Map
	areasFile string
	plugins   exposure.PluginMode
	cache     *exposureCache
}

// exposureCache holds the inventories an exposureLintSource built, by plugin mode, for the rules of one run.
// The key is the mode alone because the source that owns the cache fixes the directory and the overrides; a
// cache shared across sources would need them in the key. The area map is not cached: each rule that reads it
// loads it again (AR9W5, the leakage and collision rules), which costs an areas_file read each.
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

// base is the absolute project root, which the [exposure] areas_file and the path claims are relative to.
func (s exposureLintSource) base() (string, error) {
	base, err := filepath.Abs(s.cfg.BaseDir)
	return base, err //nolint:wrapcheck // the rules stay silent on an error
}

// inventoryRoot is the absolute directory the inventory is of: dir, else the project root.
func (s exposureLintSource) inventoryRoot() (string, error) {
	if s.dir != "" {
		return s.dir, nil
	}
	return s.base()
}

// Root implements lint.ExposureSource: the directory the inventory is of, "" when it cannot be resolved.
func (s exposureLintSource) Root() string {
	root, err := s.inventoryRoot()
	if err != nil {
		return ""
	}
	return root
}

// areaMap is the area map and the absolute path of its file ("" for the inline form); nil when there is none or
// it has problems, which AR9W5 reports.
func (s exposureLintSource) areaMap() (m *areas.Map, file string) {
	if s.areas != nil {
		return s.areas, s.areasFile
	}
	base, err := s.base()
	if err != nil {
		return nil, ""
	}
	exp := s.cfg.Exposure
	m, problems := areas.Load(exp, base)
	if m == nil || len(problems) > 0 {
		return nil, ""
	}
	return m, s.configuredAreasFile(base)
}

// configuredAreasFile is the absolute path of the [exposure] areas_file, "" when the map is inline.
func (s exposureLintSource) configuredAreasFile(base string) string {
	if s.cfg.Exposure == nil || s.cfg.Exposure.AreasFile == "" {
		return ""
	}
	return filepath.Join(base, filepath.FromSlash(s.cfg.Exposure.AreasFile))
}

// mode is the plugin mode of the inventory: --plugins, else [exposure] plugins, else enabled.
func (s exposureLintSource) mode() exposure.PluginMode {
	if s.plugins != "" {
		return s.plugins
	}
	return exposure.PluginMode(s.cfg.Exposure.EffectivePlugins())
}

// inventory builds the inventory of the source's directory in mode, once per mode when the source has a cache.
// The path claims of the area map read the scopes relative to the project root.
func (s exposureLintSource) inventory(mode exposure.PluginMode) (*exposure.Inventory, error) {
	root, err := s.inventoryRoot()
	if err != nil {
		return nil, err
	}
	base, err := s.base()
	if err != nil {
		return nil, err
	}
	opts := exposure.Options{Root: root, AreaBase: base, Plugins: mode}
	if s.cache == nil {
		return exposure.Build(s.ctx, opts) //nolint:wrapcheck // already contextual
	}
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	if inv, ok := s.cache.invs[mode]; ok {
		return inv, nil
	}
	inv, err := exposure.Build(s.ctx, opts)
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
// Inventory.Start, which fails closed), the start-of-session artifacts kept unread, whose text no rule can
// check, and the skill_areas keys that name no skill. A key that misses under the plugin mode is looked up
// again among the skills of every marketplace plugin, so a plugin that is not enabled still places its skills.
func (s exposureLintSource) Leakage() (lint.ExposureLeakage, bool) {
	m, file := s.areaMap()
	if m == nil {
		return lint.ExposureLeakage{}, false
	}
	mode := s.mode()
	inv, err := s.inventory(mode)
	if err != nil {
		logger.Warn("Skipped the session exposure leakage checks", "error", err)
		return lint.ExposureLeakage{}, false
	}
	start := inv.Start()
	out := lint.ExposureLeakage{Checked: len(start), AreasFile: file}
	leaks := exposure.Leaks(start, m)
	for i := range leaks {
		l := &leaks[i]
		out.Leaks = append(out.Leaks, lint.ExposureLeak{
			Ambiguous: l.Severity == exposure.LeakWarning, Area: l.Area, Term: l.Term, Kind: string(l.Kind), Name: l.Name,
			Path: l.Path, Scope: l.Scope, Line: l.Line, Excerpt: l.Excerpt, Message: l.Message(),
		})
	}
	for _, u := range exposure.UncheckedArtifacts(start) {
		out.Unchecked = append(out.Unchecked, lint.ExposureUnchecked{Path: u.Path, Kind: string(u.Kind), Name: u.Name, Reason: u.Reason})
	}
	skillAreas := m.SkillAreas()
	unknown := exposure.UnknownSkillAreas(skillAreas, inv)
	if len(unknown) > 0 && mode != exposure.PluginsAll {
		if all, err := s.inventory(exposure.PluginsAll); err == nil {
			unknown = exposure.UnknownSkillAreas(skillAreas, all)
		}
	}
	for _, key := range unknown {
		out.UnknownSkills = append(out.UnknownSkills, lint.ExposureSkillArea{Skill: key, Area: skillAreas[key]})
	}
	return out, true
}

// Inputs implements lint.ExposureSource: the absolute paths of the files besides the artifacts that decide
// the exposure: the areas file, the configuration file, and the root settings and marketplace of the
// inventory's directory.
func (s exposureLintSource) Inputs() []string {
	var out []string
	switch {
	case s.areas != nil:
		if s.areasFile != "" {
			out = append(out, s.areasFile)
		}
	default:
		if base, err := s.base(); err == nil {
			if file := s.configuredAreasFile(base); file != "" {
				out = append(out, file)
			}
		}
	}
	if s.cfg.ConfigFile != "" {
		if p, err := filepath.Abs(s.cfg.ConfigPath()); err == nil {
			out = append(out, p)
		}
	}
	if root, err := s.inventoryRoot(); err == nil {
		for _, rel := range exposure.StartInputs() {
			out = append(out, filepath.Join(root, filepath.FromSlash(rel)))
		}
	}
	return out
}

// StartExposure implements lint.ExposureSource: the start-of-session report of `ai-rulez exposure` with the
// [exposure] context_window, and the files of its start-of-session artifacts; nil when the inventory cannot be
// built.
func (s exposureLintSource) StartExposure() *lint.ExposureStart {
	inv, err := s.inventory(s.mode())
	if err != nil {
		logger.Warn("Skipped the session exposure checks", "error", err)
		return nil
	}
	rep := exposure.NewReport(inv, exposure.ReportOptions{ContextWindow: s.cfg.Exposure.EffectiveContextWindow(), StartBudget: s.cfg.Exposure.StartBudget})
	return &lint.ExposureStart{
		ContextWindow:      rep.Listing.ContextWindow,
		StartArtifacts:     rep.StartArtifacts(),
		StartTokens:        rep.StartTokens,
		ListingSkills:      rep.Listing.Skills,
		ListingChars:       rep.Listing.Chars,
		ListingBudgetChars: rep.Listing.BudgetChars,
		StartPaths:         startPaths(inv),
	}
}

// startPaths are the absolute paths of the files of the start-of-session artifacts of inv, each once, in
// inventory order.
func startPaths(inv *exposure.Inventory) []string {
	root := filepath.FromSlash(inv.Root)
	seen := map[string]bool{}
	var out []string
	start := inv.Start()
	for i := range start {
		a := &start[i]
		if a.Path == "" || seen[a.Path] {
			continue
		}
		seen[a.Path] = true
		out = append(out, filepath.Join(root, filepath.FromSlash(a.Path)))
	}
	return out
}

// Collisions implements lint.ExposureSource. A map with problems, or an inventory that cannot be built, gives
// none: AR9W5 reports the map. The shared inventory is classified on a copy.
func (s exposureLintSource) Collisions(minScore float64) []lint.ExposureCollision {
	m, _ := s.areaMap()
	if m == nil {
		return nil
	}
	inv, err := s.inventory(s.mode())
	if err != nil {
		logger.Warn("Skipped the session exposure collision checks", "error", err)
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
