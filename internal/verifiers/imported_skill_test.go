package verifiers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func skillVerifierProject(t *testing.T, toml string) *config.Config {
	t.Helper()
	cfg := specProject(t, map[string]string{"a.sql": "DROP TABLE t;\n"}, "")
	cfg.Content.ImportedVerifiers = []config.ImportedVerifierFile{{Include: "deploy", Skill: true, Name: "skill.toml", Data: toml}}
	return cfg
}

func TestLoadSpecs_SkillShippedVerifier(t *testing.T) {
	// Arrange
	cfg := skillVerifierProject(t, importedRegex)

	// Act
	specs, problems := LoadSpecs(cfg)
	rep := Run(context.Background(), cfg, Options{})

	// Assert
	require.Empty(t, problems)
	require.Len(t, specs, 1)
	assert.Equal(t, "skill:deploy/verifiers/skill.toml", specs[0].Source())
	require.Len(t, rep.Results, 1)
	assert.Equal(t, StatusFail, rep.Results[0].Status, "a skill's non-executing verifier runs like a local one")
}

func TestLoadSpecs_SkillShippedCommandIsNeverAllowed(t *testing.T) {
	tests := []struct {
		name     string
		settings *config.VerifiersSettings
	}{
		{"by default", nil},
		{"even when an include of the same name is trusted", &config.VerifiersSettings{TrustExecFrom: []string{"deploy"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := skillVerifierProject(t, importedCmd)
			cfg.VerifiersSettings = tt.settings
			cfg.Includes = []config.IncludeConfig{{Name: "deploy", Source: "https://example.com/org/shared.git"}}

			// Act
			specs, problems := LoadSpecs(cfg)

			// Assert
			assert.Empty(t, specs)
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0].Message, "installed skill")
			assert.Contains(t, problems[0].Message, "may never use")
		})
	}
}
