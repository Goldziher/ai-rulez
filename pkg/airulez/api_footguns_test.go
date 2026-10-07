package airulez_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

// TestValidateHonoursItsContext is RV-ENGINE-8: Validate took a context and
// ignored it, so a strict validation walk could not be canceled.
func TestValidateHonoursItsContext(t *testing.T) {
	tests := []struct {
		name   string
		strict bool
	}{
		{name: "configuration check"},
		{name: "strict check", strict: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeSources(t, dir)
			ws, err := airulez.DirWorkspace(dir)
			require.NoError(t, err)
			p, err := airulez.Load(t.Context(), airulez.Options{Workspace: ws})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			// Act
			report, err := p.Validate(ctx, airulez.ValidateOptions{Strict: tt.strict})

			// Assert
			require.Error(t, err)
			assert.Nil(t, report)
			var aerr *airulez.Error
			require.ErrorAs(t, err, &aerr)
			assert.Equal(t, airulez.CodeValidate, aerr.Code)
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}

// TestDenyAllIsTheDefaultAndCannotBeReplaced is RV-ENGINE-8: DenyAll was an
// exported variable any package could reassign, silently granting process
// execution to every later Load. It is a function now; each call refuses.
func TestDenyAllIsTheDefaultAndCannotBeReplaced(t *testing.T) {
	// Act
	res := airulez.DenyAll().Run(t.Context(), airulez.Spec{Argv: []string{"git", "status"}})

	// Assert
	assert.Equal(t, airulez.StatusUnavailable, res.Status)
	assert.Equal(t, -1, res.ExitCode)
	require.Error(t, res.Err)
	assert.False(t, errors.Is(res.Err, context.Canceled))
}
