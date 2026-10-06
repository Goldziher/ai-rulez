package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// narrowingProject writes a project whose "full" profile carries two domains and
// whose "narrow" profile carries one, so regenerating with --profile narrow drops
// every output the "drop" domain contributed. The dropped skill owns a
// references/ file too, which is the nested-directory case.
func narrowingProject(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ai-rulez")

	keptSkill := filepath.Join(configDir, "domains", "keep", "skills", "keeper")
	droppedSkill := filepath.Join(configDir, "domains", "drop", "skills", "dropped")
	require.NoError(t, os.MkdirAll(keptSkill, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(droppedSkill, "references"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"4.0\"\n"+
			"name = \"narrowing\"\n"+
			"presets = [\"claude\"]\n"+
			"default = \"full\"\n"+
			"gitignore = false\n\n"+
			"[profiles]\n"+
			"full = [\"keep\", \"drop\"]\n"+
			"narrow = [\"keep\"]\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(keptSkill, "SKILL.md"),
		[]byte("---\ndescription: keeper\n---\nkeeper body\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(droppedSkill, "SKILL.md"),
		[]byte("---\ndescription: dropped\n---\ndropped body\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(droppedSkill, "references", "note.md"),
		[]byte("reference body\n"), 0o644))

	return tempDir
}

func generateProfile(t *testing.T, dir, profile string) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(profile))
}

// TestGenerator_PrunesDirectoriesEmptiedByStaleCleanup is the regression test for
// the narrowing-generate defect: the stale pass deleted the SKILL.md files the
// new profile no longer emits but left their directories behind, and an empty
// `<id>/` under skills/ reads to a human — and to tooling that lists the
// directory — as a live skill that has lost its body.
func TestGenerator_PrunesDirectoriesEmptiedByStaleCleanup(t *testing.T) {
	t.Parallel()

	tempDir := narrowingProject(t)
	skillsRoot := filepath.Join(tempDir, ".claude", "skills")
	droppedOut := filepath.Join(skillsRoot, "dropped")

	generateProfile(t, tempDir, "full")
	require.FileExists(t, filepath.Join(droppedOut, "SKILL.md"))
	require.FileExists(t, filepath.Join(droppedOut, "references", "note.md"))
	require.FileExists(t, filepath.Join(skillsRoot, "keeper", "SKILL.md"))

	generateProfile(t, tempDir, "narrow")

	assert.NoDirExists(t, filepath.Join(droppedOut, "references"),
		"the references directory lost its only file and must not survive")
	assert.NoDirExists(t, droppedOut,
		"an empty skill directory reads as a skill that has lost its body")

	// Blast radius: the surviving skill and the roots above it are untouched.
	assert.FileExists(t, filepath.Join(skillsRoot, "keeper", "SKILL.md"))
	assert.DirExists(t, skillsRoot)
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

// TestGenerator_KeepsDirectoryHoldingUnownedFile pins the blast radius: a
// directory ai-rulez emptied but which still holds anything ai-rulez did not
// write must survive, because the user put that file there.
func TestGenerator_KeepsDirectoryHoldingUnownedFile(t *testing.T) {
	t.Parallel()

	tempDir := narrowingProject(t)
	droppedOut := filepath.Join(tempDir, ".claude", "skills", "dropped")

	generateProfile(t, tempDir, "full")

	userFile := filepath.Join(droppedOut, "references", "hand-written.md")
	require.NoError(t, os.WriteFile(userFile, []byte("mine\n"), 0o644))

	generateProfile(t, tempDir, "narrow")

	assert.FileExists(t, userFile, "a hand-written file must never be deleted")
	assert.DirExists(t, filepath.Join(droppedOut, "references"),
		"a directory still holding a user file must not be pruned")
	assert.DirExists(t, droppedOut,
		"a directory whose child survives must not be pruned")
}

// TestGenerator_PruneStaysInsideProject guards the walk: pruning must never
// remove the project root, the .ai-rulez source tree, or anything above them,
// even when every generated file under an output root is gone.
func TestGenerator_PruneStaysInsideProject(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	tempDir := filepath.Join(parent, "project")
	require.NoError(t, os.MkdirAll(tempDir, 0o755))

	configDir := filepath.Join(tempDir, ".ai-rulez")
	skillDir := filepath.Join(configDir, "skills", "solo")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"4.0\"\nname = \"solo\"\npresets = [\"claude\"]\ngitignore = false\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\ndescription: solo\n---\nbody\n"), 0o644))

	generateProfile(t, tempDir, "default")
	require.FileExists(t, filepath.Join(tempDir, ".claude", "skills", "solo", "SKILL.md"))

	// Remove the only skill at the source, so the next run's stale pass empties
	// .claude/skills/ entirely.
	require.NoError(t, os.RemoveAll(skillDir))
	generateProfile(t, tempDir, "default")

	assert.NoDirExists(t, filepath.Join(tempDir, ".claude", "skills", "solo"))
	assert.DirExists(t, tempDir, "the project root must survive")
	assert.DirExists(t, configDir, "the .ai-rulez source tree must survive")
	assert.DirExists(t, parent, "nothing above the project root may be touched")
}
