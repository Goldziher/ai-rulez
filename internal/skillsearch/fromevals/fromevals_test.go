package fromevals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

func TestDerive(t *testing.T) {
	t.Parallel()
	// Arrange
	root := t.TempDir()
	write(t, root, "skills/refunds/SKILL.md", "---\nname: refunds\ndescription: refunds\n---\n")
	write(t, root, "skills/refunds/evals/cases.eval.yaml", `cases:
  - id: money-back
    prompt: the customer wants money back
    expect_trigger: true
    tags: [billing]
    near_miss:
      - how do I charge a customer
  - id: unrelated
    prompt: write a haiku
    expect_trigger: false
  - id: empty-prompt
    expect_trigger: true
`)
	write(t, root, "domains/ops/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: deploy\n---\n")
	write(t, root, "domains/ops/skills/deploy/evals/deploy.eval.yaml", "id: ship\nprompt: ship it to prod\nexpect_trigger: true\n")
	write(t, root, "skills/silent/SKILL.md", "---\nname: silent\ndescription: x\n---\n")

	// Act
	got, err := Derive(root)

	// Assert
	require.NoError(t, err)
	byID := map[string]skillsearch.Case{}
	for _, c := range got.Cases {
		byID[c.ID] = c
	}
	pos := byID["refunds/money-back"]
	assert.Equal(t, "the customer wants money back", pos.Query)
	assert.Equal(t, []string{"refunds"}, pos.ExpectIDs())
	assert.Equal(t, []string{Tag, "billing"}, pos.Tags)
	near := byID["refunds/money-back.near-miss-1"]
	assert.Empty(t, near.Expect, "a near miss expects nothing")
	assert.Equal(t, []string{"refunds"}, near.Avoid, "and the skill must not rank first")
	assert.Contains(t, near.Tags, Tag)
	assert.Equal(t, []string{"refunds"}, byID["refunds/unrelated"].Avoid)
	assert.Equal(t, []string{"deploy"}, byID["deploy/ship"].ExpectIDs())
	assert.NotContains(t, byID, "refunds/empty-prompt")
	assert.Len(t, got.Cases, 4)
}

func TestDerive_NoEvalsIsEmpty(t *testing.T) {
	t.Parallel()
	got, err := Derive(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, got.Cases)
}

// The catalog keys skills by their frontmatter name, which need not be the directory name.
func TestDerive_UsesTheFrontmatterNameNotTheDirectoryName(t *testing.T) {
	t.Parallel()
	// Arrange
	root := t.TempDir()
	write(t, root, "skills/dir-name/SKILL.md", "---\nname: catalog-name\ndescription: d\n---\n")
	write(t, root, "skills/dir-name/evals/a.eval.yaml", "id: one\nprompt: do the thing\nexpect_trigger: true\n")
	write(t, root, "skills/no-name/SKILL.md", "---\ndescription: d\n---\n")
	write(t, root, "skills/no-name/evals/a.eval.yaml", "id: two\nprompt: do another\nexpect_trigger: false\n")

	// Act
	got, err := Derive(root)

	// Assert
	require.NoError(t, err)
	byID := map[string]skillsearch.Case{}
	for _, c := range got.Cases {
		byID[c.ID] = c
	}
	assert.Equal(t, []string{"catalog-name"}, byID["dir-name/one"].ExpectIDs(), "expected skills use the name the catalog serves")
	assert.Equal(t, []string{"no-name"}, byID["no-name/two"].Avoid, "a skill without a name falls back to its directory")
}
