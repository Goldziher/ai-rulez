package commands

import (
	"path/filepath"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnforceScanImports_AcceptedTrees(t *testing.T) {
	tests := []struct {
		name    string
		accept  func(vendor string) []string
		wantErr bool
	}{
		{name: "nothing accepted stops", accept: func(string) []string { return nil }, wantErr: true},
		{name: "another tree accepted stops", accept: func(v string) []string { return []string{filepath.Join(v, "other")} }, wantErr: true},
		{name: "the accepted tree passes", accept: func(v string) []string { return []string{v} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := importingConfig(t, "error")
			vendor := filepath.Join(cfg.BaseDir, "vendor", "imp")

			// Act
			err := enforceScanImports(cfg, tt.accept(vendor))

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				oe, ok := oops.AsOops(err)
				require.True(t, ok)
				assert.Contains(t, oe.Hint(), "trust = \"warn\"")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRunGenerate_RefreshRemoteAcceptFindingsGeneratesFromTheAcceptedTree(t *testing.T) {
	// Arrange: upstream moves to a commit whose skill ships a secret
	p := newRefreshProject(t, refreshKindInst, "")
	require.NoError(t, runGenerate(GenerateCmd, nil))
	head := p.upstream(t, crossSkillBody(refreshNewBody+"\n"+leakyRule))
	require.NoError(t, GenerateCmd.Flags().Set(flagAcceptFindings, "true"))
	t.Cleanup(func() { require.NoError(t, GenerateCmd.Flags().Set(flagAcceptFindings, "false")) })

	// Act
	_, err := runRefreshGenerate(t)

	// Assert: the lock and the outputs agree on the accepted commit
	require.NoError(t, err)
	assert.Equal(t, head, p.pinnedCommit(t, refreshKindInst))
	assert.Contains(t, p.output(t), refreshNewBody)
}
