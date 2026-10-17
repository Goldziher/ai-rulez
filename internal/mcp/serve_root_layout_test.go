package mcp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const rootServeBase = "version = \"5.0\"\nname = \"solo\"\ngitignore = false\npresets = [\"claude\"]\n"

// rootServeProject is a root-layout project (ai-rulez.toml alone) with a skill
// source directory vendor/ inside it.
func rootServeProject(t *testing.T, extra string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "vendor/skills/alpha/SKILL.md", skillFile("alpha", "Alpha skill", ""))
	writeFile(t, root, "ai-rulez.toml", rootServeBase+"\n[[skill_sources]]\nname = \"vendor\"\nurl = \""+filepath.ToSlash(filepath.Join(root, "vendor"))+"\"\npath = \"skills\"\ntrust = \"warn\"\n"+extra)
	return root
}

func TestServeSetup_RootLayoutServesItsSkillSources(t *testing.T) {
	// Arrange
	root := rootServeProject(t, "")

	// Act
	srv := newServerFor(t, &ServeSetup{WorkDir: root})

	// Assert
	assert.Equal(t, []string{"alpha"}, catalogNames(srv.Catalog()))
	assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
}

func TestServeSetup_ProjectFingerprintOfARootLayout(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(root string)
		changed bool
	}{
		{"an unrelated project file", func(root string) { writeFile(t, root, "src/main.go", "package main\n") }, false},
		{"a hand-written instruction file", func(root string) { writeFile(t, root, "CLAUDE.md", "# Mine\n") }, false},
		{"the configuration", func(root string) { writeFile(t, root, "ai-rulez.toml", rootServeBase+"description = \"edited\"\n") }, true},
		{"the local overlay", func(root string) { writeFile(t, root, "ai-rulez.local.toml", "description = \"mine\"\n") }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootServeProject(t, "")
			setup := &ServeSetup{WorkDir: root}
			baseline, err := setup.projectFingerprint()
			require.NoError(t, err)

			// Act
			tt.edit(root)
			got, err := setup.projectFingerprint()

			// Assert
			require.NoError(t, err)
			if tt.changed {
				assert.NotEqual(t, baseline, got)
			} else {
				assert.Equal(t, baseline, got)
			}
		})
	}
}

func TestServeSetup_WatchRootsOfARootLayout(t *testing.T) {
	// Arrange
	root := rootServeProject(t, "")
	writeFile(t, root, "ai-rulez.local.toml", "description = \"mine\"\n")
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	require.NoError(t, err)
	require.True(t, cfg.Contentless())

	// Act
	got := (&ServeSetup{}).watchRoots(&built{cfg: cfg})

	// Assert: the two config files, never the whole project root
	assert.Equal(t, []string{filepath.Join(root, "ai-rulez.toml"), filepath.Join(root, "ai-rulez.local.toml")}, got)
}

func TestServeSetup_UsageLogPathOfARootLayoutIsHidden(t *testing.T) {
	tests := []struct {
		name  string
		setup ServeSetup
		extra string
		want  func(root string) string
	}{
		{
			"skills_index on", ServeSetup{}, "\n[usage]\nskills_index = true\n",
			func(root string) string { return filepath.Join(root, ".ai-rulez-local.d", "usage.jsonl") },
		},
		{"skills_index off", ServeSetup{}, "", func(string) string { return "" }},
		{
			"an explicit log wins", ServeSetup{UsageLog: "/tmp/explicit.jsonl"}, "\n[usage]\nskills_index = true\n",
			func(string) string { return "/tmp/explicit.jsonl" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootServeProject(t, tt.extra)
			cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
			require.NoError(t, err)

			// Act
			got := tt.setup.usageLogPath(cfg)

			// Assert
			assert.Equal(t, tt.want(root), got)
		})
	}
}

func TestWatch_ReloadsARootLayoutWhenItsConfigChanges(t *testing.T) {
	// Arrange
	root := rootServeProject(t, "")
	setup := &ServeSetup{WorkDir: root, PollInterval: 10 * time.Millisecond, CacheDir: filepath.Join(t.TempDir(), "cache")}
	srv, err := setup.NewServer(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Watch(ctx)
	require.Equal(t, []string{"alpha"}, catalogNames(srv.Catalog()))
	writeFile(t, root, "vendor/skills/beta/SKILL.md", skillFile("beta", "Beta skill", ""))

	// Act: the new skill is in the source; touching ai-rulez.toml reloads the catalog
	writeFile(t, root, "ai-rulez.toml", rootServeBase+"description = \"edited\"\n\n[[skill_sources]]\nname = \"vendor\"\nurl = \""+
		filepath.ToSlash(filepath.Join(root, "vendor"))+"\"\npath = \"skills\"\ntrust = \"warn\"\n")

	// Assert
	require.Eventually(t, func() bool {
		return len(catalogNames(srv.Catalog())) == 2
	}, 5*time.Second, 10*time.Millisecond, "the config edit was never reloaded")
}
