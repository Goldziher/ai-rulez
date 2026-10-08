package config

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// OKF (Open Knowledge Format) frontmatter keys the loader owns. A content file
// may be written as an OKF concept: `type` and `title` describe it to OKF
// readers and carry nothing for ai-rulez, and `x-ai-rulez` holds the ai-rulez
// metadata (`metadata:` is the former top-level frontmatter; `kind`, `id` and
// `domain` are redundant with the file's location).
const (
	okfKeyType      = "type"
	okfKeyTitle     = "title"
	okfKeyExtension = "x-ai-rulez"
	okfKeyMetadata  = "metadata"
)

// IsOKFReservedKey reports whether a top-level frontmatter key belongs to the
// OKF layer (type, title, x-ai-rulez) and is therefore never ai-rulez metadata.
func IsOKFReservedKey(key string) bool {
	switch key {
	case okfKeyType, okfKeyTitle, okfKeyExtension:
		return true
	}
	return false
}

// normalizeOKFFrontmatter maps an OKF concept's frontmatter onto the native
// shape before anything parses it: the reserved keys are removed and the
// entries of x-ai-rulez.metadata are hoisted to the top level, so typed
// fields, Extra and the typed extra nodes are filled exactly as they are for
// the native layout. Frontmatter without a reserved key is returned unchanged.
func normalizeOKFFrontmatter(frontmatterYAML string) string {
	if !strings.Contains(frontmatterYAML, okfKeyType) &&
		!strings.Contains(frontmatterYAML, okfKeyTitle) &&
		!strings.Contains(frontmatterYAML, okfKeyExtension) {
		return frontmatterYAML
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &doc); err != nil {
		return frontmatterYAML
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return frontmatterYAML
	}
	root := doc.Content[0]
	reserved := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		if k := root.Content[i]; k.Kind == yaml.ScalarNode && IsOKFReservedKey(k.Value) {
			reserved = true
			break
		}
	}
	if !reserved {
		return frontmatterYAML
	}

	var hoisted []*yaml.Node
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: root.Tag, Style: root.Style}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		if key.Kind == yaml.ScalarNode && key.Value == okfKeyExtension {
			hoisted = append(hoisted, extensionMetadata(val)...)
			continue
		}
		if key.Kind == yaml.ScalarNode && IsOKFReservedKey(key.Value) {
			continue
		}
		out.Content = append(out.Content, key, val)
	}
	out.Content = withoutKeys(out.Content, hoisted)
	out.Content = append(out.Content, hoisted...)

	text, err := yaml.Marshal(out)
	if err != nil {
		return frontmatterYAML
	}
	return strings.TrimRight(string(text), "\n")
}

// extensionMetadata returns the key/value node pairs of x-ai-rulez.metadata.
func extensionMetadata(ext *yaml.Node) []*yaml.Node {
	if ext.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(ext.Content); i += 2 {
		if ext.Content[i].Value == okfKeyMetadata && ext.Content[i+1].Kind == yaml.MappingNode {
			return ext.Content[i+1].Content
		}
	}
	return nil
}

// withoutKeys drops from pairs the entries whose key is also in drop, so a
// hoisted x-ai-rulez value wins over a same-named top-level one.
func withoutKeys(pairs, drop []*yaml.Node) []*yaml.Node {
	if len(drop) == 0 {
		return pairs
	}
	names := make(map[string]bool, len(drop)/2)
	for i := 0; i+1 < len(drop); i += 2 {
		names[drop[i].Value] = true
	}
	kept := pairs[:0:0]
	for i := 0; i+1 < len(pairs); i += 2 {
		if !names[pairs[i].Value] {
			kept = append(kept, pairs[i], pairs[i+1])
		}
	}
	return kept
}

// isOKFListing reports whether the file is an OKF index.md or log.md as
// "migrate okf" and "export okf" write them: a generated directory listing, not
// content. A file with one of those names that holds prose or its own frontmatter
// is still a rule, context file or agent, as it was before OKF.
func (s *contentScanner) isOKFListing(filePath string) bool {
	switch strings.ToLower(filepath.Base(filePath)) {
	case "index.md", "log.md":
	default:
		return false
	}
	data, err := readCapped(s.v, filePath)
	return err == nil && IsOKFListing(data)
}

// IsOKFListing reports whether data has the shape of a generated OKF index or
// log (it lists at least one entry): at most an okf_version frontmatter, and a body of headings, list entries
// and blank lines.
func IsOKFListing(data []byte) bool {
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	if strings.HasPrefix(text, "---\n") {
		end := strings.Index(text[4:], "\n---")
		if end < 0 {
			return false
		}
		for _, line := range strings.Split(text[4:4+end], "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "okf_version:") {
				return false
			}
		}
		text = text[4+end+4:]
	}
	listed := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "* ["), strings.HasPrefix(line, "- ["):
			listed = true
		default:
			return false
		}
	}
	return listed
}

// okfIdentity returns the top-level `type` and `title` scalars of an OKF
// concept's frontmatter, empty when absent or not scalar.
func okfIdentity(frontmatterYAML string) (typ, title string) {
	if !strings.Contains(frontmatterYAML, okfKeyType) && !strings.Contains(frontmatterYAML, okfKeyTitle) {
		return "", ""
	}
	var fm map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &fm); err != nil {
		return "", ""
	}
	scalar := func(key string) string {
		if n, ok := fm[key]; ok && n.Kind == yaml.ScalarNode {
			return strings.TrimSpace(n.Value)
		}
		return ""
	}
	return scalar(okfKeyType), scalar(okfKeyTitle)
}
