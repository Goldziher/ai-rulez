package crud

import (
	"strings"

	"github.com/samber/oops"
)

// isWindowsDeviceName reports whether name is a reserved Windows device name:
// CON, PRN, AUX, NUL, COM1-9 and LPT1-9, in any case, with or without an
// extension ("con.txt" is the console too).
func isWindowsDeviceName(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) {
		return stem[3] >= '1' && stem[3] <= '9'
	}
	return false
}

// checkNotDeviceName rejects a Windows device name for the named field.
func checkNotDeviceName(field, name string) error {
	if !isWindowsDeviceName(name) {
		return nil
	}
	return oops.
		With("field", field).
		With("value", name).
		Hint("CON, PRN, AUX, NUL, COM1-9 and LPT1-9 are Windows device names and cannot be files there. Choose a different name.").
		Errorf("name is reserved on Windows: %s", name)
}
