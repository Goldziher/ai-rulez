package providers

import (
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"

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
		if i := slices.Index(keys, keyName); i > 0 {
			keys = append([]string{keyName}, slices.Delete(keys, i, i+1)...)
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

// bareModelAliases are the tool-neutral Claude model aliases and the inherit
// marker, none of which is a model id in another tool's namespace.
var bareModelAliases = map[string]bool{"sonnet": true, "opus": true, "haiku": true, "inherit": true}

// mapTools applies tool_names and tool_case to a Claude tool list.
func (s *FrontmatterSpec) mapTools(tools []string) []string {
	if len(s.ToolNames) == 0 && s.ToolCase == "" {
		return tools
	}
	table := make(map[string]string, len(s.ToolNames))
	for k, v := range s.ToolNames {
		table[strings.ToLower(k)] = v
	}
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool)
		if len(table) > 0 {
			mapped, ok := table[strings.ToLower(name)]
			if !ok {
				continue
			}
			name = mapped
		}
		if s.ToolCase == ToolCaseLower {
			name = strings.ToLower(name)
		}
		if name != "" && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// mapModel applies model_aliases and drop_bare_aliases to a resolved model.
func (s *FrontmatterSpec) mapModel(model string) string {
	if model == "" {
		return ""
	}
	for k, v := range s.ModelAliases {
		if strings.EqualFold(k, model) {
			return v
		}
	}
	if s.DropBareAliases && bareModelAliases[strings.ToLower(model)] {
		return ""
	}
	return model
}

// rewrite applies Replace to the item's content. When it changed anything and
// ReplaceFlag is set, it returns a copy of the frontmatter spec that also writes
// the flag true; otherwise the frontmatter spec comes back as given.
func (b *BodySpec) rewrite(item config.ContentFile, fm *FrontmatterSpec) (config.ContentFile, *FrontmatterSpec) {
	if b == nil || len(b.Replace) == 0 {
		return item, fm
	}
	content, changed := replaceOnce(item.Content, b.Replace)
	item.Content = content
	if !changed || b.ReplaceFlag == "" {
		return item, fm
	}
	flagged := FrontmatterSpec{}
	if fm != nil {
		flagged = *fm
	}
	flagged.Constants = map[string]any{b.ReplaceFlag: true}
	for k, v := range fm.constants() {
		if k != b.ReplaceFlag {
			flagged.Constants[k] = v
		}
	}
	return item, &flagged
}

// constants is the spec's constant table, nil-safe.
func (s *FrontmatterSpec) constants() map[string]any {
	if s == nil {
		return nil
	}
	return s.Constants
}

// replaceOnce rewrites content in one left-to-right pass: at each position the
// longest matching key is replaced and scanning resumes after it, so a key that is
// a prefix of another cannot shadow it and a replacement is never rewritten again.
func replaceOnce(content string, replace map[string]string) (string, bool) {
	keys := make([]string, 0, len(replace))
	for k := range replace {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	var out strings.Builder
	changed := false
	for i := 0; i < len(content); {
		matched := false
		for _, k := range keys {
			if strings.HasPrefix(content[i:], k) {
				out.WriteString(replace[k])
				i += len(k)
				matched, changed = true, true
				break
			}
		}
		if !matched {
			out.WriteByte(content[i])
			i++
		}
	}
	return out.String(), changed
}
