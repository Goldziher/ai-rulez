package lint

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint/scanners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFillProfileDefaultsBatchingAndVersion(t *testing.T) {
	// No embedded profile sets batch_size or version, so a synthetic one covers the fill-in.
	profile := scanners.Profile{Name: "synthetic", BatchSize: 1, Version: ">=2, <3"}
	tests := []struct {
		name        string
		entry       config.LintExternal
		profile     scanners.Profile
		wantBatch   int
		wantVersion string
	}{
		{
			name:        "an entry with only a profile inherits batch_size and version",
			entry:       config.LintExternal{Name: "ss"},
			profile:     profile,
			wantBatch:   1,
			wantVersion: ">=2, <3",
		},
		{
			name:        "keys set on the entry win",
			entry:       config.LintExternal{Name: "ss", BatchSize: 4, Version: ">=2.5"},
			profile:     profile,
			wantBatch:   4,
			wantVersion: ">=2.5",
		},
		{
			name:    "a profile without batching leaves batch_size alone",
			entry:   config.LintExternal{Name: "a"},
			profile: scanners.Profile{Name: "plain"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rs := resolvedScanner{LintExternal: tt.entry}
			// Act
			fillProfileDefaults(&rs, tt.profile)
			// Assert
			assert.Equal(t, tt.wantBatch, rs.BatchSize)
			assert.Equal(t, tt.wantVersion, rs.Version)
		})
	}
}

func TestEmbeddedProfilesFillCommandWithoutProblems(t *testing.T) {
	for _, p := range scanners.Profiles() {
		t.Run(p.Name, func(t *testing.T) {
			// Act
			rs := fromProfile(config.LintExternal{Name: "x", Profile: p.Name})
			// Assert
			assert.Empty(t, rs.Problems)
			assert.Equal(t, p.Command, rs.Command)
		})
	}
}

func TestProfileBlocksItsEgressOptions(t *testing.T) {
	tests := []struct {
		flag string
	}{{"--use-llm"}, {"--use-virustotal"}, {"--llm-provider"}}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			// Arrange
			rs := fromProfile(config.LintExternal{Name: "cs", Profile: "cisco-skill-scanner",
				Command: []string{"skill-scanner", "scan-all", tt.flag, "{skill_dirs}"}})
			// Act / Assert
			assert.Equal(t, tt.flag, rs.egressFlag())
		})
	}
	rs := fromProfile(config.LintExternal{Name: "cs", Profile: "cisco-skill-scanner"})
	assert.Empty(t, rs.egressFlag(), "the profile's own command carries no egress option")
}

func TestFromProfileRejectsAnUnknownProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
	}{
		{"the removed skillspector profile", "skillspector"},
		{"a made-up name", "nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			rs := fromProfile(config.LintExternal{Name: "x", Profile: tt.profile})
			// Assert
			require.Len(t, rs.Problems, 1)
			assert.Contains(t, rs.Problems[0], `profile "`+tt.profile+`" is not one of `)
			for _, name := range scanners.Names() {
				assert.Contains(t, rs.Problems[0], name)
			}
		})
	}
}
