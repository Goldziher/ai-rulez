package publish

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// PluginsFile is the aggregate file that names every plugin of a multi-plugin
// release, so a plugin directory removed from (or swapped in) the tree is noticed.
const PluginsFile = "plugins.json"

// PluginsDir is the directory of a multi-plugin dist that holds one dist
// directory per plugin.
const PluginsDir = "plugins"

// PluginRef is one plugin the release promises: where its dist directory is and
// the digests of its manifest and archive.
type PluginRef struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Dir            string `json:"dir"`
	ManifestDigest string `json:"manifest_digest"` //nolint:tagliatelle // matches the manifest's snake_case fields
	BundleDigest   string `json:"bundle_digest"`   //nolint:tagliatelle // matches the manifest's snake_case fields
}

// PluginList is plugins.json.
type PluginList struct {
	SchemaVersion int         `json:"schema_version"` //nolint:tagliatelle // matches the manifest's snake_case fields
	Plugins       []PluginRef `json:"plugins"`
}

// NewPluginRef describes a built plugin dist for the list.
func NewPluginRef(d *Dist) PluginRef {
	m := d.Manifest
	name := m.Name + "-" + m.Version + ".manifest.json"
	return PluginRef{
		Name: m.Name, Version: m.Version, Dir: PluginsDir + "/" + m.Name,
		ManifestDigest: Digest(d.Files[name]), BundleDigest: m.Bundle.Digest,
	}
}

// Marshal renders the list sorted by plugin name.
func (l PluginList) Marshal() ([]byte, error) {
	l.SchemaVersion = SchemaVersion
	l.Plugins = append([]PluginRef(nil), l.Plugins...)
	sort.Slice(l.Plugins, func(i, j int) bool { return l.Plugins[i].Name < l.Plugins[j].Name })
	return marshalJSON(l)
}

// VerifyPluginList checks the plugin directories of a multi-plugin dist root
// against aggregate/plugins.json: every listed plugin has its directory with the
// recorded manifest, and no directory is unlisted. A multi-plugin dist whose
// aggregate does not name its plugins cannot tell a missing plugin from one that
// was never published, so that is a problem too.
func VerifyPluginList(root string) []Problem {
	var res VerifyResult
	raw, err := readRegular(filepath.Join(root, "aggregate", PluginsFile))
	if err != nil {
		res.add("aggregate/"+PluginsFile, "the aggregate does not name the plugins of this release (%v); a missing plugin directory would go unnoticed", err)
		return res.Problems
	}
	var list PluginList
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&list); err != nil || list.SchemaVersion != SchemaVersion {
		res.add("aggregate/"+PluginsFile, "invalid plugin list")
		return res.Problems
	}
	listed := map[string]bool{}
	for _, p := range list.Plugins {
		if ValidateName(p.Name, p.Version) != nil || p.Dir != PluginsDir+"/"+p.Name || listed[p.Name] {
			res.add("aggregate/"+PluginsFile, "invalid or duplicate plugin entry %q", p.Name)
			continue
		}
		listed[p.Name] = true
		manifest := path.Join(p.Dir, p.Name+"-"+p.Version+".manifest.json")
		data, rerr := readRegular(filepath.Join(root, filepath.FromSlash(manifest)))
		switch {
		case rerr != nil:
			res.add(p.Dir, "the release lists this plugin but its manifest is missing: %v", rerr)
		case Digest(data) != p.ManifestDigest:
			res.add(manifest, "digest is %s, %s records %s", Digest(data), PluginsFile, p.ManifestDigest)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, PluginsDir))
	if err != nil {
		res.add(PluginsDir, "cannot list the plugin directories: %v", err)
		return res.Problems
	}
	for _, e := range entries {
		if !listed[e.Name()] {
			res.add(PluginsDir+"/"+e.Name(), "present but not listed in aggregate/%s", PluginsFile)
		}
	}
	return res.Problems
}
