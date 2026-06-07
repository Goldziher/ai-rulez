package providers

import (
	"github.com/Goldziher/ai-rulez/internal/config"
)

// init registers every embedded provider spec under its declared name with
// the shared PresetRegistry. Replaces the per-preset init() functions that
// used to live in internal/generator/presets/*.go.
//
// LoadBuiltin failures here are fatal at init time — an embedded spec that
// doesn't parse is a build-time bug, not a runtime configuration problem.
func init() {
	names, err := BuiltinNames()
	if err != nil {
		panic("providers: enumerate builtin specs: " + err.Error())
	}
	for _, name := range names {
		gen, err := LoadBuiltin(name)
		if err != nil {
			panic("providers: load builtin " + name + ": " + err.Error())
		}
		config.RegisterPreset(name, gen)
	}
}
