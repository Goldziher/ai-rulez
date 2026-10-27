package presets

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/markdown"
)

// Limits of a generated skill description; harnesses budget the listing of
// every skill, so a grouped skill, which stands for several items, stays shorter.
const (
	scopedSkillDescriptionLimit = 250
	scopedGroupDescriptionLimit = 160
	scopedGroupPrefix           = "scoped-"
	scopedRootGroup             = "root"
	builtinOriginPrefix         = "builtin-"
	globWildcards               = "*?[{"
)

// scopedItem is one glob-scoped rule or context item that scoped_fallback serves
// as a skill: the positive globs it applies to and the skill id it has alone.
type scopedItem struct {
	file   config.ContentFile
	prefix string
	id     string
	title  string
	globs  []string
}

// ScopedFallbackSkills is what rules.scoped_fallback = "skill" puts in the shared
// .agents/skills tree in place of the glob-scoped rules and context the shared
// AGENTS.md no longer inlines: one skill per item, named rule-<id> or
// context-<id>, carrying the item's positive globs as paths (Claude Code and
// Cursor load such a skill only for matching files; other readers list it and
// load it on demand). With rules.scoped_fallback_group set, the items that share
// a group (an include, or a directory their globs point into) become one
// scoped-<group> skill whose paths are the union of their globs; a group of one
// item keeps its own skill. The items are exactly the ones SharedAgentsMD leaves
// out: selected for AGENTS.md by their targets, glob-activated with at least one
// positive glob, not opted out with fallback: false, and not moved into a nested
// AGENTS.md by the baz preset.
func ScopedFallbackSkills(content *config.ContentTree, cfg *config.Config, owners []string) []config.ContentFile {
	items := scopedFallbackItems(content, cfg, owners)
	grouping := cfg.ScopedFallbackGrouping()
	type group struct {
		id    string
		label string
		items []*scopedItem
	}
	var order []*group
	byID := map[string]*group{}
	for idx := range items {
		item := &items[idx]
		id, label := scopedGroupOf(item, grouping)
		if id == "" {
			id, label = item.id, item.title
		}
		grp, ok := byID[id]
		if !ok {
			grp = &group{id: id, label: label}
			byID[id] = grp
			order = append(order, grp)
		}
		grp.items = append(grp.items, item)
	}
	skills := make([]config.ContentFile, 0, len(order))
	for _, grp := range order {
		if len(grp.items) == 1 {
			skills = append(skills, scopedItemSkill(grp.items[0]))
			continue
		}
		skills = append(skills, scopedGroupSkill(grp.id, grp.label, grp.items))
	}
	return skills
}

// ScopedFallbackItemCount is the number of glob-scoped items scoped_fallback
// would serve, before they are grouped.
func ScopedFallbackItemCount(content *config.ContentTree, cfg *config.Config, owners []string) int {
	return len(scopedFallbackItems(content, cfg, owners))
}

func scopedFallbackItems(content *config.ContentTree, cfg *config.Config, owners []string) []scopedItem {
	if cfg.ScopedFallbackGrouping() == config.ScopedFallbackGroupInclude {
		content = withBuiltinOrigins(content)
	}
	root := rulefiles.RootTarget(codexPresetName, agentsMDFile)
	root.Owners, root.RootAliases = sharedAgentsMDOwners(owners)
	var items []scopedItem
	for _, kind := range []struct {
		prefix string
		files  []config.ContentFile
	}{
		{"rule-", withoutBazNested(rulefiles.FilterInline(allInlineRules(content), root), cfg)},
		{"context-", withoutBazNested(rulefiles.FilterInline(allInlineContext(content), root), cfg)},
	} {
		for idx := range kind.files {
			if item, ok := newScopedItem(kind.files[idx], kind.prefix); ok {
				items = append(items, item)
			}
		}
	}
	return items
}

// withBuiltinOrigins returns the tree with the rules and context of each builtin
// domain tagged with the pack they came from, for grouping by include.
func withBuiltinOrigins(content *config.ContentTree) *config.ContentTree {
	tagged := *content
	tagged.Domains = make(map[string]*config.Domain, len(content.Domains))
	for name, domain := range content.Domains {
		if !domain.Builtin {
			tagged.Domains[name] = domain
			continue
		}
		copied := *domain
		copied.Rules = taggedWithInclude(domain.Rules, builtinOriginPrefix+name)
		copied.Context = taggedWithInclude(domain.Context, builtinOriginPrefix+name)
		tagged.Domains[name] = &copied
	}
	return &tagged
}

func taggedWithInclude(files []config.ContentFile, origin string) []config.ContentFile {
	out := make([]config.ContentFile, len(files))
	copy(out, files)
	for idx := range out {
		if out[idx].Include == "" {
			out[idx].Include = origin
		}
	}
	return out
}

// newScopedItem wraps a glob-scoped item; false when it is not glob-scoped by at
// least one positive glob or opts out of the fallback with fallback: false.
func newScopedItem(cf config.ContentFile, prefix string) (scopedItem, bool) {
	globs, ok := scopedFallbackGlobs(cf)
	if !ok {
		return scopedItem{}, false
	}
	title := cf.Name
	if cf.Metadata != nil && cf.Metadata.OKFTitle != "" {
		title = cf.Metadata.OKFTitle
	}
	return scopedItem{file: cf, prefix: prefix, id: prefix + rulefiles.ItemID(cf.Name), title: title, globs: globs}, true
}

// scopedFallbackGlobs returns the positive globs of a glob-scoped item that
// takes part in the fallback.
func scopedFallbackGlobs(cf config.ContentFile) ([]string, bool) {
	act := cf.Metadata.ResolveActivation()
	if act.Mode != config.ActivationGlob || rulefiles.EffectiveModeOf(cf) != config.ActivationGlob {
		return nil, false
	}
	if cf.Metadata.FallbackOptOut() {
		return nil, false
	}
	var globs []string
	for _, g := range act.Globs {
		if !strings.HasPrefix(g, "!") {
			globs = append(globs, g)
		}
	}
	return globs, len(globs) > 0
}

// scopedFallbackOptedOut reports whether the item is glob-scoped by a positive
// glob and keeps itself out of the fallback with fallback: false.
func scopedFallbackOptedOut(cf config.ContentFile) bool {
	if !cf.Metadata.FallbackOptOut() {
		return false
	}
	act := cf.Metadata.ResolveActivation()
	if act.Mode != config.ActivationGlob || rulefiles.EffectiveModeOf(cf) != config.ActivationGlob {
		return false
	}
	for _, g := range act.Globs {
		if !strings.HasPrefix(g, "!") {
			return true
		}
	}
	return false
}

// scopedGroupOf names the group an item belongs to under the grouping mode: the
// skill id and a label for its heading, or "" when the item stays on its own.
func scopedGroupOf(item *scopedItem, grouping string) (id, label string) {
	switch grouping {
	case config.ScopedFallbackGroupInclude:
		if item.file.Include != "" {
			return scopedGroupPrefix + rulefiles.ID(item.file.Include), item.file.Include
		}
	case config.ScopedFallbackGroupDirectory:
		dir := commonGlobDir(item.globs)
		if dir == "" {
			return scopedGroupPrefix + scopedRootGroup, "repository root"
		}
		return scopedGroupPrefix + rulefiles.ID(dir), dir
	}
	return "", ""
}

// commonGlobDir is the longest directory every glob starts below: the segments
// before the first wildcard, without the file name of a glob that has none.
func commonGlobDir(globs []string) string {
	var common []string
	for i, glob := range globs {
		dir := globDir(glob)
		if i == 0 {
			common = dir
			continue
		}
		n := 0
		for n < len(common) && n < len(dir) && common[n] == dir[n] {
			n++
		}
		common = common[:n]
	}
	return strings.Join(common, "/")
}

func globDir(glob string) []string {
	segments := strings.Split(strings.Trim(strings.TrimPrefix(glob, "./"), "/"), "/")
	var dir []string
	for idx, segment := range segments {
		if strings.ContainsAny(segment, globWildcards) || idx == len(segments)-1 {
			break
		}
		dir = append(dir, segment)
	}
	return dir
}

// scopedItemSkill renders one glob-scoped item as a skill.
func scopedItemSkill(item *scopedItem) config.ContentFile {
	description := strings.Join(strings.Fields(item.file.Metadata.ResolveActivation().Description), " ")
	if description == "" {
		description = item.title + ": conventions for files matching " + strings.Join(item.globs, ", ") +
			". Use when reading or editing those files."
	}
	body := "# " + item.title + "\n\n" + appliesTo(item.globs) + "\n\n" + scopedBody(item) + "\n"
	return scopedSkill(item.id, item.file.Path, body, item.globs, truncateDescription(description, scopedSkillDescriptionLimit))
}

// scopedGroupSkill renders several glob-scoped items as one skill: each under a
// heading with its globs, the skill's paths the union of theirs.
func scopedGroupSkill(id, label string, items []*scopedItem) config.ContentFile {
	var paths, titles []string
	seen := map[string]bool{}
	var body strings.Builder
	body.WriteString("# Scoped conventions: " + label + "\n\n")
	for _, item := range items {
		titles = append(titles, item.title)
		for _, g := range item.globs {
			if !seen[g] {
				seen[g] = true
				paths = append(paths, g)
			}
		}
		body.WriteString("## " + item.title + "\n\n" + appliesTo(item.globs) + "\n\n" + scopedBody(item) + "\n\n")
	}
	description := fmt.Sprintf("%d scoped conventions (%s). Use when editing the files they cover.", len(items), strings.Join(titles, ", "))
	return scopedSkill(id, items[0].file.Path, strings.TrimRight(body.String(), "\n")+"\n", paths,
		truncateDescription(description, scopedGroupDescriptionLimit))
}

func scopedSkill(id, path, body string, paths []string, description string) config.ContentFile {
	return config.ContentFile{
		Name:     id,
		ID:       id,
		Path:     path,
		Content:  body,
		Metadata: &config.Metadata{Paths: paths, Extra: map[string]string{"description": description}},
	}
}

func appliesTo(globs []string) string {
	spans := make([]string, len(globs))
	for i, g := range globs {
		spans[i] = "`" + g + "`"
	}
	return "_Applies to: " + strings.Join(spans, ", ") + "_"
}

func scopedBody(item *scopedItem) string {
	return strings.TrimSpace(markdown.ProcessEmbeddedContent(item.file.Content))
}

func truncateDescription(description string, limit int) string {
	if len(description) <= limit {
		return description
	}
	cut := limit - 3
	for cut > 0 && !utf8.RuneStart(description[cut]) {
		cut--
	}
	return strings.TrimSpace(description[:cut]) + "..."
}

// ScopedFallbackSkillsUnclaimed is ScopedFallbackSkills minus the skills whose id
// an authored skill, a command or an earlier generated skill already uses: the
// authored item keeps its directory. onClaimed, when not nil, is called for each
// skill left out.
func ScopedFallbackSkillsUnclaimed(
	content *config.ContentTree, cfg *config.Config, owners []string, onClaimed func(skill *config.ContentFile),
) []config.ContentFile {
	taken := map[string]bool{}
	for _, items := range [][]config.ContentFile{allSkills(content), allCommands(content)} {
		for idx := range items {
			taken[config.SkillID(items[idx])] = true
		}
	}
	var kept []config.ContentFile
	generated := ScopedFallbackSkills(content, cfg, owners)
	for idx := range generated {
		skill := &generated[idx]
		if taken[skill.ID] {
			if onClaimed != nil {
				onClaimed(skill)
			}
			continue
		}
		taken[skill.ID] = true
		kept = append(kept, *skill)
	}
	return kept
}
