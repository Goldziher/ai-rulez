package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteRawOutput_ReplacesTheFileInsteadOfWritingThroughIt(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, generated, elsewhere string)
	}{
		{
			name: "a hard link elsewhere keeps the old bytes",
			check: func(t *testing.T, _, elsewhere string) {
				t.Helper()
				data, err := os.ReadFile(elsewhere)
				require.NoError(t, err)
				assert.Equal(t, "#!/bin/sh\necho one\n", string(data))
			},
		},
		{
			name: "the generated script holds the new bytes",
			check: func(t *testing.T, generated, _ string) {
				t.Helper()
				data, err := os.ReadFile(generated)
				require.NoError(t, err)
				assert.Equal(t, "#!/bin/sh\necho two\n", string(data))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietWarnings(t)
			// Arrange
			dir := hashesProject(t, "")
			source := filepath.Join(dir, ".ai-rulez", "skills", "alpha", "scripts", "run.sh")
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
			require.NoError(t, os.WriteFile(source, []byte("#!/bin/sh\necho one\n"), 0o755))
			require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
			generated := filepath.Join(dir, ".claude", "skills", "alpha", "scripts", "run.sh")
			elsewhere := filepath.Join(t.TempDir(), "run.sh")
			require.NoError(t, os.Link(generated, elsewhere))
			require.NoError(t, os.WriteFile(source, []byte("#!/bin/sh\necho two\n"), 0o755))

			// Act
			require.NoError(t, newProjectGenerator(t, dir).Generate("default"))

			// Assert
			tt.check(t, generated, elsewhere)
		})
	}
}
