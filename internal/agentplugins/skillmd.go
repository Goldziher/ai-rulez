package agentplugins

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Agent Skills frontmatter limits (https://agentskills.io/specification).
const (
	maxDescriptionLen   = 1024
	maxCompatibilityLen = 500
)

// skillFields are the frontmatter fields the Agent Skills specification defines.
var skillFields = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}

// ValidSkillName reports whether name satisfies the Agent Skills name rules:
// 1-64 lowercase letters, digits and hyphens, no leading, trailing or
// consecutive hyphens.
func ValidSkillName(name string) bool {
	if name == "" || len(name) > maxNameLen || strings.Contains(name, "--") {
		return false
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isLowerAlnum(name[i]) && name[i] != '-' {
			return false
		}
	}
	return true
}

// skillIssue is one Agent Skills violation; fatal ones make clients skip the skill.
type skillIssue struct {
	fatal bool
	msg   string
}

// checkSkill checks a skill directory name and its SKILL.md against the Agent
// Skills specification.
func checkSkill(dir string, skillMD []byte) []skillIssue {
	if !ValidSkillName(dir) {
		return []skillIssue{{true, fmt.Sprintf("skill directory name %q is not a valid Agent Skills name "+
			"(1-64 lowercase letters, digits and single hyphens)", dir)}}
	}
	fm, err := frontmatter(skillMD)
	if err != nil {
		return []skillIssue{{true, err.Error()}}
	}
	issues := checkSkillFields(dir, fm)
	for _, key := range sortedKeys(fm) {
		if !slices.Contains(skillFields, key) {
			issues = append(issues, skillIssue{false, fmt.Sprintf(
				"frontmatter field %q is not defined by Agent Skills; other clients may ignore it", key)})
		}
	}
	return issues
}

// checkSkillFields checks the frontmatter fields Agent Skills defines.
func checkSkillFields(dir string, fm map[string]any) []skillIssue {
	var issues []skillIssue
	fatal := func(format string, a ...any) { issues = append(issues, skillIssue{true, fmt.Sprintf(format, a...)}) }

	if name, ok := fm["name"].(string); !ok || name == "" {
		fatal("frontmatter name is required")
	} else if name != dir {
		fatal("frontmatter name %q must match the skill directory %q", name, dir)
	}
	switch desc, ok := fm["description"].(string); {
	case !ok || strings.TrimSpace(desc) == "":
		fatal("frontmatter description is required")
	case utf8.RuneCountInString(desc) > maxDescriptionLen:
		fatal("frontmatter description exceeds %d characters", maxDescriptionLen)
	}
	if v, ok := fm["compatibility"]; ok {
		if s, isStr := v.(string); !isStr || utf8.RuneCountInString(s) > maxCompatibilityLen {
			fatal("frontmatter compatibility must be a string of at most %d characters", maxCompatibilityLen)
		}
	}
	for _, key := range []string{"license", "allowed-tools"} {
		if v, ok := fm[key]; ok {
			if _, isStr := v.(string); !isStr {
				fatal("frontmatter %s must be a string", key)
			}
		}
	}
	if v, ok := fm["metadata"]; ok && !stringMap(v) {
		fatal("frontmatter metadata must map strings to strings")
	}
	return issues
}

// frontmatter parses the YAML block between the leading "---" lines.
func frontmatter(doc []byte) (map[string]any, error) {
	doc = bytes.ReplaceAll(doc, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(doc, []byte("---\n"))
	if !ok {
		return nil, fmt.Errorf("SKILL.md must start with a YAML frontmatter block")
	}
	var block []byte
	if !bytes.HasPrefix(rest, []byte("---\n")) && !bytes.Equal(rest, []byte("---")) {
		end := bytes.Index(rest, []byte("\n---\n"))
		switch {
		case end >= 0:
		case bytes.HasSuffix(rest, []byte("\n---")):
			end = len(rest) - len("\n---")
		default:
			return nil, fmt.Errorf("SKILL.md frontmatter is not terminated by ---")
		}
		block = rest[:end]
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return nil, fmt.Errorf("SKILL.md frontmatter is not valid YAML: %w", err)
	}
	return fm, nil
}

func stringMap(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for _, val := range m {
		if _, isStr := val.(string); !isStr {
			return false
		}
	}
	return true
}
