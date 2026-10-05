package providers

import (
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

// marshalFrontmatter renders the frontmatter map as YAML. Without NameFirst or
// QuotedFields it is plain yaml.Marshal (keys alphabetical); with them the keys
// are laid out as a yaml.Node so the document matches a tool's exact style.
func marshalFrontmatter(frontmatter map[string]any, spec *FrontmatterSpec) ([]byte, error) {
	if spec == nil || (!spec.NameFirst && len(spec.QuotedFields) == 0) {
		return yaml.Marshal(frontmatter)
	}
	keys := make([]string, 0, len(frontmatter))
	for k := range frontmatter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if spec.NameFirst {
		if i := slices.Index(keys, "name"); i > 0 {
			keys = append([]string{"name"}, slices.Delete(keys, i, i+1)...)
		}
	}

	doc := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range keys {
		key, val := &yaml.Node{}, &yaml.Node{}
		if err := key.Encode(k); err != nil {
			return nil, err
		}
		if err := val.Encode(frontmatter[k]); err != nil {
			return nil, err
		}
		if val.Kind == yaml.ScalarNode && val.Tag == "!!str" && slices.Contains(spec.QuotedFields, k) {
			val.Style = yaml.DoubleQuotedStyle
		}
		doc.Content = append(doc.Content, key, val)
	}
	return yaml.Marshal(doc)
}
