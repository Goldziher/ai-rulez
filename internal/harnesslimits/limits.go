// Package harnesslimits holds the table of documented harness size limits
// (limits.toml), the single source of the numbers the generator, the lint
// traps and the config defaults use.
package harnesslimits

import (
	_ "embed" // the limits table
	"fmt"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

//go:embed limits.toml
var limitsTOML []byte

// Limit is one row of the table.
type Limit struct {
	ID           string `toml:"id"`
	Harness      string `toml:"harness"`
	Subject      string `toml:"subject"`
	Unit         string `toml:"unit"`     // chars or bytes
	Value        int    `toml:"value"`    // the documented limit
	Behavior     string `toml:"behavior"` // truncated | dropped | ignored | error
	Source       string `toml:"source"`
	Quote        string `toml:"quote"`
	VerifiedOn   string `toml:"verified_on"`
	Configurable string `toml:"configurable"` // the ai-rulez setting that overrides it, if any
}

type table struct {
	Schema int     `toml:"schema"`
	Limit  []Limit `toml:"limit"`
}

var (
	once   sync.Once
	loaded []Limit
	errLd  error
)

// All returns every row of the embedded table.
func All() ([]Limit, error) {
	once.Do(func() {
		var t table
		if err := toml.Unmarshal(limitsTOML, &t); err != nil {
			errLd = fmt.Errorf("parse limits.toml: %w", err)
			return
		}
		loaded = t.Limit
	})
	return loaded, errLd
}

// Get returns the row with the id.
func Get(id string) (Limit, bool) {
	rows, _ := All() //nolint:errcheck // an unreadable table has no rows; MustValue reports it
	for i := range rows {
		if rows[i].ID == id {
			return rows[i], true
		}
	}
	return Limit{}, false
}

// MustValue returns the value of a row. The table is embedded and tested, so a
// missing id is a programming error.
func MustValue(id string) int {
	l, ok := Get(id)
	if !ok {
		panic("harnesslimits: unknown limit " + id)
	}
	return l.Value
}
