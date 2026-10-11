package config

import (
	"fmt"
	"maps"
	"slices"

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
		return nil // not TOML: the caller's decode error stands
	}
	return swallowedTopLevelKeys(path, doc)
}

// swallowedTopLevelKeys reports the first top-level key a known table swallowed.
func swallowedTopLevelKeys(path string, doc map[string]any) error {
	for _, table := range slices.Sorted(maps.Keys(doc)) {
		body, ok := doc[table].(map[string]any)
		if !ok || len(knownKeys(table)) == 0 {
			continue // a list, a free-form map, or a table the config does not know
		}
		for _, key := range slices.Sorted(maps.Keys(body)) {
			if !topLevelKeyOutsideItsTable(table, key) {
				continue
			}
			return oops.With("path", path).
				Hint(fmt.Sprintf("Move `%s` above the first [table], so TOML reads it as a top-level key "+
					"instead of a key of [%s]", key, table)).
				Errorf("`%s` is a top-level key but was written after [%s], so it was read as a key of that "+
					"table and ignored; move `%s` above the first [table]", key, table, key)
		}
	}
	return nil
}
