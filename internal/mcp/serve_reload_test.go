package mcp

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncompleteSkillFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "empty", content: "", want: true},
		{name: "whitespace", content: " \n", want: true},
		{name: "part of the delimiter", content: "--", want: true},
		{name: "frontmatter not closed", content: "---\ndescription: half", want: true},
		{name: "frontmatter cut in the closing delimiter", content: "---\ndescription: x\n--", want: true},
		{name: "complete skill", content: skillFile("a", "A skill", ""), want: false},
		{name: "complete frontmatter, body cut", content: "---\ndescription: x\n---\n\n# a", want: false},
		{name: "no frontmatter at all", content: "# just a heading\n", want: false},
		{name: "crlf complete", content: "---\r\ndescription: x\r\n---\r\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := incompleteSkillFile([]byte(tt.content))

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

// RV-DYN-3: a rebuild during which the files changed may have read a save in
// progress, so it is not swapped in; the next tick builds from settled files.
func TestWatch_DiscardsABuildTheFilesChangedUnder(t *testing.T) {
	t.Parallel()
	// Arrange
	var fp atomic.Value
	fp.Store("v1")
	var builds atomic.Int32
	next := loadCatalog(t)
	srv := NewSkillServerWith("test", &Catalog{byName: map[string]*CatalogSkill{}, byURI: map[string]*CatalogSkill{}, byFile: map[string]*CatalogFile{}}, ServeOptions{
		PollInterval: 10 * time.Millisecond,
		Baseline:     "v1",
		Fingerprint:  func() (string, error) { return fp.Load().(string), nil },
		Rebuild: func() (*Catalog, error) {
			if builds.Add(1) == 1 {
				fp.Store("v3") // an edit lands while the first build runs
			}
			return next, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Watch(ctx)

	// Act
	fp.Store("v2")

	// Assert
	require.Eventually(t, func() bool { return len(srv.Catalog().Skills()) > 0 }, 5*time.Second, 5*time.Millisecond)
	assert.GreaterOrEqual(t, builds.Load(), int32(2), "the build the files changed under was swapped in")
}

// RV-DYN-3: an in-place save truncates SKILL.md before writing it. While the
// file is empty the previous version keeps serving; a deleted skill still goes.
func TestWatch_KeepsTheServedSkillWhileItsFileIsMidSave(t *testing.T) {
	tests := []struct {
		name       string
		edit       func(t *testing.T, path string)
		wantServed bool
	}{
		{name: "truncated", edit: func(t *testing.T, p string) { require.NoError(t, os.WriteFile(p, nil, 0o644)) }, wantServed: true},
		{name: "frontmatter not closed", edit: func(t *testing.T, p string) {
			require.NoError(t, os.WriteFile(p, []byte("---\ndescription: Core"), 0o644))
		}, wantServed: true},
		{name: "deleted", edit: func(t *testing.T, p string) { require.NoError(t, os.RemoveAll(filepath.Dir(p))) }, wantServed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := project(t, baseConfig, map[string]string{
				"skills/core/SKILL.md":  skillFile("core", "Core conventions", "delivery: served\n"),
				"skills/other/SKILL.md": skillFile("other", "Other skill", "delivery: served\n"),
			})
			setup := &ServeSetup{WorkDir: root, PollInterval: 10 * time.Millisecond, CacheDir: filepath.Join(t.TempDir(), "cache")}
			srv, err := setup.NewServer(context.Background())
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go srv.Watch(ctx)
			path := filepath.Join(root, ".ai-rulez", "skills", "core", "SKILL.md")

			// Act
			tt.edit(t, path)
			writeFile(t, root, ".ai-rulez/skills/other/SKILL.md", skillFile("other", "Other skill, edited", "delivery: served\n"))
			require.Eventually(t, func() bool {
				other, ok := srv.Catalog().Lookup("other")
				return ok && other.Description == "Other skill, edited" || tt.wantServed
			}, 5*time.Second, 10*time.Millisecond)
			time.Sleep(200 * time.Millisecond)

			// Assert
			core, served := srv.Catalog().Lookup("core")
			require.Equal(t, tt.wantServed, served)
			if !tt.wantServed {
				return
			}
			assert.Equal(t, "Core conventions", core.Description, "the previous version keeps serving")
			writeFile(t, root, ".ai-rulez/skills/core/SKILL.md", skillFile("core", "Core conventions, saved", "delivery: served\n"))
			require.Eventually(t, func() bool {
				core, ok := srv.Catalog().Lookup("core")
				other, _ := srv.Catalog().Lookup("other")
				return ok && core.Description == "Core conventions, saved" && other != nil && other.Description == "Other skill, edited"
			}, 5*time.Second, 10*time.Millisecond, "the completed save is picked up")
		})
	}
}
