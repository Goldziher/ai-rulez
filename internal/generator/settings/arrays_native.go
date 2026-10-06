package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// isJSONDocument reports whether the document at docPath is JSON (or JSONC); an
// empty path, which names no document yet, is treated as JSON.
func isJSONDocument(docPath string) bool {
	switch strings.ToLower(filepath.Ext(docPath)) {
	case ".toml", ".yaml", ".yml":
		return false
	}
	return true
}

// readNativeArray reads the array at path of a TOML or YAML document with the
// values in the types the document's own parser gives them, so the user's
// elements are written back as they were. It returns nil when the document or
// the key is absent or not an array.
func readNativeArray(docPath string, path []string) []any {
	data, err := os.ReadFile(docPath) //nolint:gosec // path is derived from the preset layout and the base directory
	if err != nil {
		return nil
	}
	var tree map[string]any
	switch strings.ToLower(filepath.Ext(docPath)) {
	case ".toml":
		err = toml.Unmarshal(data, &tree)
	default:
		err = yaml.Unmarshal(data, &tree)
	}
	if err != nil {
		return nil
	}
	var node any = tree
	for _, key := range path {
		object, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node = object[key]
	}
	list, _ := node.([]any)
	return list
}

// nativeArrayKey is arrayKey for a TOML or YAML document: the same ownership
// rules, with elements as plain values instead of raw JSON, because those
// engines write what they are given.
func nativeArrayKey(cfg *config.Config, docPath string, path []string, ours []json.RawMessage) jsonmerge.OwnedKey {
	wanted := make([]any, 0, len(ours))
	for _, raw := range ours {
		var value any
		if json.Unmarshal(raw, &value) == nil {
			wanted = append(wanted, value)
		}
	}
	value, claimed := planElements(previousElementClaims(cfg, docPath, path), readNativeArray(docPath, path), wanted)
	if value == nil {
		value = []any{}
	}
	if claimed == nil {
		claimed = []any{}
	}
	return jsonmerge.OwnedKey{Path: path, Value: value, Elements: claimed}
}
