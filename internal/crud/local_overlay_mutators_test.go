package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

const mutatorSharedConfig = `version = "4.0"
name = "test-project"
presets = ["claude"]
default = "base"

[profiles]
base = ["backend"]
extra = ["backend"]

[[includes]]
name = "inc"
source = "git@github.com:example/inc.git"

[[installed_skills]]
name = "sk"
source = "git@github.com:example/sk.git"
`

const mutatorLocalConfig = `name = "mine"

[profiles]
mine-profile = ["backend"]

[[includes]]
name = "local-inc"
source = "git@github.com:example/local-inc.git"

[[mcp_servers]]
name = "local-server"
command = "local-cmd"
`

func TestMutators_NeverPersistLocalOverlay(t *testing.T) {
	tests := []struct {
		name string
		op   func(op crud.Operator) error
	}{
		{"RemoveProfile", func(op crud.Operator) error { return op.RemoveProfile(context.Background(), "extra") }},
		{"SetDefaultProfile", func(op crud.Operator) error { return op.SetDefaultProfile(context.Background(), "extra") }},
		{"InstallSkill", func(op crud.Operator) error {
			return op.InstallSkill(context.Background(), &crud.InstallSkillRequest{
				Name: "new-sk", Source: "git@github.com:example/new-sk.git",
			})
		}},
		{"UninstallSkill", func(op crud.Operator) error { return op.UninstallSkill(context.Background(), "sk") }},
		{"RemoveInclude", func(op crud.Operator) error { return op.RemoveInclude(context.Background(), "inc") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			dir := filepath.Join(baseDir, ".ai-rulez")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			configPath := filepath.Join(dir, "config.toml")
			require.NoError(t, os.WriteFile(configPath, []byte(mutatorSharedConfig), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(mutatorLocalConfig), 0o600))
			op, err := crud.NewOperator(baseDir)
			require.NoError(t, err)

			// Act
			err = tt.op(op)

			// Assert
			require.NoError(t, err)
			saved, err := os.ReadFile(configPath)
			require.NoError(t, err)
			for _, leaked := range []string{"mine", "local-inc", "local-server", "local-cmd"} {
				assert.NotContains(t, string(saved), leaked)
			}
			assert.NotEqual(t, mutatorSharedConfig, string(saved), "the mutation must have been saved")
		})
	}
}
