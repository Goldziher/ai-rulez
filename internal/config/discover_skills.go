package config

import (
	"strings"

	"github.com/samber/oops"
)

// The modes of [mcp] discover_skills and of `mcp --serve-skills --discover-skills`.
const (
	// DiscoverOff serves only the skills ai-rulez defines and its skill sources.
	DiscoverOff = "off"
	// DiscoverRepo also serves every skill directory found in the working tree.
	DiscoverRepo = "repo"
	// DiscoverAll is DiscoverRepo plus the installed and included skills, whatever
	// their delivery.
	DiscoverAll = "all"
)

// ParseDiscoverSkills normalises a discover_skills value: empty is off, and the
// case and surrounding space are ignored. Any other word than off, repo or all
// is an error.
func ParseDiscoverSkills(value string) (string, error) {
	switch mode := strings.ToLower(strings.TrimSpace(value)); mode {
	case "", DiscoverOff:
		return DiscoverOff, nil
	case DiscoverRepo, DiscoverAll:
		return mode, nil
	default:
		return "", oops.
			With("field", "mcp.discover_skills").
			Hint(`Use "off" (the default), "repo" or "all"`).
			Errorf("invalid discover_skills %q", value)
	}
}
