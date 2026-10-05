package tomlmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// renderKey renders one key part, quoting it when it is not a bare key.
func renderKey(part string) string {
	if bareKey.MatchString(part) {
		return part
	}
	quoted, _ := json.Marshal(part)
	return string(quoted)
}

func renderPath(path []string) string {
	parts := make([]string, len(path))
	for i, part := range path {
		parts[i] = renderKey(part)
	}
	return strings.Join(parts, ".")
}

// asTable converts a string-keyed map value to a generic map.
func asTable(value any) (map[string]any, bool) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	table := make(map[string]any, rv.Len())
	for _, k := range rv.MapKeys() {
		table[k.String()] = rv.MapIndex(k).Interface()
	}
	return table, true
}

// asTableArray converts a non-empty slice of maps to generic maps; TOML writes
// those as repeated [[tables]] rather than inline.
func asTableArray(value any) ([]map[string]any, bool) {
	rv := reflect.ValueOf(value)
	if (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) || rv.Len() == 0 {
		return nil, false
	}
	tables := make([]map[string]any, rv.Len())
	for i := range tables {
		table, ok := asTable(rv.Index(i).Interface())
		if !ok {
			return nil, false
		}
		tables[i] = table
	}
	return tables, true
}

// renderInline renders a value that is not a table as the right-hand side of a
// key/value pair. Strings are basic ("...") strings, arrays are written on one
// line, and a map nested in an array becomes an inline table; anything else (a
// date, say) is left to the TOML encoder.
func renderInline(value any) (string, error) {
	if value == nil {
		return "", oops.Errorf("TOML has no null value")
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", oops.Errorf("TOML has no null value")
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.String:
		return quote(rv.String()), nil
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		// A number that came through JSON is always a float64; write 5, not 5.0,
		// when it has no fraction.
		if f := rv.Float(); f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			return strconv.FormatInt(int64(f), 10), nil
		}
	case reflect.Slice, reflect.Array:
		return renderArray(rv)
	case reflect.Map:
		if table, ok := asTable(rv.Interface()); ok {
			return renderInlineTable(table)
		}
	default:
	}
	return encodeWithLibrary(rv.Interface())
}

// quote renders s as a TOML basic string.
func quote(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.ReplaceAll(strings.TrimSuffix(buf.String(), "\n"), "\x7f", `\u007f`)
}

func renderArray(rv reflect.Value) (string, error) {
	parts := make([]string, rv.Len())
	for i := range parts {
		part, err := renderInline(rv.Index(i).Interface())
		if err != nil {
			return "", err
		}
		parts[i] = part
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

func renderInlineTable(table map[string]any) (string, error) {
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		part, err := renderInline(table[name])
		if err != nil {
			return "", fmt.Errorf("key %q: %w", name, err)
		}
		parts[i] = renderKey(name) + " = " + part
	}
	if len(parts) == 0 {
		return "{}", nil
	}
	return "{ " + strings.Join(parts, ", ") + " }", nil
}

// encodeWithLibrary renders a scalar the hand-written cases do not cover.
func encodeWithLibrary(value any) (string, error) {
	encoded, err := toml.Marshal(map[string]any{"v": value})
	if err != nil {
		return "", oops.Wrapf(err, "encode TOML value")
	}
	rest, ok := strings.CutPrefix(strings.TrimSuffix(string(encoded), "\n"), "v = ")
	if !ok {
		return "", oops.Errorf("cannot render %T as an inline TOML value", value)
	}
	return rest, nil
}

// renderKeyValues renders the non-table entries of table as sorted key/value
// lines, and returns the names of the entries that are tables.
func renderKeyValues(table map[string]any) (lines string, tables []string, err error) {
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		value := table[name]
		if _, ok := asTable(value); ok {
			tables = append(tables, name)
			continue
		}
		if _, ok := asTableArray(value); ok {
			tables = append(tables, name)
			continue
		}
		rendered, err := renderInline(value)
		if err != nil {
			return "", nil, fmt.Errorf("key %q: %w", name, err)
		}
		b.WriteString(renderKey(name) + " = " + rendered + "\n")
	}
	return b.String(), tables, nil
}

// tableBlocks renders the table at path (a map or an array of maps) as TOML
// blocks: its header and key/value lines, then each sub-table in name order.
// Blocks are meant to be joined by a blank line.
func tableBlocks(path []string, value any) ([]string, error) {
	if elements, ok := asTableArray(value); ok {
		var blocks []string
		for _, element := range elements {
			more, err := blockFor("[["+renderPath(path)+"]]", path, element)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, more...)
		}
		return blocks, nil
	}
	table, ok := asTable(value)
	if !ok {
		return nil, oops.Errorf("value at %s is not a table", renderPath(path))
	}
	return blockFor("["+renderPath(path)+"]", path, table)
}

func blockFor(header string, path []string, table map[string]any) ([]string, error) {
	lines, subTables, err := renderKeyValues(table)
	if err != nil {
		return nil, err
	}
	var blocks []string
	// A table that holds only sub-tables needs no header of its own; an array
	// element always does.
	if lines != "" || len(subTables) == 0 || strings.HasPrefix(header, "[[") {
		blocks = append(blocks, header+"\n"+lines)
	}
	for _, name := range subTables {
		sub, err := tableBlocks(append(append([]string{}, path...), name), table[name])
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, sub...)
	}
	return blocks, nil
}

// renderDocument renders a whole document from a nested map: top-level
// key/value lines first, then tables, sorted by name.
func renderDocument(root map[string]any) (string, error) {
	lines, tables, err := renderKeyValues(root)
	if err != nil {
		return "", err
	}
	var blocks []string
	if lines != "" {
		blocks = append(blocks, lines)
	}
	for _, name := range tables {
		sub, err := tableBlocks([]string{name}, root[name])
		if err != nil {
			return "", err
		}
		blocks = append(blocks, sub...)
	}
	return strings.Join(blocks, "\n"), nil
}
