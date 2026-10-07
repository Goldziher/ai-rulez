package airulez_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

var sources = map[string]string{
	".ai-rulez/config.toml":            "version = \"5.0\"\nname = \"svc\"\npresets = [\"claude\", \"cursor\", \"codex\"]\nagents_md = false\n\n[permissions]\nallow = [\"Bash(go test:*)\"]\n",
	".ai-rulez/rules/style.md":         "---\npriority: high\n---\n# Style\n\nBe concise.\n",
	".ai-rulez/context/arch.md":        "# Architecture\n\nA CLI.\n",
	".ai-rulez/skills/review/SKILL.md": "---\nname: review\ndescription: Review code\n---\nLook for bugs.\n",
}

func writeSources(t *testing.T, dir string) {
	t.Helper()
	for name, content := range sources {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

func commit(t *testing.T, dir string) string {
	t.Helper()
	git := func(args ...string) string {
		all := append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		cmd := gitutil.CommandNoContext(dir, all...)
		cmd.Env = append(gitutil.Env(nil), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	git("init", "-q", ".")
	git("add", "-A")
	git("commit", "-q", "-m", "sources")
	return git("rev-parse", "HEAD")[:40]
}

func memWith(files map[string]string) *airulez.MemWorkspace {
	ws := airulez.NewMemWorkspace()
	for name, content := range files {
		ws.Set(name, content, 0o644)
	}
	return ws
}

func TestSameSourcesGiveTheSamePlanFromEveryWorkspace(t *testing.T) {
	// Arrange: the sources on a disk that holds no outputs, in memory and in a commit.
	dir := t.TempDir()
	writeSources(t, dir)
	rev := commit(t, dir)
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	snapshot, err := airulez.GitSnapshot(t.Context(), dir, rev, nil)
	require.NoError(t, err)
	workspaces := map[string]airulez.Workspace{"disk": disk, "memory": memWith(sources), "git snapshot": snapshot}

	// Act
	plans := map[string]*airulez.Plan{}
	for name, ws := range workspaces {
		project, err := airulez.Load(t.Context(), airulez.Options{Workspace: ws})
		require.NoError(t, err, name)
		plans[name], err = project.Plan(t.Context(), airulez.PlanOptions{})
		require.NoError(t, err, name)
	}

	// Assert: byte-identical documents, not merely equal digests.
	want, err := plans["disk"].JSON()
	require.NoError(t, err)
	assert.NotEmpty(t, plans["disk"].Files)
	for _, name := range []string{"memory", "git snapshot"} {
		got, err := plans[name].JSON()
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), name)
		assert.Equal(t, plans["disk"].Digest, plans[name].Digest, name)
	}
}

func TestPlanWritesNothingAndIsStable(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeSources(t, dir)
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk})
	require.NoError(t, err)

	// Act
	first, err := project.Plan(t.Context(), airulez.PlanOptions{})
	require.NoError(t, err)
	second, err := project.Plan(t.Context(), airulez.PlanOptions{})
	require.NoError(t, err)

	// Assert
	assert.Equal(t, first.Digest, second.Digest)
	_, statErr := os.Stat(filepath.Join(dir, "CLAUDE.md"))
	assert.True(t, os.IsNotExist(statErr), "planning must not write the outputs")
	var paths []string
	for _, f := range first.Files {
		paths = append(paths, f.Path)
	}
	assert.Contains(t, paths, "CLAUDE.md")
	assert.Contains(t, paths, ".cursor/rules/style.mdc")
}

func TestGenerateNeedsADirectoryToWrite(t *testing.T) {
	// Arrange
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: memWith(sources)})
	require.NoError(t, err)

	// Act
	_, err = project.Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.Write})

	// Assert
	var apiErr *airulez.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, airulez.CodeDiskRequired, apiErr.Code)
	assert.True(t, errors.Is(err, airulez.ErrDiskRequired))
}

func TestGenerateModesOnADirectory(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeSources(t, dir)
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	load := func() *airulez.Project {
		p, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk})
		require.NoError(t, err)
		return p
	}

	// Act and assert, in the order a caller would use them.
	dry, err := load().Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.DryRun})
	require.NoError(t, err)
	assert.Contains(t, dry.Lines, "write-file: CLAUDE.md")
	_, statErr := os.Stat(filepath.Join(dir, "CLAUDE.md"))
	assert.True(t, os.IsNotExist(statErr))

	before, err := load().Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.Check})
	require.NoError(t, err)
	assert.NotEmpty(t, before.Drift)

	written, err := load().Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.Write})
	require.NoError(t, err)
	assert.Positive(t, written.Written)
	assert.FileExists(t, filepath.Join(dir, "CLAUDE.md"))

	after, err := load().Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.Check})
	require.NoError(t, err)
	assert.Empty(t, after.Drift, "a project that was just generated has nothing to report")
}

func TestLoadRejectsWhatItCannotUse(t *testing.T) {
	// A missing workspace and a workspace without a configuration are load errors.
	_, err := airulez.Load(t.Context(), airulez.Options{})
	var apiErr *airulez.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, airulez.CodeLoad, apiErr.Code)

	_, err = airulez.Load(t.Context(), airulez.Options{Workspace: airulez.NewMemWorkspace()})
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, airulez.CodeLoad, apiErr.Code)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		config string
		wantOK bool
	}{
		{name: "a valid configuration", config: sources[".ai-rulez/config.toml"], wantOK: true},
		{name: "an unknown preset", config: "version = \"5.0\"\nname = \"x\"\npresets = [\"no-such-tool\"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{".ai-rulez/config.toml": tt.config}
			project, err := airulez.Load(t.Context(), airulez.Options{Workspace: memWith(files)})
			if err != nil {
				require.False(t, tt.wantOK, "load failed: %v", err)
				return
			}

			// Act
			report, err := project.Validate(t.Context(), airulez.ValidateOptions{})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, report.OK(), "%+v", report.Findings)
		})
	}
}

func TestStrictValidationNeedsADirectory(t *testing.T) {
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: memWith(sources)})
	require.NoError(t, err)

	_, err = project.Validate(t.Context(), airulez.ValidateOptions{Strict: true})

	var apiErr *airulez.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, airulez.CodeDiskRequired, apiErr.Code)
}

func TestTwoProjectsPlanConcurrently(t *testing.T) {
	// Arrange
	a := memWith(sources)
	other := map[string]string{}
	for k, v := range sources {
		other[k] = v
	}
	other[".ai-rulez/rules/style.md"] = "# Style\n\nA different project.\n"
	b := memWith(other)

	// Act
	digests := make([]string, 2)
	errs := make([]error, 2)
	done := make(chan int)
	for i, ws := range []airulez.Workspace{a, b} {
		go func() {
			defer func() { done <- i }()
			project, err := airulez.Load(t.Context(), airulez.Options{Workspace: ws})
			if err != nil {
				errs[i] = err
				return
			}
			plan, err := project.Plan(t.Context(), airulez.PlanOptions{})
			if err != nil {
				errs[i] = err
				return
			}
			digests[i] = plan.Digest
		}()
	}
	<-done
	<-done

	// Assert
	require.NoError(t, errors.Join(errs...))
	assert.NotEqual(t, digests[0], digests[1])
}
