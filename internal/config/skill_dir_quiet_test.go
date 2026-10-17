package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// A hand-written skill is only linted, so a subdirectory generate would not copy is no warning;
// an installed skill is bundled whole, and one narrowed by `resources` still names what is left out.
func TestUnrecognizedSkillSubdirectoryWarning(t *testing.T) {
	tests := []struct {
		name     string
		load     func(t *testing.T, root, skillDir string, log logger.Logger)
		wantWarn bool
	}{
		{"linting a hand-written skill", func(t *testing.T, root, skillDir string, log logger.Logger) {
			_, ok := LoadSkillDir(logger.WithContext(t.Context(), log), root, skillDir)
			require.True(t, ok)
		}, false},
		{"loading an installed skill bundles hooks/ whole", func(t *testing.T, _, skillDir string, log logger.Logger) {
			_, err := LoadSkillResourcesContext(logger.WithContext(t.Context(), log), skillDir)
			require.NoError(t, err)
		}, false},
		{"resources none leaves hooks/ out on purpose", resourcesLoader("none"), false},
		{"resources that exclude hooks/ by name", resourcesLoader("*", "!hooks"), false},
		{"resources that bundle hooks/ by name", resourcesLoader("hooks"), false},
		{"resources that do not name hooks/ still warn", resourcesLoader("rules"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			skillDir := filepath.Join(root, ".claude", "skills", "x")
			require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "hooks"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: x\ndescription: d\n---\nbody\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(skillDir, "hooks", "a.sh"), []byte("#!/bin/sh\n"), 0o755))
			log := &recordingLogger{}

			// Act
			tt.load(t, root, skillDir, log)

			// Assert
			assert.Equal(t, tt.wantWarn, strings.Contains(strings.Join(log.warns, "\n"), "Unrecognized skill subdirectory"), log.warns)
		})
	}
}

// resourcesLoader loads an installed skill with the given `resources` patterns.
func resourcesLoader(patterns ...string) func(t *testing.T, root, skillDir string, log logger.Logger) {
	return func(t *testing.T, _, skillDir string, log logger.Logger) {
		ctx := WithSkillResources(logger.WithContext(t.Context(), log), patterns)
		_, err := LoadSkillResourcesContext(ctx, skillDir)
		require.NoError(t, err)
	}
}
