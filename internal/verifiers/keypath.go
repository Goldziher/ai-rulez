package verifiers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// lookupKey follows parsed key segments through maps and (numeric segments)
// lists. found is false when a segment is missing; a present null is returned
// as (nil, true) so callers can tell it from a missing key.
func lookupKey(doc any, segs []string) (val any, found bool) {
	cur := doc
	for _, seg := range segs {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case map[any]any:
			// YAML with a non-string key; compare by its text.
			var next any
			ok := false
			for k, v := range node {
				if fmt.Sprint(k) == seg {
					next, ok = v, true
					break
				}
			}
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			cur = node[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// scalarString renders a decoded scalar as text; ok is false for maps and
// lists. Numbers keep their source spelling where the decoder allows it, and
// date-times are RFC 3339.
func scalarString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case json.Number:
		return x.String(), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case time.Time:
		return x.Format(time.RFC3339Nano), true
	case map[string]any, map[any]any, []any:
		return "", false
	default:
		// ints, and the go-toml local date/time types, which print as RFC 3339.
		return fmt.Sprint(x), true
	}
}
