package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// YAMLToTOML converts a YAML (or JSON, which is a YAML subset) configuration to
// TOML text. Key order and comments are kept; `$schema` becomes the TOML key
// `schema`; null values are dropped, since TOML cannot hold them.
func YAMLToTOML(data []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, oops.Wrapf(err, "parse YAML")
	}
	if doc.Kind == 0 {
		return nil, oops.Errorf("the file is empty")
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	root = resolveAlias(root)
	if root.Kind != yaml.MappingNode {
		return nil, oops.Errorf("the top level must be a mapping")
	}
	var b strings.Builder
	writeComment(&b, doc.HeadComment, "")
	writeComment(&b, root.HeadComment, "")
	if err := emitTable(&b, root, nil, false); err != nil {
		return nil, err
	}
	out := strings.TrimRight(b.String(), "\n") + "\n"
	return []byte(out), nil
}

func resolveAlias(n *yaml.Node) *yaml.Node {
	for depth := 0; n != nil && n.Kind == yaml.AliasNode && n.Alias != nil && depth < 32; depth++ {
		n = n.Alias
	}
	return n
}

// writeComment writes a YAML comment block as TOML comment lines.
func writeComment(b *strings.Builder, comment, indent string) {
	if strings.TrimSpace(comment) == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimRight(comment, "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			b.WriteString("\n")
		case strings.HasPrefix(line, "#"):
			b.WriteString(indent + line + "\n")
		default:
			b.WriteString(indent + "# " + line + "\n")
		}
	}
}

func lineComment(n *yaml.Node) string {
	c := strings.TrimSpace(n.LineComment)
	if c == "" {
		return ""
	}
	if !strings.HasPrefix(c, "#") {
		c = "# " + c
	}
	return " " + c
}

func tomlKey(k string) string {
	if k == "$schema" {
		return "schema"
	}
	if bareKey.MatchString(k) {
		return k
	}
	return tomlQuote(k)
}

type entry struct {
	key   *yaml.Node
	value *yaml.Node
}

func entries(m *yaml.Node) ([]entry, error) {
	var out []entry
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], resolveAlias(m.Content[i+1])
		if k.Kind != yaml.ScalarNode {
			return nil, oops.Errorf("line %d: a mapping key must be a scalar", k.Line)
		}
		if k.Value == "<<" {
			return nil, oops.Errorf("line %d: YAML merge keys (<<) are not supported; expand them first", k.Line)
		}
		if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
			continue
		}
		out = append(out, entry{k, v})
	}
	return out, nil
}

func isTableSeq(n *yaml.Node) bool {
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		return false
	}
	for _, c := range n.Content {
		if resolveAlias(c).Kind != yaml.MappingNode {
			return false
		}
	}
	return true
}

// emitTable writes the entries of mapping m under the table path: scalar and
// inline values first, then sub-tables, then arrays of tables.
func emitTable(b *strings.Builder, m *yaml.Node, path []string, str bool) error { //nolint:gocyclo // scalars, arrays, inline tables and sub-tables in one ordered pass
	es, err := entries(m)
	if err != nil {
		return err
	}
	var subTables, tableArrays []entry
	for _, e := range es {
		switch {
		case e.value.Kind == yaml.MappingNode && len(e.value.Content) > 0:
			subTables = append(subTables, e)
		case isTableSeq(e.value):
			tableArrays = append(tableArrays, e)
		default:
			writeComment(b, e.key.HeadComment, "")
			s, err := inlineValue(e.value, 0, str || stringMapKey(e.key.Value))
			if err != nil {
				return err
			}
			if e.key.Value == "version" && len(path) == 0 {
				s = tomlQuote(e.value.Value)
			}
			fmt.Fprintf(b, "%s = %s%s\n", tomlKey(e.key.Value), s, lineComment(e.value))
		}
	}
	for _, e := range subTables {
		p := append(append([]string{}, path...), tomlKey(e.key.Value))
		b.WriteString("\n")
		writeComment(b, e.key.HeadComment, "")
		fmt.Fprintf(b, "[%s]\n", strings.Join(p, "."))
		if err := emitTable(b, e.value, p, str || stringMapKey(e.key.Value)); err != nil {
			return err
		}
	}
	for _, e := range tableArrays {
		p := append(append([]string{}, path...), tomlKey(e.key.Value))
		for _, item := range e.value.Content {
			item = resolveAlias(item)
			b.WriteString("\n")
			writeComment(b, item.HeadComment, "")
			if len(e.value.Content) > 0 && item == resolveAlias(e.value.Content[0]) {
				writeComment(b, e.key.HeadComment, "")
			}
			fmt.Fprintf(b, "[[%s]]\n", strings.Join(p, "."))
			if err := emitTable(b, item, p, str); err != nil {
				return err
			}
		}
	}
	return nil
}

// inlineValue renders a value in TOML inline form.
func inlineValue(n *yaml.Node, depth int, str bool) (string, error) {
	n = resolveAlias(n)
	if depth > 32 {
		return "", oops.Errorf("line %d: value nested too deeply", n.Line)
	}
	switch n.Kind {
	case yaml.ScalarNode:
		if str {
			return tomlQuote(n.Value), nil
		}
		return scalarValue(n), nil
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			if cc := resolveAlias(c); cc.Kind == yaml.ScalarNode && cc.Tag == "!!null" {
				continue
			}
			s, err := inlineValue(c, depth+1, str)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case yaml.MappingNode:
		es, err := entries(n)
		if err != nil {
			return "", err
		}
		parts := make([]string, 0, len(es))
		for _, e := range es {
			s, err := inlineValue(e.value, depth+1, str || stringMapKey(e.key.Value))
			if err != nil {
				return "", err
			}
			parts = append(parts, tomlKey(e.key.Value)+" = "+s)
		}
		if len(parts) == 0 {
			return "{}", nil
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	default:
		return "", oops.Errorf("line %d: unsupported YAML node", n.Line)
	}
}

func scalarValue(n *yaml.Node) string {
	switch n.Tag {
	case "!!bool":
		if n.Value == "true" || n.Value == "True" || n.Value == "TRUE" {
			return "true"
		}
		return "false"
	case "!!int":
		var v int64
		if err := n.Decode(&v); err == nil {
			return strconv.FormatInt(v, 10)
		}
	case "!!float":
		var v float64
		if err := n.Decode(&v); err == nil {
			return strconv.FormatFloat(v, 'g', -1, 64)
		}
	}
	// Strings, timestamps and every other tag keep their source text.
	if strings.Contains(n.Value, "\n") {
		return multilineString(n.Value)
	}
	return tomlQuote(n.Value)
}

// multilineString renders s as a TOML multi-line literal when it can, and as an
// escaped basic string otherwise.
func multilineString(s string) string {
	if !strings.Contains(s, "'''") && !strings.ContainsRune(s, '\r') {
		return "'''\n" + s + "'''"
	}
	return tomlQuote(s)
}

// tomlQuote renders s as a TOML basic string. The JSON string escapes (\b \f
// \n \r \t \" \\ \uXXXX) are a subset of the TOML ones.
func tomlQuote(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return strings.TrimRight(buf.String(), "\n")
}

// stringMapKey names the maps whose values are always strings (environment
// variables and HTTP headers): YAML lets an author write `PORT: 8080`, TOML
// decoding into a string map does not.
func stringMapKey(k string) bool { return k == "env" || k == "headers" }
