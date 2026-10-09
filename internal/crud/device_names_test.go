package crud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A name that is a Windows device (CON, NUL, COM1, ...) cannot be a file on
// Windows, with or without an extension, in any case. It is refused everywhere so
// a repository written on one platform checks out on the others.
func TestNamesRejectWindowsDeviceNames(t *testing.T) {
	for _, name := range []string{"CON", "con", "Prn", "AUX", "nul", "COM1", "com9", "LPT1", "lpt9", "con.txt", "NUL.v2"} {
		assert.Error(t, ValidateFileName(name), name)
		assert.Error(t, ValidateNewFileName(name), name)
		assert.Error(t, ValidateDomainName(name), name)
	}
}

func TestNamesAcceptLookAlikesOfDeviceNames(t *testing.T) {
	for _, name := range []string{"console", "com", "com10", "com0", "lpt", "auxiliary", "nullable", "my-con", "con-x"} {
		assert.NoError(t, ValidateFileName(name), name)
	}
}
