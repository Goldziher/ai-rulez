package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEffort(t *testing.T) {
	t.Run("accepts empty string", func(t *testing.T) {
		assert.NoError(t, validateEffort("", "defaults.effort"))
	})

	t.Run("accepts all spec values", func(t *testing.T) {
		for _, v := range []string{"low", "medium", "high", "xhigh", "max", "inherit"} {
			assert.NoError(t, validateEffort(v, "defaults.effort"), "value %q should pass", v)
		}
	})

	t.Run("rejects unknown value", func(t *testing.T) {
		err := validateEffort("extreme", "defaults.effort")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extreme")
		assert.Contains(t, err.Error(), "defaults.effort")
	})

	t.Run("is case-sensitive", func(t *testing.T) {
		err := validateEffort("HIGH", "defaults.effort")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HIGH")
	})
}

func TestConfigValidateDefaults(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
		}
	}

	t.Run("nil defaults is valid", func(t *testing.T) {
		cfg := base()
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid defaults.effort passes", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{Effort: "high"}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid defaults.effort fails", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{Effort: "extreme"}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "defaults.effort")
	})

	t.Run("empty defaults.effort is treated as not set", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{Effort: ""}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid defaults.effort_by_preset passes", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{
			EffortByPreset: map[string]string{"claude": "xhigh", "codex": "high"},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid effort value in effort_by_preset fails with preset path", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{
			EffortByPreset: map[string]string{"codex": "extreme"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "defaults.effort_by_preset.codex")
	})

	t.Run("unknown preset key in effort_by_preset fails", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{
			EffortByPreset: map[string]string{"not-a-preset": "high"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not-a-preset")
	})

	// Normalization contract: users always write the canonical vocabulary, never
	// preset-native values. Provider-specific values like Codex's "minimal" must
	// be rejected — they only exist in generated output, never in user config.
	t.Run("provider-native values are rejected at every input site", func(t *testing.T) {
		providerNative := []string{"minimal", "MAX", "extreme", "xxhigh"}
		for _, v := range providerNative {
			t.Run("defaults.effort/"+v, func(t *testing.T) {
				cfg := base()
				cfg.Defaults = &DefaultsConfig{Effort: v}
				assert.Error(t, cfg.Validate(), "value %q must be rejected from defaults.effort", v)
			})
			t.Run("defaults.effort_by_preset/"+v, func(t *testing.T) {
				cfg := base()
				cfg.Defaults = &DefaultsConfig{EffortByPreset: map[string]string{"codex": v}}
				assert.Error(t, cfg.Validate(), "value %q must be rejected from defaults.effort_by_preset", v)
			})
			t.Run("agent.metadata.effort/"+v, func(t *testing.T) {
				cfg := base()
				cfg.Content = &ContentTree{Agents: []ContentFile{
					{Name: "x", Metadata: &Metadata{Effort: v}},
				}}
				assert.Error(t, cfg.Validate(), "value %q must be rejected from agent metadata", v)
			})
		}
	})
}

func TestConfigValidateAgentEffort(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			Content: &ContentTree{},
		}
	}

	t.Run("valid root agent effort passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Agents = []ContentFile{
			{Name: "reviewer", Metadata: &Metadata{Effort: "high"}},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid root agent effort fails with agent name in error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Agents = []ContentFile{
			{Name: "reviewer", Metadata: &Metadata{Effort: "extreme"}},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reviewer")
		assert.Contains(t, err.Error(), "extreme")
	})

	t.Run("invalid domain agent effort fails", func(t *testing.T) {
		cfg := base()
		cfg.Content.Domains = map[string]*Domain{
			"backend": {
				Name: "backend",
				Agents: []ContentFile{
					{Name: "db-tuner", Metadata: &Metadata{Effort: "BOGUS"}},
				},
			},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.True(t,
			strings.Contains(err.Error(), "backend") || strings.Contains(err.Error(), "db-tuner"),
			"error should reference scope or agent name; got: %s", err.Error())
	})

	t.Run("agent without metadata is fine", func(t *testing.T) {
		cfg := base()
		cfg.Content.Agents = []ContentFile{{Name: "noop"}}
		assert.NoError(t, cfg.Validate())
	})
}

func TestConfigValidateMalformedFrontmatter(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			Content: &ContentTree{},
		}
	}

	t.Run("valid config passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "ok", Path: "skills/ok/SKILL.md", Metadata: &Metadata{Extra: map[string]string{"description": "fine"}}},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("content without frontmatter passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{{Name: "plain", Path: "skills/plain/SKILL.md"}}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("root skill with malformed frontmatter fails and names the file", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "zz-probe", Path: "skills/zz-probe/SKILL.md", MalformedFrontmatter: true},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skills/zz-probe/SKILL.md")
	})

	t.Run("domain skill with malformed frontmatter fails", func(t *testing.T) {
		cfg := base()
		cfg.Content.Domains = map[string]*Domain{
			"security": {
				Name: "security",
				Skills: []ContentFile{
					{Name: "zz-probe", Path: "skills/zz-probe/SKILL.md", MalformedFrontmatter: true},
				},
			},
		}
		require.Error(t, cfg.Validate())
	})

	t.Run("multiple malformed files are all reported", func(t *testing.T) {
		cfg := base()
		cfg.Content.Rules = []ContentFile{
			{Name: "a", Path: "rules/a.md", MalformedFrontmatter: true},
		}
		cfg.Content.Agents = []ContentFile{
			{Name: "b", Path: "agents/b.md", MalformedFrontmatter: true},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rules/a.md")
		assert.Contains(t, err.Error(), "agents/b.md")
	})
}

func TestConfigValidateOutputNamespaceCollisions(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			Content: &ContentTree{},
		}
	}

	t.Run("root skill and root command with same id error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "code-review", Path: "skills/code-review/SKILL.md"},
		}
		cfg.Content.Commands = []ContentFile{
			{Name: "code-review", Path: "commands/code-review.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "code-review")
		assert.Contains(t, err.Error(), "skills/code-review/SKILL.md")
		assert.Contains(t, err.Error(), "commands/code-review.md")
	})

	t.Run("skill in domain A and command in domain B with same id error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Domains = map[string]*Domain{
			"qa": {
				Name: "qa",
				Skills: []ContentFile{
					{Name: "playwright-test", Path: "domains/qa/skills/playwright-test/SKILL.md"},
				},
			},
			"backend": {
				Name: "backend",
				Commands: []ContentFile{
					{Name: "playwright-test", Path: "domains/backend/commands/playwright-test.md"},
				},
			},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "playwright-test")
		assert.Contains(t, err.Error(), "domains/qa/skills/playwright-test/SKILL.md")
		assert.Contains(t, err.Error(), "domains/backend/commands/playwright-test.md")
	})

	t.Run("ids differing only by case collide after sanitization", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "MySkill", Path: "skills/MySkill/SKILL.md"},
		}
		cfg.Content.Commands = []ContentFile{
			{Name: "myskill", Path: "commands/myskill.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "myskill")
	})

	// Only commands have their underscores rewritten to dashes; a skill id is the
	// directory name verbatim. So an underscore in a command name collides with a
	// dashed skill, while an underscore in a skill name does not collide with a
	// dashed command. The check has to follow that asymmetry or it reports
	// collisions that never happen and misses the ones that do.
	t.Run("underscored command collides with dashed skill", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "test-skill", Path: "skills/test-skill/SKILL.md"},
		}
		cfg.Content.Commands = []ContentFile{
			{Name: "test_skill", Path: "commands/test_skill.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commands/test_skill.md")
	})

	t.Run("underscored skill does not collide with dashed command", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "test_skill", Path: "skills/test_skill/SKILL.md"},
		}
		cfg.Content.Commands = []ContentFile{
			{Name: "test-skill", Path: "commands/test-skill.md"},
		}
		assert.NoError(t, cfg.Validate(),
			"skills/test_skill/ and skills/test-skill/ are different directories on disk")
	})

	t.Run("skill and command with genuinely distinct ids pass", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "review", Path: "skills/review/SKILL.md"},
		}
		cfg.Content.Commands = []ContentFile{
			{Name: "test", Path: "commands/test.md"},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("root skill shadowing domain skill of same name still passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "shared", Path: "skills/shared/SKILL.md"},
		}
		cfg.Content.Domains = map[string]*Domain{
			"backend": {
				Name: "backend",
				Skills: []ContentFile{
					{Name: "shared", Path: "domains/backend/skills/shared/SKILL.md"},
				},
			},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("multiple collisions reported", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "foo", Path: "skills/foo/SKILL.md"},
			{Name: "bar", Path: "skills/bar/SKILL.md"},
		}
		cfg.Content.Commands = []ContentFile{
			{Name: "foo", Path: "commands/foo.md"},
			{Name: "bar", Path: "commands/bar.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		// Every collision must be named — fixing one at a time, with a rerun to
		// learn about the next, is what makes a 200-skill repository unfixable.
		// The order is sorted, so the message is reproducible run to run.
		assert.Equal(t,
			`skill and command ids collide in the output namespace: `+
				`skill "bar" (skills/bar/SKILL.md) vs command "bar" (commands/bar.md); `+
				`skill "foo" (skills/foo/SKILL.md) vs command "foo" (commands/foo.md)`,
			err.Error())
	})

	t.Run("command directory form is recognized", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "advanced", Path: "skills/advanced/SKILL.md"},
		}
		cfg.Content.Commands = []ContentFile{
			{Name: "advanced", Path: "commands/advanced/COMMAND.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "advanced")
	})
}

func TestConfigValidateDuplicateOutputIDs(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			Content: &ContentTree{},
		}
	}

	t.Run("flat and directory command with the same name error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Commands = []ContentFile{
			{Name: "deploy", Path: "commands/deploy/COMMAND.md"},
			{Name: "deploy", Path: "commands/deploy.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commands/deploy/COMMAND.md")
		assert.Contains(t, err.Error(), "commands/deploy.md")
	})

	// The directory form takes the directory name verbatim while the output id
	// is case-folded and dashed, so these two only meet after normalization.
	t.Run("command ids colliding only after normalization error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Commands = []ContentFile{
			{Name: "My_Command", Path: "commands/My_Command/COMMAND.md"},
			{Name: "my-command", Path: "commands/my-command.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commands/My_Command/COMMAND.md")
		assert.Contains(t, err.Error(), "commands/my-command.md")
	})

	t.Run("two skills differing only by case error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "Review", Path: "skills/Review/SKILL.md"},
			{Name: "review", Path: "skills/review/SKILL.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skills/Review/SKILL.md")
		assert.Contains(t, err.Error(), "skills/review/SKILL.md")
	})

	t.Run("duplicate ids inside one domain error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Domains = map[string]*Domain{
			"qa": {
				Name: "qa",
				Commands: []ContentFile{
					{Name: "rca", Path: "domains/qa/commands/rca/COMMAND.md"},
					{Name: "rca", Path: "domains/qa/commands/rca.md"},
				},
			},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "domains/qa/commands/rca/COMMAND.md")
		assert.Contains(t, err.Error(), "domains/qa/commands/rca.md")
	})

	// Cross-scope duplicates are resolved deliberately by the scanner (domain
	// content overrides root, and domain ids are namespaced), so they must keep
	// loading.
	t.Run("same command id in root and in a domain passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Commands = []ContentFile{
			{Name: "deploy", Path: "commands/deploy.md"},
		}
		cfg.Content.Domains = map[string]*Domain{
			"qa": {
				Name:     "qa",
				Commands: []ContentFile{{Name: "deploy", Path: "domains/qa/commands/deploy.md"}},
			},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("same command id in two domains passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Domains = map[string]*Domain{
			"qa": {
				Name:     "qa",
				Commands: []ContentFile{{Name: "deploy", Path: "domains/qa/commands/deploy.md"}},
			},
			"backend": {
				Name:     "backend",
				Commands: []ContentFile{{Name: "deploy", Path: "domains/backend/commands/deploy.md"}},
			},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("distinct ids in one scope pass", func(t *testing.T) {
		cfg := base()
		cfg.Content.Commands = []ContentFile{
			{Name: "deploy", Path: "commands/deploy/COMMAND.md"},
			{Name: "release", Path: "commands/release.md"},
		}
		cfg.Content.Skills = []ContentFile{
			{Name: "review", Path: "skills/review/SKILL.md"},
		}
		assert.NoError(t, cfg.Validate())
	})

	// Include merges carry the same entry into more than one slice; one file
	// cannot overwrite itself, so only distinct sources are an authoring error.
	t.Run("the same source file listed twice passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Commands = []ContentFile{
			{Name: "deploy", Path: "commands/deploy.md"},
			{Name: "deploy", Path: "commands/deploy.md"},
		}
		assert.NoError(t, cfg.Validate())
	})

	// A flat skills/name.md file resolves to the *directory* name as its output
	// id, so every flat skill in one directory reports the same id. That
	// degenerate derivation predates the directory form for commands and is not
	// this check's business.
	t.Run("flat skill files sharing a directory pass", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{Name: "alpha", Path: "skills/alpha.md"},
			{Name: "beta", Path: "skills/beta.md"},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("every duplicate is named in a reproducible order", func(t *testing.T) {
		cfg := base()
		cfg.Content.Commands = []ContentFile{
			{Name: "deploy", Path: "commands/deploy/COMMAND.md"},
			{Name: "deploy", Path: "commands/deploy.md"},
		}
		cfg.Content.Skills = []ContentFile{
			{Name: "Review", Path: "skills/Review/SKILL.md"},
			{Name: "review", Path: "skills/review/SKILL.md"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Equal(t,
			`duplicate output ids: `+
				`command "deploy" (commands/deploy/COMMAND.md) vs command "deploy" (commands/deploy.md); `+
				`skill "Review" (skills/Review/SKILL.md) vs skill "review" (skills/review/SKILL.md)`,
			err.Error())
	})
}

func TestConfigValidateSkillArgumentHint(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			Content: &ContentTree{},
		}
	}

	t.Run("skill with argument-hint warns", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{
				Name: "my-skill",
				Path: "skills/my-skill/SKILL.md",
				Metadata: &Metadata{
					Extra: map[string]string{"argument-hint": "some hint"},
				},
			},
		}
		assert.NoError(t, cfg.Validate(), "an inert argument-hint is advisory, never fatal")
		assert.Equal(t, []skillWarning{{
			Scope:   scopeRoot,
			Skill:   "my-skill",
			Path:    "skills/my-skill/SKILL.md",
			Message: warnInertSkillArgumentHint,
		}}, cfg.skillArgumentHintWarnings())
	})

	t.Run("domain skill with argument-hint warns", func(t *testing.T) {
		cfg := base()
		cfg.Content.Domains = map[string]*Domain{
			"qa": {
				Name: "qa",
				Skills: []ContentFile{
					{
						Name: "test-skill",
						Path: "domains/qa/skills/test-skill/SKILL.md",
						Metadata: &Metadata{
							Extra: map[string]string{"argument-hint": "args"},
						},
					},
				},
			},
		}
		assert.NoError(t, cfg.Validate())
		assert.Equal(t, []skillWarning{{
			Scope:   "domain qa",
			Skill:   "test-skill",
			Path:    "domains/qa/skills/test-skill/SKILL.md",
			Message: warnInertSkillArgumentHint,
		}}, cfg.skillArgumentHintWarnings())
	})

	t.Run("command with argument-hint does not warn", func(t *testing.T) {
		cfg := base()
		cfg.Content.Commands = []ContentFile{
			{
				Name: "my-command",
				Path: "commands/my-command.md",
				Metadata: &Metadata{
					Extra: map[string]string{"argument-hint": "valid"},
				},
			},
		}
		assert.NoError(t, cfg.Validate())
		assert.Empty(t, cfg.skillArgumentHintWarnings())
	})

	t.Run("skill without argument-hint does not warn", func(t *testing.T) {
		cfg := base()
		cfg.Content.Skills = []ContentFile{
			{
				Name: "clean-skill",
				Path: "skills/clean-skill/SKILL.md",
				Metadata: &Metadata{
					Extra: map[string]string{"description": "a skill"},
				},
			},
		}
		assert.NoError(t, cfg.Validate())
		assert.Empty(t, cfg.skillArgumentHintWarnings())
	})
}
