package generator

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExistingOutputReadsGoThroughTheProjectWorkspace keeps the planning reads
// of existing outputs off the operating-system path: a project held in memory
// or in a commit must see its own outputs, not this machine's disk.
func TestExistingOutputReadsGoThroughTheProjectWorkspace(t *testing.T) {
	t.Parallel()
	// user.go also probes the real home directory for symlinks that escape it: those
	// os.Lstat calls look at the disk on purpose and are not reads of an output.
	all := regexp.MustCompile(`\bos\.(ReadFile|Lstat|Stat|Open)\(|[^.]\blooksGenerated\(|\bscanStoredHashes\(|\bextractStoredHashes\(`)
	user := regexp.MustCompile(`\bos\.ReadFile\(|[^.]\blooksGenerated\(`)
	tests := []struct {
		file   string
		direct *regexp.Regexp
	}{
		{"stale_verify.go", all}, {"roles_prior.go", all}, {"user.go", user},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			// Arrange
			src, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			// Act
			found := tt.direct.FindAllString(string(src), -1)

			// Assert
			assert.Empty(t, found, "read existing outputs with g.config.ReadExisting, StatExisting or LstatExisting")
		})
	}
}
