package testutil

import "strconv"

// TOMLString returns s as a TOML basic string, quotes included. A Windows path
// holds backslashes that TOML reads as escapes, so a path placed in a fixture
// between bare double quotes does not parse there.
func TOMLString(s string) string { return strconv.Quote(s) }
