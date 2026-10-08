package lint

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

// fmKey is one top-level frontmatter key with its 1-based line in the file.
type fmKey struct {
	Name  string
	Line  int
	Value any
}

// frontmatter is a file's YAML frontmatter decoded with its types and lines
// intact. The loader flattens every value to a string; the checks that need a
// real date, a list or a nested map read the file themselves.
type frontmatter struct {
	keys []fmKey
	meta map[string]fmKey // entries of the Agent Skills `metadata` map
}

func parseFrontmatterDoc(d doc) frontmatter {
	var fm frontmatter
	if d.bodyStart == 0 {
		return fm
	}
	// Line 0 is the opening ---, the closing one is bodyStart-1.
	block := strings.Join(d.lines[1:d.bodyStart-1], "\n")
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(block), &root); err != nil || len(root.Content) == 0 {
		return fm
	}
	m := root.Content[0]
	if m.Kind != yaml.MappingNode {
		return fm
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		fm.add(k, v)
		if k.Value == okf.ExtensionKey {
			// An OKF concept keeps the ai-rulez frontmatter in x-ai-rulez.metadata;
			// the checks read those entries as the top-level keys they stand for.
			fm.addExtension(v)
		}
	}
	return fm
}

// add records one top-level key.
func (f *frontmatter) add(k, v *yaml.Node) {
	var val any
	if err := v.Decode(&val); err != nil {
		val = nil
	}
	f.keys = append(f.keys, fmKey{Name: k.Value, Line: k.Line + 1, Value: val})
	if k.Value == "metadata" && v.Kind == yaml.MappingNode {
		f.meta = map[string]fmKey{}
		for j := 0; j+1 < len(v.Content); j += 2 {
			var mv any
			if err := v.Content[j+1].Decode(&mv); err != nil {
				mv = nil
			}
			f.meta[v.Content[j].Value] = fmKey{Name: v.Content[j].Value, Line: v.Content[j].Line + 1, Value: mv}
		}
	}
}

// addExtension records the entries of an x-ai-rulez.metadata mapping as keys.
func (f *frontmatter) addExtension(ext *yaml.Node) {
	if ext.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(ext.Content); i += 2 {
		md := ext.Content[i+1]
		if ext.Content[i].Value != "metadata" || md.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(md.Content); j += 2 {
			f.add(md.Content[j], md.Content[j+1])
		}
	}
}

// top returns the top-level key.
func (f frontmatter) top(name string) (fmKey, bool) {
	for _, k := range f.keys {
		if k.Name == name {
			return k, true
		}
	}
	return fmKey{}, false
}

// lookup finds a key at the top level, then inside the `metadata` map, where the
// Agent Skills specification puts custom properties such as an owner.
func (f frontmatter) lookup(name string) (fmKey, bool) {
	if k, ok := f.top(name); ok {
		return k, true
	}
	k, ok := f.meta[name]
	return k, ok
}

// scalar renders a decoded value the way the user wrote it.
func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case time.Time:
		return t.Format("2006-01-02")
	case []any, map[string]any:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

// parseDate accepts the date shapes people write for a verification date.
func parseDate(v any) (time.Time, bool) {
	if t, ok := v.(time.Time); ok {
		return t, true
	}
	s := scalar(v)
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
