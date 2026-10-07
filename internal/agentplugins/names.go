package agentplugins

import "strings"

const maxNameLen = 64

func isLowerAlnum(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') }

// ValidPluginName reports whether name satisfies the manifest name rules
// (§5.5): 1-64 characters from [a-z0-9.-], alphanumeric first and last
// characters, and no "--" or "..".
func ValidPluginName(name string) bool {
	if name == "" || len(name) > maxNameLen {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !isLowerAlnum(c) && c != '-' && c != '.' {
			return false
		}
	}
	return isLowerAlnum(name[0]) && isLowerAlnum(name[len(name)-1]) &&
		!strings.Contains(name, "--") && !strings.Contains(name, "..")
}
