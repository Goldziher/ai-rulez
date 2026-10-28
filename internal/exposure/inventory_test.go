package exposure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// writeRepo writes files below a new directory and returns it.
func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		writeFile(t, root, rel, content)
	}
	return root
}

// listing lists every file and symlink below root, as git would list them.
func listing(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		files = append(files, filepath.ToSlash(rel))
		return relErr
	}))
	sort.Strings(files)
	return files
}

func build(t *testing.T, root string, opts ...func(*Options)) *Inventory {
	t.Helper()
	o := Options{Root: root, Files: listing(t, root)}
	for _, f := range opts {
		f(&o)
	}
	inv, err := Build(context.Background(), o)
	require.NoError(t, err)
	return inv
}

func skillMD(name, desc string, extra ...string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n" + strings.Join(extra, "") + "---\nBody of " + name + ".\n"
}

func paths(arts []Artifact, kinds ...Kind) []string {
	var out []string
	for _, a := range arts {
		for _, k := range kinds {
			if a.Kind == k {
				out = append(out, a.Path)
			}
		}
	}
	return out
}

func named(t *testing.T, inv *Inventory, kind Kind, name string) Artifact {
	t.Helper()
	for _, a := range inv.Artifacts {
		if a.Kind == kind && a.QualifiedName == name {
			return a
		}
	}
	require.Failf(t, "no artifact", "%s %q in %v", kind, name, inv.Artifacts)
	return Artifact{}
}

func names(inv *Inventory, kind Kind) []string {
	var out []string
	for _, a := range inv.Artifacts {
		if a.Kind == kind {
			out = append(out, a.QualifiedName)
		}
	}
	sort.Strings(out)
	return out
}

func TestBuild_DiscoversEveryKind(t *testing.T) {
	// Arrange
	root := writeRepo(t, map[string]string{
		"CLAUDE.md":                     "# root\n",
		"services/CLAUDE.md":            "# services\n",
		".claude/rules/always.md":       "No frontmatter rule.\n",
		".claude/rules/scoped.md":       "---\npaths:\n  - services/**\n---\nScoped.\n",
		".claude/rules/nested/deep.md":  "Deep rule.\n",
		".claude/skills/alpha/SKILL.md": skillMD("alpha", "Does alpha things."),
		".claude/skills/a/b/SKILL.md":   skillMD("too-deep", "Not a skill: skills are one level deep."),
		".claude/skills/alpha/notes.md": "a reference, not a skill",
		".claude/agents/builder.md":     "---\nname: builder\ndescription: Builds.\n---\nBody\n",
		".claude/agents/sub/helper.md":  "---\ndescription: Not an agent: agents are one level deep.\n---\n",
		".claude/agents/BUILD":          "ignored",
		".claude/commands/deploy.md":    "---\ndescription: Deploy it.\n---\nbody",
		".claude/settings.json":         `{"enabledPlugins": {"one@market": true, "off@market": false}}`,
		".mcp.json":                     `{"mcpServers": {"one": {"type": "http"}, "two": {"command": "x"}}}`,
		"node_modules/pkg/CLAUDE.md":    "pruned",
		"pkg/.venv/lib/CLAUDE.md":       "pruned",
		"docs/README.md":                "not an artifact",
	})

	// Act
	inv := build(t, root)

	// Assert
	assert.Equal(t, []string{"CLAUDE.md", "services/CLAUDE.md"}, paths(inv.Artifacts, KindClaudeMD))
	assert.Equal(t, []string{".claude/rules/always.md", ".claude/rules/scoped.md", ".claude/rules/nested/deep.md"}, paths(inv.Artifacts, KindRule))
	assert.Equal(t, []string{"always", "nested/deep", "scoped"}, names(inv, KindRule))
	assert.Equal(t, []string{".claude/commands/deploy.md", ".claude/skills/alpha/SKILL.md"}, paths(inv.Artifacts, KindSkill))
	assert.Equal(t, []string{".claude/agents/builder.md"}, paths(inv.Artifacts, KindAgent))
	assert.Equal(t, []string{".claude/settings.json"}, paths(inv.Artifacts, KindSettings))
	assert.Equal(t, []string{"one@market"}, names(inv, KindPlugin))
	assert.Equal(t, []string{"one", "two"}, names(inv, KindMCPServer))
	assert.Equal(t, SchemaInventory, inv.SchemaVersion)
	assert.Equal(t, HarnessClaude, inv.Harness)
	assert.Empty(t, inv.Problems)
}

func TestBuild_VisibilityAndPhase(t *testing.T) {
	tests := []struct {
		name           string
		file, content  string
		wantVisibility Visibility
		wantPhase      Phase
		wantPaths      []string
	}{
		{name: "rule paths as a list", file: ".claude/rules/r.md", content: "---\npaths:\n  - a/**\n  - b/**\n---\nx", wantVisibility: VisibilityPathGated, wantPhase: PhaseOnDemand, wantPaths: []string{"a/**", "b/**"}},
		{name: "rule paths as one string", file: ".claude/rules/r.md", content: "---\npaths: a/**\n---\nx", wantVisibility: VisibilityPathGated, wantPhase: PhaseOnDemand, wantPaths: []string{"a/**"}},
		{name: "rule without paths", file: ".claude/rules/r.md", content: "---\ndescription: d\n---\nx", wantVisibility: VisibilityAlways, wantPhase: PhaseStart},
		{name: "rule without frontmatter", file: ".claude/rules/r.md", content: "plain", wantVisibility: VisibilityAlways, wantPhase: PhaseStart},
		{name: "skill with paths", file: ".claude/skills/s/SKILL.md", content: "---\ndescription: d\npaths: [a/**]\n---\nx", wantVisibility: VisibilityPathGated, wantPhase: PhaseOnDemand, wantPaths: []string{"a/**"}},
		{name: "user-only skill", file: ".claude/skills/s/SKILL.md", content: "---\ndescription: d\ndisable-model-invocation: true\n---\nx", wantVisibility: VisibilityManualOnly, wantPhase: PhaseManual},
		{name: "model-invocable skill", file: ".claude/skills/s/SKILL.md", content: "---\ndescription: d\ndisable-model-invocation: false\n---\nx", wantVisibility: VisibilityAlways, wantPhase: PhaseStart},
		{name: "manual wins over paths", file: ".claude/skills/s/SKILL.md", content: "---\ndisable-model-invocation: true\npaths: [a/**]\n---\nx", wantVisibility: VisibilityManualOnly, wantPhase: PhaseManual, wantPaths: []string{"a/**"}},
		{name: "manual wins over a nested scope", file: "svc/.claude/skills/s/SKILL.md", content: "---\ndisable-model-invocation: true\n---\nx", wantVisibility: VisibilityManualOnly, wantPhase: PhaseManual},
		{name: "an agent ignores disable-model-invocation", file: ".claude/agents/a.md", content: "---\ndescription: d\ndisable-model-invocation: true\n---\nx", wantVisibility: VisibilityAlways, wantPhase: PhaseStart},
		{name: "an agent ignores paths", file: ".claude/agents/a.md", content: "---\ndescription: d\npaths: [a/**]\n---\nx", wantVisibility: VisibilityAlways, wantPhase: PhaseStart, wantPaths: []string{"a/**"}},
		{name: "a nested CLAUDE.md waits for its directory", file: "svc/CLAUDE.md", content: "nested", wantVisibility: VisibilityAlways, wantPhase: PhaseOnDemand},
		{name: "a nested unconditional rule waits for its directory", file: "svc/.claude/rules/r.md", content: "x", wantVisibility: VisibilityAlways, wantPhase: PhaseOnDemand},
		{name: "a nested skill waits for its directory", file: "svc/.claude/skills/s/SKILL.md", content: skillMD("s", "d"), wantVisibility: VisibilityAlways, wantPhase: PhaseOnDemand},
		{name: "a user-only command", file: ".claude/commands/team/review.md", content: "---\ndescription: Review it.\ndisable-model-invocation: true\n---\nbody", wantVisibility: VisibilityManualOnly, wantPhase: PhaseManual},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := writeRepo(t, map[string]string{tt.file: tt.content})

			// Act
			inv := build(t, root)

			// Assert
			require.Len(t, inv.Artifacts, 1)
			a := inv.Artifacts[0]
			assert.Equal(t, tt.wantVisibility, a.Visibility)
			assert.Equal(t, tt.wantPhase, a.Phase)
			want := tt.wantPaths
			if want == nil {
				want = []string{}
			}
			assert.Equal(t, want, a.Paths)
			assert.True(t, a.Frontmatter.OK)
		})
	}
}

func TestBuild_StartOfSessionIsRootAlwaysVisible(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"CLAUDE.md":                          "root",
		"services/CLAUDE.md":                 "nested",
		".claude/rules/always.md":            "always",
		".claude/rules/scoped.md":            "---\npaths: [x]\n---\nx",
		".claude/skills/s/SKILL.md":          skillMD("s", "d"),
		".claude/skills/m/SKILL.md":          skillMD("m", "d", "disable-model-invocation: true\n"),
		"services/.claude/skills/n/SKILL.md": skillMD("n", "d"),
		".claude/agents/a.md":                "---\ndescription: d\n---\n",
		".mcp.json":                          `{"mcpServers": {"srv": {}}}`,
		"services/.mcp.json":                 `{"mcpServers": {"nested-srv": {}}}`,
		".claude/settings.json":              `{"enabledPlugins": {"p@m": true}}`,
	})

	start := build(t, root).Start()

	var got []string
	for _, a := range start {
		got = append(got, string(a.Kind)+" "+a.QualifiedName)
	}
	assert.Equal(t, []string{
		"mcp_server srv", "claude_md CLAUDE.md", "settings settings.json", "plugin p@m", "agent a", "rule always", "skill s",
	}, got)
}

func TestInventory_StartCountsAnUnknownPhase(t *testing.T) {
	root := writeRepo(t, map[string]string{
		".claude/skills/s/SKILL.md":     skillMD("s", "d"),
		".claude/skills/u/SKILL.md":     "---\ndescription: Use when: x\npaths: [src/**]\n---\n",
		".claude/rules/u.md":            "---\npaths: [src/**\n---\nx",
		".claude/rules/gated.md":        "---\npaths: [src/**]\n---\nx",
		"svc/.claude/skills/n/SKILL.md": "---\ndescription: Use when: x\npaths: [src/**]\n---\n",
	})
	inv := build(t, root)
	require.Equal(t, PhaseUnknown, named(t, inv, KindSkill, "u").Phase)

	start := inv.Start()

	var got []string
	for _, a := range start {
		got = append(got, string(a.Kind)+" "+a.QualifiedName+" "+string(a.Phase))
	}
	assert.Equal(t, []string{"rule u unknown", "skill s start", "skill u unknown"}, got, "a start guard counts unknown as start (fail closed)")
}

func TestBuild_ScopeIsTheDirectoryOwningTheConvention(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"services/.claude/skills/s/SKILL.md": skillMD("s", "d"),
		".github/CLAUDE.md":                  "x",
		".claude/CLAUDE.md":                  "root memory",
		"services/.claude/CLAUDE.md":         "services memory",
		"services/api/.mcp.json":             `{"mcpServers": {"api": {}}}`,
	})

	inv := build(t, root)

	got := map[string]string{}
	for _, a := range inv.Artifacts {
		got[a.Path] = a.Scope + " " + string(a.Phase)
	}
	assert.Equal(t, map[string]string{
		"services/.claude/skills/s/SKILL.md": "services on_demand",
		".github/CLAUDE.md":                  ".github on_demand",
		".claude/CLAUDE.md":                  ". start",
		"services/.claude/CLAUDE.md":         "services on_demand",
		"services/api/.mcp.json":             "services/api on_demand",
	}, got)
}

func TestBuild_ListingText(t *testing.T) {
	long := strings.Repeat("x", 1530)
	tests := []struct {
		name          string
		file, content string
		wantText      string
		wantTruncated bool
		wantChars     int
	}{
		{name: "name and description", file: ".claude/skills/alpha/SKILL.md", content: skillMD("alpha", "Does alpha things."), wantText: "alpha: Does alpha things."},
		{name: "the name falls back to the directory", file: ".claude/skills/beta/SKILL.md", content: "---\ndescription: d\n---\nbody", wantText: "beta: d"},
		{name: "an empty name falls back too", file: ".claude/skills/beta/SKILL.md", content: "---\nname: \"\"\ndescription: d\n---\nbody", wantText: "beta: d"},
		{name: "when_to_use follows the description", file: ".claude/skills/s/SKILL.md", content: skillMD("s", "Does things.", "when_to_use: When the user asks for things.\n"), wantText: "s: Does things. When the user asks for things."},
		{name: "whitespace is squashed", file: ".claude/skills/s/SKILL.md", content: "---\ndescription: |\n  Line one.\n\n  Line   two.\nwhen_to_use: \"  a\\tb \"\n---\n", wantText: "s: Line one. Line two. a b"},
		{name: "no description", file: ".claude/skills/s/SKILL.md", content: "body only", wantText: "s: "},
		{name: "an agent is not capped", file: ".claude/agents/a.md", content: "---\ndescription: " + strings.Repeat("y", 2000) + "\n---\n", wantText: "a: " + strings.Repeat("y", 2000)},
		{name: "a command is named by its path", file: ".claude/commands/team/review.md", content: "---\ndescription: Review it.\n---\nbody", wantText: "team:review: Review it."},
		{name: "1536 characters fit", file: ".claude/skills/s/SKILL.md", content: skillMD("s", strings.Repeat("x", 1536)), wantText: "s: " + strings.Repeat("x", 1536)},
		{name: "1537 characters are cut", file: ".claude/skills/s/SKILL.md", content: skillMD("s", strings.Repeat("x", 1537)), wantText: "s: " + strings.Repeat("x", 1536), wantTruncated: true},
		{name: "when_to_use counts toward the cap", file: ".claude/skills/s/SKILL.md", content: skillMD("s", long, "when_to_use: abcdefghij\n"), wantText: "s: " + long + " abcde", wantTruncated: true},
		{name: "the cap counts characters, not bytes", file: ".claude/skills/s/SKILL.md", content: skillMD("s", strings.Repeat("é", 1537)), wantText: "s: " + strings.Repeat("é", 1536), wantTruncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{tt.file: tt.content})

			inv := build(t, root)

			require.Len(t, inv.Artifacts, 1)
			a := inv.Artifacts[0]
			assert.Equal(t, tt.wantText, a.ListingText)
			assert.Equal(t, tt.wantText, a.Text)
			assert.Equal(t, tt.wantTruncated, a.Truncated)
			assert.Equal(t, len([]rune(tt.wantText)), a.ListingChars)
		})
	}
}

func TestBuild_BodyTextAndLineOffset(t *testing.T) {
	tests := []struct {
		name, content, wantText string
		wantOffset              int
	}{
		{name: "frontmatter lines are counted", content: "---\ndescription: d\n---\nfirst\n\nsecond\n", wantText: "first\n\nsecond", wantOffset: 3},
		{name: "no frontmatter keeps the file", content: "first\nsecond\n", wantText: "first\nsecond\n"},
		{name: "CRLF lines are joined by LF", content: "---\na: 1\r\n---\r\nfirst\r\nsecond\r\n", wantText: "first\nsecond", wantOffset: 3},
		{name: "an unclosed block is body text", content: "---\na: 1\nbody\n", wantText: "---\na: 1\nbody\n"},
		{name: "imports are not followed", content: "See @docs/big.md\n", wantText: "See @docs/big.md\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{".claude/rules/r.md": tt.content, "docs/big.md": strings.Repeat("imported ", 100)})

			inv := build(t, root)

			require.Len(t, inv.Artifacts, 1)
			a := inv.Artifacts[0]
			assert.Equal(t, tt.wantText, a.Text)
			assert.Equal(t, tt.wantOffset, a.LineOffset)
			assert.Empty(t, a.ListingText)
			sum := sha256.Sum256([]byte(tt.wantText))
			assert.Equal(t, hex.EncodeToString(sum[:]), a.TextSHA256)
			assert.Equal(t, len(tt.wantText), a.TextBytes)
			assert.Positive(t, a.Tokens)
		})
	}
}

func TestBuild_FrontmatterStatus(t *testing.T) {
	tests := []struct {
		name, file, content string
		wantError           string
		wantPhase           Phase
		wantVisibility      Visibility
		wantName            string
	}{
		{name: "invalid YAML naming paths is unknown", file: ".claude/skills/s/SKILL.md", content: "---\nname: gated\ndescription: Use when: deploying\npaths: [src/**]\n---\nx", wantError: "mapping values are not allowed", wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "invalid YAML naming disable-model-invocation is unknown", file: ".claude/skills/s/SKILL.md", content: "---\ndescription: a: b\ndisable-model-invocation: true\n---\nx", wantError: "mapping values", wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "invalid YAML naming user-invocable is unknown", file: ".claude/commands/c.md", content: "---\ndescription: a: b\nuser-invocable: false\n---\nx", wantError: "mapping values", wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "c"},
		{name: "a broken gated rule is unknown", file: ".claude/rules/r.md", content: "---\npaths: [src/**\n---\nx", wantError: "yaml", wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "r"},
		{name: "an unclosed block naming paths is unknown", file: ".claude/skills/s/SKILL.md", content: "---\npaths: [src/**]\nbody\n", wantError: "never closes", wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "invalid YAML without a gate key is start", file: ".claude/skills/s/SKILL.md", content: "---\ndescription: Use when: deploying\n---\nx", wantError: "mapping values", wantPhase: PhaseStart, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "a broken nested skill is on demand", file: "svc/.claude/skills/s/SKILL.md", content: "---\ndescription: a: b\npaths: [x]\n---\nx", wantError: "mapping values", wantPhase: PhaseOnDemand, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "a broken agent is not gated", file: ".claude/agents/a.md", content: "---\ndescription: a: b\npaths: [x]\n---\nx", wantError: "mapping values", wantPhase: PhaseStart, wantVisibility: VisibilityAlways, wantName: "a"},
		{name: "a list is not a mapping", file: ".claude/skills/s/SKILL.md", content: "---\n- paths\n---\nx", wantError: "cannot unmarshal", wantPhase: PhaseStart, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "a description of the wrong type empties the header", file: ".claude/skills/s/SKILL.md", content: "---\nname: other\ndescription: 42\n---\nx", wantError: `"description" is not a string`, wantPhase: PhaseStart, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "paths of the wrong type is unknown", file: ".claude/skills/s/SKILL.md", content: "---\npaths: [1, 2]\n---\nx", wantError: `"paths" is not a string or a list`, wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "a YAML 1.1 boolean is not a boolean and is unknown", file: ".claude/skills/s/SKILL.md", content: "---\ndisable-model-invocation: yes\n---\nx", wantError: `"disable-model-invocation" is not a boolean`, wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "a type error empties the header the decode kept", file: ".claude/skills/s/SKILL.md", content: "---\nname: badkey\ndescription: d\n? [a, b]\n: v\npaths: [src/**]\n---\nx", wantError: "cannot unmarshal", wantPhase: PhaseUnknown, wantVisibility: VisibilityAlways, wantName: "s"},
		{name: "valid frontmatter", file: ".claude/skills/s/SKILL.md", content: skillMD("ok", "d"), wantPhase: PhaseStart, wantVisibility: VisibilityAlways, wantName: "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{tt.file: tt.content})

			inv := build(t, root)

			require.Len(t, inv.Artifacts, 1)
			a := inv.Artifacts[0]
			require.NotNil(t, a.Frontmatter)
			assert.Equal(t, tt.wantError == "", a.Frontmatter.OK)
			if tt.wantError != "" {
				assert.Contains(t, a.Frontmatter.Error, tt.wantError)
			}
			assert.Equal(t, tt.wantPhase, a.Phase)
			assert.Equal(t, tt.wantVisibility, a.Visibility)
			assert.Equal(t, tt.wantName, a.Name)
		})
	}
}

func TestBuild_NonMarkdownArtifactsHaveNoFrontmatterStatus(t *testing.T) {
	root := writeRepo(t, map[string]string{".mcp.json": `{"mcpServers": {"srv": {}}}`, ".claude/settings.json": `{}`})

	for _, a := range build(t, root).Artifacts {
		assert.Nil(t, a.Frontmatter, a.Path)
	}
}

func TestBuild_MCPServers(t *testing.T) {
	tests := []struct {
		name, content string
		want          []string
		wantProblem   bool
	}{
		{name: "servers in file order", content: `{"mcpServers": {"zeta": {"command": "z"}, "alpha": {"type": "http"}}}`, want: []string{"zeta", "alpha"}},
		{name: "no servers", content: `{}`},
		{name: "malformed JSON", content: `{not json`, wantProblem: true},
		{name: "not an object", content: `[]`, wantProblem: true},
		{name: "servers not an object", content: `{"mcpServers": ["a"]}`, wantProblem: true},
		{name: "a server that is not an object lists none", content: `{"mcpServers": {"a": {}, "b": "x"}}`, wantProblem: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeRepo(t, map[string]string{".mcp.json": tt.content})

			inv := build(t, root)

			var got []string
			for _, a := range inv.Artifacts {
				require.Equal(t, KindMCPServer, a.Kind)
				assert.Equal(t, a.Name, a.Text)
				got = append(got, a.Name)
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantProblem, len(inv.Problems) == 1, "%v", inv.Problems)
		})
	}
}

// link makes rel below root a symlink to target, creating its directory.
func link(t *testing.T, root, target, rel string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	testutil.SymlinkOrSkip(t, target, abs)
}

func problemPaths(inv *Inventory) []string {
	var out []string
	for _, p := range inv.Problems {
		out = append(out, p.Path)
	}
	sort.Strings(out)
	return out
}

// Claude Code reads a symlinked file as the file it points to (CLAUDE.md ->
// AGENTS.md is common); only symlinked directories are not walked.
func TestBuild_SymlinkedFilesInsideTheRootAreRead(t *testing.T) {
	// Arrange
	root := writeRepo(t, map[string]string{
		"AGENTS.md":     "Shared instructions.\n",
		"docs/skill.md": skillMD("linked", "Linked skill."),
		"docs/rule.md":  "---\npaths: [src/**]\n---\nLinked rule.\n",
	})
	link(t, root, "AGENTS.md", "CLAUDE.md")
	link(t, root, "../AGENTS.md", "sub/CLAUDE.md")
	link(t, root, "../../../docs/skill.md", ".claude/skills/linked/SKILL.md")
	link(t, root, filepath.Join(root, "docs", "rule.md"), ".claude/rules/r.md")

	// Act
	inv := build(t, root)

	// Assert
	assert.Empty(t, inv.Problems)
	top := named(t, inv, KindClaudeMD, "CLAUDE.md")
	assert.Equal(t, "Shared instructions.\n", top.Text)
	assert.Equal(t, PhaseStart, top.Phase)
	assert.True(t, top.Frontmatter.OK)
	sub := named(t, inv, KindClaudeMD, "sub/CLAUDE.md")
	assert.Equal(t, "Shared instructions.\n", sub.Text)
	assert.Equal(t, PhaseOnDemand, sub.Phase)
	skill := named(t, inv, KindSkill, "linked")
	assert.Equal(t, ".claude/skills/linked/SKILL.md", skill.Path)
	assert.Equal(t, "linked: Linked skill.", skill.Text)
	assert.Equal(t, PhaseStart, skill.Phase)
	rule := named(t, inv, KindRule, "r")
	assert.Equal(t, []string{"src/**"}, rule.Paths)
	assert.Equal(t, PhaseOnDemand, rule.Phase)
}

// A symlink whose target lies outside the root is not followed, but the
// artifact stays in the inventory, unread, so a start count still sees it.
func TestBuild_SymlinksOutsideTheRootAreKeptUnread(t *testing.T) {
	// Arrange
	outside := writeRepo(t, map[string]string{"CLAUDE.md": "elsewhere", "dir/CLAUDE.md": "elsewhere", "rule.md": "---\npaths: [x]\n---\nx"})
	root := writeRepo(t, map[string]string{"CLAUDE.md": "root"})
	link(t, root, filepath.Join(outside, "dir"), "linked")
	link(t, root, filepath.Join(outside, "CLAUDE.md"), "svc/CLAUDE.md")
	link(t, root, filepath.Join(outside, "rule.md"), ".claude/rules/out.md")
	link(t, root, filepath.Join(root, "missing.md"), "gone/CLAUDE.md")

	// Act
	inv := build(t, root)

	// Assert
	assert.Equal(t, []string{"CLAUDE.md", "svc/CLAUDE.md"}, paths(inv.Artifacts, KindClaudeMD), "a directory symlink is not walked; a dangling one loads nothing")
	svc := named(t, inv, KindClaudeMD, "svc/CLAUDE.md")
	assert.Empty(t, svc.Text)
	assert.Equal(t, PhaseOnDemand, svc.Phase)
	require.NotNil(t, svc.Frontmatter)
	assert.False(t, svc.Frontmatter.OK)
	assert.Contains(t, svc.Frontmatter.Error, "outside the repository root")
	out := named(t, inv, KindRule, "out")
	assert.Equal(t, PhaseUnknown, out.Phase, "the gate of an unread root rule is unknown")
	assert.Contains(t, inv.Start(), out)
	assert.Equal(t, []string{".claude/rules/out.md", "gone/CLAUDE.md", "svc/CLAUDE.md"}, problemPaths(inv))
}

// A file over the read limit is kept with its phase from the path rules and an
// empty text, not dropped; its size stands in for the text in text_bytes and
// tokens, so a start budget still counts it.
func TestBuild_FilesOverTheSizeLimitAreKeptUnread(t *testing.T) {
	// Arrange
	big := strings.Repeat("x", repoindex.MaxFileBytes+1)
	root := writeRepo(t, map[string]string{
		"CLAUDE.md":                   big,
		"svc/CLAUDE.md":               big,
		".claude/rules/big.md":        big,
		".claude/skills/big/SKILL.md": big,
		".claude/agents/big.md":       big,
		".claude/rules/small.md":      "small",
	})

	// Act
	inv := build(t, root)

	// Assert
	assert.Equal(t, []string{".claude/agents/big.md", ".claude/rules/big.md", ".claude/skills/big/SKILL.md", "CLAUDE.md", "svc/CLAUDE.md"}, problemPaths(inv))
	for _, tt := range []struct {
		kind      Kind
		name      string
		wantPhase Phase
	}{
		{KindClaudeMD, "CLAUDE.md", PhaseStart},
		{KindClaudeMD, "svc/CLAUDE.md", PhaseOnDemand},
		{KindRule, "big", PhaseUnknown},
		{KindSkill, "big", PhaseUnknown},
		{KindAgent, "big", PhaseStart},
	} {
		a := named(t, inv, tt.kind, tt.name)
		assert.Equal(t, tt.wantPhase, a.Phase, "%s %s", tt.kind, tt.name)
		assert.Equal(t, VisibilityAlways, a.Visibility)
		assert.Empty(t, a.Text)
		assert.Equal(t, len(big), a.TextBytes, "the size of the file stands in for its text")
		assert.Equal(t, EstimateTokens(len(big)), a.Tokens, "an unread file is estimated from its size, never 0")
		require.NotNil(t, a.Frontmatter)
		assert.False(t, a.Frontmatter.OK)
		assert.Contains(t, a.Frontmatter.Error, "over the read limit")
	}
	assert.Equal(t, "small", named(t, inv, KindRule, "small").Text)
}

func TestBuild_ListsTheRepositoryWhenNoFilesAreGiven(t *testing.T) {
	root := writeRepo(t, map[string]string{"CLAUDE.md": "root", ".claude/skills/s/SKILL.md": skillMD("s", "d")})

	inv, err := Build(context.Background(), Options{Root: root})

	require.NoError(t, err)
	assert.Equal(t, []string{"CLAUDE.md", ".claude/skills/s/SKILL.md"}, paths(inv.Artifacts, KindClaudeMD, KindSkill))
}

// Outside a git repository the file listing is a walk, which lists a symlinked
// file as git would, so the reader decides what to follow.
func TestBuild_WalkListsSymlinkedFiles(t *testing.T) {
	// Arrange
	root := writeRepo(t, map[string]string{"AGENTS.md": "Shared instructions.\n", ".claude/rules/a.md": "Rule a.\n"})
	link(t, root, "AGENTS.md", "CLAUDE.md")
	link(t, root, "a.md", ".claude/rules/b.md")
	link(t, root, ".claude", "linked-dir")

	// Act
	inv, err := Build(context.Background(), Options{Root: root})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, inv.Problems)
	assert.Equal(t, "Shared instructions.\n", named(t, inv, KindClaudeMD, "CLAUDE.md").Text)
	assert.Equal(t, "Rule a.\n", named(t, inv, KindRule, "b").Text)
}

// A UTF-8 byte order mark is stripped before the frontmatter is parsed, so a
// BOM-prefixed file keeps its gate, and it is not part of the text.
func TestBuild_StripsAByteOrderMark(t *testing.T) {
	// Arrange
	const bom = "\ufeff"
	root := writeRepo(t, map[string]string{
		"CLAUDE.md":          bom + "Root.\n",
		".claude/rules/r.md": bom + "---\npaths: [src/**]\n---\nGated.\n",
	})

	// Act
	inv := build(t, root)

	// Assert
	top := named(t, inv, KindClaudeMD, "CLAUDE.md")
	assert.Equal(t, "Root.\n", top.Text)
	sum := sha256.Sum256([]byte("Root.\n"))
	assert.Equal(t, hex.EncodeToString(sum[:]), top.TextSHA256)
	rule := named(t, inv, KindRule, "r")
	assert.True(t, rule.Frontmatter.OK)
	assert.Equal(t, []string{"src/**"}, rule.Paths)
	assert.Equal(t, VisibilityPathGated, rule.Visibility)
	assert.Equal(t, PhaseOnDemand, rule.Phase)
	assert.Equal(t, "Gated.", rule.Text)
}

func TestEstimateTokens(t *testing.T) {
	for _, tt := range []struct{ bytes, want int }{
		{0, 0}, {1, 1}, {3, 1}, {4, 2}, {28, 8}, {39, 10}, {40, 11}, {390, 100}, {391, 101},
	} {
		assert.Equal(t, tt.want, EstimateTokens(tt.bytes), "%d bytes", tt.bytes)
	}
}

func TestBuild_RejectsBadOptions(t *testing.T) {
	_, err := Build(context.Background(), Options{})
	require.Error(t, err)

	_, err = Build(context.Background(), Options{Root: t.TempDir(), Plugins: "some"})
	require.ErrorContains(t, err, "unknown plugin mode")
}

func TestBuild_ArtifactsComeInWalkOrder(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"b/CLAUDE.md":               "b",
		"CLAUDE.md":                 "root",
		"a/z/CLAUDE.md":             "a/z",
		"a/CLAUDE.md":               "a",
		".claude/rules/r.md":        "r",
		".claude/rules/sub/deep.md": "deep",
		".claude/rules/zz.md":       "zz",
	})

	inv := build(t, root)

	assert.Equal(t, []string{"CLAUDE.md", ".claude/rules/r.md", ".claude/rules/zz.md", ".claude/rules/sub/deep.md", "a/CLAUDE.md", "a/z/CLAUDE.md", "b/CLAUDE.md"}, paths(inv.Artifacts, KindClaudeMD, KindRule))
}

func TestInventory_JSONShape(t *testing.T) {
	root := writeRepo(t, map[string]string{".claude/skills/s/SKILL.md": skillMD("s", "d"), "CLAUDE.md": "root"})
	inv := build(t, root)

	full, err := json.Marshal(inv)
	require.NoError(t, err)
	bare, err := json.Marshal(inv.WithoutText())
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(full, &doc))
	for _, key := range []string{"schema_version", "harness", "root", "settings", "plugins", "artifacts"} {
		assert.Contains(t, doc, key)
	}
	settings, ok := doc["settings"].(map[string]any)
	require.True(t, ok)
	for _, key := range []string{"skill_overrides", "claude_md_excludes", "enabled_plugins"} {
		assert.Contains(t, settings, key)
	}
	arts, ok := doc["artifacts"].([]any)
	require.True(t, ok)
	art, ok := arts[1].(map[string]any)
	require.True(t, ok)
	for _, key := range []string{"kind", "source", "name", "qualified_name", "path", "scope", "visibility", "phase", "paths", "listing_text", "listing_chars", "truncated", "text_bytes", "text_sha256", "tokens", "line_offset", "frontmatter", "text"} {
		assert.Contains(t, art, key)
	}
	assert.Contains(t, string(full), `"text":"s: d"`)
	assert.NotContains(t, string(bare), `"text":`)
	assert.Equal(t, "s: d", inv.Artifacts[1].Text, "WithoutText leaves the inventory as it was")
}

// Empty lists and maps print as [] and {}, never null, as every JSON document of the CLI does.
func TestInventory_JSONHasNoNull(t *testing.T) {
	inv := build(t, writeRepo(t, map[string]string{"README.md": "nothing"}))

	data, err := json.Marshal(inv)

	require.NoError(t, err)
	assert.NotContains(t, string(data), "null")
	assert.Contains(t, string(data), `"problems":[]`)
	assert.Contains(t, string(data), `"skill_overrides":{}`)
	assert.Contains(t, string(data), `"schema_version":1`)
}

// An empty inventory prints "artifacts": [] with or without the texts.
func TestInventory_EmptyArtifactsAreAnEmptyList(t *testing.T) {
	inv := build(t, writeRepo(t, map[string]string{"README.md": "nothing"}))

	data, err := json.Marshal(inv.WithoutText())

	require.NoError(t, err)
	assert.Contains(t, string(data), `"artifacts":[]`)
}
