package config

import (
	"strconv"
	"strings"

	"github.com/samber/oops"
)

// ConfigVersionV5 is the config format version this release reads. Older
// formats are converted by `ai-rulez migrate v5`.
const ConfigVersionV5 = "5.0"

// MigrateCommandHint is the command every legacy-config error points at.
const MigrateCommandHint = "ai-rulez migrate v5"

// versionMajor returns the major number of a config version string such as
// "4.0" or "5", and false when it is not a dotted number.
func versionMajor(v string) (int, bool) {
	v = strings.TrimSpace(v)
	major, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(major)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// IsLegacyVersion reports whether v names a config format older than v5
// ("2.0", "3.0", "4.0", "4.1", ...).
func IsLegacyVersion(v string) bool {
	n, ok := versionMajor(v)
	return ok && n > 0 && n < 5
}

// CheckVersion verifies that v is a config version this release reads. A 4.x
// version is rejected with an error that names the migration command; a 2.x or
// 3.x version is older than `migrate v5` reads and must go through ai-rulez 4.x
// first.
func CheckVersion(v string) error {
	if n, ok := versionMajor(v); ok && n > 0 && n < 4 {
		return oops.
			With("field", "version").
			With("actual_version", v).
			Hint("Install ai-rulez 4.x, let it migrate this configuration to 4.0, then run `"+MigrateCommandHint+"`").
			Errorf("config version %q is no longer supported; install ai-rulez 4.x to migrate it to 4.0, then run `%s`", v, MigrateCommandHint)
	}
	if IsLegacyVersion(v) {
		return oops.
			With("field", "version").
			With("actual_version", v).
			Hint("Run `"+MigrateCommandHint+"` to rewrite this configuration for v5 (add --dry-run to preview the change list)").
			Errorf("config version %q is no longer supported: run `%s`", v, MigrateCommandHint)
	}
	if v != ConfigVersionV5 {
		return oops.
			With("field", "version").
			With("actual_version", v).
			Hint("Set version = \""+ConfigVersionV5+"\" in config.toml").
			Errorf("invalid version: expected %q, got %q", ConfigVersionV5, v)
	}
	return nil
}

// DecodeTOML decodes config.toml bytes without validating them or touching the
// file system; path is used for error context only. `migrate v5` uses it to
// prove that a rewritten configuration still decodes.
func DecodeTOML(data []byte, path string) (*Config, error) {
	return decodeConfigTOML(data, path)
}
