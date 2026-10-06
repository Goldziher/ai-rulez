// Package registry builds the preset registry of the engine: every Go preset and
// every embedded provider spec, plus the factories custom presets are built with.
//
// config cannot import the packages that implement presets, and presets import
// config, so something above both has to assemble the registry. Doing it in one
// function, called when the registry is first needed, replaces the init()
// registration this used to be: a program gets the registry it asks for, and a
// service that wants different presets builds its own with config.NewRegistry.
package registry

import (
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
)

var (
	once sync.Once
	def  *config.Registry
)

// New builds a fresh registry with every built-in preset.
func New() *config.Registry {
	r := config.NewRegistry()
	presets.Register(r)
	providers.Register(r)
	return r
}

// Default returns the registry of the built-in presets. It is built on first use and
// never changes, so one value serves every project in the process.
func Default() *config.Registry {
	once.Do(func() { def = New() })
	return def
}
