package config

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// MigrateLocalOverlayToTOML converts a config.local.yaml, .yml or .json file in
// configDir to config.local.toml at the document level (no Config round trip,
// so nothing is dropped or defaulted). The old file is removed only after the
// new one is written. It returns the old and new paths, or "" when there is
// nothing to convert.
func MigrateLocalOverlayToTOML(configDir string) (from, to string, err error) {
	target := filepath.Join(configDir, localVariantName(configTOMLFilename))
	var sources []string
	for _, name := range []string{
		localVariantName(configYAMLFilename), localVariantName(configYMLFilename), localVariantName(configJSONFilename),
	} {
		if fileExists(filepath.Join(configDir, name)) {
			sources = append(sources, filepath.Join(configDir, name))
		}
	}
	if len(sources) == 0 {
		return "", "", nil
	}
	if len(sources) > 1 || fileExists(target) {
		return "", "", oops.
			With("config_dir", configDir).
			Hint("Keep a single config.local.* file, then migrate again").
			Errorf("cannot migrate the local config: more than one config.local.* file exists")
	}

	doc, err := readConfigDoc(sources[0])
	if err != nil {
		return "", "", err
	}
	data, err := toml.Marshal(tomlSafeValue(pickNumberMode(doc)))
	if err != nil {
		return "", "", oops.With("path", sources[0]).Wrapf(err, "convert %s to TOML", filepath.Base(sources[0]))
	}
	// The overlay can hold secrets, so it is written owner-only.
	if err := refuseSymlink(target); err != nil {
		return "", "", err
	}
	if err := writeFileAtomic(target, data, 0o600); err != nil {
		return "", "", err
	}
	if err := os.Remove(sources[0]); err != nil {
		return "", "", oops.With("path", sources[0]).Wrapf(err, "remove %s", filepath.Base(sources[0]))
	}
	return sources[0], target, nil
}

// pickNumberMode keeps YAML numbers as numbers when the document still decodes
// into a Config that way, and otherwise as strings (an integer env value).
func pickNumberMode(doc map[string]any) map[string]any {
	if data, err := json.Marshal(doc); err == nil {
		if _, err := decodeConfigJSON(data, ""); err == nil {
			return doc
		}
	}
	return stringifyRawScalars(doc).(map[string]any) //nolint:errcheck // map in, map out
}

// tomlSafeValue prepares a decoded document for TOML encoding: YAML numbers
// kept as source text become numbers again and nulls (which TOML cannot hold)
// are dropped.
func tomlSafeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			if e != nil {
				out[k] = tomlSafeValue(e)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			if e != nil {
				out = append(out, tomlSafeValue(e))
			}
		}
		return out
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			return int64(t)
		}
		return t
	case rawScalar:
		if i, err := strconv.ParseInt(string(t), 0, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(string(t), 64); err == nil {
			return f
		}
		return string(t)
	}
	return v
}
