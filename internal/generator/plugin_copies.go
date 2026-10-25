package generator

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// GeneratedCopies renders in memory what `generate --plugin` writes for the
// configuration, the way `verify --plugin` does, and returns every bundled
// skill, agent and command file of its own [plugin] and of each [marketplace]
// member, with the definition it is rendered from. A configuration that a
// producer above it lists as a member is rendered the way that producer renders
// it. The duplicate rules of validate use it to recognize a generated copy of a
// definition, or a byte-identical copy of that, as the definition itself
// (lint.WithGeneratedCopies). Nothing is written. A bundle that cannot be
// rendered is left out, and so are domain plugins and [[plugins_authoring]]
// bundles: their copies are judged as definitions of their own. Render errors
// go to debug at Debug level (nil uses the generator's log).
func (g *Generator) GeneratedCopies(debug logger.Logger) []lint.GeneratedCopy {
	if debug == nil {
		debug = g.log()
	}
	if g.config == nil || (g.config.Plugin == nil && g.config.Marketplace == nil) {
		return nil
	}
	view, err := g.copyView()
	if err != nil {
		debug.Debug("Skipped rendering the plugin bundles for the duplicate rules", "error", err)
		return nil
	}
	out, memberDirs := view.memberCopies(debug)
	if view.config.Plugin != nil && !view.hasOtherBundles() {
		copies, err := view.ownPluginCopies(memberDirs)
		if err != nil {
			debug.Debug("Skipped rendering the plugin for the duplicate rules", "error", err)
		}
		out = append(out, copies...)
	}
	return out
}

// copyView is the generator over the configuration `generate --plugin` renders:
// the shared view, without machine-local inputs.
func (g *Generator) copyView() (*Generator, error) {
	if g.config.HasLocalInputs() && g.config.ConfigDir != "" && g.config.ConfigFile != "" {
		return g.sharedPluginView()
	}
	return g, nil
}

// hasOtherBundles reports whether domain plugins or [[plugins_authoring]]
// bundles write skills below the base directory. Pairing those files with the
// [plugin] manifest by name could pair a domain's skill with a root skill, so
// none is paired then.
func (g *Generator) hasOtherBundles() bool {
	return len(g.config.PluginsAuthoring) > 0 || (g.config.Marketplace != nil && g.config.Marketplace.HasDomainPlugins())
}

// memberCopies renders each [marketplace] member, loading its configuration
// once, and pairs its files with its definitions. It also returns the member
// directories (links resolved).
func (g *Generator) memberCopies(debug logger.Logger) (out []lint.GeneratedCopy, memberDirs []string) {
	m := g.config.Marketplace
	if m == nil {
		return nil, nil
	}
	for _, member := range m.Members {
		manifest, memberCfg, err := g.memberManifest(member)
		if err != nil {
			debug.Debug("Skipped rendering a member plugin for the duplicate rules", "member", member, "error", err)
			continue
		}
		outputs, err := plugin.GenerateMember(manifest, memberCfg.BaseDir)
		if err != nil {
			debug.Debug("Skipped rendering a member plugin for the duplicate rules", "member", member, "error", err)
			continue
		}
		bundle := newCopyBundle(memberCfg.BaseDir, manifest)
		memberDirs = append(memberDirs, bundle.dir)
		out = append(out, g.pairCopies(&bundle, outputs, nil)...)
	}
	return out, memberDirs
}

// ownPluginCopies pairs the rendering of the configuration's own [plugin] with
// its definitions, leaving out the files below a member's directory.
func (g *Generator) ownPluginCopies(memberDirs []string) ([]lint.GeneratedCopy, error) {
	outputs, found, err := g.producerOutputs()
	if err != nil {
		return nil, err
	}
	if !found {
		if outputs, err = g.collectPluginOutputs(""); err != nil {
			return nil, err
		}
	}
	manifest, err := g.buildPluginManifest("")
	if err != nil {
		return nil, err
	}
	bundle := newCopyBundle(g.config.BaseDir, manifest)
	return g.pairCopies(&bundle, outputs, memberDirs), nil
}

// producerOutputs is the rendering of a producer above the configuration that
// lists it as a [marketplace] member. Unlike the version drift check it also
// asks when the configuration authors marketplaces of its own: the bundle on
// disk is the producer's either way.
func (g *Generator) producerOutputs() ([]config.OutputFile, bool, error) {
	top := g.git().TopLevelContext(g.ctx, g.config.BaseDir)
	if top == "" {
		return nil, false, nil
	}
	return g.producerMemberOutputs(top)
}

// pairCopies pairs the rendered SKILL.md, agent and command files of the bundle
// with the definitions they are rendered from, by the definition name a path
// carries. Files below an excluded directory are left out.
func (g *Generator) pairCopies(bundle *copyBundle, outputs []config.OutputFile, exclude []string) []lint.GeneratedCopy {
	var out []lint.GeneratedCopy
	for _, o := range outputs {
		if o.IsDir {
			continue
		}
		kind, name := copyKindName(o.Path)
		if kind == "" {
			continue
		}
		path := resolveThroughLinks(g.absOutputPath(o.Path))
		if !safefs.Within(bundle.dir, path) || withinAny(exclude, path) {
			continue
		}
		if src := bundle.sources[kind][name]; src != nil {
			out = append(out, lint.GeneratedCopy{Kind: kind, Source: *src, Content: outputBytes(o)})
		}
	}
	return out
}

func withinAny(dirs []string, path string) bool {
	for _, dir := range dirs {
		if safefs.Within(dir, path) {
			return true
		}
	}
	return false
}

// copyBundle is a plugin bundle directory (with its links resolved) and its
// definitions by kind and name.
type copyBundle struct {
	dir     string
	sources map[string]map[string]*config.ContentFile
}

func newCopyBundle(dir string, m *plugin.Manifest) copyBundle {
	return copyBundle{dir: resolveThroughLinks(dir), sources: map[string]map[string]*config.ContentFile{
		kindSkill:            uniqueByName(m.Skills),
		kindAgent:            uniqueByName(m.Agents),
		placementKindCommand: uniqueByName(m.Commands),
	}}
}

// copyKindName names the kind and definition name of a bundled file from its
// path: <dir>/<name>/SKILL.md, agents/<name>.md (or .agent.md) and
// commands/<name>.md. Other files return "".
func copyKindName(path string) (kind, name string) {
	base, parent := filepath.Base(path), filepath.Base(filepath.Dir(path))
	switch {
	case base == "SKILL.md":
		return kindSkill, parent
	case parent == "agents" && strings.HasSuffix(base, ".agent.md"):
		return kindAgent, strings.TrimSuffix(base, ".agent.md")
	case parent == "agents" && strings.HasSuffix(base, ".md"):
		return kindAgent, strings.TrimSuffix(base, ".md")
	case parent == "commands" && strings.HasSuffix(base, ".md"):
		return placementKindCommand, strings.TrimSuffix(base, ".md")
	}
	return "", ""
}

// uniqueByName indexes files by name, leaving out a name two files share.
func uniqueByName(files []config.ContentFile) map[string]*config.ContentFile {
	out := map[string]*config.ContentFile{}
	seen := map[string]bool{}
	for i := range files {
		name := files[i].Name
		if seen[name] {
			delete(out, name)
			continue
		}
		seen[name] = true
		out[name] = &files[i]
	}
	return out
}
