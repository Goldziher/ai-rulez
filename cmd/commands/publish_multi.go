package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	pemit "github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
)

const (
	multiPluginsDir   = "plugins"
	multiAggregateDir = "aggregate"
)

// isMultiPlugin reports whether the project publishes several plugins: a
// [marketplace] with members or domain plugins.
func isMultiPlugin(cfg *config.Config) bool {
	m := cfg.Marketplace
	return m != nil && (len(m.Members) > 0 || m.HasDomainPlugins())
}

// findClaudeIndex returns the Claude marketplace index among the bundle files
// (the one closest to the project root) and the directory that holds its
// .claude-plugin directory, relative to the project root.
func findClaudeIndex(files []generator.PluginFile) (data []byte, root string, ok bool) {
	for _, f := range files {
		dir, found := "", false
		if f.Path == ".claude-plugin/marketplace.json" {
			found = true
		} else if d, cut := strings.CutSuffix(f.Path, "/.claude-plugin/marketplace.json"); cut {
			dir, found = d, true
		}
		if found && (!ok || len(dir) < len(root)) {
			data, root, ok = f.Data, dir, true
		}
	}
	return data, root, ok
}

// indexPlugin is one entry of the generated marketplace index.
type indexPlugin struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Category    string   `json:"category"`
	Keywords    []string `json:"keywords"`
	Source      string   `json:"source"`
}

func parseIndexPlugins(data []byte) ([]indexPlugin, error) {
	var doc struct {
		Plugins []indexPlugin `json:"plugins"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, oops.Wrapf(err, "parse the marketplace index")
	}
	return doc.Plugins, nil
}

// runtimeMarkers maps a file only one runtime writes to that runtime.
var runtimeMarkers = []struct{ file, runtime string }{
	{".claude-plugin/plugin.json", config.PluginRuntimeClaude},
	{".cursor-plugin/plugin.json", config.PluginRuntimeCursor},
	{".codex-plugin/plugin.json", config.PluginRuntimeCodex},
	{"gemini-extension.json", config.PluginRuntimeGemini},
	{"kimi.plugin.json", config.PluginRuntimeKimi},
	{".factory-plugin/plugin.json", config.PluginRuntimeFactory},
	{"plugin.json", config.PluginRuntimeAgentPlugins},
}

// detectRuntimes lists the runtimes a plugin directory carries a manifest for.
func detectRuntimes(files []publish.File) []string {
	have := map[string]bool{}
	for _, f := range files {
		have[f.Path] = true
	}
	var out []string
	for _, m := range runtimeMarkers {
		if have[m.file] {
			out = append(out, m.runtime)
		}
	}
	for _, f := range files {
		if strings.HasPrefix(f.Path, ".hermes/plugins/") && !slices.Contains(out, config.PluginRuntimeHermes) {
			out = append(out, config.PluginRuntimeHermes)
		}
	}
	slices.Sort(out)
	return out
}

// pluginVersionOf reads the version of a plugin directory's Claude manifest.
func pluginVersionOf(files []publish.File) string {
	for _, f := range files {
		if f.Path != ".claude-plugin/plugin.json" {
			continue
		}
		var m struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(f.Data, &m) == nil {
			return m.Version
		}
	}
	return ""
}

// multiSpecs splits the bundle into one plugin per marketplace entry: the files
// below the entry's source directory, with their paths made relative to it.
func (pc *publishContext) multiSpecs(files []generator.PluginFile) ([]*pluginSpec, error) {
	index, indexRoot, ok := findClaudeIndex(files)
	if !ok {
		return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "the marketplace index is written by the claude runtime",
			"the bundle has no Claude marketplace index to list the plugins from")
	}
	plugins, err := parseIndexPlugins(index)
	if err != nil {
		return nil, err
	}
	var specs []*pluginSpec
	for _, ip := range plugins {
		rel := path.Clean(strings.TrimPrefix(ip.Source, "./"))
		dir := path.Join(indexRoot, rel)
		spec := &pluginSpec{name: ip.Name, description: ip.Description, version: ip.Version, category: ip.Category, keywords: ip.Keywords}
		for _, f := range files {
			sub, found := f.Path, dir == "."
			if !found {
				sub, found = strings.CutPrefix(f.Path, dir+"/")
			}
			if found && sub != "" {
				spec.files = append(spec.files, publish.File{Path: sub, Data: f.Data, Executable: f.Executable})
			}
		}
		if len(spec.files) == 0 {
			return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "", "plugin %s has no files below %s", ip.Name, dir)
		}
		if spec.version == "" {
			spec.version = pluginVersionOf(spec.files)
		}
		if err := publish.ValidateName(spec.name, spec.version); err != nil {
			return nil, err //nolint:wrapcheck // a publish.Error carries the exit status
		}
		spec.runtimes = detectRuntimes(spec.files)
		spec.tag = spec.name + "-v" + spec.version
		specs = append(specs, spec)
	}
	return pc.selectPlugins(specs)
}

// selectPlugins applies --only; a name that matches no plugin is an error.
func (pc *publishContext) selectPlugins(specs []*pluginSpec) ([]*pluginSpec, error) {
	if len(publishOnly) == 0 {
		return specs, nil
	}
	var out []*pluginSpec
	for _, name := range publishOnly {
		i := slices.IndexFunc(specs, func(s *pluginSpec) bool { return s.name == name })
		if i < 0 {
			names := make([]string, len(specs))
			for j, s := range specs {
				names[j] = s.name
			}
			return nil, publish.Errorf(publish.CodeConfig, publish.ExitFailed, "the plugins are: "+strings.Join(names, ", "), "--only %q matches no plugin", name)
		}
		if !slices.Contains(out, specs[i]) {
			out = append(out, specs[i])
		}
	}
	return out, nil
}

// runPublishMulti publishes every plugin of the marketplace: one dist directory
// each under plugins/<name>, and an aggregate directory with the pinned index and
// the emitter output.
func runPublishMulti(out io.Writer, pc *publishContext, emitOnly string) error {
	files := pc.pre.files
	if len(pc.opts.runtimes) > 0 {
		var err error
		if files, err = pc.filteredFiles(pc.opts.runtimes); err != nil {
			return err
		}
	}
	specs, err := pc.multiSpecs(files)
	if err != nil {
		return err
	}
	if publishTag != "" {
		return publish.Errorf(publish.CodeConfig, publish.ExitFailed, "tags are <plugin>-v<version> for each plugin; --tag names the ref the marketplace index pins",
			"--tag does not apply to a multi-plugin release")
	}
	var dists []*publish.Dist
	var plugins []pemit.Plugin
	for _, spec := range specs {
		in, err := pc.newInput(spec)
		if err != nil {
			return err
		}
		d, err := publish.Build(*in)
		if err != nil {
			return err //nolint:wrapcheck // a publish.Error carries the exit status
		}
		dists = append(dists, d)
		plugins = append(plugins, emitPluginOf(spec, d))
	}
	agg, err := pc.buildAggregate(plugins)
	if err != nil {
		return err
	}
	if emitOnly != "" {
		if agg == nil {
			return publish.Errorf(publish.CodeConfig, publish.ExitFailed, "", "emitter %s produced no files", emitOnly)
		}
		return pc.writeEmitOnly(out, agg, emitOnly)
	}
	if !publishDryRun {
		if err := writeMulti(pc.distAbs, specs, dists, agg); err != nil {
			return err
		}
	}
	for i, d := range dists {
		warnAll(d.Warnings)
		if err := printMultiPlugin(out, specs[i], d, filepath.Join(pc.distAbs, multiPluginsDir, specs[i].name)); err != nil {
			return err
		}
	}
	if agg != nil {
		warnAll(agg.Warnings)
		fmt.Fprintf(out, "aggregate   %d files to %s\n", len(agg.Files), filepath.Join(pc.distAbs, multiAggregateDir))
		for _, a := range agg.Plan.Artifacts {
			fmt.Fprintf(out, "            %-40s %s  %d bytes\n", a.Path, a.Digest, a.Size)
		}
	}
	if !publishExecute {
		return nil
	}
	for i, d := range dists {
		if err := executeDist(pc.ctx, d, filepath.Join(pc.distAbs, multiPluginsDir, specs[i].name)); err != nil {
			return err
		}
	}
	return nil
}

func emitPluginOf(spec *pluginSpec, d *publish.Dist) pemit.Plugin {
	files := make([]pemit.File, 0, len(spec.files))
	for _, f := range spec.files {
		files = append(files, pemit.File{Path: f.Path, Data: f.Data})
	}
	return pemit.Plugin{
		Name: spec.name, Description: spec.description, Version: spec.version, Category: spec.category,
		Keywords: spec.keywords, Runtimes: d.Manifest.Runtimes, BundleFile: d.Manifest.Bundle.File,
		BundleDigest: d.Manifest.Bundle.Digest, Files: files,
	}
}

// buildAggregate builds the aggregate directory, or nil when neither a pinned
// index nor an emitter was asked for.
func (pc *publishContext) buildAggregate(plugins []pemit.Plugin) (*publish.Dist, error) {
	pin, err := pc.pinFor(publishTag)
	if err != nil {
		return nil, err
	}
	req := pc.emitRequestFor("")
	if pin == nil && req == nil {
		return nil, nil //nolint:nilnil // nothing to aggregate
	}
	return publish.BuildAggregate(pc.market().Name, publish.Extras{
		Channel: publishChannel, Pin: pin, PinCommit: pc.src.Source.Commit, PinDirty: pc.src.Source.Dirty,
		Emit: req, Plugins: plugins,
	})
}

// writeMulti writes every plugin dist and the aggregate below the dist directory.
func writeMulti(root string, specs []*pluginSpec, dists []*publish.Dist, agg *publish.Dist) error {
	for i, d := range dists {
		if err := d.Write(filepath.Join(root, multiPluginsDir, specs[i].name)); err != nil {
			return err //nolint:wrapcheck // a publish.Error carries the exit status
		}
	}
	if agg != nil {
		if err := agg.Write(filepath.Join(root, multiAggregateDir)); err != nil {
			return err //nolint:wrapcheck // a publish.Error carries the exit status
		}
	}
	return nil
}

func printMultiPlugin(out io.Writer, spec *pluginSpec, d *publish.Dist, dir string) error {
	if publishFormat == formatJSON {
		_, err := out.Write(d.Files[publish.PlanFile])
		return oops.Wrapf(err, "write plan")
	}
	fmt.Fprintf(out, "plugin      %s %s -> %s\n", spec.name, spec.version, dir)
	return printPublish(out, d, dir)
}
