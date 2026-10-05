package providers

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
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
	if isGenericSidecarKind(sc.Kind) {
		return g.renderGenericSidecar(sc, cfg, outputPath)
	}
	return g.renderSidecar(sc.Kind, cfg, outputPath)
}

// renderGenericSidecar merges what ai-rulez owns into the document at outputPath,
// whatever its format, leaving every other member alone.
func (g *Generator) renderGenericSidecar(sc *SidecarSpec, cfg *config.Config, outputPath string) (sidecarRender, error) {
	switch sc.Kind {
	case SidecarMCP:
		dialect, err := mcpDialectFor(sc.Dialect)
		if err != nil {
			return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
		}
		if dialect.arrayKey != "" {
			key, err := arrayOwnedKey(sc, dialect, cfg, outputPath)
			if err != nil {
				return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
			}
			return mergeDocument(outputPath, sc.DocFormat(), []jsonmerge.OwnedKey{key})
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
				return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).Wrap(err)
			}
			if ok {
				owned = append(owned, key)
			}
		}
		return mergeDocument(outputPath, sc.DocFormat(), owned)
	case SidecarPermissions, SidecarHooks:
		// Reserved: validated by the loader so specs can declare them, rendered
		// once their owner lands.
		return sidecarRender{}, oops.With("preset", g.Spec.Name, "path", sc.Path).
			Errorf("sidecar kind %q is not yet supported", sc.Kind)
	}
	return sidecarRender{}, oops.Errorf("unknown generic sidecar kind %q", sc.Kind)
}

// mergeDocument merges owned keys into the document at outputPath, whatever its
// format (json, jsonc, toml or yaml), through docmerge.
func mergeDocument(outputPath, format string, owned []jsonmerge.OwnedKey) (sidecarRender, error) {
	return docmerge.Apply(outputPath, docmerge.Format(format), owned)
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
			if sc == nil || !SidecarIsMergedDocument(sc.Kind) || seen[sc.Path] {
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
