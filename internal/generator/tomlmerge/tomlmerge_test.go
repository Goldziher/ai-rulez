package tomlmerge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/tomlmerge"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func server(command string) map[string]any {
	return map[string]any{"command": command, "args": []string{"-y", "pkg"}}
}

func servers(entries map[string]any) []tomlmerge.OwnedKey {
	return []tomlmerge.OwnedKey{{Name: "mcp_servers", Value: entries, Members: true}}
}

const handAuthored = `# my codex config
model = "gpt-5" # default model
approval_policy = "never"

[profiles.fast]
model = "mini"
notes = """
multi # not a comment
"""

[mcp_servers.mine]
command = "mine" # keep me
args = [
  "a", # first
  "b",
]
`

func TestApply_Table(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		owned []tomlmerge.OwnedKey
		want  string
	}{
		{
			name:  "appends a new member table and keeps everything else",
			doc:   handAuthored,
			owned: servers(map[string]any{"gen": server("x")}),
			want: handAuthored + `
[mcp_servers.gen]
args = ["-y", "pkg"]
command = "x"
`,
		},
		{
			name: "replaces an owned member in place with its sub-tables",
			doc: `[a]
x = 1

# the server
[mcp_servers.gen]
command = "stale"
[mcp_servers.gen.env]
OLD = "1"

[mcp_servers.mine]
command = "mine"
`,
			owned: servers(map[string]any{"gen": map[string]any{"command": "x", "env": map[string]any{"K": "v"}}}),
			want: `[a]
x = 1

# the server
[mcp_servers.gen]
command = "x"

[mcp_servers.gen.env]
K = "v"

[mcp_servers.mine]
command = "mine"
`,
		},
		{
			name: "new member lands after its siblings, not at the end of the file",
			doc: `[mcp_servers.mine]
command = "mine"

[other]
y = 2
`,
			owned: servers(map[string]any{"gen": map[string]any{"command": "x"}}),
			want: `[mcp_servers.mine]
command = "mine"

[mcp_servers.gen]
command = "x"

[other]
y = 2
`,
		},
		{
			name:  "dotted sub-table path",
			doc:   "[permissions]\nmode = \"ask\" # mine\n",
			owned: []tomlmerge.OwnedKey{{Path: []string{"permissions", "ai-rulez", "filesystem"}, Value: map[string]any{"read": []string{"."}}}},
			want:  "[permissions]\nmode = \"ask\" # mine\n\n[permissions.ai-rulez.filesystem]\nread = [\".\"]\n",
		},
		{
			name:  "array of tables",
			doc:   "x = 1\n",
			owned: []tomlmerge.OwnedKey{{Name: "hooks", Value: []map[string]any{{"event": "a"}, {"event": "b"}}}},
			want:  "x = 1\n\n[[hooks]]\nevent = \"a\"\n\n[[hooks]]\nevent = \"b\"\n",
		},
		{
			name:  "root scalar goes before the first header",
			doc:   "# top\n[a]\nx = 1\n",
			owned: []tomlmerge.OwnedKey{{Name: "model", Value: "opus"}},
			want:  "# top\nmodel = \"opus\"\n\n[a]\nx = 1\n",
		},
		{
			name:  "root scalar replaced in place keeps its comment",
			doc:   "model = \"old\" # pinned\n\n[a]\nx = 1\n",
			owned: []tomlmerge.OwnedKey{{Name: "model", Value: "opus"}},
			want:  "model = \"opus\" # pinned\n\n[a]\nx = 1\n",
		},
		{
			name:  "scalar inside an existing table",
			doc:   "[a]\nx = 1\n\n[b]\ny = 2\n",
			owned: []tomlmerge.OwnedKey{{Path: []string{"a", "z"}, Value: []int{1, 2}}},
			want:  "[a]\nx = 1\nz = [1, 2]\n\n[b]\ny = 2\n",
		},
		{
			name:  "scalar under a missing table creates it",
			doc:   "x = 1\n",
			owned: []tomlmerge.OwnedKey{{Path: []string{"features", "web"}, Value: true}},
			want:  "x = 1\n\n[features]\nweb = true\n",
		},
		{
			name:  "value type change from table to scalar",
			doc:   "[a]\nx = 1\n\n[b]\ny = 2\n",
			owned: []tomlmerge.OwnedKey{{Name: "a", Value: "now a string"}},
			want:  "a = \"now a string\"\n\n[b]\ny = 2\n",
		},
		{
			name:  "remove drops the table and the ancestor it empties",
			doc:   "[keep]\nx = 1\n\n[mcp_servers]\n\n[mcp_servers.gen]\ncommand = \"x\"\n",
			owned: []tomlmerge.OwnedKey{{Path: []string{"mcp_servers", "gen"}, Remove: true}},
			want:  "[keep]\nx = 1\n",
		},
		{
			name:  "remove of an absent key is a no-op",
			doc:   handAuthored,
			owned: []tomlmerge.OwnedKey{{Path: []string{"nope", "x"}, Remove: true}},
			want:  handAuthored,
		},
		{
			name:  "empty owned map claims nothing and leaves existing members",
			doc:   handAuthored,
			owned: servers(map[string]any{}),
			want:  handAuthored,
		},
		{
			name:  "CRLF documents keep CRLF",
			doc:   "x = 1\r\n\r\n[a]\r\ny = 2\r\n",
			owned: servers(map[string]any{"gen": map[string]any{"command": "x"}}),
			want:  "x = 1\r\n\r\n[a]\r\ny = 2\r\n\r\n[mcp_servers.gen]\r\ncommand = \"x\"\r\n",
		},
		{
			name:  "document without a trailing newline",
			doc:   "x = 1",
			owned: []tomlmerge.OwnedKey{{Name: "y", Value: 2}},
			want:  "x = 1\ny = 2\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.toml", tt.doc)

			// Act
			result, err := tomlmerge.Apply(path, tt.owned)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Body)
			var parsed map[string]any
			require.NoError(t, toml.Unmarshal([]byte(result.Body), &parsed), "result must be valid TOML")

			// Idempotent: a second pass over its own output changes nothing.
			again, err := tomlmerge.Apply(writeFixture(t, "again.toml", result.Body), tt.owned)
			require.NoError(t, err)
			assert.Equal(t, result.Body, again.Body)
		})
	}
}

func TestApply_NewAndEmptyFiles(t *testing.T) {
	tests := []struct {
		name string
		doc  *string
	}{
		{"missing file", nil},
		{"empty file", ptr("")},
		{"whitespace only file", ptr("\n  \n")},
	}
	owned := []tomlmerge.OwnedKey{
		{Name: "model", Value: "opus"},
		{Name: "mcp_servers", Value: map[string]any{"b": server("b"), "a": server("a")}, Members: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "config.toml")
			if tt.doc != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.doc), 0o600))
			}

			// Act
			result, err := tomlmerge.Apply(path, owned)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, `model = "opus"

[mcp_servers.a]
args = ["-y", "pkg"]
command = "a"

[mcp_servers.b]
args = ["-y", "pkg"]
command = "b"
`, result.Body)
			assert.False(t, result.PartiallyOwned)
			assert.NotEmpty(t, result.Claims)
		})
	}
}

func ptr(s string) *string { return &s }

func TestApply_PartiallyOwned(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want bool
	}{
		{"only owned keys", "[mcp_servers.gen]\ncommand = \"old\"\n", false},
		{"a user key", "model = \"x\"\n[mcp_servers.gen]\ncommand = \"old\"\n", true},
		{"a user server beside ours", "[mcp_servers.mine]\ncommand = \"m\"\n", true},
		{"a comment", "# mine\n[mcp_servers.gen]\ncommand = \"old\"\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.toml", tt.doc)

			// Act
			result, err := tomlmerge.Apply(path, servers(map[string]any{"gen": map[string]any{"command": "x"}}))

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.PartiallyOwned)
		})
	}
}

func TestApply_Refusals(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"malformed", "x = \n"},
		{"unterminated table", "[a\nx = 1\n"},
		{"merge into an inline table", "mcp_servers = { mine = { command = \"m\" } }\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.toml", tt.doc)

			// Act
			_, err := tomlmerge.Apply(path, servers(map[string]any{"gen": map[string]any{"command": "x"}}))

			// Assert
			require.Error(t, err)
			onDisk, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.doc, string(onDisk))
		})
	}
}

func TestApply_RefusesNull(t *testing.T) {
	// Act
	_, err := tomlmerge.Apply("", []tomlmerge.OwnedKey{{Name: "x", Value: nil}})

	// Assert
	require.Error(t, err)
}

func TestUnmerge_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		original string
		owned    []tomlmerge.OwnedKey
	}{
		{"member table appended", handAuthored, servers(map[string]any{"gen": server("x")})},
		{
			"member table among siblings",
			"[mcp_servers.mine]\ncommand = \"mine\"\n\n[other]\ny = 2\n",
			servers(map[string]any{"gen": map[string]any{"command": "x", "env": map[string]any{"K": "v"}}}),
		},
		{
			"member table with no blank line before the next header",
			"[mcp_servers.mine]\ncommand = \"mine\"\n[other]\ny = 2\n",
			servers(map[string]any{"gen": map[string]any{"command": "x"}}),
		},
		{"root scalar", "# top\n[a]\nx = 1\n", []tomlmerge.OwnedKey{{Name: "model", Value: "opus"}}},
		{"root scalar after other scalars", "a = 1\n\n[t]\nx = 1\n", []tomlmerge.OwnedKey{{Name: "model", Value: "opus"}}},
		{"scalar in a table", "[a]\nx = 1\n\n[b]\ny = 2\n", []tomlmerge.OwnedKey{{Path: []string{"a", "z"}, Value: 1}}},
		{
			"dotted table under a user table",
			"[permissions]\nmode = \"ask\"\n",
			[]tomlmerge.OwnedKey{{Path: []string{"permissions", "ai-rulez", "filesystem"}, Value: map[string]any{"r": 1}}},
		},
		{"array of tables", "x = 1\n", []tomlmerge.OwnedKey{{Name: "hooks", Value: []map[string]any{{"e": "a"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.toml", tt.original)
			applied, err := tomlmerge.Apply(path, tt.owned)
			require.NoError(t, err)
			require.NotEqual(t, tt.original, applied.Body)

			// Act
			unmerged, err := tomlmerge.UnmergeDocument(path, applied.Body, applied.Claims)

			// Assert
			require.NoError(t, err)
			assert.True(t, unmerged.Changed)
			assert.False(t, unmerged.Empty)
			assert.Equal(t, tt.original, unmerged.Body)
		})
	}
}

func TestUnmerge_GeneratedFileIsEmpty(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "config.toml")
	owned := servers(map[string]any{"gen": server("x")})
	applied, err := tomlmerge.Apply(path, owned)
	require.NoError(t, err)

	// Act
	unmerged, err := tomlmerge.UnmergeDocument(path, applied.Body, applied.Claims)

	// Assert
	require.NoError(t, err)
	assert.True(t, unmerged.Changed)
	assert.True(t, unmerged.Empty)
	assert.Empty(t, unmerged.Body)
}

func TestUnmerge_KeepsEditedValues(t *testing.T) {
	// Arrange
	owned := servers(map[string]any{"gen": server("x")})
	applied, err := tomlmerge.Apply("", owned)
	require.NoError(t, err)
	edited := "# mine\n[mcp_servers.gen]\ncommand = \"mine now\"\n"

	// Act
	unmerged, err := tomlmerge.UnmergeDocument("config.toml", edited, applied.Claims)

	// Assert
	require.NoError(t, err)
	assert.False(t, unmerged.Changed)
	assert.Equal(t, [][]string{{"mcp_servers", "gen"}}, unmerged.Kept)
}

func TestUnmerge_AloneAndElements(t *testing.T) {
	tests := []struct {
		name   string
		doc    string
		claims []tomlmerge.Claim
		want   string
		change bool
	}{
		{
			name:   "alone claim applies only when nothing else is left",
			doc:    "x = 1\nnote = \"n\"\n",
			claims: []tomlmerge.Claim{{Path: []string{"note"}, Alone: true}},
			want:   "",
			change: false,
		},
		{
			name:   "alone claim applies to a sole key",
			doc:    "note = \"n\"\n",
			claims: []tomlmerge.Claim{{Path: []string{"note"}, Alone: true}},
			want:   "",
			change: true,
		},
		{
			name:   "elements are taken out of the user's array",
			doc:    "instructions = [\"mine\", \"ours\"]\n",
			claims: []tomlmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"ours"}}},
			want:   "instructions = [\"mine\"]\n",
			change: true,
		},
		{
			name:   "an array left empty is removed",
			doc:    "x = 1\ninstructions = [\"ours\"]\n",
			claims: []tomlmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"ours"}}},
			want:   "x = 1\n",
			change: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result, err := tomlmerge.UnmergeDocument("config.toml", tt.doc, tt.claims)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.change, result.Changed)
			if tt.change {
				assert.Equal(t, tt.want, result.Body)
			}
		})
	}
}

func TestUnmerge_MissingFileAndMalformed(t *testing.T) {
	// Act
	missing, err := tomlmerge.Unmerge(filepath.Join(t.TempDir(), "none.toml"), []tomlmerge.Claim{{Path: []string{"a"}}})

	// Assert
	require.NoError(t, err)
	assert.False(t, missing.Changed)

	_, err = tomlmerge.UnmergeDocument("x.toml", "a = \n", []tomlmerge.Claim{{Path: []string{"a"}}})
	require.Error(t, err)
}

// TestApply_TrickyUnownedContentSurvives covers syntax that trips up line-based
// scanners: markers inside strings and multi-line values, quoted and dotted keys,
// inline tables, dates and literal strings.
func TestApply_TrickyUnownedContentSurvives(t *testing.T) {
	const doc = `title = "has # hash and [brackets]"
literal = 'C:\path # not a comment'
"quoted key" = 1
dotted.key.here = { a = 1, b = [1, 2, { c = 3 }] }
when = 1979-05-27 07:32:00Z # a date with a space
text = """
[not.a.table]
x = 1
"""
list = [
  # comment inside
  "a", "]", # bracket in string
  "b",
]

[ "quoted table" . sub ]
k = 'v'

[[servers]]
name = "one"

[[servers]]
name = "two"
`
	// Arrange
	path := writeFixture(t, "config.toml", doc)
	owned := servers(map[string]any{"gen": map[string]any{"command": "x"}})

	// Act
	result, err := tomlmerge.Apply(path, owned)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, doc+"\n[mcp_servers.gen]\ncommand = \"x\"\n", result.Body)

	unmerged, err := tomlmerge.UnmergeDocument(path, result.Body, result.Claims)
	require.NoError(t, err)
	assert.Equal(t, doc, unmerged.Body)
}

func TestApply_OwnedTableInsideQuotedHeader(t *testing.T) {
	// Arrange
	const doc = "[\"mcp_servers\".\"my server\"]\ncommand = \"old\"\n\n[x]\ny = 1\n"
	path := writeFixture(t, "config.toml", doc)

	// Act
	result, err := tomlmerge.Apply(path, servers(map[string]any{"my server": map[string]any{"command": "new"}}))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[mcp_servers.\"my server\"]\ncommand = \"new\"\n\n[x]\ny = 1\n", result.Body)
}
