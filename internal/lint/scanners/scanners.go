// Package scanners holds the embedded profiles and presets of external
// scanners: data only, versioned with the binary, never downloaded.
package scanners

import (
	_ "embed"
	"fmt"
	"sort"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

//go:embed profiles.toml
var profilesTOML []byte

// Layouts a profile may ask the stage to use.
const (
	// LayoutProject keeps the repository-relative paths (the default).
	LayoutProject = ""
	// LayoutPlugin puts skills, agents and commands at skills/<name>/, agents/
	// and commands/ under the stage root, the layout plugin validators read.
	LayoutPlugin = "plugin"
)

// Condition names for Profile.When.
const (
	// WhenPlugin makes a preset member apply only when [plugin] or [marketplace] is configured.
	WhenPlugin = "plugin"
)

// Profile is one scanner's command template and facts.
type Profile struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	Egress      bool              `toml:"egress"`
	Command     []string          `toml:"command"`
	Format      string            `toml:"format"`
	Layout      string            `toml:"layout"`
	Inputs      []string          `toml:"inputs"`
	EgressFlags []string          `toml:"egress_flags"`
	SeverityMap map[string]string `toml:"severity_map"`
	MaxSeverity string            `toml:"max_severity"`
	// When limits a preset member to projects with that configuration (WhenPlugin).
	When        string   `toml:"when"`
	RequiresEnv []string `toml:"requires_env"`
	// DataSent lists, for an egress profile, what the vendor documents receiving.
	DataSent       []string `toml:"data_sent"`
	Source         string   `toml:"source"`
	VerifiedOn     string   `toml:"verified_on"`
	TestedVersions []string `toml:"tested_versions"`
}

// Preset is a named list of profiles.
type Preset struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Profiles    []string `toml:"profiles"`
	// FailOn is the scanner failure threshold the preset implies ("" for none).
	FailOn string `toml:"fail_on"`
}

type file struct {
	Profile []Profile `toml:"profile"`
	Preset  []Preset  `toml:"preset"`
}

var (
	loadOnce sync.Once
	loaded   file
	loadErr  error
)

func load() file {
	loadOnce.Do(func() { loadErr = toml.Unmarshal(profilesTOML, &loaded) })
	if loadErr != nil {
		panic(fmt.Sprintf("scanners: embedded profiles.toml is invalid: %v", loadErr)) // a build defect, caught by the package tests
	}
	return loaded
}

// Lookup returns the profile called name.
func Lookup(name string) (Profile, bool) {
	profiles := load().Profile
	for i := range profiles {
		if profiles[i].Name == name {
			return profiles[i], true
		}
	}
	return Profile{}, false
}

// Names lists the profile names, sorted.
func Names() []string {
	var out []string
	profiles := load().Profile
	for i := range profiles {
		out = append(out, profiles[i].Name)
	}
	sort.Strings(out)
	return out
}

// Profiles returns every profile in file order.
func Profiles() []Profile { return append([]Profile(nil), load().Profile...) }

// LookupPreset returns the preset called name ("off" and "" are not presets).
func LookupPreset(name string) (Preset, bool) {
	for _, p := range load().Preset {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// PresetNames lists the preset names, sorted.
func PresetNames() []string {
	var out []string
	for _, p := range load().Preset {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

// PresetsOf lists the presets that contain the profile.
func PresetsOf(profile string) []string {
	var out []string
	for _, p := range load().Preset {
		for _, m := range p.Profiles {
			if m == profile {
				out = append(out, p.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}
