package config

import (
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// Registry maps preset names to their generators and holds the factories custom
// presets are built with. config cannot import the packages that implement the
// presets (they import config), so the engine builds a Registry above both
// (internal/generator/registry) and hands it to the load (WithRegistry) or to the
// Generator. A Registry is filled while it is constructed and only read afterwards,
// which makes one value safe to share between runs on different projects; nothing is
// registered from an init().
type Registry struct {
	presets map[string]PresetGenerator
	// Custom builds the generator of a custom preset declared in the config.
	Custom func(Preset) PresetGenerator
	// Provider builds the generator of a provider-backed custom preset from its spec
	// file, resolved against baseDir (the project root) and read through view, so a
	// project held in memory or in a snapshot, and the symlink policy of the
	// workspace, apply to the spec as to any other source.
	Provider func(preset Preset, baseDir string, view workspace.View) (PresetGenerator, error)
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{presets: make(map[string]PresetGenerator)}
}

// Register adds a built-in preset generator. It is for the code that builds the
// Registry, before anything reads it.
func (r *Registry) Register(name string, generator PresetGenerator) {
	r.presets[name] = generator
}

// Generator returns the built-in preset generator with the given name, or
// ErrInvalidPreset.
func (r *Registry) Generator(name string) (PresetGenerator, error) {
	if r == nil {
		return nil, oops.Wrapf(ErrInvalidPreset, "no preset registry")
	}
	generator, ok := r.presets[name]
	if !ok {
		return nil, ErrInvalidPreset
	}
	return generator, nil
}

// Names returns the registered preset names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.presets))
	for name := range r.presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Each calls fn for every registered generator, in name order.
func (r *Registry) Each(fn func(name string, generator PresetGenerator)) {
	for _, name := range r.Names() {
		fn(name, r.presets[name])
	}
}

// WithRegistry loads the configuration with the preset registry that validation
// and generation resolve presets through.
func WithRegistry(r *Registry) LoadOption {
	return func(o *loadOptions) { o.registry = r }
}

// RulesDirOwner is implemented by preset generators that write one native rule
// file per rule into a folder beyond the built-in rules folders (a custom
// provider spec with a split rules output). The folder gets the protections the
// built-in ones have.
type RulesDirOwner interface {
	// SplitRulesDir returns the folder, or "" when the generator has none.
	SplitRulesDir() string
}
