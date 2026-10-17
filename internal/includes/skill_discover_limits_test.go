package includes

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// writeNumberedSkills writes n skill directories named s0000, s0001, ... below dir.
func writeNumberedSkills(t *testing.T, dir string, n int) {
	t.Helper()
	for i := range n {
		skill := filepath.Join(dir, fmt.Sprintf("s%04d", i))
		require.NoError(t, os.MkdirAll(skill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skill, skillMarkerFile), []byte("# s\n"), 0o644))
	}
}

func TestDiscoverSkillDirsEnforcesTheSkillLimit(t *testing.T) {
	tests := []struct {
		name    string
		count   int
		include []string
		wantLen int
		wantErr string
	}{
		{"exactly the limit is accepted", DefaultMaxSkills, nil, DefaultMaxSkills, ""},
		{"one over the limit is refused", DefaultMaxSkills + 1, nil, 0, "holds 201 skills, more than the limit of 200"},
		{"include narrows below the limit", DefaultMaxSkills + 50, []string{"s000*"}, 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeNumberedSkills(t, root, tt.count)
			// Act
			dirs, err := discoverSkillDirs(t.Context(), []string{root}, tt.include, nil)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, dirs, tt.wantLen)
		})
	}
}

func TestDiscoverSkillDirsStaysQuietBelowTheWalkCap(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeNumberedSkills(t, root, 5)
	rec := &testutil.LogRecorder{}
	// Act
	dirs, err := discoverSkillDirs(logger.WithContext(t.Context(), rec), []string{root}, nil, nil)
	// Assert
	require.NoError(t, err)
	assert.Len(t, dirs, 5)
	assert.Empty(t, rec.Level("WARN"))
}

func TestDiscoverSkillDirsDepthBoundary(t *testing.T) {
	tests := []struct {
		name  string
		depth int
		found bool
	}{
		{"a skill at the root", 0, true},
		{"a skill at the last searched level", maxSkillDepth, true},
		{"a skill one level too deep", maxSkillDepth + 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			skill := root
			for range tt.depth {
				skill = filepath.Join(skill, "d")
			}
			writeSkill(t, skill, "deepskill")
			// Act
			dirs, err := discoverSkillDirs(t.Context(), []string{root}, nil, nil)
			// Assert
			require.NoError(t, err)
			if tt.found {
				assert.Equal(t, []string{skill}, dirs)
			} else {
				assert.Empty(t, dirs)
			}
		})
	}
}

func TestDiscoverSkillDirsNeverFollowsASymlinkLoop(t *testing.T) {
	// Arrange: skills/loop points back at the root of the walk
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "real"), "real")
	testutil.SymlinkOrSkip(t, root, filepath.Join(root, "skills", "loop"))
	// Act
	dirs, err := discoverSkillDirs(t.Context(), []string{root}, nil, nil)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "skills", "real")}, dirs)
}

func TestDiscoverSkillDirsSkipsHiddenAndVendoredDirectories(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "visible"), "visible")
	writeSkill(t, filepath.Join(root, ".hidden", "skill"), "hidden")
	writeSkill(t, filepath.Join(root, "node_modules", "dep", "skill"), "vendored")
	writeSkill(t, filepath.Join(root, "group", ".git", "skill"), "git")
	// Act
	dirs, err := discoverSkillDirs(t.Context(), []string{root}, nil, nil)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "visible")}, dirs)
}

func TestDiscoverSkillDirsDuplicatesKeepTheShallowestThenTheEarlierRoot(t *testing.T) {
	// Arrange
	base := t.TempDir()
	first, second := filepath.Join(base, "first"), filepath.Join(base, "second")
	writeSkill(t, filepath.Join(first, "a", "b", "dup"), "deep")
	writeSkill(t, filepath.Join(second, "dup"), "shallow")
	writeSkill(t, filepath.Join(first, "tie"), "first")
	writeSkill(t, filepath.Join(second, "tie"), "second")
	rec := &testutil.LogRecorder{}
	// Act
	dirs, err := discoverSkillDirs(logger.WithContext(t.Context(), rec), []string{first, second}, nil, nil)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(second, "dup"), filepath.Join(first, "tie")}, dirs)
	warns := rec.Level("WARN")
	require.Len(t, warns, 2)
	assert.Contains(t, strings.Join(warns, "\n"), "name=dup")
	assert.Contains(t, strings.Join(warns, "\n"), "skipped="+filepath.Join(first, "a", "b", "dup"))
	assert.Contains(t, strings.Join(warns, "\n"), "skipped="+filepath.Join(second, "tie"))
}

func TestDiscoverSkillDirsIgnoresADuplicateTheEntryDidNotSelect(t *testing.T) {
	// Arrange: the shallow dup is excluded, so it must not shadow the deeper one
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "dup"), "shallow")
	writeSkill(t, filepath.Join(root, "g", "keep"), "keep")
	rec := &testutil.LogRecorder{}
	// Act
	dirs, err := discoverSkillDirs(logger.WithContext(t.Context(), rec), []string{root}, nil, []string{"dup"})
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "g", "keep")}, dirs)
	assert.Empty(t, rec.Level("WARN"))
}

func TestDiscoverSkillDirsToleratesAMissingRoot(t *testing.T) {
	// Act
	dirs, err := discoverSkillDirs(t.Context(), []string{filepath.Join(t.TempDir(), "gone")}, nil, nil)
	// Assert
	require.NoError(t, err)
	assert.Empty(t, dirs)
}

func TestScanEntrySkillsSkipsASkillItCannotReadAndKeepsTheRest(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("windows and root can read a file with no permission bits")
	}
	// Arrange
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "good"), "good")
	writeSkill(t, filepath.Join(root, "bad"), "bad")
	require.NoError(t, os.Chmod(filepath.Join(root, "bad", skillMarkerFile), 0o000))
	rec := &testutil.LogRecorder{}
	// Act
	skills, err := scanEntrySkills(logger.WithContext(t.Context(), rec), root, false, "", "entry", []string{"*"}, nil)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"good"}, resolvedNames(skills))
	require.Len(t, rec.Level("WARN"), 1)
	assert.Contains(t, rec.String(), "Skipping a skill in an installed skill entry")
}

func TestScanEntrySkillsTreatsASymlinkedMarkerAsNoSkill(t *testing.T) {
	// Arrange
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "real.md")
	require.NoError(t, os.WriteFile(target, []byte("# x\n"), 0o644))
	testutil.SymlinkOrSkip(t, target, filepath.Join(root, skillMarkerFile))
	// Act
	skills, err := scanEntrySkills(t.Context(), root, false, "", "entry", nil, nil)
	// Assert
	require.NoError(t, err, "an entry that selects by name alone and finds nothing resolves empty; callers check holdsSkills first")
	assert.Empty(t, skills)
}
