package config

import (
	"errors"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// skillsArrayHint answers `[[skills]]`, which is not a config key: `skills` is a
// table (dynamic skill loading), so an array of tables cannot be stored in it.
const skillsArrayHint = "`skills` is a table ([skills], dynamic skill loading), not a list of skills. " +
	"Put skills in .ai-rulez/skills/<name>/SKILL.md, install one with `ai-rulez skill install` (recorded as " +
	"[[installed_skills]]), or point at a repository of skills with [[skill_sources]]"

// describeTOMLDecodeError turns a decode error into one that names the key and
// the line. The decoder's own text ("cannot store an array table in a struct")
// names neither.
func describeTOMLDecodeError(path string, err error) error {
	var decodeErr *toml.DecodeError
	if !errors.As(err, &decodeErr) {
		return nil
	}
	row, _ := decodeErr.Position()
	key := strings.Join(decodeErr.Key(), ".")
	hint := "Check the TOML syntax - ensure proper formatting\nCommon issues: missing quotes around strings, incorrect table syntax"
	if key == "skills" && strings.Contains(decodeErr.Error(), "array table") {
		hint = skillsArrayHint
		return oops.With("path", path).With("line", row).Hint(hint).
			Errorf("parse TOML config: [[skills]] at line %d is not a config key. %s", row, skillsArrayHint)
	}
	b := oops.With("path", path).With("line", row).Hint(hint)
	if key == "" {
		return b.Wrapf(err, "parse TOML config at line %d", row)
	}
	return b.Wrapf(err, "parse TOML config: key %q at line %d", key, row)
}
