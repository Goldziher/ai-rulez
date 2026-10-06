package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImprovePR_IsolationFlag(t *testing.T) {
	// Assert: the flag exists on pr and is documented
	flag := improvePRCmd.Flags().Lookup("isolation")
	require.NotNil(t, flag)
	assert.Contains(t, flag.Usage, "worktree")

	tests := []struct {
		name    string
		mode    string
		wantErr string
	}{
		{"an unknown mode is refused before anything runs", "paranoid", "unknown isolation"},
		{"require without a working backend or run is refused by the run lookup", "require", "no saved run"},
		{"none is accepted", "none", "no saved run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			resetImproveFlags(t)
			improveProject(t)
			require.NoError(t, improvePRCmd.Flags().Set("isolation", tt.mode))
			t.Cleanup(func() { improvePRCmd.Flags().Lookup("isolation").Changed = false })
			improvePRCmd.SetOut(&bytes.Buffer{})
			improvePRCmd.SetErr(&bytes.Buffer{})

			// Act
			err := improvePRCmd.RunE(improvePRCmd, []string{"imp-00000000"})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
