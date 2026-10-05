// Package userscope maps what a preset renders for a project onto the per-user
// directories its tool reads (`generate --user`).
//
// There is no table in this package. Every preset declares its own user-scope
// layout once: provider specs in a [global] block (plus global_path on their
// sidecars), Go presets through presets.GlobalOutputProvider. A preset also
// declares where it writes the project-level counterparts
// (presets.ProjectLayoutProvider; specs derive that from their outputs). Resolve
// joins the two into a Layout, so an output is written below the home directory
// only where its preset declares a user-level location for that kind of content.
// Everything else a preset renders (MCP files, plugin sidecars, kinds the tool has
// no user-level folder for) is dropped.
//
// Home-relocating environment variables a layout names (CODEX_HOME,
// HERMES_HOME, ...) move the paths below them, and only absolute values are
// honored.
package userscope

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
)

// Kind classifies what a row carries.
type Kind string

// Content kinds of a row.
const (
	KindInstructions Kind = "instructions"
	KindRules        Kind = "rules"
	KindSkills       Kind = "skills"
	KindAgents       Kind = "agents"
	KindCommands     Kind = "commands"
	KindSettings     Kind = "settings"
)

// Row maps a project-relative output of one preset to its user-level location.
// From is slash-separated and relative to the project root; a directory row maps
// the files below it one to one. To is absolute, or empty when the tool has no
// user-level counterpart of that kind (the output is classified but dropped).
type Row struct {
	Kind Kind
	From string
	To   string
	// Dir is true when From and To are directories.
	Dir bool
}

// Layout is the user-scope mapping of one preset.
type Layout struct {
	Preset string
	Rows   []Row
	// SkillReaders are the user-level skill directories the tool reads, absolute;
	// SkillPrecedence is the vendor's rule for a skill present at both levels.
	SkillReaders    []string
	SkillPrecedence string
	// RelocatedHome is the absolute directory the tool's home variable points at
	// (CODEX_HOME), when set. Destinations below it lie outside the home directory
	// on purpose.
	RelocatedHome string
}

// UnsupportedError reports a preset user scope cannot write for.
type UnsupportedError struct {
	Preset string
	Reason string
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("preset %s has no user-level location: %s", e.Preset, e.Reason)
}

// IsUnsupported reports whether err says a preset has no user scope.
func IsUnsupported(err error) bool {
	var target *UnsupportedError
	return errors.As(err, &target)
}

// Resolve builds the layout of a built-in preset below home, which must be an
// absolute path; getenv looks up the layout's home-relocating variable (pass
// os.Getenv). It returns an *UnsupportedError when the preset declares no user
// scope.
func Resolve(preset, home string, getenv func(string) string) (*Layout, error) {
	return ResolveFor(nil, preset, home, getenv)
}

// ResolveFor is Resolve for a config: a preset whose project layout depends on it
// (codex_skills_dir) maps the folder the config names. cfg may be nil.
func ResolveFor(cfg *config.Config, preset, home string, getenv func(string) string) (*Layout, error) {
	if !filepath.IsAbs(home) {
		return nil, fmt.Errorf("the home directory %q must be an absolute path", home)
	}
	gen, err := config.GetPresetGenerator(preset)
	if err != nil {
		return nil, &UnsupportedError{preset, "it is not a built-in preset"}
	}
	provider, ok := gen.(presets.GlobalOutputProvider)
	if !ok {
		return nil, &UnsupportedError{preset, "no vendor-documented user-level location is declared for it"}
	}
	global := provider.GlobalOutputPaths(home, getenv)
	if global == nil {
		return nil, &UnsupportedError{preset, "no vendor-documented user-level location is declared for it"}
	}
	if reason := unsafeRelocation(global, home); reason != "" {
		return nil, &UnsupportedError{preset, reason}
	}
	layouter, ok := gen.(presets.ProjectLayoutProvider)
	if !ok {
		return nil, &UnsupportedError{preset, "it does not declare its project layout"}
	}
	project := layouter.ProjectLayout()
	if configured, ok := gen.(presets.ConfiguredProjectLayoutProvider); ok {
		project = configured.ProjectLayoutFor(cfg)
	}
	layout := build(preset, project, global)
	if !layout.hasContentDestination() {
		return nil, &UnsupportedError{preset, "none of the instructions, rules, skills, agents or commands it renders has a user-level location (MCP servers are not generated at user level)"}
	}
	return layout, nil
}

// unsafeRelocation explains why the tool home its variable names cannot be written
// to: the filesystem root, the home directory itself or a directory above it would
// spread the tool's files over places that are not the tool's.
func unsafeRelocation(global *presets.GlobalPaths, home string) string {
	reloc := global.RelocatedHome
	if reloc == "" {
		return ""
	}
	reloc = filepath.Clean(reloc)
	isRoot := filepath.Dir(reloc) == reloc
	rel, err := filepath.Rel(reloc, filepath.Clean(home))
	contains := err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if !isRoot && !contains {
		return ""
	}
	return fmt.Sprintf("its home variable points at %s, the filesystem root, your home directory or a directory above it; "+
		"point it at the tool's own folder", global.RelocatedHome)
}

func build(preset string, project presets.ProjectLayout, global *presets.GlobalPaths) *Layout {
	layout := &Layout{Preset: preset, SkillReaders: global.SkillReaders, SkillPrecedence: global.SkillPrecedence,
		RelocatedHome: global.RelocatedHome}
	add := func(kind Kind, from, to string, dir bool) {
		if from == "" {
			return
		}
		layout.Rows = append(layout.Rows, Row{Kind: kind, From: path.Clean(from), To: to, Dir: dir})
	}
	add(KindInstructions, project.RootFile, global.RootFile, false)
	add(KindRules, project.RulesDir, global.RulesDir, true)
	add(KindSkills, project.SkillsDir, global.SkillsDir, true)
	add(KindAgents, project.AgentsDir, global.AgentsDir, true)
	// A tool that keeps commands in its skills folder (Claude Code) renders them
	// there, and they leave with the skills.
	if path.Clean(project.CommandsDir) != path.Clean(project.SkillsDir) {
		add(KindCommands, project.CommandsDir, global.CommandsDir, true)
	}
	sidecars := make([]string, 0, len(global.Sidecars))
	for from := range global.Sidecars {
		sidecars = append(sidecars, from)
	}
	sort.Strings(sidecars)
	for _, from := range sidecars {
		add(KindSettings, from, global.Sidecars[from], false)
	}
	return layout
}

// hasContentDestination reports whether the layout writes instructions, rules,
// skills, agents or commands. Settings documents alone do not count: a preset that
// declares only an MCP file has nothing --user would write.
func (l *Layout) hasContentDestination() bool {
	return slices.ContainsFunc(l.Rows, func(r Row) bool { return r.To != "" && r.Kind != KindSettings })
}

// Classify returns the row an output of the preset belongs to, even when the
// preset has no user-level location for that kind.
func (l *Layout) Classify(rel string) (Row, bool) {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	best, found := Row{}, false
	for _, row := range l.Rows {
		if !row.covers(rel) {
			continue
		}
		if !found || len(row.From) > len(best.From) {
			best, found = row, true
		}
	}
	return best, found
}

func (r Row) covers(rel string) bool {
	if rel == r.From {
		return true
	}
	return r.Dir && strings.HasPrefix(rel, r.From+"/")
}

// Map returns the absolute user-level destination of a project-relative output.
// ok is false for an output the preset has no user-level location for. The
// longest matching row wins, so a commands folder inside a rules folder keeps its
// own destination.
func (l *Layout) Map(rel string) (dest string, row Row, ok bool) {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	row, found := l.Classify(rel)
	if !found || row.To == "" {
		return "", Row{}, false
	}
	if rel == row.From {
		return row.To, row, true
	}
	return filepath.Join(row.To, filepath.FromSlash(strings.TrimPrefix(rel, row.From+"/"))), row, true
}

// Roots returns the user-level directories whose contents the preset owns,
// sorted: clean may remove directories emptied inside them, never one above.
func (l *Layout) Roots() []string {
	var out []string
	for _, row := range l.Rows {
		if row.Dir && row.To != "" {
			out = append(out, row.To)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// Kinds lists the content kinds the layout writes, in table order.
func (l *Layout) Kinds() []Kind {
	var out []Kind
	for _, row := range l.Rows {
		if row.To != "" && !slices.Contains(out, row.Kind) {
			out = append(out, row.Kind)
		}
	}
	return out
}

// Precedence is the vendor's rule for a skill present at user and project level;
// presets that document none get the generic advice.
func (l *Layout) Precedence() string {
	if l.SkillPrecedence != "" {
		return l.SkillPrecedence
	}
	return l.Preset + " does not document a precedence; keep names unique"
}

// All resolves every built-in preset (except the shared mcp preset, which is no
// harness). layouts holds the supported ones by name, unsupported the reason for
// each of the others.
func All(home string, getenv func(string) string) (layouts map[string]*Layout, unsupported map[string]string, err error) {
	return AllFor(nil, home, getenv)
}

// AllFor is All for a config; see ResolveFor.
func AllFor(cfg *config.Config, home string, getenv func(string) string) (layouts map[string]*Layout, unsupported map[string]string, err error) {
	layouts, unsupported = map[string]*Layout{}, map[string]string{}
	for _, name := range config.IndividualPresetNames() {
		layout, err := ResolveFor(cfg, name, home, getenv)
		var reason *UnsupportedError
		switch {
		case errors.As(err, &reason):
			unsupported[name] = reason.Reason
		case err != nil:
			return nil, nil, err
		default:
			layouts[name] = layout
		}
	}
	return layouts, unsupported, nil
}

// Supported lists the names of the supported presets in layouts, sorted.
func Supported(layouts map[string]*Layout) []string {
	out := make([]string, 0, len(layouts))
	for name := range layouts {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
