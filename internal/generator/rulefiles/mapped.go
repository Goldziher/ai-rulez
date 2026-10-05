package rulefiles

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// DialectMapped builds frontmatter from an ActivationMap declared in a provider
// spec instead of from Go, so a rules folder needs no dialect of its own.
const DialectMapped Dialect = "mapped"

// Formats of a mapped dialect's frontmatter block.
const (
	MappedFormatYAML  = "yaml"
	MappedFormatLines = "lines"
)

// Placeholders a mapped frontmatter value may use.
const (
	placeholderGlobs       = "{globs}"
	placeholderGlobsList   = "{globs_list}"
	placeholderDescription = "{description}"
	placeholderName        = "{name}"
)

// ActivationMap is the frontmatter each activation mode of a mapped dialect
// writes. A nil table means the mode has no frontmatter of its own.
type ActivationMap struct {
	Always, Glob, Auto, Manual map[string]any
	Format                     string
}

// IsMappedLines reports whether t writes its frontmatter as bare "key: value"
// lines rather than YAML.
func (t Target) IsMappedLines() bool {
	return t.Dialect == DialectMapped && t.Mapping != nil && t.Mapping.Format == MappedFormatLines
}

// MappedLines renders a mapped frontmatter map as "key: value" lines in key
// order; list values are joined with ", ".
func MappedLines(fm map[string]any) string {
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(singleLine(k) + ": ")
		switch v := fm[k].(type) {
		case []string:
			parts := make([]string, len(v))
			for i, e := range v {
				parts[i] = singleLine(e)
			}
			b.WriteString(strings.Join(parts, ", "))
		default:
			b.WriteString(singleLine(toString(v)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// lineBreaks are the characters that would end a "key: value" line (or, for a
// tab, shift its columns); a value holding one could inject frontmatter keys.
var lineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ")

// singleLine collapses every line break and tab of s to a space.
func singleLine(s string) string { return lineBreaks.Replace(s) }

// activationKey is the shape of a frontmatter key a spec may declare.
var activationKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// ValidActivationKey reports whether key is a safe frontmatter key.
func ValidActivationKey(key string) bool { return activationKey.MatchString(key) }

// ActivationValueHasEmbeddedList reports whether a value uses {globs_list} inside
// a longer string; the placeholder only works as the whole value.
func ActivationValueHasEmbeddedList(s string) bool {
	return s != placeholderGlobsList && strings.Contains(s, placeholderGlobsList)
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	return strings.ReplaceAll(fmt.Sprint(v), "\n", " ")
}

// table picks the map of a mode; ok is false when the mode has none.
func (m *ActivationMap) table(mode config.ActivationMode) (map[string]any, bool) {
	var t map[string]any
	switch mode {
	case config.ActivationGlob:
		t = m.Glob
	case config.ActivationAuto:
		t = m.Auto
	case config.ActivationManual:
		t = m.Manual
	default:
		return m.Always, true
	}
	return t, t != nil
}

// mappedFrontmatter resolves the frontmatter of one item from the activation
// map. A mode without a table is loaded always, which is reported as a downgrade.
func mappedFrontmatter(m *ActivationMap, it Item, mode config.ActivationMode) (map[string]any, []Note) {
	if m == nil {
		return nil, nil
	}
	table, ok := m.table(mode)
	var notes []Note
	if !ok {
		table = m.Always
		notes = fallbackNote(DialectMapped, it, mode)
	}
	out := make(map[string]any, len(table))
	for key, value := range table {
		if resolved, keep := resolveTemplate(value, it); keep {
			out[key] = resolved
		}
	}
	if len(out) == 0 {
		return nil, notes
	}
	return out, notes
}

// resolveTemplate expands the placeholders of a string value. A value that is
// exactly {globs_list} becomes a list, exactly {globs} a joined string; anything
// that resolves to nothing is dropped. Non-string values pass through.
func resolveTemplate(value any, it Item) (any, bool) {
	s, ok := value.(string)
	if !ok {
		return value, true
	}
	globs := it.Activation.Globs
	switch s {
	case placeholderGlobsList:
		list := make([]string, len(globs))
		for i, g := range globs {
			list[i] = singleLine(g)
		}
		return list, len(list) > 0
	case placeholderGlobs:
		return singleLine(strings.Join(globs, ",")), len(globs) > 0
	}
	if !strings.Contains(s, "{") {
		return s, true
	}
	out := singleLine(strings.NewReplacer(
		placeholderGlobs, strings.Join(globs, ","),
		placeholderDescription, it.Activation.Description,
		placeholderName, it.File.Name,
	).Replace(s))
	return out, strings.TrimSpace(out) != ""
}
