package includes

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The "--no-fetch specified" reason is only true when the user passed the flag;
// a command that reads cached includes by design says so instead.
func TestSourceFetch_OfflineReasonNamesTheRealCause(t *testing.T) {
	tests := []struct {
		name     string
		skipFlag bool
		want     string
		notWant  string
	}{
		{"no flag", false, "does not fetch includes", "--no-fetch"},
		{"flag passed", true, "--no-fetch specified", "does not fetch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			src, err := NewGitSource("reason-"+tt.name, "https://example.invalid/none/repo.git", "", "", t.TempDir(), nil, "")
			require.NoError(t, err)
			skill, err := NewSkillGitSource("reason-skill-"+tt.name, "https://example.invalid/none/repo.git", "", "", "")
			require.NoError(t, err)
			ctx := config.WithOfflineIncludes(context.Background())
			if tt.skipFlag {
				ctx = config.WithNoFetch(ctx)
			}

			// Act
			_, errInc := src.Fetch(ctx)
			_, errSkill := skill.Fetch(ctx)

			// Assert
			for _, e := range []error{errInc, errSkill} {
				require.Error(t, e)
				assert.Contains(t, e.Error(), tt.want)
				assert.False(t, strings.Contains(e.Error(), tt.notWant), e.Error())
			}
		})
	}
}
