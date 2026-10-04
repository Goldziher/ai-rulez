package config

// UsageConfig configures usage-telemetry support. Everything here is off by
// default and never makes a network call.
type UsageConfig struct {
	// SkillsIndex makes `generate` write <config dir>/skills-index.json: one record per
	// generated skill with its stable id, source path, content hash, owner,
	// version and the outputs written for each preset. The usage hook and
	// `ai-rulez report usage` join a usage log against it.
	SkillsIndex bool `yaml:"skills_index,omitempty" json:"skills_index,omitempty" toml:"skills_index,omitempty"` //nolint:tagliatelle
}
