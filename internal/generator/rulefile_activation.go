package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"gopkg.in/yaml.v3"
)

// ruleFileDialect is the frontmatter dialect a native rules folder speaks.
type ruleFileDialect int

const (
	dialectUnknown ruleFileDialect = iota
	dialectPaths                   // claude, cline: `paths` makes a rule path-scoped
	dialectCursor
	dialectTrigger // devin, antigravity
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
	{".devin/rules/", dialectTrigger},
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
	block := frontmatter.SplitString(content)
	if !block.Closed {
		return nil, false
	}
	if err := yaml.Unmarshal([]byte(block.Raw), &fields); err != nil {
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

// activationFromMappedRuleFile classifies a rule file in the folder of a provider
// whose frontmatter comes from an activation map: the mode whose table the
// file's frontmatter matches. ok is false when the path is in no mapped folder.
func activationFromMappedRuleFile(path, content string, folders []providers.MappedRulesFolder) (config.ActivationMode, bool) {
	slashed := "/" + filepath.ToSlash(path)
	for _, folder := range folders {
		if !strings.Contains(slashed, "/"+strings.TrimSuffix(folder.Dir, "/")+"/") {
			continue
		}
		return mappedActivation(folder.Mapping, mappedFrontmatterFields(content, folder.Mapping.Format)), true
	}
	return config.ActivationAlways, false
}

// mappedFrontmatterFields reads the frontmatter of a mapped rule file in its
// declared format. A "lines" block is bare `key: value` lines that need not be
// valid YAML (a glob such as **/*.go is not), so it is split by hand.
func mappedFrontmatterFields(content, format string) map[string]any {
	if format != rulefiles.MappedFormatLines {
		fields, _ := ruleFileFrontmatter(content)
		return fields
	}
	block := frontmatter.SplitString(content)
	if !block.Closed {
		return nil
	}
	fields := map[string]any{}
	for _, line := range strings.Split(block.Raw, "\n") {
		if key, value, found := strings.Cut(line, ":"); found {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return fields
}

// mappedActivation picks the mode of the activation map the frontmatter matches
// best: every key of the mode's table must be present, and a literal value must
// equal the file's. The mode with the most keys wins, always on a tie; a file
// that matches no table is conservatively always-on.
func mappedActivation(mapping *rulefiles.ActivationMap, fields map[string]any) config.ActivationMode {
	best, bestScore := config.ActivationAlways, -1
	for _, candidate := range []struct {
		mode  config.ActivationMode
		table map[string]any
	}{
		{config.ActivationAlways, mapping.Always},
		{config.ActivationGlob, mapping.Glob},
		{config.ActivationAuto, mapping.Auto},
		{config.ActivationManual, mapping.Manual},
	} {
		if candidate.table == nil || !mappedTableMatches(candidate.table, fields) {
			continue
		}
		if len(candidate.table) > bestScore {
			best, bestScore = candidate.mode, len(candidate.table)
		}
	}
	return best
}

func mappedTableMatches(table, fields map[string]any) bool {
	for key, want := range table {
		got, present := fields[key]
		if !present {
			return false
		}
		if text, isText := want.(string); isText && strings.Contains(text, "{") {
			continue // a placeholder: present is enough
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return false
		}
	}
	return true
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
