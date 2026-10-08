package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseFrontmatterPublic is the exported version of parseFrontmatter for use by other packages
func ParseFrontmatterPublic(content string) (metadata *Metadata, body string) {
	metadata, body, _ = parseFrontmatter(content)
	return metadata, body
}

// ParseFrontmatterChecked is ParseFrontmatterPublic that also reports a delimited
// frontmatter block whose YAML could not be parsed (it is dropped from body), so
// the caller, which knows the file and its logger, can say so.
func ParseFrontmatterChecked(content string) (metadata *Metadata, body string, malformed bool) {
	return parseFrontmatter(content)
}

// hasUnclosedFrontmatter reports whether content opens a frontmatter block with
// "---" and never closes it.
func hasUnclosedFrontmatter(content string) bool {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return false
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return false
		}
	}
	return true
}

// parseFrontmatter parses optional YAML frontmatter from content.
// Returns metadata (nil if none), the actual content (without frontmatter),
// and a malformed flag set to true when a delimited frontmatter block was
// present but its YAML was unparseable (e.g. unquoted values containing ": ").
func parseFrontmatter(content string) (metadata *Metadata, body string, malformed bool) {
	// Check if content starts with ---
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, content, false
	}

	// Find the closing ---
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return nil, content, false
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		// No closing ---, treat as regular content
		return nil, content, false
	}

	// Extract frontmatter YAML
	frontmatterLines := lines[1:endIdx]
	frontmatterYAML := normalizeOKFFrontmatter(strings.Join(frontmatterLines, "\n"))

	// Extract actual content (after the closing ---). Computed up front so a
	// parse failure still strips the delimited block from the body.
	body = strings.Join(lines[endIdx+1:], "\n")
	body = strings.TrimPrefix(body, "\n")

	// First try direct unmarshal into Metadata (works for simple key-value frontmatter)
	var parsedMetadata Metadata
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &parsedMetadata); err != nil {
		// Direct unmarshal failed (e.g., nested YAML objects in Extra fields).
		// Fall back to raw map parsing: extract known fields and stringify the rest.
		result, ok := parseFrontmatterFromRawMap(frontmatterYAML)
		if !ok {
			// A delimited frontmatter block is present but its YAML is
			// unparseable (e.g. an unquoted value containing ": "). Do NOT
			// return the content unstripped — that would re-emit the raw block
			// after the generated frontmatter (#156). Warn loudly and strip it.
			// Mark it malformed so Config.Validate can fail fast (#175); the
			// caller, which knows the file and the logger, says so.
			return nil, body, true
		}
		parsedMetadata = result
	}

	parsedMetadata.extraNodes = extraNodes(frontmatterYAML)
	metadata = &parsedMetadata
	return metadata, body, false
}

// parseFrontmatterFromRawMap parses frontmatter YAML into Metadata via a raw map.
// Used as fallback when direct unmarshal fails (e.g., nested YAML objects).
func parseFrontmatterFromRawMap(frontmatterYAML string) (Metadata, bool) {
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &rawMap); err != nil {
		return Metadata{}, false
	}

	m := Metadata{Extra: make(map[string]string)}
	nodes := extraNodes(frontmatterYAML)
	for k, v := range rawMap {
		if dst := m.scalarField(k); dst != nil {
			*dst = fmt.Sprintf("%v", v)
			continue
		}
		switch k {
		case "priority":
			m.Priority = fmt.Sprintf("%v", v)
		case "targets":
			m.Targets = stringSliceFromAny(v)
		case "aliases":
			m.Aliases = stringSliceFromAny(v)
		case "tools":
			m.Tools = stringSliceFromAny(v)
		case "skills":
			m.Skills = stringSliceFromAny(v)
		case "keywords":
			m.Keywords = stringSliceFromAny(v)
		case "globs":
			m.Globs = NormalizeGlobs(stringSliceFromAny(v))
		case "paths":
			m.Paths = NormalizeGlobs(stringSliceFromAny(v))
		default:
			m.Extra[k] = extraText(nodes[k], v)
		}
	}

	return m, true
}

// extraText is the string form of an extra frontmatter value. A scalar keeps its
// source text (a date stays 2026-10-01, not the Go time rendering); a collection
// becomes flow YAML.
func extraText(n *yaml.Node, v any) string {
	if n != nil {
		if n.Kind == yaml.ScalarNode {
			return n.Value
		}
		if text := flowText(n); text != "" {
			return text
		}
	}
	return fmt.Sprintf("%v", v)
}

// scalarField returns the destination for a plain string frontmatter key, or
// nil when the key is not one of them.
func (m *Metadata) scalarField(key string) *string {
	switch key {
	case "usage":
		return &m.Usage
	case "shortcut":
		return &m.Shortcut
	case "category":
		return &m.Category
	case "effort":
		return &m.Effort
	case "activation":
		return &m.Activation
	}
	return nil
}

// stringSliceFromAny coerces a YAML value into a []string. Accepts a sequence
// (most common case for tools/targets/etc) or a scalar (treated as a single-element
// list). Returns nil for unsupported types so callers can detect the absence.
func stringSliceFromAny(v interface{}) []string {
	switch t := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	case []string:
		return append([]string(nil), t...)
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}
