package signing_test

import (
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func installedProject(t *testing.T, pub testSigner, skill config.ContentFile, requires ...string) (*config.Config, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "keys"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "keys", "publisher.pub"), pub.pubPEM, 0o644))
	dir := filepath.Join(root, ".ai-rulez", "skills", "deploy")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: deploy\n---\nShip it.\n"), 0o644))
	skill.Name, skill.Path = "deploy", filepath.Join(dir, "SKILL.md")
	return &config.Config{
		BaseDir: root, ConfigDir: filepath.Join(root, ".ai-rulez"),
		InstalledSkills: []config.InstalledSkillConfig{{Name: "deploy", Source: "https://example.org/skills.git"}},
		Content:         &config.ContentTree{Skills: []config.ContentFile{skill}},
		Signing: &config.SigningConfig{
			Require: requires,
			Trust:   []config.SigningTrust{{Subject: "skill", Source: "deploy", KeyFile: "keys/publisher.pub"}},
		},
	}, dir
}

func TestCheckInstalledSkills(t *testing.T) {
	tests := []struct {
		name     string
		requires []string
		sign     bool
		tamper   bool
		wantCode string
	}{
		{name: "not required is not checked", requires: nil},
		{name: "required and unsigned is AR720", requires: []string{"skill"}, wantCode: CodeMissing},
		{name: "required and signed passes", requires: []string{"skill"}, sign: true},
		{name: "signed then edited is AR724", requires: []string{"skill"}, sign: true, tamper: true, wantCode: CodeSubjectMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			pub := newTestSigner(t, "skill", "deploy")
			cfg, dir := installedProject(t, pub, config.ContentFile{}, tt.requires...)
			if tt.sign {
				data := signTree(t, pub, SubjectSkill, dir)
				require.NoError(t, os.WriteFile(filepath.Join(dir, SidecarName), data, 0o644))
			}
			if tt.tamper {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.sh"), []byte("echo hi"), 0o644))
			}

			// Act
			findings, err := CheckInstalledSkills(cfg, time.Now())

			// Assert
			require.NoError(t, err)
			if tt.wantCode == "" {
				assert.Empty(t, findings)
				return
			}
			require.Len(t, findings, 1)
			assert.Equal(t, "deploy", findings[0].Name)
			assert.Equal(t, tt.wantCode, findings[0].Err.Code)
		})
	}
}

func TestCheckInstalledSkillsLeavesServedSkillsToTheServer(t *testing.T) {
	// Arrange: a served installed skill is gated when it is served, not when generating.
	pub := newTestSigner(t, "skill", "deploy")
	served := config.ContentFile{Metadata: &config.Metadata{Extra: map[string]string{"delivery": "served"}}}
	cfg, _ := installedProject(t, pub, served, "skill")

	// Act
	findings, err := CheckInstalledSkills(cfg, time.Now())

	// Assert
	require.NoError(t, err)
	assert.Empty(t, findings)
}
