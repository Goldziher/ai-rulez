package schema

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/samber/oops"
)

//go:embed ai-rules-local.schema.json
var localSchemaJSON []byte

// LocalSchemaFile is the schema for config.local.* overlay files.
const LocalSchemaFile = "ai-rules-local.schema.json"

const (
	schemaTypeKey     = "type"
	schemaNameKey     = "name"
	schemaRequiredKey = "required"
)

// removableListKeys are the named lists whose overlay entries may carry
// `remove = true`.
var removableListKeys = []string{"mcp_servers", "plugins", "includes", "installed_skills", "marketplaces", "scopes", "roles"}

// DeriveLocalSchema derives the overlay schema from the main config schema:
// nothing is required, list entries accept `remove`, and presets accept
// "!name" strings that drop a shared preset.
func DeriveLocalSchema(main []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(main, &doc); err != nil {
		return nil, oops.Wrapf(err, "parse main schema")
	}

	identity := captureIdentityRequired(doc)
	dropRequired(doc)
	delete(doc, "examples")
	if id, ok := doc["$id"].(string); ok {
		doc["$id"] = strings.Replace(id, ConfigSchemaFile, LocalSchemaFile, 1)
	}
	doc["title"] = "AI Rules Local Overlay Configuration"
	doc["description"] = "Schema for config.local.* machine-local overlay files merged onto the shared ai-rulez config"

	props, ok := doc[propertiesField].(map[string]any)
	if !ok {
		return nil, oops.Errorf("main schema has no properties")
	}
	for _, key := range removableListKeys {
		if err := addRemoveField(props, key); err != nil {
			return nil, err
		}
	}
	restoreIdentityRequired(props, identity)
	if err := addPresetDrop(props); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, oops.Wrapf(err, "encode local schema")
	}
	return buf.Bytes(), nil
}

// captureIdentityRequired records which named-list items require `name` in the
// main schema; an overlay entry without one cannot be matched to a shared entry.
func captureIdentityRequired(doc map[string]any) map[string]bool {
	out := map[string]bool{}
	props, _ := doc[propertiesField].(map[string]any) //nolint:errcheck // nil map is fine
	for _, key := range removableListKeys {
		list, _ := props[key].(map[string]any)     //nolint:errcheck // nil map is fine
		items, _ := list["items"].(map[string]any) //nolint:errcheck // nil map is fine
		req, _ := items["required"].([]any)        //nolint:errcheck // nil slice is fine
		for _, r := range req {
			if r == schemaNameKey {
				out[key] = true
			}
		}
	}
	return out
}

// restoreIdentityRequired re-requires `name` on named-list items (scopes may
// identify an entry by `name` or `path` instead).
func restoreIdentityRequired(props map[string]any, identity map[string]bool) {
	for key := range identity {
		list, _ := props[key].(map[string]any)     //nolint:errcheck // checked by addRemoveField
		items, _ := list["items"].(map[string]any) //nolint:errcheck // checked by addRemoveField
		items["required"] = []any{"name"}
	}
	scopes, _ := props["scopes"].(map[string]any) //nolint:errcheck // checked by addRemoveField
	if items, ok := scopes["items"].(map[string]any); ok {
		items["anyOf"] = []any{
			map[string]any{schemaRequiredKey: []any{schemaNameKey}},
			map[string]any{schemaRequiredKey: []any{"path"}},
		}
	}
}

// dropRequired removes every `required` list (a property literally named
// "required" is a map and is left alone).
func dropRequired(v any) {
	switch t := v.(type) {
	case map[string]any:
		if _, isList := t["required"].([]any); isList {
			delete(t, "required")
		}
		for _, e := range t {
			dropRequired(e)
		}
	case []any:
		for _, e := range t {
			dropRequired(e)
		}
	}
}

func addRemoveField(props map[string]any, key string) error {
	list, _ := props[key].(map[string]any)                  //nolint:errcheck // nil map is handled below
	items, _ := list["items"].(map[string]any)              //nolint:errcheck // nil map is handled below
	itemProps, _ := items[propertiesField].(map[string]any) //nolint:errcheck // nil map is handled below
	if itemProps == nil {
		return oops.Errorf("main schema property %s has no item properties", key)
	}
	itemProps["remove"] = map[string]any{
		schemaTypeKey: "boolean",
		"description": "Set to true to delete the shared entry with the same name",
	}
	return nil
}

func addPresetDrop(props map[string]any) error {
	presets, _ := props["presets"].(map[string]any) //nolint:errcheck // nil map is handled below
	items, _ := presets["items"].(map[string]any)   //nolint:errcheck // nil map is handled below
	oneOf, _ := items["oneOf"].([]any)              //nolint:errcheck // nil slice is handled below
	if oneOf == nil {
		return oops.Errorf("main schema presets has no item alternatives")
	}
	items["oneOf"] = append(oneOf, map[string]any{
		schemaTypeKey: "string",
		"pattern":     "^!.+",
		"description": "\"!name\" drops the shared preset of that name",
	})
	return nil
}

// ValidateLocalFile validates a config.local.* overlay file against the local
// overlay schema.
func ValidateLocalFile(path string) error {
	return validateFileAgainst(path, localSchemaJSON, "local config", true)
}
