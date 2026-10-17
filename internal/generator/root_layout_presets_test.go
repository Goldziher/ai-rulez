package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// projectRootFiles lists the files directly in the project root.
func projectRootFiles(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	out := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			out[e.Name()] = true
		}
	}
	return out
}

func TestRootLayoutGenerate_NoPresetWritesAFileTheProjectKeepsByHand(t *testing.T) {
	// Arrange: every preset, a hand-written instruction file per name a preset could own
	root := t.TempDir()
	writeRootGlueFile(t, root, "ai-rulez.toml", "version = \"5.0\"\nname = \"solo\"\npresets = "+"["+quotedList(config.AllPresetNames())+"]"+
		"\n\n[[installed_skills]]\nname = \"demo\"\nsource = \"./shared\"\n")
	writeRootGlueFile(t, root, "shared/skills/demo/SKILL.md", "---\nname: demo\ndescription: A demo skill installed from a local source for tests\n---\n\nDo it.\n")
	writeRootGlueFile(t, root, "CLAUDE.md", "# Mine\n")
	writeRootGlueFile(t, root, "AGENTS.md", "# Mine\n")
	before := projectRootFiles(t, root)
	g, _ := rootGlueGenerator(t, root)

	// Act
	require.NoError(t, g.Generate(""))

	// Assert: settings files may join the root, instruction files, llms.txt and the OKF bundle do not
	for name := range projectRootFiles(t, root) {
		if before[name] {
			continue
		}
		isDoc := strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".mdc") || strings.HasSuffix(name, ".txt")
		assert.False(t, isDoc, "generate wrote the instruction file %s", name)
	}
	assert.Equal(t, "# Mine\n", readFile(t, filepath.Join(root, "CLAUDE.md")))
	assert.Equal(t, "# Mine\n", readFile(t, filepath.Join(root, "AGENTS.md")))
	assert.NoDirExists(t, filepath.Join(root, "docs", "okf"))
	assert.NoFileExists(t, filepath.Join(root, "llms.txt"))
	assert.NoFileExists(t, filepath.Join(root, "llms-full.txt"))
}
