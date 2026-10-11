package progress

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProgressIsSuppressedWhenNotInteractive(t *testing.T) {
	// Arrange
	SetInteractive(false)
	SetQuiet(false)
	t.Cleanup(func() {
		SetInteractive(true)
		SetQuiet(false)
	})

	// Act / Assert
	assert.True(t, New(10, "x").quiet, "no animated bar without an interactive stream")
	assert.Nil(t, New(10, "x").bar)
	assert.True(t, NewBytes(10, "x").quiet)
	assert.True(t, NewSpinner("x").quiet)
	assert.True(t, NewFileCounter(3, "x").bar.quiet)
}

func TestProgressIsSuppressedByQuiet(t *testing.T) {
	// Arrange
	SetInteractive(true)
	SetQuiet(true)
	t.Cleanup(func() { SetQuiet(false) })

	// Act / Assert
	assert.True(t, New(10, "x").quiet, "--quiet suppresses the animated UI")
}

func TestSetInteractiveAndQuietAreIndependent(t *testing.T) {
	// Arrange
	t.Cleanup(func() {
		SetInteractive(true)
		SetQuiet(false)
	})

	// Act / Assert
	SetInteractive(false)
	assert.False(t, IsQuiet(), "a non-interactive run is not the same as --quiet")
	SetQuiet(true)
	assert.True(t, IsQuiet())
}
