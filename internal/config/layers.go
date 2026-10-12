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

// activeLayerNames resolves the layers to merge, in order: WithLayers, else
// AI_RULEZ_LAYERS, else AI_RULEZ_PROFILE. Blank, "default" and repeated names are
// dropped (a repeated name keeps its first position).
func activeLayerNames(lo loadOptions) ([]string, error) {
	raw := lo.layers
	if !lo.layersSet {
		value := strings.TrimSpace(lo.host.GetEnv(envLayers))
		if value == "" {
			value = strings.TrimSpace(lo.host.GetEnv(envLayersByProf))
		}
		raw = strings.Split(value, ",")
	}
	var names []string
	seen := map[string]bool{}
	for _, name := range raw {
		name = strings.TrimSpace(name)
		if name == "" || name == "default" || seen[name] {
			continue
		}
		if name == "local" {
			return nil, oops.
				Hint("config.local.toml is the machine-local overlay and is always applied last; name the layer differently").
				Errorf("layer name %q is reserved", name)
		}
		if !layerNameRe.MatchString(name) {
			return nil, oops.
				Hint("A layer name is letters, digits, '.', '_' or '-'").
				Errorf("invalid layer name %q", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

// withLayers merges the config.<name>.toml files of the active layers over the
// main config, in order, so a later layer wins. A layer with no file is skipped.
// It returns the merged Config, the merged document (nil when no layer applied)
// and the layers that applied. Layers use the same rules as the local overlay:
// tables merge per key, named lists such as mcp_servers and installed_skills
// merge entry by entry, other values are replaced.
func withLayers(v workspace.View, cfg *Config, mainPath, configDir string, lo loadOptions) (*Config, map[string]any, []string, error) {
	names, err := activeLayerNames(lo)
	if err != nil || len(names) == 0 {
		return cfg, nil, nil, err
	}
	var doc map[string]any
	var applied []string
	for _, name := range names {
		path := filepath.Join(configDir, layerFileName(name))
		if !v.Exists(path) {
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
			if doc, err = readConfigDoc(v, mainPath); err != nil {
				return nil, nil, nil, err
			}
		}
		merged, warnings, err := MergeConfigDocs(doc, layer)
		if err != nil {
			return nil, nil, nil, oops.With("path", path).Wrapf(err, "apply layer %s", name)
		}
		for _, w := range warnings {
			cfg.Warn(w, "path", path)
		}
		doc = merged
		applied = append(applied, name)
	}
	if doc == nil {
		return cfg, nil, nil, nil
	}
	out, err := decodeMergedDoc(mainPath, filepath.Join(configDir, "config.<layer>.toml"), doc)
	if err != nil {
		return nil, nil, nil, err
	}
	out.ConfigFile = cfg.ConfigFile
	out.ActiveLayers = applied
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
