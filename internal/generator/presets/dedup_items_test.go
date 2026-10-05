package presets

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skillFile builds a directory-form skill whose Name matches its output id, the
// shape scanSkills and the builtin loader both produce.
func skillFile(id, source, body string) config.ContentFile {
	return config.ContentFile{
		Name:    id,
		Path:    source + "/skills/" + id + "/SKILL.md",
		Content: body,
	}
}

// commandFile builds a flat-form command, the shape scanCommands produces for
// commands/<name>.md.
func commandFile(id, source, body string) config.ContentFile {
	return config.ContentFile{
		Name:    id,
		Path:    source + "/commands/" + id + ".md",
		Content: body,
	}
}

// collidingItemTree models one skill id and one command id claimed by all four
// content sources at once. Fixture domains carry the Builtin / FromInclude flags
// directly so the test does not depend on any builtin pack's current contents.
func collidingItemTree() *config.ContentTree {
	return &config.ContentTree{
		Skills:   []config.ContentFile{skillFile("shared", "root", "ROOT")},
		Commands: []config.ContentFile{commandFile("shared-cmd", "root", "ROOT")},
		Domains: map[string]*config.Domain{
			"ondisk": {
				Name:     "ondisk",
				Skills:   []config.ContentFile{skillFile("shared", "ondisk", "ONDISK"), skillFile("ondisk-only", "ondisk", "x")},
				Commands: []config.ContentFile{commandFile("shared-cmd", "ondisk", "ONDISK")},
			},
			"included": {
				Name:        "included",
				FromInclude: true,
				Skills:      []config.ContentFile{skillFile("shared", "include", "INCLUDE"), skillFile("include-only", "include", "x")},
				Commands:    []config.ContentFile{commandFile("shared-cmd", "include", "INCLUDE")},
			},
			"testing": {
				Name:     "testing",
				Builtin:  true,
				Skills:   []config.ContentFile{skillFile("shared", "builtin", "BUILTIN"), skillFile("builtin-only", "builtin", "x")},
				Commands: []config.ContentFile{commandFile("shared-cmd", "builtin", "BUILTIN")},
			},
		},
	}
}

// contentByName returns the Content of the single file with the given Name,
// failing when it is absent or duplicated — a duplicate is the defect itself.
func contentByName(t *testing.T, files []config.ContentFile, name string) string {
	t.Helper()
	var found []config.ContentFile
	for _, f := range files {
		if f.Name == name {
			found = append(found, f)
		}
	}
	require.Len(t, found, 1, "expected exactly one %q entry, got %d", name, len(found))
	return found[0].Content
}

func TestAllSkills_sourcePrecedence(t *testing.T) {
	t.Parallel()

	got := allSkills(collidingItemTree())

	// Every id appears exactly once: a second copy would be a second write to
	// the same .claude/skills/{id}/SKILL.md path.
	assert.Equal(t, []string{"builtin-only", "include-only", "ondisk-only", "shared"}, ruleNames(got))
	assert.Equal(t, "ROOT", contentByName(t, got, "shared"), "root skill wins over domain, include and builtin")
}

func TestAllSkills_domainBeatsInclude(t *testing.T) {
	t.Parallel()

	tree := collidingItemTree()
	tree.Skills = nil

	got := allSkills(tree)
	assert.Equal(t, "ONDISK", contentByName(t, got, "shared"), "on-disk domain beats include")
}

func TestAllSkills_includeBeatsBuiltin(t *testing.T) {
	t.Parallel()

	tree := collidingItemTree()
	tree.Skills = nil
	delete(tree.Domains, "ondisk")

	got := allSkills(tree)
	assert.Equal(t, "INCLUDE", contentByName(t, got, "shared"), "include beats builtin")
}

func TestAllSkills_domainBeatsBuiltin(t *testing.T) {
	t.Parallel()

	tree := collidingItemTree()
	tree.Skills = nil
	delete(tree.Domains, "included")

	got := allSkills(tree)
	assert.Equal(t, "ONDISK", contentByName(t, got, "shared"), "on-disk domain beats builtin")
}

func TestAllSkills_noCollisionKeepsEveryItem(t *testing.T) {
	t.Parallel()

	tree := &config.ContentTree{
		Skills: []config.ContentFile{skillFile("a", "root", "A")},
		Domains: map[string]*config.Domain{
			"d1":      {Name: "d1", Skills: []config.ContentFile{skillFile("b", "ondisk", "B")}},
			"testing": {Name: "testing", Builtin: true, Skills: []config.ContentFile{skillFile("c", "builtin", "C")}},
		},
	}

	assert.Equal(t, []string{"a", "b", "c"}, ruleNames(allSkills(tree)))
}

func TestAllCommands_sourcePrecedence(t *testing.T) {
	t.Parallel()

	got := allCommands(collidingItemTree())

	require.Len(t, got, 1, "the four copies collapse to one output")
	assert.Equal(t, "ROOT", contentByName(t, got, "shared-cmd"), "root command wins over domain, include and builtin")
}

func TestDuplicateContentWarnings_namesWinnerAndLoserPerKind(t *testing.T) {
	t.Parallel()

	warnings := duplicateContentWarnings(collidingItemTree())

	byKind := make(map[string]duplicateContentWarning, len(warnings))
	for _, w := range warnings {
		byKind[w.Kind] = w
	}

	skill, ok := byKind["skill"]
	require.True(t, ok, "a dropped skill must be reported")
	assert.Equal(t, "shared", skill.Duplicate.Name)
	assert.Equal(t, "root/skills/shared/SKILL.md", skill.Duplicate.Winner)
	assert.Equal(t, []string{
		"ondisk/skills/shared/SKILL.md",
		"include/skills/shared/SKILL.md",
		"builtin/skills/shared/SKILL.md",
	}, skill.Duplicate.Losers, "losers are listed in descending precedence")

	command, ok := byKind["command"]
	require.True(t, ok, "a dropped command must be reported")
	assert.Equal(t, "shared-cmd", command.Duplicate.Name)
	assert.Equal(t, "root/commands/shared-cmd.md", command.Duplicate.Winner)
	assert.Len(t, command.Duplicate.Losers, 3)
}

func TestDuplicateContentWarnings_silentWithoutCollisions(t *testing.T) {
	t.Parallel()

	tree := &config.ContentTree{
		Rules:    []config.ContentFile{{Name: "r", Path: "root/rules/r.md"}},
		Context:  []config.ContentFile{{Name: "c", Path: "root/context/c.md"}},
		Skills:   []config.ContentFile{skillFile("a", "root", "A")},
		Commands: []config.ContentFile{commandFile("x", "root", "X")},
		Domains: map[string]*config.Domain{
			"testing": {
				Name:     "testing",
				Builtin:  true,
				Rules:    []config.ContentFile{{Name: "r2", Path: "builtin/rules/r2.md"}},
				Skills:   []config.ContentFile{skillFile("b", "builtin", "B")},
				Commands: []config.ContentFile{commandFile("y", "builtin", "Y")},
			},
		},
	}

	assert.Empty(t, duplicateContentWarnings(tree), "distinct names must not warn")
}

func TestDuplicateContentWarnings_nilTree(t *testing.T) {
	t.Parallel()

	assert.Nil(t, duplicateContentWarnings(nil))
}

// TestAllSkills_sameSourceListedTwiceCollapses covers an include merge that
// carries one file into both the root and a domain slice: the two entries are the
// same source, so exactly one survives and generation stays idempotent.
func TestAllSkills_sameSourceListedTwiceCollapses(t *testing.T) {
	t.Parallel()

	shared := skillFile("shared", "include", "INCLUDE")
	tree := &config.ContentTree{
		Skills: []config.ContentFile{shared},
		Domains: map[string]*config.Domain{
			"included": {Name: "included", FromInclude: true, Skills: []config.ContentFile{shared}},
		},
	}

	got := allSkills(tree)
	require.Len(t, got, 1)
	assert.Equal(t, "INCLUDE", got[0].Content)
}

func TestCommandAsSkills_WarnsWhenACommandIDCollides(t *testing.T) {
	// Arrange
	var warnings []string
	t.Cleanup(rulefiles.SetWarnSink(func(msg string, _ ...any) { warnings = append(warnings, msg) }))
	content := &config.ContentTree{
		Skills:   []config.ContentFile{{Name: "ship", Path: "skills/ship/SKILL.md", Content: "skill"}},
		Commands: []config.ContentFile{{Name: "ship", Path: "commands/ship.md", Content: "cmd"}, {Name: "my-cmd", Path: "commands/a.md", Content: "d1"}, {Name: "my_cmd", Path: "commands/b.md", Content: "d2"}},
	}

	// Act
	skills := commandAsSkills(content, "codex")

	// Assert
	require.Len(t, skills, 1)
	assert.Equal(t, "d1", skills[0].Content)
	joined := strings.Join(warnings, "\n")
	assert.Contains(t, joined, `command "ship" is not written as a codex skill`)
	assert.Contains(t, joined, `command "my_cmd" is not written as a codex skill`)
}
