package presets

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderSkillResourcesIndex(t *testing.T) {
	t.Parallel()

	t.Run("empty when no resources", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", RenderSkillResourcesIndex(&config.ContentFile{}))
	})

	t.Run("groups by kind and renders markdown links", func(t *testing.T) {
		t.Parallel()
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindReferences, RelPath: "references/api.md", Description: "API endpoints"},
				{Kind: config.SkillKindReferences, RelPath: "references/patterns.md", Description: ""},
				{Kind: config.SkillKindScripts, RelPath: "scripts/run.sh"},
				{Kind: config.SkillKindAssets, RelPath: "assets/logo.png"},
			},
		}
		out := RenderSkillResourcesIndex(&skill)

		// Section heading and explanation present.
		assert.Contains(t, out, "## Resources")
		assert.Contains(t, out, "Read them on demand")

		// References subsection: link uses relative path so Claude / Codex can
		// follow it without knowing the absolute skill dir.
		assert.Contains(t, out, "### References")
		assert.Contains(t, out, "[`references/api.md`](references/api.md) — API endpoints")
		// No-description references render without the em-dash trailer.
		assert.Contains(t, out, "[`references/patterns.md`](references/patterns.md)\n")
		assert.NotContains(t, out, "patterns.md`](references/patterns.md) —")

		// Scripts and assets render as plain code spans, no link.
		assert.Contains(t, out, "### Scripts")
		assert.Contains(t, out, "`scripts/run.sh`")
		assert.Contains(t, out, "### Assets")
		assert.Contains(t, out, "`assets/logo.png`")
	})

	t.Run("omits empty kind sections", func(t *testing.T) {
		t.Parallel()
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindReferences, RelPath: "references/x.md"},
			},
		}
		out := RenderSkillResourcesIndex(&skill)
		assert.Contains(t, out, "### References")
		assert.NotContains(t, out, "### Scripts")
		assert.NotContains(t, out, "### Assets")
	})
}

func TestSkillResourceOutputs(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when no resources", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, SkillResourceOutputs(&config.ContentFile{}, "/tmp/skills/x"))
	})

	t.Run("emits one OutputFile per resource plus parent dir markers", func(t *testing.T) {
		t.Parallel()
		raw := []byte{0x00, 0xff, 0x00, 0xff}
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindReferences, RelPath: "references/api.md", Content: []byte("text\n")},
				{Kind: config.SkillKindAssets, RelPath: "assets/blob.bin", Content: raw},
			},
		}
		base := filepath.Join("base", ".claude", "skills", "demo")
		outputs := SkillResourceOutputs(&skill, base)

		dirs := map[string]bool{}
		files := map[string]config.OutputFile{}
		for _, o := range outputs {
			if o.IsDir {
				dirs[o.Path] = true
			} else {
				files[o.Path] = o
			}
		}

		// Parent kind directories are emitted as IsDir entries so
		// cleanManagedDirs walks them on the next regeneration.
		assert.True(t, dirs[filepath.Join(base, "references")],
			"expected references/ dir marker, got dirs=%v", dirs)
		assert.True(t, dirs[filepath.Join(base, "assets")],
			"expected assets/ dir marker")

		// Files are emitted with raw bytes and empty Content.
		ref := files[filepath.Join(base, "references", "api.md")]
		assert.Equal(t, []byte("text\n"), ref.RawContent)
		assert.Empty(t, ref.Content)

		blob := files[filepath.Join(base, "assets", "blob.bin")]
		assert.Equal(t, raw, blob.RawContent)
	})

	t.Run("absolute RelPath is skipped instead of looping forever", func(t *testing.T) {
		t.Parallel()
		// LoadSkillResources never produces an absolute RelPath, but the
		// walk-up loop must defend against it because it would otherwise
		// hang indefinitely (filepath.Dir on an absolute path converges
		// at "/" and never reaches ".").
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindReferences, RelPath: "/etc/passwd", Content: []byte("x")},
			},
		}
		done := make(chan []config.OutputFile, 1)
		go func() { done <- SkillResourceOutputs(&skill, "/skill") }()
		select {
		case outputs := <-done:
			// Any malformed entries pass through as files but no parent
			// directory markers are emitted (the walk-up was skipped).
			for _, o := range outputs {
				assert.False(t, o.IsDir, "no parent dirs should be emitted for absolute RelPath")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("SkillResourceOutputs hung on absolute RelPath")
		}
	})

	t.Run("emits parent directories for nested resource paths", func(t *testing.T) {
		t.Parallel()
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindReferences, RelPath: "references/api/v1/users.md", Content: []byte("x")},
			},
		}
		outputs := SkillResourceOutputs(&skill, "/skill")

		var dirs []string
		for _, o := range outputs {
			if o.IsDir {
				dirs = append(dirs, o.Path)
			}
		}
		// Every parent on the way up to skillDir must be emitted so
		// cleanManagedDirs can sweep them all.
		assert.Contains(t, dirs, filepath.Join("/skill", "references"))
		assert.Contains(t, dirs, filepath.Join("/skill", "references", "api"))
		assert.Contains(t, dirs, filepath.Join("/skill", "references", "api", "v1"))
	})

	t.Run("each emitted file is independent of source bytes", func(t *testing.T) {
		t.Parallel()
		original := []byte("seed")
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindReferences, RelPath: "references/a.md", Content: original},
			},
		}
		outputs := SkillResourceOutputs(&skill, "/tmp")

		var fileOut *config.OutputFile
		for i := range outputs {
			if !outputs[i].IsDir {
				fileOut = &outputs[i]
				break
			}
		}
		require.NotNil(t, fileOut)

		// Mutating the original must not bleed into the OutputFile.
		original[0] = 'X'
		assert.Equal(t, byte('s'), fileOut.RawContent[0],
			"OutputFile.RawContent must be a copy, not an alias")
	})
}

func TestInlineSkillResources(t *testing.T) {
	t.Parallel()

	t.Run("empty when no resources", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", InlineSkillResources(&config.ContentFile{}))
	})

	t.Run("inlines references with frontmatter stripped", func(t *testing.T) {
		t.Parallel()
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{
					Kind:    config.SkillKindReferences,
					RelPath: "references/api.md",
					Content: []byte("---\ndescription: x\n---\n\nAPI body.\n"),
				},
			},
		}
		out := InlineSkillResources(&skill)
		assert.Contains(t, out, "## Reference: api")
		assert.Contains(t, out, "API body.")
		// Frontmatter is hidden from the inlined view.
		assert.NotContains(t, out, "description: x")
	})

	t.Run("ignores scripts and assets", func(t *testing.T) {
		t.Parallel()
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindScripts, RelPath: "scripts/x.sh", Content: []byte("#!/bin/sh\n")},
				{Kind: config.SkillKindAssets, RelPath: "assets/y.png", Content: []byte{0x89}},
			},
		}
		assert.Equal(t, "", InlineSkillResources(&skill))
	})

	t.Run("skips empty reference bodies", func(t *testing.T) {
		t.Parallel()
		skill := config.ContentFile{
			Resources: []config.SkillResource{
				{Kind: config.SkillKindReferences, RelPath: "references/empty.md", Content: []byte("---\nx: y\n---\n\n  \n")},
				{Kind: config.SkillKindReferences, RelPath: "references/full.md", Content: []byte("Body.\n")},
			},
		}
		out := InlineSkillResources(&skill)
		assert.NotContains(t, out, "## Reference: empty")
		assert.Contains(t, out, "## Reference: full")
	})
}

func TestReferenceDisplayName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"references/api.md":         "api",
		"references/api/v1.md":      "api/v1",
		"references/no-extension":   "no-extension",
		"references/.dotfile.md":    ".dotfile",
		"references/nested/deep.md": "nested/deep",
	}
	for in, want := range cases {
		got := referenceDisplayName(in)
		if !strings.EqualFold(got, want) {
			t.Errorf("referenceDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}
