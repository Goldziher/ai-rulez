package mcp

// Skill metadata (description, keywords, frontmatter, file lists) comes from
// skill authors and remote sources. The session byte budget charges only file
// content, so every metadata-carrying reply is bounded here instead: a hostile
// skill cannot flood find_skill, list_skill_resources or skills/list.
const (
	// maxMetaBytes bounds one metadata string. It matches the agentskills
	// description limit.
	maxMetaBytes = 1024
	// maxMetaItems bounds the entries of a metadata list (keywords, triggers, YAML sequences).
	maxMetaItems = 64
	// maxMetaItemBytes bounds one keyword or trigger.
	maxMetaItemBytes = 128
	// maxMetaDepth bounds the nesting of frontmatter values that is kept.
	maxMetaDepth = 4
	// maxListedResources bounds the file entries one reply lists.
	maxListedResources = 200
)

// boundText cuts s to maxMetaBytes without splitting a rune.
func boundText(s string) string { return truncateUTF8(s, maxMetaBytes) }

func boundList(in []string) []string {
	out := make([]string, 0, min(len(in), maxMetaItems))
	for _, v := range in {
		if len(out) == maxMetaItems {
			break
		}
		out = append(out, truncateUTF8(v, maxMetaItemBytes))
	}
	return out
}

// boundFrontmatter returns a copy of the frontmatter with every string, list and
// map bounded.
func boundFrontmatter(front map[string]any) map[string]any {
	bounded, ok := boundValue(front, 0).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return bounded
}

func boundValue(v any, depth int) any {
	switch t := v.(type) {
	case string:
		return boundText(t)
	case []any:
		if depth >= maxMetaDepth {
			return nil
		}
		out := make([]any, 0, min(len(t), maxMetaItems))
		for _, e := range t {
			if len(out) == maxMetaItems {
				break
			}
			out = append(out, boundValue(e, depth+1))
		}
		return out
	case map[string]any:
		if depth >= maxMetaDepth {
			return nil
		}
		out := make(map[string]any, min(len(t), maxMetaItems))
		for k, e := range t {
			if len(out) == maxMetaItems {
				break
			}
			out[truncateUTF8(k, maxMetaItemBytes)] = boundValue(e, depth+1)
		}
		return out
	default:
		return v
	}
}
