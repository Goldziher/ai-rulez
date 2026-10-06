package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
