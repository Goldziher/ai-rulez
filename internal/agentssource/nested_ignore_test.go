package agentssource_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentssource"
	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func loadRecorded(t *testing.T, root string) (*config.Config, *testutil.LogRecorder) {
	t.Helper()
	rec := &testutil.LogRecorder{}
	cfg, err := config.LoadConfig(t.Context(), root,
		config.WithResolvers(config.Resolvers{Agents: agentssource.Read}), config.WithHost(ambient.Host{Log: rec}))
	require.NoError(t, err)
	return cfg, rec
}

func gitignoreWarnings(rec *testutil.LogRecorder) []string {
	var out []string
	for _, line := range rec.Level("WARN") {
		if strings.Contains(line, ".gitignore") && strings.Contains(line, "not a git repository") {
			out = append(out, line)
		}
	}
	return out
}

func TestNestedWarnsWhenGitignoreCannotBeApplied(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantWarns int
	}{
		{"outside a repository with a root .gitignore", map[string]string{".gitignore": "scratch/\n", "svc/AGENTS.md": "# S\n"}, 1},
		{"outside a repository without a .gitignore", map[string]string{"svc/AGENTS.md": "# S\n"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := nestedProject(t, tt.files)

			// Act
			cfg, rec := loadRecorded(t, root)

			// Assert
			assert.Len(t, gitignoreWarnings(rec), tt.wantWarns, rec.String())
			assert.Contains(t, names(cfg.Content.Context), "agents-svc", "nothing is ignored without git")
		})
	}
}

func TestNestedDoesNotWarnInsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Arrange
	root := nestedProject(t, map[string]string{".gitignore": "scratch/\n", "svc/AGENTS.md": "# S\n"})
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())

	// Act
	_, rec := loadRecorded(t, root)

	// Assert
	assert.Empty(t, gitignoreWarnings(rec), rec.String())
}

func TestNestedProjectConfigThroughASymlinkDoesNotOwnTheDirectory(t *testing.T) {
	// Arrange
	root := nestedProject(t, map[string]string{
		"svc/AGENTS.md":   "# Service\n\nTEXT\n",
		"other/config.md": agentsConfig,
	})
	write(t, root, "svc/.ai-rulez/placeholder", "x")
	testutil.SymlinkOrSkip(t, filepath.Join("..", "..", "other", "config.md"), filepath.Join(root, "svc", ".ai-rulez", "config.toml"))

	// Act
	cfg := load(t, root)

	// Assert
	assert.Contains(t, names(cfg.Content.Context), "agents-svc", "a symlinked config is not a project of its own")
}
