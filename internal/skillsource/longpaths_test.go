package skillsource

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// The cache tree of a skill source sits deep below the user cache directory,
// and git's pack files add more: on Windows that passes MAX_PATH ("Filename
// too long") unless git uses long paths. Every git command enables them.
func TestRunGit_EnablesLongPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "fetch", args: []string{"fetch", flagQuiet, "--", "origin", "main"}},
		{name: "ls-remote", args: []string{"ls-remote", "--", "file:///r.git", refHEAD}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{}
			ctx := runner.WithContext(context.Background(), fake)

			// Act
			_, err := runGit(ctx, t.TempDir(), tt.args...)

			// Assert
			require.NoError(t, err)
			calls := fake.Calls()
			require.Len(t, calls, 1)
			argv := calls[0].Argv
			i := slices.Index(argv, "core.longpaths=true")
			require.Positive(t, i, "argv: %v", argv)
			assert.Equal(t, "-c", argv[i-1])
			assert.Less(t, i, slices.Index(argv, tt.args[0]), "a -c option must come before the subcommand")
		})
	}
}
