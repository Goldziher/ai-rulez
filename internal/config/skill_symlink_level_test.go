package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// levelLogger records the level of each message.
type levelLogger struct{ warns, errors []string }

func (l *levelLogger) Debug(string, ...any)       {}
func (l *levelLogger) Info(string, ...any)        {}
func (l *levelLogger) Warn(msg string, _ ...any)  { l.warns = append(l.warns, msg) }
func (l *levelLogger) Error(msg string, _ ...any) { l.errors = append(l.errors, msg) }

func TestInstalledSkillSymlinkIsRefusedWithAWarning(t *testing.T) {
	// Arrange
	skillDir := filepath.Join(t.TempDir(), "skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: x\ndescription: d\n---\nbody\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "real.md"), []byte("real\n"), 0o644))
	testutil.SymlinkOrSkip(t, filepath.Join(skillDir, "real.md"), filepath.Join(skillDir, "link.md"))
	log := &levelLogger{}

	// Act
	_, err := LoadSkillResourcesContext(logger.WithContext(t.Context(), log), skillDir)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, log.errors)
	assert.Contains(t, log.warns, "Refusing symlink in included content; symlinks are not followed")
}
