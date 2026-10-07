package providers

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
	"github.com/samber/oops"
)

// isGenericSidecarKind reports whether kind is one of the format-agnostic kinds
// that merge into any JSON, JSONC, TOML or YAML document.
func isGenericSidecarKind(kind string) bool {
	return slices.Contains([]string{SidecarMCP, SidecarChecks, SidecarPermissions, SidecarHooks}, kind)
}

// isDocFormat reports whether format is a document format a generic sidecar can
// target.
func isDocFormat(format string) bool {
	return slices.Contains([]string{DocFormatJSON, DocFormatJSONC, DocFormatTOML, DocFormatYAML}, format)
}

// DocFormat is the document format of a generic sidecar: the explicit format, or
// the one the path's extension names (JSONC for a ".json" path of a dialect whose
// files carry comments, see mcpDialect.jsoncDefault). It is "" for a path with an unknown
// extension and no explicit format.
func (s *SidecarSpec) DocFormat() string {
	if s.Format != "" {
		return s.Format
	}
	switch strings.ToLower(path.Ext(s.Path)) {
	case ".json":
		if s.Kind == SidecarMCP {
			if d, err := mcpDialectFor(s.Dialect); err == nil && d.jsoncDefault {
				return DocFormatJSONC
			}
		}
		return DocFormatJSON
	case ".jsonc":
		return DocFormatJSONC
	case ".toml":
		return DocFormatTOML
	case ".yaml", ".yml":
		return DocFormatYAML
	}
	return ""
}

// ownedKeyPath is the member path a generic sidecar owns: its explicit key, or
// the dialect's default.
func (s *SidecarSpec) ownedKeyPath(d mcpDialect) []string {
	if len(s.Key) > 0 {
		return s.Key
	}
	return d.defaultKey
}

// renderSidecarSpec renders one sidecar of the spec: a generic kind through its
// format-agnostic renderer, a tool-specific kind through renderSidecar.
func (g *Generator) renderSidecarSpec(sc *SidecarSpec, cfg *config.Config, outputPath string) (sidecarRender, error) {
	if sc.Kind == SidecarHookPlugin {
		return g.renderHookPlugin(sc, cfg, outputPath)
	}
	if isGenericSidecarKind(sc.Kind) {
		return g.renderGenericSidecar(sc, cfg, outputPath)
	}
	return g.renderSidecar(sc.Kind, cfg, outputPath)
}

// renderGenericSidecar merges what ai-rulez owns into the document at outputPath,
// whatever its format, leaving every other member alone.
func (g *Generator) renderGenericSidecar(sc *SidecarSpec, cfg *config.Config, outputPath string) (sidecarRender, error) {
	if sc.Kind == SidecarHooks && settings.HookDialectOwnsFile(sc.Dialect) {
		// A hooks file of its own (Copilot's): written whole, never merged.
		keys, ok, err := settings.OwnedHooksKeys(cfg, sc.Dialect)
		if err != nil || !ok {
			return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
		}
		body, err := settings.RenderOwnedHooks(keys)
		if err != nil {
			return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
		}
		return sidecarRender{Body: body, Owned: keys}, nil
	}
	owned, err := g.genericOwnedKeys(sc, cfg, outputPath)
	if err != nil {
		return sidecarRender{}, err
	}
	if len(owned) == 0 && (sc.Kind == SidecarHooks || sc.Kind == SidecarPermissions) {
		return sidecarRender{}, nil // nothing applies: the user's file is not rewritten or registered
	}
	return mergeDocument(cfg, outputPath, sc.DocFormat(), owned)
}

// sharedSidecars returns the sidecars of the spec that merge into the document
// the sidecar sc merges into (an MCP sidecar and a hooks sidecar of one settings
// file), in spec order, those whose predicate does not hold left out. It is
// just sc when the document is not shared.
func (g *Generator) sharedSidecars(sc *SidecarSpec, cfg *config.Config) []*SidecarSpec {
	if !isMergedGenericSidecar(sc) {
		return []*SidecarSpec{sc}
	}
	var group []*SidecarSpec
	for _, other := range g.Spec.Sidecars {
		if other != nil && other.Path == sc.Path && isMergedGenericSidecar(other) &&
			g.evalPredicate(other.EmitWhen, cfg) && (!other.UserOnly || cfg.UserScope) {
			group = append(group, other)
		}
	}
	return group
}

// sidecarIsMerged is SidecarIsMergedDocument for one sidecar: a hooks file the
// harness loads next to others and ai-rulez writes whole (Copilot's) is not merged.
func sidecarIsMerged(sc *SidecarSpec) bool {
	return SidecarIsMergedDocument(sc.Kind) && !(sc.Kind == SidecarHooks && settings.HookDialectOwnsFile(sc.Dialect))
}

// isMergedGenericSidecar reports whether the sidecar merges into a shared document.
func isMergedGenericSidecar(sc *SidecarSpec) bool {
	return isGenericSidecarKind(sc.Kind) && sc.Kind != SidecarChecks &&
		!(sc.Kind == SidecarHooks && settings.HookDialectOwnsFile(sc.Dialect))
}

// renderSidecarGroup merges the owned keys of several sidecars into their one
// document: each merge starts from what is on disk, so rendering them one after
// the other would keep only the last.
func (g *Generator) renderSidecarGroup(group []*SidecarSpec, cfg *config.Config, outputPath string) (sidecarRender, error) {
	var owned []jsonmerge.OwnedKey
	for _, sc := range group {
		if !sameDocFormat(sc.DocFormat(), group[0].DocFormat()) {
			return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).
				Errorf("sidecars %q and %q merge into %s with different formats", group[0].Kind, sc.Kind, sc.Path)
		}
		keys, err := g.genericOwnedKeys(sc, cfg, outputPath)
		if err != nil {
			return sidecarRender{}, err
		}
		owned = append(owned, keys...)
	}
	if len(owned) == 0 {
		return sidecarRender{}, nil // nothing applies: the user's file is not rewritten or registered
	}
	return mergeDocument(cfg, outputPath, group[0].DocFormat(), owned)
}

// sameDocFormat reports whether two formats are one syntax: JSON and JSONC are
// merged by the same engine, which keeps the comments of a document that has them.
func sameDocFormat(a, b string) bool {
	jsonLike := func(f string) bool { return f == DocFormatJSON || f == DocFormatJSONC }
	return a == b || (jsonLike(a) && jsonLike(b))
}

// genericOwnedKeys is what a generic sidecar owns in the document at outputPath.
func (g *Generator) genericOwnedKeys(sc *SidecarSpec, cfg *config.Config, outputPath string) ([]jsonmerge.OwnedKey, error) {
	switch sc.Kind {
	case SidecarMCP:
		dialect, err := mcpDialectFor(sc.Dialect)
		if err != nil {
			return nil, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
		}
		if dialect.arrayKey != "" {
			key, err := arrayOwnedKey(sc, dialect, cfg, outputPath)
			if err != nil {
				return nil, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
			}
			return []jsonmerge.OwnedKey{key}, nil
		}
		// Elements that are project-relative globs mean nothing in the user scope.
		elements := sc.Elements
		if elements != nil && elements.ProjectOnly && cfg != nil && cfg.UserScope {
			elements = nil
		}
		entries := mcpDialectEntriesFor(dialect, cfg, &mcpEntryOpts{transports: sc.Transports, refSyntax: sc.EnvRefSyntax})
		var owned []jsonmerge.OwnedKey
		if len(entries) > 0 || elements == nil {
			owned = append(owned, jsonmerge.OwnedKey{Path: sc.ownedKeyPath(dialect), Value: entries, Members: true})
		}
		if elements != nil {
			key, ok, err := elementsOwnedKey(sc, cfg, outputPath)
			if err != nil {
				return nil, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
			}
			if ok {
				owned = append(owned, key)
			}
		}
		return owned, nil
	case SidecarHooks:
		keys, err := settings.HookKeys(cfg, sc.Dialect, outputPath)
		if err != nil {
			return nil, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
		}
		return keys, nil
	case SidecarPermissions:
		keys, err := settings.PermissionKeys(cfg, sc.Dialect, outputPath)
		if err != nil {
			return nil, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
		}
		return keys, nil
	}
	return nil, oops.Errorf("unknown generic sidecar kind %q", sc.Kind)
}

// mergeDocument merges owned keys into the document at outputPath, whatever its
// format (json, jsonc, toml or yaml), through docmerge.
func mergeDocument(cfg *config.Config, outputPath, format string, owned []jsonmerge.OwnedKey) (sidecarRender, error) {
	return docmerge.ApplyWith(cfg.ReadExisting, outputPath, docmerge.Format(format), owned)
}

// MergedSidecarDoc is a merged document declared by a builtin spec.
type MergedSidecarDoc struct {
	Path   string // base-relative, slash-separated
	Format string // json, jsonc, toml, yaml or markdown
}

// MergedSidecarDocs is MergedSidecarPaths with each document's format, for
// callers that must read or unmerge a document by format.
func MergedSidecarDocs() []MergedSidecarDoc {
	seen := make(map[string]bool)
	var docs []MergedSidecarDoc
	for _, spec := range loadBuiltinSpecs() {
		for _, sc := range spec.Sidecars {
			if sc == nil || !sidecarIsMerged(sc) || seen[sc.Path] {
				continue
			}
			seen[sc.Path] = true
			format := DocFormatJSON
			if isGenericSidecarKind(sc.Kind) {
				format = sc.DocFormat()
			}
			docs = append(docs, MergedSidecarDoc{Path: filepath.ToSlash(sc.Path), Format: format})
		}
		if path := aggregateChecksPath(spec); path != "" && !seen[path] {
			seen[path] = true
			docs = append(docs, MergedSidecarDoc{Path: path, Format: string(docmerge.FormatMarkdown)})
		}
	}
	slices.SortFunc(docs, func(a, b MergedSidecarDoc) int { return strings.Compare(a.Path, b.Path) })
	return docs
}

// sidecarMergeSource records what a generic sidecar's document was rendered from,
// so that presets writing it with different keys are combined instead of
// conflicting (see config.MergeSource). It is nil for a sidecar of another kind.
func sidecarMergeSource(sc *SidecarSpec, outputPath string, rendered sidecarRender) *config.MergeSource {
	if len(rendered.Owned) == 0 || !isGenericSidecarKind(sc.Kind) || sc.Kind == SidecarChecks {
		return nil
	}
	format := sc.DocFormat()
	if sc.Kind == SidecarHooks && settings.HookDialectOwnsFile(sc.Dialect) {
		format = config.MergeFormatOwnedHooks
	}
	return &config.MergeSource{Path: outputPath, Format: format, Owned: rendered.Owned}
}
