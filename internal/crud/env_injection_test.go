package crud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestValidateLocalPathUsesTheInjectedEnv(t *testing.T) {
	// Arrange
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "shared", ".ai-rulez"), 0o755))
	env := ambient.MapEnv{Home: home, Vars: map[string]string{"TEAM_DIR": filepath.Join(home, "shared")}}
	tests := []struct {
		name    string
		path    string
		env     ambient.Env
		wantErr bool
	}{
		{"tilde resolves in the injected home", "~/shared", env, false},
		{"variable resolves in the injected env", "${TEAM_DIR}", env, false},
		{"a missing directory is refused", "~/missing", env, true},
		{"no home directory is an error", "~/shared", ambient.MapEnv{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := validateLocalPath(tt.env, tt.path)
			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
