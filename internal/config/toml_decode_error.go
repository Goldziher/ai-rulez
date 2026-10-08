package config

import (
	"errors"
	"fmt"
	"regexp"
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
	if wrong, want, ok := tomlTypeMismatch(decodeErr.Error()); ok && key != "" {
		return oops.With("path", path).With("line", row).
			Hint(fmt.Sprintf("Set %s to %s, for example %s", key, want.article, want.example)).
			Errorf("parse TOML config: %s at line %d must be %s, not %s", key, row, want.article, wrong)
	}
	b := oops.With("path", path).With("line", row).Hint(hint)
	if key == "" {
		return b.Wrapf(err, "parse TOML config at line %d", row)
	}
	return b.Wrapf(err, "parse TOML config: key %q at line %d", key, row)
}

// tomlMismatch describes the type a key needs.
type tomlMismatch struct{ article, example string }

var tomlTypeMismatchRe = regexp.MustCompile(`cannot decode TOML ([a-z ]+?) into (?:a )?struct field [\w./*-]+ of type (\S+)`)

// tomlTypeMismatch reads the decoder's "cannot decode TOML array into struct
// field config.RoleConfig.Extends of type string" and returns the TOML type
// that was found and what the key needs, without the Go type names.
func tomlTypeMismatch(text string) (found string, want tomlMismatch, ok bool) {
	m := tomlTypeMismatchRe.FindStringSubmatch(text)
	if m == nil {
		return "", tomlMismatch{}, false
	}
	switch goType := m[2]; {
	case goType == "string":
		want = tomlMismatch{"a string", `"value"`}
	case goType == "bool":
		want = tomlMismatch{"true or false", boolTrue}
	case strings.HasPrefix(goType, "[]"):
		want = tomlMismatch{"a list", `["a", "b"]`}
	case strings.HasPrefix(goType, "int"), strings.HasPrefix(goType, "uint"), strings.HasPrefix(goType, "float"):
		want = tomlMismatch{"a number", "1"}
	case strings.HasPrefix(goType, "map["):
		want = tomlMismatch{"a table", "[table]"}
	default:
		return "", tomlMismatch{}, false
	}
	found = "a " + m[1]
	if m[1] == "array" {
		found = "a list"
	}
	return found, want, true
}
