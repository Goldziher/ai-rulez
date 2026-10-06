package providers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

func writeDoc(t *testing.T, name, body string) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return dir, path
}

// TestElementsOwnedKey_NeverClaimsAHandWrittenDuplicate pins that a value the user
// listed themselves is not recorded as ai-rulez's, so clean cannot delete it.
func TestElementsOwnedKey_NeverClaimsAHandWrittenDuplicate(t *testing.T) {
	// Arrange
	dir, path := writeDoc(t, "kilo.jsonc", `{"instructions":["a.md","b.md"]}`)
	cfg := &config.Config{BaseDir: dir, Run: config.NewRunState()}
	sc := &SidecarSpec{Elements: &ElementsSpec{Key: []string{"instructions"}, Values: []string{"b.md", "c.md"}}}

	// Act
	key, ok, err := elementsOwnedKey(sc, cfg, path)

	// Assert
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []any{"a.md", "b.md", "c.md"}, key.Value)
	assert.Equal(t, []any{"c.md"}, key.Elements)
}

func TestGitlabOwnedKey_NeverClaimsAHandWrittenGroup(t *testing.T) {
	// Arrange
	checks := []config.ContentFile{{Name: "sec", Content: "Check secrets."}}
	ours := gitlabInstructions(checks)[0].(map[string]any)
	dir, path := writeDoc(t, "review.yaml",
		"instructions:\n  - name: sec\n    instructions: "+ours["instructions"].(string)+"\n")
	cfg := &config.Config{BaseDir: dir, Run: config.NewRunState()}

	// Act
	key, ok, err := gitlabOwnedKey(checks, cfg, path, "review.yaml")

	// Assert
	require.NoError(t, err)
	require.True(t, ok)
	assert.Len(t, key.Value, 1)
	assert.Empty(t, key.Elements, "an identical group the user wrote stays theirs")
}

func TestClaimableAugmentAreas_DropsAHandWrittenIdenticalArea(t *testing.T) {
	// Arrange
	checks := []config.ContentFile{{Name: "sec", Content: "Check secrets."}}
	area := augmentAreas(checks)["sec"].(map[string]any)
	rule := area["rules"].([]any)[0].(map[string]any)
	dir, path := writeDoc(t, "g.yaml", "areas:\n  sec:\n    description: "+area["description"].(string)+
		"\n    globs: ['**']\n    rules:\n      - id: sec\n        description: "+rule["description"].(string)+
		"\n        severity: "+rule["severity"].(string)+"\n")
	cfg := &config.Config{BaseDir: dir, Run: config.NewRunState()}
	var warned []string
	t.Cleanup(rulefiles.SetWarnSink(func(msg string, _ ...any) { warned = append(warned, msg) }))

	// Act
	areas, err := claimableAugmentAreas(checks, cfg, path, "g.yaml")

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, areas, "sec", "an identical area the user wrote is not claimed")
	assert.Empty(t, warned, "the area is identical, so it is not a clash")
}
