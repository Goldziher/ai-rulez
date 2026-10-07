package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// runRole renders role (or the plain project) the way the generate command does:
// reconcile hand-written skillOverrides, then generate.
func runRole(t *testing.T, dir, role string) {
	t.Helper()
	g := roleGenerator(t, dir, role)
	require.NoError(t, g.ReconcileRoleSkillOverrides())
	require.NoError(t, g.Generate(""))
}

func writeSettings(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(body), 0o644))
}

func skillOverridesOf(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if os.IsNotExist(err) {
		return nil // nothing is left of a document ai-rulez wrote whole
	}
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	got, _ := doc["skillOverrides"].(map[string]any) //nolint:errcheck // nil is asserted by callers
	return got
}

func TestRoleRestoresHandWrittenSkillOverride(t *testing.T) {
	tests := []struct {
		name  string
		steps []string // roles rendered in order; "" is a plain generate
		want  map[string]any
	}{
		{name: "role overwrites, plain generate restores", steps: []string{"backend", ""}, want: map[string]any{"migrate": "on"}},
		{name: "role overwrites while active", steps: []string{"backend"}, want: map[string]any{"migrate": "name-only", "deploy": "off"}},
		{name: "role switch restores the value", steps: []string{"backend", "frontend"}, want: map[string]any{"migrate": "on", "ui": "user-invocable-only"}},
		{name: "role to role keeping the skill keeps the hand-written prior", steps: []string{"backend", "backend", ""}, want: map[string]any{"migrate": "on"}},
		{name: "restore is stable across runs", steps: []string{"backend", "", ""}, want: map[string]any{"migrate": "on"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := rolesProject(t)
			writeSettings(t, dir, `{"skillOverrides": {"migrate": "on"}}`+"\n")

			// Act
			for _, role := range tt.steps {
				runRole(t, dir, role)
			}

			// Assert
			assert.Equal(t, tt.want, skillOverridesOf(t, dir))
		})
	}
}

func TestRoleLeavesAnEditedValueAlone(t *testing.T) {
	// Arrange: a role replaced the hand-written value, then the person changed it.
	dir := rolesProject(t)
	writeSettings(t, dir, `{"skillOverrides": {"migrate": "on"}}`+"\n")
	runRole(t, dir, "backend")
	writeSettings(t, dir, `{"skillOverrides": {"migrate": "off", "deploy": "off"}}`+"\n")

	// Act
	runRole(t, dir, "")

	// Assert: the role's claim no longer matches, so the value is the person's.
	assert.Equal(t, "off", skillOverridesOf(t, dir)["migrate"])
}

func TestRoleWithoutHandWrittenValueLeavesNoLedger(t *testing.T) {
	// Arrange
	dir := rolesProject(t)

	// Act
	runRole(t, dir, "backend")
	runRole(t, dir, "")

	// Assert
	assert.Empty(t, skillOverridesOf(t, dir))
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", "local", roleLedgerName))
}

func TestRoleSameValueAsHandWrittenIsNotAnOverwrite(t *testing.T) {
	// Arrange: the hand-written value is the one the role sets.
	dir := rolesProject(t)
	writeSettings(t, dir, `{"skillOverrides": {"migrate": "name-only"}}`+"\n")

	// Act
	runRole(t, dir, "backend")
	runRole(t, dir, "")

	// Assert
	assert.Equal(t, map[string]any{"migrate": "name-only"}, skillOverridesOf(t, dir))
}

func TestWriteRoleLedger_DoesNotWriteThroughSymlinks(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, g *Generator, outside string) (victim string)
	}{
		{"ledger is a link to an outside file", func(t *testing.T, g *Generator, outside string) string {
			victim := filepath.Join(outside, "victim")
			require.NoError(t, os.WriteFile(victim, []byte("precious"), 0o600))
			require.NoError(t, os.MkdirAll(filepath.Dir(g.roleLedgerPath()), 0o755))
			testutil.SymlinkOrSkip(t, victim, g.roleLedgerPath())
			return victim
		}},
		{"local directory is a link", func(t *testing.T, g *Generator, outside string) string {
			require.NoError(t, os.MkdirAll(g.manifestDir(), 0o755))
			testutil.SymlinkOrSkip(t, outside, filepath.Dir(g.roleLedgerPath()))
			return filepath.Join(outside, roleLedgerName)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g := rawGenerator(t, t.TempDir())
			outside := t.TempDir()
			victim := tt.setup(t, g, outside)
			before, readErr := os.ReadFile(victim)

			// Act
			err := g.writeRoleLedger(roleLedger{Prior: map[string]json.RawMessage{"s": json.RawMessage(`"on"`)}})

			// Assert
			after, afterErr := os.ReadFile(victim)
			if readErr != nil {
				require.Error(t, err)
				assert.Error(t, afterErr, "a file was created through the link")
				return
			}
			require.NoError(t, afterErr)
			assert.Equal(t, string(before), string(after))
		})
	}
}

func TestRoleKeepsLedgerEntryWhenSettingsAreNotStrictJSON(t *testing.T) {
	// Arrange: a role replaced the hand-written value, then the person added a comment.
	dir := rolesProject(t)
	writeSettings(t, dir, `{"skillOverrides": {"migrate": "on"}}`+"\n")
	runRole(t, dir, "backend")
	ledger := filepath.Join(dir, ".ai-rulez", "local", roleLedgerName)
	require.FileExists(t, ledger)
	writeSettings(t, dir, "// mine\n"+`{"skillOverrides": {"migrate": "name-only", "deploy": "off"}}`+"\n")

	// Act
	g := roleGenerator(t, dir, "")
	require.NoError(t, g.ReconcileRoleSkillOverrides())

	// Assert: the prior value is still remembered for when the file parses again.
	data, err := os.ReadFile(ledger)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"migrate"`)
	assert.Contains(t, string(data), `"on"`)
}
