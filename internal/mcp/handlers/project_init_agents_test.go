package handlers

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// with_agents is advertised as "Include sample agent configurations": it must
// create one, and leave the agents directory empty without it.
func TestInitProjectHandler_WithAgentsCreatesASampleAgent(t *testing.T) {
	tests := []struct {
		name       string
		withAgents any
		wantAgent  bool
	}{
		{"with_agents true", true, true},
		{"with_agents false", false, false},
		{"with_agents omitted", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			args := map[string]any{"project_name": "demo", "working_directory": dir}
			if tt.withAgents != nil {
				args["with_agents"] = tt.withAgents
			}

			res, err := InitProjectHandler(t.Context(), newRequestWithArgs(args))

			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			agent := filepath.Join(dir, ".ai-rulez", "agents", "code-reviewer.md")
			if tt.wantAgent {
				assert.FileExists(t, agent)
			} else {
				assert.NoFileExists(t, agent)
			}
		})
	}
}
