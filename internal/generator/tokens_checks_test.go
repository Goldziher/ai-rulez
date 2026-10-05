package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

func TestTokenReport_CountsCheckOutputsAsUnmodeled(t *testing.T) {
	// Arrange
	for _, preset := range []string{"kilo", "cursor"} {
		t.Run(preset, func(t *testing.T) {
			base := writeChecksProject(t, []string{preset}, map[string]string{"checks/security.md": securityCheck}, "")

			// Act
			report, err := NewGenerator(mustLoad(t, base)).TokenReport(TokenReportOptions{Counter: tokens.CL100KBase()})

			// Assert: the check file has a cost, and ai-rulez does not claim to know when it loads.
			require.NoError(t, err)
			runtime := findRuntime(t, report, preset)
			total := 0
			for _, entry := range runtime.Entries {
				if entry.Bucket == BucketUnmodeled {
					total += entry.Tokens
				}
			}
			assert.Positive(t, total, "the check file must show up in the report as unmodeled surface: %+v", runtime.Entries)
		})
	}
}
