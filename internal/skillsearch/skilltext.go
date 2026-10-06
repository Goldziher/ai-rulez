package skillsearch

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// SplitSkill separates a SKILL.md into its frontmatter and body. A file without
// valid frontmatter has an empty map and the whole text as body.
func SplitSkill(content []byte) (front map[string]any, body string) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	front = map[string]any{}
	if !strings.HasPrefix(text, "---\n") {
		return front, text
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return front, text
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &front); err != nil || front == nil {
		return map[string]any{}, text
	}
	body = rest[end+len("\n---"):]
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	}
	return front, strings.TrimSpace(body)
}

// ItemFromSkill builds the searchable item of a SKILL.md the way the served
// catalog does: the frontmatter name (else id), the description (else the name),
// triggers and keywords as a list or one comma-separated string.
func ItemFromSkill(id, domain string, content []byte) Item {
	front, body := SplitSkill(content)
	name, _ := front["name"].(string) //nolint:errcheck // a missing key is the empty string
	if strings.TrimSpace(name) == "" {
		name = id
	}
	desc, _ := front["description"].(string) //nolint:errcheck // a missing key is the empty string
	if strings.TrimSpace(desc) == "" {
		desc = name
	}
	return Item{
		ID: name, Domain: domain, Body: body,
		Doc: Doc{Name: name, Description: desc, Triggers: listValue(front["triggers"]), Keywords: listValue(front["keywords"])},
	}
}

func listValue(v any) []string {
	var out []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, p := range strings.Split(x, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
