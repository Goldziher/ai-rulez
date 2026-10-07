package improve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveArgv_RelativePathsAreTheInvokingDirectorys(t *testing.T) {
	// Arrange: a directory holding the optimizer script.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "optimize.py"), []byte("print()"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tools"), 0o750))
	abs := filepath.Join(t.TempDir(), "elsewhere.py")
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"an interpreter and a script", []string{"python", "optimize.py"}, []string{"python", filepath.Join(dir, "optimize.py")}},
		{"a relative program", []string{"./optimize.py", "--fast"}, []string{filepath.Join(dir, "optimize.py"), "--fast"}},
		{"a bare program stays a PATH lookup", []string{"optimize", "x"}, []string{"optimize", "x"}},
		{"a flag and a missing file stay", []string{"python", "-u", "missing.py"}, []string{"python", "-u", "missing.py"}},
		{"a directory is not a script", []string{"python", "tools"}, []string{"python", "tools"}},
		{"an absolute path stays", []string{"python", abs}, []string{"python", abs}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := ResolveArgv(tt.argv, dir)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
