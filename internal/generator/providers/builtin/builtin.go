// Package builtin embeds the declarative provider specs of the built-in presets
// and derives, from them, the few facts other packages need without loading a spec
// into a generator: the preset names, the root file each writes and the rules
// folder of a split rules output.
//
// It is a leaf package (it imports no ai-rulez package), so config and rulefiles can
// ask it for those facts. They used to be pushed into those packages by an init()
// in internal/generator/providers, which made the answer depend on what the binary
// linked. Derived on demand from embedded data, the answer is the same
// everywhere and nothing is registered.
package builtin

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

// FS holds the embedded spec files, one <name>.toml per preset, at the root.
//
//go:embed *.toml
var FS embed.FS

// Summary is what the rest of the engine needs to know about one built-in spec.
type Summary struct {
	// Name is the preset name; it equals the file name without ".toml".
	Name string
	// RootFile is the file the preset writes at the project root ("" for none).
	RootFile string
	// RulesDir is the folder of a split rules output ("" when it has none).
	RulesDir string
}

type spec struct {
	Name string `toml:"name"`
	Root *struct {
		File string `toml:"file"`
	} `toml:"root"`
	Outputs map[string]struct {
		Dir   string `toml:"dir"`
		Split bool   `toml:"split"`
	} `toml:"outputs"`
}

var (
	once      sync.Once
	summaries []Summary
)

// Summaries returns every embedded spec, sorted by name. An embedded spec that does
// not parse is a build-time bug, so it panics.
func Summaries() []Summary {
	once.Do(func() {
		entries, err := FS.ReadDir(".")
		if err != nil {
			panic("builtin: read embedded specs: " + err.Error())
		}
		for _, entry := range entries {
			name, ok := strings.CutSuffix(entry.Name(), ".toml")
			if entry.IsDir() || !ok {
				continue
			}
			raw, err := FS.ReadFile(entry.Name())
			if err != nil {
				panic("builtin: read " + entry.Name() + ": " + err.Error())
			}
			var s spec
			if err := toml.Unmarshal(raw, &s); err != nil {
				panic(fmt.Sprintf("builtin: parse %s: %v", entry.Name(), err))
			}
			sum := Summary{Name: name}
			if s.Root != nil {
				sum.RootFile = s.Root.File
			}
			if rules, ok := s.Outputs["rules"]; ok && rules.Split {
				sum.RulesDir = path.Clean(rules.Dir)
			}
			summaries = append(summaries, sum)
		}
		sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	})
	return summaries
}

// Names returns the names of the embedded specs, sorted.
func Names() []string {
	sums := Summaries()
	names := make([]string, len(sums))
	for i, s := range sums {
		names[i] = s.Name
	}
	return names
}
