package lint

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromProfileFillsBatchingAndVersionFromTheProfile(t *testing.T) {
	tests := []struct {
		name        string
		entry       config.LintExternal
		wantBatch   int
		wantVersion string
		wantProblem string
	}{
		{
			name:        "an entry with only a profile inherits batch_size and version",
			entry:       config.LintExternal{Name: "ss", Profile: "skillspector"},
			wantBatch:   1,
			wantVersion: ">=2, <3",
		},
		{
			name:        "keys set on the entry win",
			entry:       config.LintExternal{Name: "ss", Profile: "skillspector", BatchSize: 4, Version: ">=2.5"},
			wantBatch:   4,
			wantVersion: ">=2.5",
		},
		{
			name: "dropping --no-llm from the command is a configuration problem",
			entry: config.LintExternal{Name: "ss", Profile: "skillspector",
				Command: []string{"skillspector", "scan", "--format", "sarif", "{skill_dirs}"}},
			wantBatch:   1,
			wantVersion: ">=2, <3",
			wantProblem: `profile "skillspector" requires --no-llm`,
		},
		{
			name:        "a profile without batching leaves batch_size alone",
			entry:       config.LintExternal{Name: "a", Profile: "agnix"},
			wantBatch:   0,
			wantVersion: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			rs := fromProfile(tt.entry)
			// Assert
			assert.Equal(t, tt.wantBatch, rs.BatchSize)
			assert.Equal(t, tt.wantVersion, rs.Version)
			if tt.wantProblem == "" {
				assert.Empty(t, rs.Problems)
				return
			}
			require.Len(t, rs.Problems, 1)
			assert.Contains(t, rs.Problems[0], tt.wantProblem)
		})
	}
}

func TestSkillspectorProfileBlocksItsEgressOptions(t *testing.T) {
	tests := []struct {
		flag string
	}{{"--transitive"}, {"--mcp-registry"}, {"--mcp-registry-compare"}}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			// Arrange
			rs := fromProfile(config.LintExternal{Name: "ss", Profile: "skillspector",
				Command: []string{"skillspector", "scan", "--no-llm", tt.flag, "{skill_dirs}"}})
			// Act / Assert
			assert.Equal(t, tt.flag, rs.egressFlag())
		})
	}
	rs := fromProfile(config.LintExternal{Name: "ss", Profile: "skillspector"})
	assert.Empty(t, rs.egressFlag(), "the profile's own command carries no egress option")
}
