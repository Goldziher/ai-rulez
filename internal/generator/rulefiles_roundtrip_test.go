package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func splitFrontmatter(t *testing.T, text string) (fm map[string]any, rest string) {
	t.Helper()
	if !strings.HasPrefix(text, "---\n") {
		return nil, text
	}
	end := strings.Index(text[4:], "\n---\n")
	require.GreaterOrEqual(t, end, 0)
	require.NoError(t, yaml.Unmarshal([]byte(text[4:4+end+1]), &fm))
	return fm, text[4+end+5:]
}

func TestRender_HashInjectionRoundTrip(t *testing.T) {
	targets := []rulefiles.Target{
		{Preset: "claude", Ext: ".md", Dialect: rulefiles.DialectClaude, Banner: true},
		{Preset: "cursor", Ext: ".mdc", Dialect: rulefiles.DialectCursor},
		{Preset: "windsurf", Ext: ".md", Dialect: rulefiles.DialectTrigger, Banner: true},
		{Preset: "copilot", Ext: ".instructions.md", Dialect: rulefiles.DialectCopilot, Banner: true},
		{Preset: "cline", Ext: ".md", Dialect: rulefiles.DialectCline, Banner: true},
		{Preset: "continue", Ext: ".md", Dialect: rulefiles.DialectContinue, Banner: true},
		{Preset: "junie", Ext: ".md", Dialect: rulefiles.DialectJunie, Banner: true},
	}
	modes := []config.ActivationMode{
		config.ActivationAlways, config.ActivationGlob, config.ActivationAuto, config.ActivationManual,
	}
	cfg := &config.Config{ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez"}
	const tricky = "Text with an arrow --> and a rule\n\n---\n\nmore after.\n\n-->\n\nend"

	for _, tg := range targets {
		for _, mode := range modes {
			t.Run(tg.Preset+"/"+string(mode), func(t *testing.T) {
				it := rulefiles.Item{
					File: config.ContentFile{
						Name: "Go Style", Path: "/p/.ai-rulez/rules/go.md", Content: tricky,
						Metadata: &config.Metadata{Priority: "high"},
					},
					ID: "Go-Style",
					Activation: config.Activation{
						Mode: mode, Globs: []string{"*.{ts,tsx}", "!vendor/**"}, Description: "use: when\nneeded",
					},
				}
				text, _, err := rulefiles.Render(tg, it, cfg)
				require.NoError(t, err)
				path := filepath.Join(t.TempDir(), "Go-Style"+tg.Ext)

				body := stripHeader(text, path)
				final := injectHashes(text, path, templates.HashContent(body), "src")

				assert.Equal(t, body, stripHeader(final, path))
				assert.Contains(t, body, tricky)
				require.NoError(t, os.WriteFile(path, []byte(final), 0o600))
				contentHash, sourceHash := extractStoredHashes(path)
				assert.Equal(t, templates.HashContent(body), contentHash)
				assert.Equal(t, "src", sourceHash)

				wantFM, _ := rulefiles.Frontmatter(tg.Dialect, it)
				gotFM, rest := splitFrontmatter(t, final)
				if wantFM == nil {
					assert.Nil(t, gotFM)
				} else {
					wantYAML, err := yaml.Marshal(wantFM)
					require.NoError(t, err)
					var want map[string]any
					require.NoError(t, yaml.Unmarshal(wantYAML, &want))
					assert.Equal(t, want, gotFM)
				}
				assert.NotContains(t, rest[strings.Index(rest, "# Go Style"):], "Content-Hash")

				// A second generation pass renders and injects again: same bytes.
				text2, _, err := rulefiles.Render(tg, it, cfg)
				require.NoError(t, err)
				assert.Equal(t, final, injectHashes(text2, path, templates.HashContent(stripHeader(text2, path)), "src"))
			})
		}
	}
}
