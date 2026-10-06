// Package okfbridge maps ai-rulez content to and from OKF bundles
// (internal/okf). See docs/okf.md for the mapping table.
package okfbridge

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind is an ai-rulez content kind.
type Kind string

// Content kinds. The string values are the directory names under .ai-rulez/.
const (
	KindRule      Kind = "rules"
	KindContext   Kind = "context"
	KindSkill     Kind = "skills"
	KindAgent     Kind = "agents"
	KindCommand   Kind = "commands"
	KindCheck     Kind = "checks"
	kindResource  Kind = "skill-resource"
	dirDomains         = "domains"
	fileSkill          = "SKILL.md"
	fileCommand        = "COMMAND.md"
	maxSanitizeID      = 128
)

// Frontmatter keys and YAML tags the bridge writes and reads.
const (
	keyType        = "type"
	keyTitle       = "title"
	keyDescription = "description"
	keyKind        = "kind"
	keyDomain      = "domain"
	tagStr         = "!!str"
)

// AllKinds lists the kinds in export order.
var AllKinds = []Kind{KindRule, KindContext, KindSkill, KindAgent, KindCommand, KindCheck}

// ParseKinds parses a comma-separated include list; an empty list means all kinds.
func ParseKinds(list []string) ([]Kind, error) {
	if len(list) == 0 {
		return AllKinds, nil
	}
	seen := map[Kind]bool{}
	for _, raw := range list {
		for _, part := range strings.Split(raw, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			if part == "" {
				continue
			}
			k, ok := kindFromName(part)
			if !ok {
				return nil, &UnknownKindError{Name: part}
			}
			seen[k] = true
		}
	}
	var out []Kind
	for _, k := range AllKinds {
		if seen[k] {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return AllKinds, nil
	}
	return out, nil
}

// UnknownKindError reports an include entry that names no kind.
type UnknownKindError struct{ Name string }

func (e *UnknownKindError) Error() string {
	return "unknown content kind " + `"` + e.Name + `"` + " (use rules, context, skills, agents, commands or checks)"
}

func kindFromName(name string) (Kind, bool) {
	switch strings.TrimSuffix(name, "s") {
	case "rule":
		return KindRule, true
	case "context":
		return KindContext, true
	case "skill":
		return KindSkill, true
	case "agent":
		return KindAgent, true
	case "command":
		return KindCommand, true
	case "check":
		return KindCheck, true
	}
	return "", false
}

// defaultType is the OKF type written for a kind (docs/okf.md, Mapping).
func defaultType(k Kind) string {
	switch k {
	case KindRule:
		return "Decision"
	case KindContext:
		return "Concept"
	case KindSkill:
		return "Playbook"
	default:
		return "Reference"
	}
}

func kindLabel(k Kind) string {
	switch k {
	case KindRule:
		return "Rules"
	case KindContext:
		return "Context"
	case KindSkill:
		return "Skills"
	case KindAgent:
		return "Agents"
	case KindCommand:
		return "Commands"
	case KindCheck:
		return "Checks"
	}
	return string(k)
}

var typeToKind = map[string]Kind{
	"decision": KindRule, "rule": KindRule, "convention": KindRule, "policy": KindRule,
	"guideline": KindRule, "standard": KindRule,
	"howto": KindSkill, "playbook": KindSkill, "runbook": KindSkill, "procedure": KindSkill, "skill": KindSkill,
}

// kindForType maps an OKF type to a kind; anything unknown is context.
func kindForType(t string) Kind {
	key := strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(t))
	if k, ok := typeToKind[key]; ok {
		return k
	}
	return KindContext
}

var (
	idOK      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	idUnsafe  = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	resourceK = map[string]bool{"references": true, "scripts": true, "assets": true}
)

// ValidID reports whether s is safe to use as a file or directory name.
func ValidID(s string) bool {
	return len(s) <= maxSanitizeID && idOK.MatchString(s) && !strings.Contains(s, "..")
}

// validImportID reports whether the x-ai-rulez id of a bundle can be the file
// stem of an imported source: any non-empty UTF-8 name without a path
// separator, control character, leading dot, ".." or character that is
// reserved on a common file system. Unlike ValidID it keeps non-ASCII names,
// so an export and an import give the original file back.
func validImportID(s string) bool {
	if s == "" || len(s) > maxSanitizeID || !utf8.ValidString(s) || strings.Contains(s, "..") {
		return false
	}
	if s != strings.TrimSpace(s) || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) || r == utf8.RuneError {
			return false
		}
	}
	return true
}

// sanitizeID turns arbitrary text into a safe single path segment.
func sanitizeID(s string) string {
	s = strings.Trim(idUnsafe.ReplaceAllString(s, "-"), "-.")
	s = strings.ReplaceAll(s, "..", "-")
	if len(s) > maxSanitizeID {
		s = s[:maxSanitizeID]
	}
	if s == "" {
		return "item"
	}
	return s
}

func sortedStrings(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
