package config

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// Environment variables that name the active layers. AI_RULEZ_LAYERS wins;
// AI_RULEZ_PROFILE (a comma-separated list composes several profiles) is the
// fallback, so one value can select both a profile and the layers named like it.
const (
	envLayers       = "AI_RULEZ_LAYERS"
	envLayersByProf = "AI_RULEZ_PROFILE"
)

var layerNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// layerFileName is the file of the layer called name, beside config.toml.
func layerFileName(name string) string {
	return "config." + name + ".toml"
}

// Layer origins: how a layer became active.
const (
	LayerOriginDefault = "default" // named in default_layers of config.toml
	LayerOriginEnv     = "env"     // AI_RULEZ_LAYERS or AI_RULEZ_PROFILE
	LayerOriginOption  = "option"  // the WithLayers load option
)

// layerRef is an active layer and how it was activated.
type layerRef struct {
	name   string
	origin string
}

// activeLayers resolves the layers to merge, in order: default_layers of
// config.toml, then WithLayers, else AI_RULEZ_LAYERS, else AI_RULEZ_PROFILE.
// Blank, "default" and repeated names are dropped (a repeated name keeps its
// first position and origin).
func activeLayers(lo loadOptions, defaults []string) ([]layerRef, error) {
	var refs []layerRef
	seen := map[string]bool{}
	add := func(names []string, origin string) error {
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" || name == "default" || seen[name] {
				continue
			}
			if name == "local" {
				return oops.
					Hint("config.local.toml is the machine-local overlay and is always applied last; name the layer differently").
					Errorf("layer name %q is reserved", name)
			}
			if !layerNameRe.MatchString(name) {
				return oops.
					Hint("A layer name is letters, digits, '.', '_' or '-'").
					Errorf("invalid layer name %q", name)
			}
			seen[name] = true
			refs = append(refs, layerRef{name: name, origin: origin})
		}
		return nil
	}
	if lo.noLayers {
		return nil, nil
	}
	if err := add(defaults, LayerOriginDefault); err != nil {
		return nil, err
	}
	if lo.layersSet {
		return refs, add(lo.layers, LayerOriginOption)
	}
	value := strings.TrimSpace(lo.host.GetEnv(envLayers))
	if value == "" {
		value = strings.TrimSpace(lo.host.GetEnv(envLayersByProf))
	}
	return refs, add(strings.Split(value, ","), LayerOriginEnv)
}

// withLayers merges the config.<name>.toml files of the active layers over the
// main config, in order, so a later layer wins. A layer activated by
// default_layers must have a file; one activated by the environment or an option
// is skipped when it has none. It returns the merged Config, the merged document
// (nil when no layer applied) and the layers that applied. Layers use the same
// rules as the local overlay: tables merge per key, named lists such as
// mcp_servers and installed_skills merge entry by entry, other values are
// replaced.
func withLayers(v workspace.View, cfg *Config, mainPath, configDir string, lo loadOptions) (*Config, map[string]any, []string, error) { //nolint:gocritic // mirrors the loader signature; a result struct would add a type for one caller
	refs, err := activeLayers(lo, cfg.DefaultLayers)
	if err != nil || len(refs) == 0 {
		return cfg, nil, nil, err
	}
	var doc map[string]any
	var applied []string
	var files []string
	origins := map[string]string{}
	for _, ref := range refs {
		file := layerFileName(ref.name)
		path := filepath.Join(configDir, file)
		if !v.Exists(path) {
			if ref.origin == LayerOriginDefault {
				return nil, nil, nil, oops.
					With("path", path).
					Hint("Create "+file+" beside config.toml or remove "+ref.name+" from default_layers").
					Errorf("default_layers names layer %q but %s does not exist", ref.name, file)
			}
			continue
		}
		if err := checkConfigFileInside(v, path); err != nil {
			return nil, nil, nil, err
		}
		layer, err := readConfigDoc(v, path)
		if err != nil {
			return nil, nil, nil, err
		}
		if doc == nil {
			if doc, err = readMainDoc(v, mainPath, configDir); err != nil {
				return nil, nil, nil, err
			}
		}
		merged, warnings, err := MergeConfigDocsFrom(doc, layer, LayerSource(file))
		if err != nil {
			return nil, nil, nil, oops.With("path", path).Wrapf(err, "apply layer %s", ref.name)
		}
		for _, w := range warnings {
			cfg.Warn(w, "path", path)
		}
		doc = merged
		applied = append(applied, ref.name)
		files = append(files, file)
		origins[ref.name] = ref.origin
	}
	if doc == nil {
		return cfg, nil, nil, nil
	}
	out, err := decodeMergedDoc(mainPath, "layers "+strings.Join(files, ", "), "the layer files ("+strings.Join(files, ", ")+")", doc)
	if err != nil {
		return nil, nil, nil, err
	}
	out.ConfigFile = cfg.ConfigFile
	out.ActiveLayers = applied
	out.LayerOrigins = origins
	return out, doc, applied, nil
}

// LayerFiles lists the layer names that have a config.<name>.toml file in
// configDir, sorted, for tooling that shows what can be activated.
func LayerFiles(configDir string) []string {
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		base := e.Name()
		if e.IsDir() || !strings.HasPrefix(base, "config.") || !strings.HasSuffix(base, ".toml") || len(base) <= len("config..toml") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(base, "config."), ".toml")
		if name == "local" || !layerNameRe.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
