package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catalogMarkdownProject(t *testing.T) {
	t.Helper()
	addMarkdownSkill(t, rolesCmdProject(t))
}

func addMarkdownSkill(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".ai-rulez", "skills", "docs", "SKILL.md"),
		"---\nname: docs\ndescription: Use when writing docs.\n---\n# Guide\n\n- one\n- two\n\n<script>alert(1)</script>\n\n[click](javascript:alert(2)) **bold**\n")
}

func itemPage(t *testing.T, files map[string]string, id string) string {
	t.Helper()
	for name, page := range files {
		if strings.HasPrefix(name, "items/") && filepath.Base(name) == id+".html" {
			return page
		}
	}
	t.Fatalf("no page for %s", id)
	return ""
}

func TestCatalogHTMLRenderMarkdownFlag(t *testing.T) {
	// Arrange
	catalogMarkdownProject(t)
	resetCatalogFlags(t)
	dir := filepath.Join(t.TempDir(), "site")
	catalogHTMLDir, catalogMarkdown, catalogMarkdownSet = dir, true, true

	// Act
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	page := itemPage(t, treeOf(t, dir), "docs")
	assert.Contains(t, page, `<h3 dir="auto">Guide</h3>`)
	assert.Contains(t, page, `<li dir="auto">one</li>`)
	assert.Contains(t, page, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.NotContains(t, page, "<script>alert")
	assert.Contains(t, page, "javascript:alert(2)")
	assert.NotContains(t, page, `href="javascript`)
}

func TestCatalogHTMLMarkdownFromConfigAndFlagOverride(t *testing.T) {
	// Arrange
	catalogConfigProject(t, "render_markdown = true\n")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	addMarkdownSkill(t, cwd)
	resetCatalogFlags(t)
	dirOn := filepath.Join(t.TempDir(), "on")
	dirOff := filepath.Join(t.TempDir(), "off")

	// Act
	catalogHTMLDir = dirOn
	require.NoError(t, runCatalog(&bytes.Buffer{}))
	catalogHTMLDir, catalogMarkdown, catalogMarkdownSet = dirOff, false, true
	require.NoError(t, runCatalog(&bytes.Buffer{}))

	// Assert
	assert.Contains(t, itemPage(t, treeOf(t, dirOn), "docs"), `<div class="md">`)
	off := itemPage(t, treeOf(t, dirOff), "docs")
	assert.NotContains(t, off, `<div class="md">`)
	assert.Contains(t, off, "# Guide", "plain text keeps the source")
}

func TestCatalogMarkdownFlagNeedsHTML(t *testing.T) {
	// Arrange
	resetCatalogFlags(t)
	catalogMarkdownSet = true

	// Act
	err := checkCatalogFlags()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--render-markdown applies to --html only")
}
