package gitutil

import (
	"fmt"
	"strings"
)

// CheckArg rejects a value that would be read as a git option or that cannot be
// a URL, ref or path at all. A repository URL or ref that comes from a
// configuration file or a command line is data; one that starts with '-' (for
// example `--upload-pack=<command>`) would make git run a program, so it is
// refused before it reaches any git command line. Control characters (NUL,
// newlines) are refused because they split arguments and config lines.
func CheckArg(what, value string) error {
	if strings.HasPrefix(strings.TrimSpace(value), "-") {
		return fmt.Errorf("%s %q starts with '-' and would be read as a git option", what, value)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s %q contains a control character", what, value)
		}
	}
	return nil
}
