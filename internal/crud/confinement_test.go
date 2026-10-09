package crud_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func symlinkedDomainProject(t *testing.T) (op *crud.OperatorImpl, outside string) {
	t.Helper()
	base := setupTestProject(t)
	outside = t.TempDir()
	testutil.SymlinkOrSkip(t, outside, filepath.Join(base, ".ai-rulez", "domains", "evil"))
	op, err := crud.NewOperator(base)
	require.NoError(t, err)
	return op, outside
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing may be written through the symlink")
}

func TestWritesRefuseASymlinkedDomain(t *testing.T) {
	ctx := context.Background()
	op, outside := symlinkedDomainProject(t)

	_, err := op.AddRule(ctx, &crud.AddFileRequest{Name: "pwn", Domain: "evil", Content: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	_, err = op.AddSkill(ctx, &crud.AddFileRequest{Name: "pwn", Domain: "evil", Content: "x"})
	require.Error(t, err)
	_, err = op.AddContext(ctx, &crud.AddFileRequest{Name: "pwn", Domain: "evil", Content: "x"})
	require.Error(t, err)
	_, err = op.AddAgent(ctx, &crud.AddFileRequest{Name: "pwn", Domain: "evil", Content: "x"})
	require.Error(t, err)
	_, err = op.UpdateFile(ctx, "evil", "rules", "pwn", "x", "", nil)
	require.Error(t, err)
	assertEmptyDir(t, outside)
}

func TestWritesRefuseASymlinkedContentDirectory(t *testing.T) {
	ctx := context.Background()
	for _, sub := range []string{"rules", "skills"} {
		t.Run(sub, func(t *testing.T) {
			base := setupTestProject(t)
			outside := t.TempDir()
			dir := filepath.Join(base, ".ai-rulez", sub)
			require.NoError(t, os.RemoveAll(dir))
			testutil.SymlinkOrSkip(t, outside, dir)
			op, err := crud.NewOperator(base)
			require.NoError(t, err)

			if sub == "rules" {
				_, err = op.AddRule(ctx, &crud.AddFileRequest{Name: "pwn", Content: "x"})
			} else {
				_, err = op.AddSkill(ctx, &crud.AddFileRequest{Name: "pwn", Content: "x"})
			}
			require.Error(t, err)
			assertEmptyDir(t, outside)
		})
	}
}

func TestReadsAndDeletesRefuseASymlinkedContentFile(t *testing.T) {
	ctx := context.Background()
	base := setupTestProject(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("TOPSECRET"), 0o600))
	link := filepath.Join(base, ".ai-rulez", "rules", "leak.md")
	testutil.SymlinkOrSkip(t, secret, link)
	op, err := crud.NewOperator(base)
	require.NoError(t, err)

	_, err = op.ReadFileContent(link)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "TOPSECRET")

	files, err := op.ListFiles(ctx, "", "rules")
	require.NoError(t, err)
	assert.Empty(t, files, "a symlinked file is not listed")

	require.Error(t, op.RequireContent(ctx, "", "rules", "leak"))
	_, err = op.UpdateFile(ctx, "", "rules", "leak", "x", "", nil)
	require.Error(t, err)
	require.Error(t, op.RemoveFile(ctx, "", "rules", "leak"))
	got, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "TOPSECRET", string(got))
}

func TestSymlinkedSkillDirectoryIsRefused(t *testing.T) {
	ctx := context.Background()
	base := setupTestProject(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("---\nname: s\n---\nbody\n"), 0o600))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(base, ".ai-rulez", "skills", "s"))
	op, err := crud.NewOperator(base)
	require.NoError(t, err)

	files, err := op.ListFiles(ctx, "", "skills")
	require.NoError(t, err)
	assert.Empty(t, files)
	require.Error(t, op.RemoveFile(ctx, "", "skills", "s"))
	_, err = os.Stat(filepath.Join(outside, "SKILL.md"))
	require.NoError(t, err)
}

func TestReservedGeneratedNames(t *testing.T) {
	ctx := context.Background()
	base := setupTestProject(t)
	op, err := crud.NewOperator(base)
	require.NoError(t, err)

	for _, name := range []string{"index", "log", "Index"} {
		_, err := op.AddRule(ctx, &crud.AddFileRequest{Name: name, Content: "x"})
		require.Error(t, err, name)
		_, err = op.AddContext(ctx, &crud.AddFileRequest{Name: name, Content: "x"})
		require.Error(t, err, name)
		_, err = op.AddAgent(ctx, &crud.AddFileRequest{Name: name, Content: "x"})
		require.Error(t, err, name)
	}

	// A generated index.md is not content.
	require.NoError(t, os.WriteFile(filepath.Join(base, ".ai-rulez", "rules", "index.md"), []byte("# Rules\n\n* [x](x.md)\n"), 0o600))
	_, err = op.AddRule(ctx, &crud.AddFileRequest{Name: "real", Content: "x"})
	require.NoError(t, err)
	files, err := op.ListFiles(ctx, "", "rules")
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "real", files[0].Name)
	require.Error(t, op.RequireContent(ctx, "", "rules", "index"))
	require.Error(t, op.RemoveFile(ctx, "", "rules", "index"))
	_, err = op.UpdateFile(ctx, "", "rules", "index", "x", "", nil)
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(base, ".ai-rulez", "rules", "index.md"))
	require.NoError(t, err)
}

func TestMissingItemHintDoesNotTalkAboutRemoval(t *testing.T) {
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)
	err = op.RequireContent(context.Background(), "", "rules", "nope")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "to remove")
	assert.NotContains(t, crudHint(err), "to remove")
}

func TestFileInfoJSONKeysAreLowerCase(t *testing.T) {
	data, err := json.Marshal(crud.FileInfo{Name: "a", Path: "p", Type: "rules", Domain: "d", Priority: "high", Targets: []string{"claude"}})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	for _, key := range []string{"name", "path", "type", "domain", "priority", "targets"} {
		assert.Contains(t, m, key)
	}
}

func TestUpdateFileKeepsPreviousFrontmatter(t *testing.T) {
	ctx := context.Background()
	op, err := crud.NewOperator(setupTestProject(t))
	require.NoError(t, err)

	rule, err := op.AddRule(ctx, &crud.AddFileRequest{Name: "r", Content: "first", Priority: "critical", Targets: []string{"claude", "cursor"}})
	require.NoError(t, err)
	_, err = op.UpdateFile(ctx, "", "rules", "r", "second", "", nil)
	require.NoError(t, err)
	got, err := os.ReadFile(rule.FullPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "priority: critical")
	assert.Contains(t, string(got), "cursor")
	assert.Contains(t, string(got), "second")

	// A given flag still overrides, the rest stays.
	_, err = op.UpdateFile(ctx, "", "rules", "r", "third", "high", nil)
	require.NoError(t, err)
	got, err = os.ReadFile(rule.FullPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "priority: high")
	assert.Contains(t, string(got), "cursor")

	skill, err := op.AddSkill(ctx, &crud.AddFileRequest{Name: "deploy", Content: "---\nname: deploy\ndescription: Ships it\nlicense: MIT\n---\n\nold"})
	require.NoError(t, err)
	_, err = op.UpdateFile(ctx, "", "skills", "deploy", "new steps", "", nil)
	require.NoError(t, err)
	got, err = os.ReadFile(skill.FullPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "Ships it")
	assert.Contains(t, string(got), "MIT")
	assert.Contains(t, string(got), "new steps")
}

// crudHint is the hint attached to err, "" when it has none.
func crudHint(err error) string {
	type hinter interface{ Hint() string }
	var h hinter
	if errors.As(err, &h) {
		return h.Hint()
	}
	return ""
}
