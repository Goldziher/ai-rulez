package frontmatter

import (
	"errors"

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
