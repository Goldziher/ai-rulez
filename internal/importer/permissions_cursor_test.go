package importer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursorRule_CommandArgsForm(t *testing.T) {
	tests := []struct {
		name   string
		rule   string
		action string
		want   string
		ok     bool
	}{
		{"deny prefix glob", "Shell(git:push*)", actionDeny, "Bash(git push:*)", true},
		{"deny generator encoding", "Shell(rm:-rf*)", actionDeny, "Bash(rm -rf:*)", true},
		{"deny any arguments", "Shell(curl:*)", actionDeny, "Bash(curl:*)", true},
		{"allow any arguments", "Shell(npm:*)", actionAllow, "Bash(npm:*)", true},
		{"allow exact arguments", "Shell(git:status)", actionAllow, "Bash(git status)", true},
		{"deny exact arguments", "Shell(rm:-rf /)", actionDeny, "Bash(rm -rf /)", true},
		{"deny inner glob widens to its literal prefix", "Shell(git:push * --force)", actionDeny, "Bash(git push:*)", true},
		{"deny leading glob widens to the command", "Shell(rm:*-rf)", actionDeny, "Bash(rm:*)", true},
		{"allow inner glob is not widened", "Shell(git:push * --force)", actionAllow, "", false},
		{"globbed command base is not guessed", "Shell(g*:push)", actionDeny, "", false},
		{"empty command base", "Shell(:push)", actionDeny, "", false},
		{"empty arguments read as the bare command", "Shell(rm:)", actionDeny, "Bash(rm:*)", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, ok := cursorRule(tt.rule, tt.action)
			// Assert
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCursorRule_RoundTripsTheGeneratorEncoding renders [permissions] to Cursor's
// cli.json with the generator and reads every rule back: each must come back as
// the rule it was rendered from, so a re-convert of generated output adds nothing.
func TestCursorRule_RoundTripsTheGeneratorEncoding(t *testing.T) {
	tests := []struct {
		name  string
		allow []string
		deny  []string
	}{
		{"shell prefixes", nil, []string{"Bash(rm -rf:*)", "Bash(git push --force:*)", "Bash(curl:*)"}},
		{"paths and mcp", []string{"Edit(src/**)", "mcp__github__create_issue"}, []string{"Read(.env)", "mcp__evil"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{Permissions: &config.Permissions{Allow: tt.allow, Deny: tt.deny}}
			keys, err := settings.PermissionKeys(cfg, "cursor", ".cursor/cli.json")
			require.NoError(t, err)
			rendered := map[string][]string{}
			for _, k := range keys {
				raw, err := json.Marshal(k.Value)
				require.NoError(t, err)
				var rules []string
				require.NoError(t, json.Unmarshal(raw, &rules))
				rendered[k.Path[len(k.Path)-1]] = rules
			}
			require.Len(t, rendered[actionDeny], len(tt.deny), "every deny rule renders for cursor")

			// Act
			back := map[string][]string{}
			for action, rules := range rendered {
				for _, r := range rules {
					claude, ok := cursorRule(r, action)
					require.True(t, ok, "%s %s", action, r)
					back[action] = append(back[action], claude)
				}
			}

			// Assert
			assert.ElementsMatch(t, tt.deny, back[actionDeny])
			assert.ElementsMatch(t, tt.allow, back[actionAllow])
		})
	}
}

// TestCursorImport_DenyReachesOpencode pins RV-CLI-1's second harness: an
// imported Cursor deny becomes a pattern opencode actually matches.
func TestCursorImport_DenyReachesOpencode(t *testing.T) {
	// Arrange
	p := planOf(t, nativeImporter{}, mapFS(map[string]string{
		".cursor/cli.json": `{"permissions":{"allow":[],"deny":["Shell(git:push*)","Shell(rm:-rf*)"]}}`,
	}), Options{})
	cfg := &config.Config{Permissions: &config.Permissions{Deny: p.Permissions.Deny}}

	// Act
	keys, err := settings.PermissionKeys(cfg, "opencode", "opencode.json")
	require.NoError(t, err)
	values := make([]any, 0, len(keys))
	for _, k := range keys {
		values = append(values, k.Value)
	}
	raw, err := json.Marshal(values)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, []string{"Bash(git push:*)", "Bash(rm -rf:*)"}, p.Permissions.Deny)
	assert.Contains(t, string(raw), `"git push *"`)
	assert.Contains(t, string(raw), `"rm -rf *"`)
	assert.NotContains(t, string(raw), `git:push`)
}

// TestCursorImport_GeneratedRulesAreNotReimported pins the re-convert half of
// RV-CLI-1: the rules generate merged into .cursor/cli.json are output, named by
// the manifest, so a second convert adds no rule (not even a lossy one, such as
// an exact Bash(sudo) deny that Cursor can only hold as Shell(sudo)); a rule the
// user wrote beside them is still imported.
func TestCursorImport_GeneratedRulesAreNotReimported(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"gen\"\npresets = [\"cursor\"]\n\n" +
			"[permissions]\ndeny = [\"Bash(rm -rf:*)\", \"Bash(sudo)\"]\n",
		".ai-rulez/rules/style.md": "---\ndescription: style\n---\nUse tabs.\n",
	})
	cfg, err := config.LoadConfigFromDir(context.Background(), dir, ".ai-rulez", config.WithoutLocal(), config.WithoutRemote())
	require.NoError(t, err)
	require.NoError(t, generator.NewGenerator(cfg).Generate(""))
	cli := filepath.Join(dir, ".cursor", "cli.json")
	body, err := os.ReadFile(cli)
	require.NoError(t, err)
	require.Contains(t, string(body), "Shell(rm:-rf*)")
	var doc map[string]map[string][]string
	require.NoError(t, json.Unmarshal(body, &doc))
	doc["permissions"]["deny"] = append(doc["permissions"]["deny"], "Shell(git:push*)")
	edited, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cli, edited, 0o600))

	// Act
	p := planOf(t, nativeImporter{}, os.DirFS(dir), Options{})

	// Assert
	assert.Equal(t, []string{"Bash(git push:*)"}, p.Permissions.Deny)
	f := findingFor(p, StatusDropped, ".cursor/cli.json", "permissions.deny")
	require.NotNil(t, f)
	assert.Contains(t, f.Reason, "2 rule(s) generated by ai-rulez")
}
