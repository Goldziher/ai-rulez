package generator

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	frontmatterFence = "---"
	byteOrderMark    = "\xef\xbb\xbf"
)

// ruleFileDialect is the frontmatter dialect a native rules folder speaks.
type ruleFileDialect int

const (
	dialectUnknown ruleFileDialect = iota
	dialectPaths                   // claude, cline: `paths` makes a rule path-scoped
	dialectCursor
	dialectTrigger // windsurf, antigravity
	dialectCopilot
	dialectContinue
	dialectAlwaysOnly // junie: no activation fields
)

// ruleFileDialects maps a rules folder to its dialect.
var ruleFileDialects = []struct {
	dir     string
	dialect ruleFileDialect
}{
	{".claude/rules/", dialectPaths},
	{".clinerules/", dialectPaths},
	{".cursor/rules/", dialectCursor},
	{".windsurf/rules/", dialectTrigger},
	{".agents/rules/", dialectTrigger},
	{".github/instructions/", dialectCopilot},
	{".continue/rules/", dialectContinue},
	{".junie/rules/", dialectAlwaysOnly},
}

func dialectForPath(path string) ruleFileDialect {
	slashed := "/" + filepath.ToSlash(path)
	for _, entry := range ruleFileDialects {
		if strings.Contains(slashed, "/"+entry.dir) {
			return entry.dialect
		}
	}
	return dialectUnknown
}

// ruleFileFrontmatter returns the parsed frontmatter of a generated rule file.
// ok is false when the file has no parseable frontmatter block.
func ruleFileFrontmatter(content string) (fields map[string]any, ok bool) {
	rest := strings.TrimLeft(strings.TrimPrefix(content, byteOrderMark), " \t\r\n")
	if !strings.HasPrefix(rest, frontmatterFence) {
		return nil, false
	}
	rest = strings.TrimPrefix(rest, frontmatterFence)
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "\n") && !strings.HasPrefix(rest, "\r\n") {
		return nil, false
	}
	end := strings.Index(rest, "\n"+frontmatterFence)
	if end < 0 {
		return nil, false
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fields); err != nil {
		return nil, false
	}
	if fields == nil {
		fields = map[string]any{}
	}
	return fields, true
}

func fieldString(fields map[string]any, key string) string {
	value, ok := fields[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func fieldPresent(fields map[string]any, key string) bool {
	switch value := fields[key].(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(value) != ""
	case []any:
		return len(value) > 0
	}
	return true
}

func fieldTrue(fields map[string]any, key string) bool {
	value, ok := fields[key].(bool)
	return ok && value
}

// activationFromRuleFile reports how an agent activates a generated rule file,
// read from the file's frontmatter in the dialect of the rules folder the path
// is in. It is the inverse of the rulefiles frontmatter writers, kept
// independent of them so the cost report can classify any rule file, including
// ones from custom providers. Anything it cannot read counts as always-on,
// which is the conservative direction for a cost report.
func activationFromRuleFile(path, content string) config.ActivationMode {
	dialect := dialectForPath(path)
	if dialect == dialectUnknown || dialect == dialectAlwaysOnly {
		return config.ActivationAlways
	}
	fields, ok := ruleFileFrontmatter(content)
	if !ok {
		return config.ActivationAlways
	}
	switch dialect {
	case dialectPaths:
		if fieldPresent(fields, "paths") {
			return config.ActivationGlob
		}
	case dialectCursor, dialectContinue:
		return alwaysApplyActivation(fields)
	case dialectTrigger:
		return triggerActivation(fields)
	case dialectCopilot:
		return copilotActivation(fields)
	}
	return config.ActivationAlways
}

// alwaysApplyActivation reads the cursor and continue dialects. A rule with
// neither alwaysApply, globs nor a description is manual, matching Cursor.
func alwaysApplyActivation(fields map[string]any) config.ActivationMode {
	switch {
	case fieldTrue(fields, "alwaysApply"):
		return config.ActivationAlways
	case fieldPresent(fields, "globs"):
		return config.ActivationGlob
	case fieldString(fields, "description") != "":
		return config.ActivationAuto
	case fields["alwaysApply"] != nil:
		return config.ActivationManual
	}
	// Continue always writes a name; a name alone with no flags is not a
	// statement of intent, so stay on the conservative side.
	if _, named := fields["name"]; named {
		return config.ActivationAlways
	}
	return config.ActivationManual
}

func triggerActivation(fields map[string]any) config.ActivationMode {
	switch fieldString(fields, "trigger") {
	case config.TriggerGlob:
		return config.ActivationGlob
	case config.TriggerModelDecision:
		return config.ActivationAuto
	case config.TriggerManual:
		return config.ActivationManual
	}
	return config.ActivationAlways
}

func copilotActivation(fields map[string]any) config.ActivationMode {
	if applyTo := fieldString(fields, "applyTo"); applyTo != "" {
		if applyTo == "**" || applyTo == "**/*" || applyTo == "*" {
			return config.ActivationAlways
		}
		return config.ActivationGlob
	}
	if fieldString(fields, "description") != "" {
		return config.ActivationAuto
	}
	return config.ActivationManual
}

// ruleFileDescription returns the frontmatter description of a rule file, or "".
func ruleFileDescription(content string) string {
	fields, ok := ruleFileFrontmatter(content)
	if !ok {
		return ""
	}
	return fieldString(fields, "description")
}
