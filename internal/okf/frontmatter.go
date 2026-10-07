package okf

import (
	"bytes"
	"strings"

	"github.com/samber/oops"

	"gopkg.in/yaml.v3"
)

// Frontmatter is a parsed YAML frontmatter block. The node tree keeps key order
// and scalar spelling so a round trip does not reformat values.
type Frontmatter struct {
	// Present is true when a leading --- block was found, even an empty one.
	Present bool
	// Root is the mapping node; nil when no block was found or it did not parse.
	Root *yaml.Node
	// Err is set when the block was present but not a parseable mapping.
	Err error
}

// SplitFrontmatter separates a leading YAML block from the markdown body. The
// closing delimiter must be a line that is exactly ---, so a horizontal rule in
// the body is never mistaken for it. A UTF-8 BOM and CRLF line ends are tolerated.
func SplitFrontmatter(src []byte) (fm Frontmatter, body string) {
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	text := string(src)
	first, rest, ok := strings.Cut(text, "\n")
	if !ok || strings.TrimRight(first, "\r") != "---" {
		return Frontmatter{}, text
	}
	lines := strings.Split(rest, "\n")
	var block strings.Builder
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == "---" {
			fm = parseBlock(block.String())
			fm.Present = true
			return fm, trimSeparator(strings.Join(lines[i+1:], "\n"))
		}
		block.WriteString(strings.TrimRight(line, "\r"))
		block.WriteByte('\n')
	}
	// No closing delimiter: the file has no usable frontmatter.
	return Frontmatter{Present: true, Err: oops.Errorf("frontmatter block is not closed with ---")}, text
}

func parseBlock(block string) Frontmatter {
	if strings.TrimSpace(block) == "" {
		return Frontmatter{Root: &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(block), &doc); err != nil {
		return Frontmatter{Err: oops.Wrapf(err, "frontmatter is not valid YAML")}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return Frontmatter{Err: oops.Errorf("frontmatter is empty")}
	}
	root := doc.Content[0]
	if root.Kind == yaml.AliasNode {
		return Frontmatter{Err: oops.Errorf("frontmatter is an alias, not a mapping")}
	}
	if root.Kind != yaml.MappingNode {
		return Frontmatter{Err: oops.Errorf("frontmatter is not a mapping")}
	}
	return Frontmatter{Root: root}
}

// Lookup returns the value node of a top-level key.
func (f Frontmatter) Lookup(key string) *yaml.Node {
	if f.Root == nil {
		return nil
	}
	for i := 0; i+1 < len(f.Root.Content); i += 2 {
		if f.Root.Content[i].Value == key {
			return f.Root.Content[i+1]
		}
	}
	return nil
}

// Scalar returns the trimmed text of a scalar key, or "" when it is absent or
// not a scalar.
func (f Frontmatter) Scalar(key string) string {
	n := f.Lookup(key)
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(n.Value)
}

// Keys lists the top-level keys in document order.
func (f Frontmatter) Keys() []string {
	if f.Root == nil {
		return nil
	}
	keys := make([]string, 0, len(f.Root.Content)/2)
	for i := 0; i+1 < len(f.Root.Content); i += 2 {
		keys = append(keys, f.Root.Content[i].Value)
	}
	return keys
}

// Field is one frontmatter entry to write.
type Field struct {
	Key   string
	Value any
}

// MarshalFrontmatter renders fields, in the order given, as a frontmatter block
// including both --- lines. A value may be a Go value or a *yaml.Node.
func MarshalFrontmatter(fields []Field) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, f := range fields {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: f.Key}
		var val yaml.Node
		if n, ok := f.Value.(*yaml.Node); ok {
			val = *n
		} else if err := val.Encode(f.Value); err != nil {
			return nil, oops.Wrapf(err, "encode frontmatter key %q", f.Key)
		}
		root.Content = append(root.Content, key, &val)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, oops.Wrapf(err, "encode frontmatter")
	}
	if err := enc.Close(); err != nil {
		return nil, oops.Wrapf(err, "encode frontmatter")
	}
	buf.WriteString("---\n")
	return buf.Bytes(), nil
}

// trimSeparator drops the single blank line conventionally written between the
// closing delimiter and the body.
func trimSeparator(body string) string {
	return strings.TrimPrefix(strings.TrimPrefix(body, "\r\n"), "\n")
}
