package config

// Dynamic skill loading: how each skill reaches the agent (static files, served
// over MCP on demand, or both), where remote skill sources come from, and whether
// the lock is enforced at serve time. See docs/mcp-server.md, "Dynamic skill
// loading". Everything here is optional; without it generation is unchanged.

// Delivery says how a skill reaches the agent.
type Delivery string

// Delivery values.
const (
	// DeliveryStatic writes the skill into every harness's skill tree (default).
	DeliveryStatic Delivery = "static"
	// DeliveryServed leaves the skill out of the static trees and serves it over
	// MCP (`ai-rulez mcp --serve-skills`) on demand.
	DeliveryServed Delivery = "served"
	// DeliveryBoth writes the skill statically and also serves it.
	DeliveryBoth Delivery = "both"
)

// ParseDelivery parses a delivery value; ok is false for anything unknown. An
// empty string is not a value (it means "not set") and reports ok=false.
func ParseDelivery(v string) (Delivery, bool) {
	switch d := Delivery(normalizeKey(v)); d {
	case DeliveryStatic, DeliveryServed, DeliveryBoth:
		return d, true
	}
	return "", false
}

// DomainConfigs maps a domain name to its [domains.<name>] table.
type DomainConfigs map[string]DomainConfig

// SkillsConfig is the [skills] table: defaults that apply to every skill.
type SkillsConfig struct {
	// Delivery is the global default delivery: static (default), served or both.
	Delivery string `yaml:"delivery,omitempty" json:"delivery,omitempty" toml:"delivery,omitempty"`
}

// DomainConfig is one [domains.<name>] table.
type DomainConfig struct {
	// Delivery is the default delivery of the domain's skills.
	Delivery string `yaml:"delivery,omitempty" json:"delivery,omitempty" toml:"delivery,omitempty"`
}

// Skill source trust (scan) levels.
const (
	// TrustError blocks a skill when the security scan finds anything at all
	// (every finding counts as an error). The default for remote sources.
	TrustError = "error"
	// TrustWarn blocks only findings whose own severity is error (secrets,
	// hidden characters, risky shell); the rest are logged.
	TrustWarn = "warn"
)

// SkillSourceConfig is one [[skill_sources]] entry: a git repository or local
// directory whose skills are served, never written to the static trees.
type SkillSourceConfig struct {
	// Name identifies the source in the lock and in provenance.
	Name string `yaml:"name" json:"name" toml:"name"`
	// URL is a git URL (https, ssh, file) or a local directory path.
	URL string `yaml:"url" json:"url" toml:"url"`
	// Ref is a tag or a full commit SHA. A branch (or none) follows a moving
	// ref and is reported as unpinned (AR010).
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty" toml:"ref,omitempty"`
	// Path is the subdirectory of the repository that holds skill directories.
	Path string `yaml:"path,omitempty" json:"path,omitempty" toml:"path,omitempty"`
	// Include keeps only skills whose directory name matches one of these globs.
	Include []string `yaml:"include,omitempty" json:"include,omitempty" toml:"include,omitempty"`
	// Exclude drops skills whose directory name matches one of these globs; it wins over Include.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" toml:"exclude,omitempty"`
	// NamePrefix is prepended to every served skill name, so two sources cannot collide.
	NamePrefix string `yaml:"name_prefix,omitempty" json:"name_prefix,omitempty" toml:"name_prefix,omitempty"` //nolint:tagliatelle
	// Trust is the scan level: "error" (default) or "warn". See TrustError.
	Trust string `yaml:"trust,omitempty" json:"trust,omitempty" toml:"trust,omitempty"`
	// MaxSkills caps how many skills the source may load (0 selects the default of 200).
	MaxSkills int `yaml:"max_skills,omitempty" json:"max_skills,omitempty" toml:"max_skills,omitempty"` //nolint:tagliatelle
	// MaxBytes caps the total size of the files the source loads (0 selects the default of 64 MiB).
	MaxBytes int `yaml:"max_bytes,omitempty" json:"max_bytes,omitempty" toml:"max_bytes,omitempty"` //nolint:tagliatelle
}
