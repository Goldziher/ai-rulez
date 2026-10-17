package config

import (
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// One file per definition: .ai-rulez/plugins/<name>.toml holds one plugin
// definition and .ai-rulez/marketplaces/<name>.toml one marketplace definition,
// with the keys of a [[plugins_authoring]] or [[marketplaces_authoring]] entry at
// the top level. They load by default, so each can have its own CODEOWNERS entry.
const (
	tomlExt            = ".toml"
	pluginDefsDir      = "plugins"
	marketplaceDefsDir = "marketplaces"
)

// definitionFile is one parsed plugins/<name>.toml or marketplaces/<name>.toml.
type definitionFile struct {
	// rel is the path relative to the config directory, with forward slashes.
	rel  string
	name string
	doc  map[string]any
}

// readDefinitionFiles reads the definition files beside config.toml, sorted by
// file name. A name inside a file must equal the file name; an absent name is
// taken from it.
func readDefinitionFiles(v workspace.View, configDir string) (plugins, marketplaces []definitionFile, err error) {
	if plugins, err = readDefinitionDir(v, configDir, pluginDefsDir, reflect.TypeFor[MarketplacePlugin](), "plugin"); err != nil {
		return nil, nil, err
	}
	if marketplaces, err = readDefinitionDir(v, configDir, marketplaceDefsDir, reflect.TypeFor[MarketplaceDefinition](), "marketplace"); err != nil {
		return nil, nil, err
	}
	return plugins, marketplaces, nil
}

// isNonDirectory reports whether path exists and is not a directory. Windows
// reports listing a file as "path not found", which must not read as an absent
// directory.
func isNonDirectory(v workspace.View, path string) bool {
	info, err := v.Stat(path)
	return err == nil && !info.IsDir()
}

func readDefinitionDir(v workspace.View, configDir, sub string, shape reflect.Type, kind string) ([]definitionFile, error) {
	dir := filepath.Join(configDir, sub)
	entries, err := v.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) && !isNonDirectory(v, dir) {
		return nil, nil // no directory means no definitions
	}
	if err != nil {
		return nil, oops.With("path", dir).Wrapf(err, "read %s definitions directory %s", kind, sub)
	}
	var out []definitionFile
	for _, e := range entries {
		base := e.Name()
		if e.IsDir() || filepath.Ext(base) != tomlExt {
			continue
		}
		path := filepath.Join(dir, base)
		rel := sub + "/" + base
		if err := checkConfigFileInside(v, path); err != nil {
			return nil, err
		}
		data, err := v.ReadFile(path)
		if err != nil {
			return nil, oops.With("path", path).Wrapf(err, "read %s definition %s", kind, rel)
		}
		doc := map[string]any{}
		if err := toml.Unmarshal(data, &doc); err != nil {
			return nil, oops.With("path", path).Hint("Check the file syntax").Wrapf(err, "parse %s", rel)
		}
		stem := strings.TrimSuffix(base, tomlExt)
		if unknown := unknownDefinitionKeys(doc, shape); len(unknown) > 0 {
			return nil, oops.With("path", path).
				Hint("Check for typos; valid keys: "+strings.Join(definitionKeys(shape), ", ")).
				Errorf("unknown key(s) in %s definition %s: %s", kind, rel, strings.Join(unknown, ", "))
		}
		switch name, _ := doc[docKeyName].(string); { //nolint:errcheck // an absent or mistyped name reads as empty
		case name == "":
			doc[docKeyName] = stem
		case name != stem:
			return nil, oops.With("path", path).
				Hint("Rename the file to "+name+tomlExt+" or set name = \""+stem+"\"").
				Errorf("%s definition %s sets name %q: the file name must equal the name", kind, rel, name)
		}
		out = append(out, definitionFile{rel: rel, name: stem, doc: doc})
	}
	return out, nil
}

// definitionKeys lists the TOML keys of a definition struct, inline structs
// included, sorted.
func definitionKeys(t reflect.Type) []string {
	keys := map[string]bool{}
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			if tag := strings.Split(f.Tag.Get("toml"), ",")[0]; tag != "" && tag != "-" {
				keys[tag] = true
			}
		}
	}
	walk(t)
	return slices.Sorted(maps.Keys(keys))
}

func unknownDefinitionKeys(doc map[string]any, t reflect.Type) []string {
	known := map[string]bool{}
	for _, k := range definitionKeys(t) {
		known[k] = true
	}
	var unknown []string
	for _, k := range slices.Sorted(maps.Keys(doc)) {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

// withDefinitionFiles adds the definitions read from plugins/ and marketplaces/ to
// cfg. A name defined both inline and in a file, or in two files of different
// kinds of plugin definition, is an error naming both places.
func withDefinitionFiles(v workspace.View, cfg *Config, configDir string) error {
	plugins, marketplaces, err := readDefinitionFiles(v, configDir)
	if err != nil {
		return err
	}
	if len(plugins) == 0 && len(marketplaces) == 0 {
		return nil
	}
	inline := map[string]string{}
	for _, p := range cfg.PluginDefinitions() {
		inline[p.Name] = "config.toml"
	}
	for _, f := range plugins {
		if where, dup := inline[f.name]; dup {
			return oops.With("plugin", f.name).
				Hint("Keep the definition in one place").
				Errorf("plugin %q is defined in both %s and %s", f.name, where, f.rel)
		}
		def, err := decodeDefinition[MarketplacePlugin](f)
		if err != nil {
			return err
		}
		cfg.PluginsAuthoring = append(cfg.PluginsAuthoring, def)
	}
	inlineMkt := map[string]bool{}
	for i := range cfg.MarketplacesAuthoring {
		inlineMkt[cfg.MarketplacesAuthoring[i].Name] = true
	}
	for _, f := range marketplaces {
		if inlineMkt[f.name] {
			return oops.With("marketplace", f.name).
				Hint("Keep the definition in one place").
				Errorf("marketplace %q is defined in both config.toml and %s", f.name, f.rel)
		}
		def, err := decodeDefinition[MarketplaceDefinition](f)
		if err != nil {
			return err
		}
		cfg.MarketplacesAuthoring = append(cfg.MarketplacesAuthoring, def)
	}
	return nil
}

func decodeDefinition[T any](f definitionFile) (T, error) {
	var out T
	data, err := toml.Marshal(f.doc)
	if err == nil {
		err = toml.Unmarshal(data, &out)
	}
	if err != nil {
		return out, oops.With("file", f.rel).Wrapf(SanitizeDecodeError(err), "decode definition %s", f.rel)
	}
	return out, nil
}

// readMainDoc reads config.toml as a generic document with the definition files
// folded in as [[plugins_authoring]] and [[marketplaces_authoring]] entries, the
// base that layers and the local overlay merge over.
func readMainDoc(v workspace.View, mainPath, configDir string) (map[string]any, error) {
	doc, err := readConfigDoc(v, mainPath)
	if err != nil {
		return nil, err
	}
	if isRootLayoutMain(mainPath, configDir) {
		return doc, nil // no directory holds definition files
	}
	plugins, marketplaces, err := readDefinitionFiles(v, configDir)
	if err != nil {
		return nil, err
	}
	for key, files := range map[string][]definitionFile{"plugins_authoring": plugins, "marketplaces_authoring": marketplaces} {
		if len(files) == 0 {
			continue
		}
		list, _ := doc[key].([]any) //nolint:errcheck // an absent or mistyped list starts empty
		for _, f := range files {
			list = append(list, f.doc)
		}
		doc[key] = list
	}
	return doc, nil
}
