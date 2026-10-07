package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
)

func TestLLMsTxtLintFiles(t *testing.T) {
	cfg := deliveryProject(t, `["claude", "llms-txt"]`, "\n[llms_txt]\ndir = \"docs\"\n", nil)
	assert.Empty(t, llmsTxtLintFiles(cfg), "a file that is not generated yet is not a finding")

	require.NoError(t, os.MkdirAll(filepath.Join(cfg.BaseDir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfg.BaseDir, "docs", "llms.txt"), []byte("# P\n\n## S\n\n- not a link\n"), 0o644))
	files := llmsTxtLintFiles(cfg)
	require.Len(t, files, 1)
	assert.Equal(t, filepath.Join("docs", "llms.txt"), files[0].Path)
	require.Len(t, files[0].Findings, 1)
	assert.Equal(t, llmstxt.CodeLinkEntryInvalid, files[0].Findings[0].Code)

	plain := deliveryProject(t, `["claude"]`, "", nil)
	assert.Empty(t, llmsTxtLintFiles(plain), "no preset, no check")
}
