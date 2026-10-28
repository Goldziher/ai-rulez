package frontmatter

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrUnclosed is the error of a block that opens with --- and never closes.
var ErrUnclosed = errors.New("the frontmatter block never closes with ---")

// Decode decodes the YAML of a block as a mapping: the decode lint judges for
// AR306 (frontmatter-malformed). A document without a block decodes to nil
// without an error; an empty block to an empty map. An unclosed block is
// ErrUnclosed, and YAML that is not a mapping (a list, a scalar, a syntax error)
// is the YAML error, so a caller keeps why the metadata was lost instead of
// reading the item as if it had none.
//
// With a YAML error the map is what yaml.v3 decoded all the same, returned
// together with the error: every other key for a *yaml.TypeError (a complex
// key, a value of the wrong type), the keys before the failure for a tag that
// does not resolve (`!!int notanint`), nil for a syntax error or YAML that is
// not a mapping. A caller that reads a broken block as an empty one ignores it.
func Decode(b Block) (map[string]any, error) {
	if !b.Present {
		return nil, nil
	}
	if !b.Closed {
		return nil, ErrUnclosed
	}
	var front map[string]any
	if err := yaml.Unmarshal([]byte(b.Raw), &front); err != nil {
		return front, err //nolint:wrapcheck // the YAML error is the message a caller reports
	}
	if front == nil {
		front = map[string]any{}
	}
	return front, nil
}

// GateKeys are the frontmatter keys by which Claude Code gates a skill or a
// rule: `paths` defers it until a matching file is read,
// `disable-model-invocation` keeps it from the model and `user-invocable` from
// the user. A block that does not decode but names one of them hides whether,
// and how, the item is gated. Each call returns a new slice.
func GateKeys() []string {
	return []string{"paths", "disable-model-invocation", "user-invocable"}
}

// StringList reads a gate value that must be one string or a list of strings,
// as Claude Code reads `paths`: ok is false for any other value, null and a list
// holding something other than a string included.
func StringList(v any) ([]string, bool) {
	switch t := v.(type) {
	case string:
		return []string{t}, true
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

// NamesKey reports whether a line of a raw block holds one of keys as a key:
// after any indentation, the key and then a colon, blanks allowed between. It
// reads the text, not the YAML, so it finds a key in a block that does not
// decode, where the key's value is lost.
func NamesKey(raw string, keys ...string) bool {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimLeft(line, " \t")
		for _, key := range keys {
			if rest, ok := strings.CutPrefix(line, key); ok && strings.HasPrefix(strings.TrimLeft(rest, " \t"), ":") {
				return true
			}
		}
	}
	return false
}
