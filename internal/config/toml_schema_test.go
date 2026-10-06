package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type schemaDoc struct {
	root map[string]any
}

// resolve follows a $ref (local "#/$defs/..." only) and returns the target node.
func (d schemaDoc) resolve(node map[string]any) map[string]any {
	for i := 0; i < 20; i++ {
		ref, ok := node["$ref"].(string)
		if !ok {
			return node
		}
		target := d.root
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			next, ok := target[part].(map[string]any)
			if !ok {
				return map[string]any{}
			}
			target = next
		}
		node = target
	}
	return node
}

// variants returns the node plus every allOf/oneOf/anyOf branch, resolved.
func (d schemaDoc) variants(node map[string]any) []map[string]any {
	node = d.resolve(node)
	out := []map[string]any{node}
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		list, _ := node[key].([]any)
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				out = append(out, d.variants(m)...)
			}
		}
	}
	return out
}

// property finds a named property across the variants of node.
func (d schemaDoc) property(node map[string]any, name string) (map[string]any, bool) {
	for _, v := range d.variants(node) {
		props, _ := v["properties"].(map[string]any)
		if p, ok := props[name].(map[string]any); ok {
			return p, true
		}
	}
	return nil, false
}

// child returns the schema of the elements of an array or the values of a map.
func (d schemaDoc) child(node map[string]any) (map[string]any, bool) {
	for _, v := range d.variants(node) {
		for _, key := range []string{"items", "additionalProperties"} {
			if m, ok := v[key].(map[string]any); ok {
				return m, true
			}
		}
		if pp, ok := v["patternProperties"].(map[string]any); ok {
			for _, p := range pp {
				if m, ok := p.(map[string]any); ok {
					return m, true
				}
			}
		}
	}
	return nil, false
}

// tomlFieldKeys lists the serialized fields of a struct, inlined structs included.
func tomlFieldKeys(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("toml")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if strings.Contains(opts, "inline") || (f.Anonymous && name == "") {
			ft := f.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k, v := range tomlFieldKeys(ft) {
					out[k] = v
				}
				continue
			}
		}
		if name == "" {
			continue
		}
		out[name] = f.Type
	}
	return out
}

// walkTOMLType checks every key of t against node and recurses into nested
// structs, slices of structs and maps of structs.
func walkTOMLType(d schemaDoc, t reflect.Type, node map[string]any, path string, stack map[reflect.Type]bool, missing *[]string) {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		if t.Kind() == reflect.Map || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			child, ok := d.child(node)
			if !ok {
				return // an untyped container: nothing to check below it
			}
			node = child
		}
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || stack[t] {
		return
	}
	stack[t] = true
	defer delete(stack, t)
	for key, ft := range tomlFieldKeys(t) {
		prop, ok := d.property(node, key)
		if !ok {
			*missing = append(*missing, path+key)
			continue
		}
		walkTOMLType(d, ft, prop, path+key+".", stack, missing)
	}
}

func loadConfigSchema(t *testing.T) schemaDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "ai-rules.schema.json"))
	require.NoError(t, err)
	var root map[string]any
	require.NoError(t, json.Unmarshal(raw, &root))
	return schemaDoc{root: root}
}

// schemaGapAllowed lists writer keys the schema leaves out on purpose.
// [plugin] hook groups share the HookGroup type with the top-level [[hooks]],
// but a plugin hook group is rendered per runtime and rejects targets and
// matchers (plugin_authoring_validation.go), so the schema does not offer them.
var schemaGapAllowed = map[string]bool{
	"plugin.hooks.targets":  true,
	"plugin.hooks.matchers": true,
}

// Every key the TOML writer can emit must be a known key of the published JSON
// schema, or editors flag a config the tool itself wrote and `validate --strict`
// rejects it.
func TestTOMLOutputKeysAreInTheSchema(t *testing.T) {
	// Arrange
	d := loadConfigSchema(t)

	// Act
	var missing []string
	walkTOMLType(d, reflect.TypeOf(tomlOutput{}), d.root, "", map[reflect.Type]bool{}, &missing)

	// Assert
	var unexpected []string
	for _, key := range missing {
		if !schemaGapAllowed[key] {
			unexpected = append(unexpected, key)
		}
	}
	sort.Strings(unexpected)
	assert.Empty(t, unexpected, "keys the TOML writer emits that schema/ai-rules.schema.json does not define")
}

// A top-level schema property the writer has no field for is dropped by every
// config rewrite (CRUD, MCP, migrate, convert).
func TestSchemaTopLevelKeysAreInTheTOMLWriter(t *testing.T) {
	// Arrange
	d := loadConfigSchema(t)
	have := tomlFieldKeys(reflect.TypeOf(tomlOutput{}))
	props, _ := d.root["properties"].(map[string]any)

	// Act
	var missing []string
	for key := range props {
		if key == "$schema" || key == "$comment" {
			continue // $schema is `schema` in the writer; $comment is JSON-schema metadata
		}
		if _, ok := have[key]; !ok {
			missing = append(missing, key)
		}
	}

	// Assert
	sort.Strings(missing)
	assert.Empty(t, missing, "schema properties tomlOutput cannot write")
}
