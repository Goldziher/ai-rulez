package config

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// swallowedTopLevelKeysTOML refuses a TOML document in which a top-level key was
// written after a [table] header. TOML then reads it as a key of that table and
// the decoder silently drops it, so builtins stop applying, agents_md falls back
// to its default and gitignore never takes effect. A genuinely unknown key stays
// a schema finding; only a key whose name is a known top-level key is an error.
func swallowedTopLevelKeysTOML(path string, data []byte) error {
	var doc map[string]any
	if toml.Unmarshal(data, &doc) != nil {
		return nil //nolint:nilerr // not TOML: the caller's decode error stands
	}
	return swallowedTopLevelKeys(path, doc)
}

// swallowedTopLevelKeys reports the first top-level key a known table swallowed.
func swallowedTopLevelKeys(path string, doc map[string]any) error {
	for _, table := range slices.Sorted(maps.Keys(doc)) {
		switch body := doc[table].(type) {
		case map[string]any:
			if err := swallowedKeyIn(path, table, "["+table+"]", body); err != nil {
				return err
			}
		case []any:
			// A table array ([[mcp_servers]]): TOML reads a top-level key after
			// the header as a key of the last element. The schema path already
			// treats `mcp_servers.0` as a swallowing parent (#344).
			for i, elem := range body {
				entry, ok := elem.(map[string]any)
				if !ok {
					continue
				}
				parent := table + "." + strconv.Itoa(i)
				if err := swallowedKeyIn(path, parent, "[["+table+"]]", entry); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// swallowedKeyIn reports the first top-level key the table at the dotted path
// parent swallowed, naming it as display in the message.
func swallowedKeyIn(path, parent, display string, body map[string]any) error {
	if len(knownKeys(parent)) == 0 {
		return nil // a free-form map, or a table the config does not know
	}
	for _, key := range slices.Sorted(maps.Keys(body)) {
		if !topLevelKeyOutsideItsTable(parent, key) || !looksLikeTopLevelValue(key, body[key]) {
			continue
		}
		return oops.With("path", path).
			Hint(fmt.Sprintf("Move `%s` above the first [table], so TOML reads it as a top-level key "+
				"instead of a key of %s", key, display)).
			Errorf("`%s` is a top-level key but was written after %s, so it was read as a key of that "+
				"table and ignored; move `%s` above the first [table]", key, display, key)
	}
	return nil
}

// looksLikeTopLevelValue reports whether the value of key reads as the top-level
// setting of that name. Most keys do; "source" is also a field of other tables
// (an include, a skill source), so a value that is no source name stays an
// unknown key of its table rather than a swallowed top-level setting.
func looksLikeTopLevelValue(key string, value any) bool {
	if key != sourceKey {
		return true
	}
	name, ok := value.(string)
	return ok && (name == SourceAgents || name == SourceAIRulez)
}
