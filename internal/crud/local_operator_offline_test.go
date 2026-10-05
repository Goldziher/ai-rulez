package crud_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

// Local mutations resolve includes from cache only, so they work offline even
// when the shared config pulls from a remote repository.
func TestLocalOperator_WorksWithARemoteIncludeOffline(t *testing.T) {
	ctx := context.Background()
	p := setupLocalOpProject(t)
	withRemote := p.shared + "\n[[includes]]\nname = \"remote\"\nsource = \"https://example.invalid/none/repo.git\"\n"
	require.NoError(t, os.WriteFile(p.sharedPath, []byte(withRemote), 0o600))
	op, err := crud.NewOperator(p.baseDir)
	require.NoError(t, err)

	require.NoError(t, op.Local().AddProfile(ctx, "mine", []string{"backend"}))

	merged, err := config.LoadConfig(config.WithOfflineIncludes(ctx), p.baseDir)
	require.NoError(t, err)
	assert.Contains(t, merged.Profiles, "mine")
}
