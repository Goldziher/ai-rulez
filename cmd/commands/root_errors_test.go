package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Execute returns the error and main prints it once; cobra must not print it too.
func TestRootCmdDoesNotPrintErrorsItself(t *testing.T) {
	// Arrange
	var stderr, stdout bytes.Buffer
	RootCmd.SetErr(&stderr)
	RootCmd.SetOut(&stdout)
	RootCmd.SetArgs([]string{"generate", "--no-such-flag"})
	t.Cleanup(func() {
		RootCmd.SetErr(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetArgs(nil)
	})

	// Act
	err := RootCmd.Execute()

	// Assert
	require.Error(t, err)
	assert.NotContains(t, stderr.String()+stdout.String(), err.Error())
	assert.True(t, RootCmd.SilenceErrors)
}

func TestRootLongHelpHasNoLiteralBackslashN(t *testing.T) {
	assert.False(t, strings.Contains(RootCmd.Long, `\n`), RootCmd.Long)
}
