package config

import (
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
)

// ValidateExcludeGlobs checks the globs of [mcp] exclude and of `mcp --exclude`:
// each is non-empty, stays inside the project and compiles.
func ValidateExcludeGlobs(globs []string) error {
	for i, g := range globs {
		trimmed := strings.TrimSpace(g)
		if trimmed == "" {
			return oops.
				Hint("Remove the empty entry, or give a glob such as \"vendor/**\"").
				Errorf("mcp.exclude entry #%d is empty", i)
		}
		for _, seg := range strings.Split(strings.ReplaceAll(trimmed, "\\", "/"), "/") {
			if seg == ".." {
				return oops.
					Hint("Globs are relative to the project root and cannot leave it").
					Errorf("mcp.exclude entry #%d %q climbs out of the project", i, g)
			}
		}
		if _, err := vspec.CompileGlob(trimmed); err != nil {
			return oops.Wrapf(err, "invalid mcp.exclude glob #%d %q", i, g)
		}
	}
	return nil
}
