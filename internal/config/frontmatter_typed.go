package config

import (
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Agent Skills specification fields (agentskills.io/specification) beyond name
// and description. They are carried into every preset's SKILL.md.
var skillSpecKeys = []string{"license", "compatibility", "metadata", "allowed-tools"}

// SkillSpecKeys returns the optional Agent Skills specification frontmatter
// keys that presets carry through to generated SKILL.md files.
func SkillSpecKeys() []string {
	return append([]string(nil), skillSpecKeys...)
}

// frontmatterDescription is the description key, always text.
const frontmatterDescription = "description"

// knownFrontmatterKeys are the frontmatter keys Metadata models as typed
// fields (the yaml tags of its struct, Extra excluded); every other key is an
// "extra" and is kept with its original YAML type.
var knownFrontmatterKeys = func() map[string]bool {
	keys := make(map[string]bool)
	t := reflect.TypeOf(Metadata{})
	for i := 0; i < t.NumField(); i++ {
		name, opts, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name == "" || name == "-" || strings.Contains(opts, "inline") {
			continue
		}
		keys[name] = true
	}
	return keys
}()

// extraNodes parses frontmatter YAML and returns the typed, normalized value of
// every key Metadata does not model as a field. It returns nil when the YAML has
// no such key or cannot be parsed as a mapping.
func extraNodes(frontmatterYAML string) map[string]*yaml.Node {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &doc); err != nil {
		return nil
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	root := doc.Content[0]
	var nodes map[string]*yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		if key.Kind != yaml.ScalarNode || knownFrontmatterKeys[key.Value] {
			continue
		}
		if nodes == nil {
			nodes = make(map[string]*yaml.Node)
		}
		nodes[key.Value] = normalizeNode(root.Content[i+1])
	}
	return nodes
}

// normalizeNode deep-copies a value node for re-emission: aliases are
// resolved, anchors and comments dropped, and collections written in block
// style, so the output does not depend on how the author laid the value out.
// Scalar text, tag and quoting are kept, which preserves booleans, numbers and
// dates exactly as written.
func normalizeNode(n *yaml.Node) *yaml.Node {
	return normalizeNodeDepth(n, 0)
}

const maxNodeDepth = 64

func normalizeNodeDepth(n *yaml.Node, depth int) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode && n.Alias != nil && depth < maxNodeDepth {
		return normalizeNodeDepth(n.Alias, depth+1)
	}
	out := &yaml.Node{Kind: n.Kind, Tag: n.Tag, Value: n.Value, Style: n.Style}
	switch n.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		out.Style &^= yaml.FlowStyle
		if depth >= maxNodeDepth {
			return out
		}
		for _, c := range n.Content {
			out.Content = append(out.Content, normalizeNodeDepth(c, depth+1))
		}
	case yaml.ScalarNode:
		// Keep Style so quoted strings stay quoted.
	}
	return out
}

// typedScalarNode reports whether a value node must be emitted as a node to
// keep its type: everything except a plain string scalar, which the string
// accessor already carries without changing how it is written.
func typedScalarNode(n *yaml.Node) bool {
	return n.Kind != yaml.ScalarNode || n.Tag != "!!str"
}

// TypedExtra returns the value of an extra frontmatter key with its original
// YAML type (bool, number, date, list, map). A string value is returned as a Go
// string. The second result is false when the key is absent or empty.
//
// Emit the returned value with yaml.Marshal; a *yaml.Node keeps the author's
// scalar spelling, so booleans and dates are not re-quoted or reformatted.
func (m *Metadata) TypedExtra(key string) (any, bool) {
	if m == nil {
		return nil, false
	}
	if n, ok := m.extraNodes[key]; ok && n != nil && key != frontmatterDescription && typedScalarNode(n) {
		return cloneNode(n), true
	}
	if v, ok := m.Extra[key]; ok && v != "" {
		return v, true
	}
	return nil, false
}

// cloneNode returns a deep copy, so callers may edit a value without touching
// the parsed metadata shared across presets.
func cloneNode(n *yaml.Node) *yaml.Node {
	return normalizeNodeDepth(n, 0)
}

// SkillSpecFields returns the Agent Skills specification fields the skill sets
// (license, compatibility, metadata, allowed-tools) with their original types.
// The result is empty when none is set.
func (m *Metadata) SkillSpecFields() map[string]any {
	fields := make(map[string]any)
	for _, key := range skillSpecKeys {
		if v, ok := m.TypedExtra(key); ok {
			fields[key] = v
		}
	}
	return fields
}

// ExtraBool reports a boolean extra frontmatter key. set is false when the key
// is absent or not a recognized boolean. It accepts the spellings Claude Code
// documents (true/false, yes/no, on/off, 1/0, in any letter case).
func (m *Metadata) ExtraBool(key string) (value, set bool) {
	if m == nil {
		return false, false
	}
	raw := ""
	if n, ok := m.extraNodes[key]; ok && n != nil && n.Kind == yaml.ScalarNode {
		raw = n.Value
	} else if v, ok := m.Extra[key]; ok {
		raw = v
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "on", "1":
		return true, true
	case "false", "no", "off", "0":
		return false, true
	}
	return false, false
}

// flowText renders a value node as compact single-line flow YAML
// ({a: 1, b: [x, y]}): a readable, valid-YAML string form for collections in
// Metadata.Extra, instead of Go's map[...] syntax. Empty on failure.
func flowText(n *yaml.Node) string {
	flow := setFlow(normalizeNode(n))
	out, err := yaml.Marshal(flow)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func setFlow(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		n.Style |= yaml.FlowStyle
		for _, c := range n.Content {
			setFlow(c)
		}
	}
	return n
}
