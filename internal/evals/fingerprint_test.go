package evals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellFields(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"plain", "python run.py --x", []string{"python", "run.py", "--x"}},
		{"double quotes keep spaces", `python "my dir/run.py" a`, []string{"python", "my dir/run.py", "a"}},
		{"single quotes", `sh 'a b.sh'`, []string{"sh", "a b.sh"}},
		{"backslash space", `sh a\ b.sh`, []string{"sh", "a b.sh"}},
		{"empty", "   ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shellFields(tt.in))
		})
	}
}

func TestCommandProgramStamp_CoversScriptArgumentsNotJustTheFirstWord(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	t.Chdir(dir)
	script := filepath.Join(dir, "run.py")
	require.NoError(t, os.WriteFile(script, []byte("print(1)\n"), 0o600))
	before := commandProgramStamp("sh run.py --flag")

	// Act: edit the script the interpreter runs
	require.NoError(t, os.WriteFile(script, []byte("print(2)\n"), 0o600))
	after := commandProgramStamp("sh run.py --flag")

	// Assert
	assert.NotEmpty(t, before)
	assert.NotEqual(t, before, after, "editing run.py must change the stamp")
}

func TestCommandProgramStamp_HandlesQuotedPaths(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll("my dir", 0o750))
	require.NoError(t, os.WriteFile("my dir/run.sh", []byte("a"), 0o600))
	before := commandProgramStamp(`sh "my dir/run.sh"`)
	require.NoError(t, os.WriteFile("my dir/run.sh", []byte("b"), 0o600))
	assert.NotEqual(t, before, commandProgramStamp(`sh "my dir/run.sh"`))
}
