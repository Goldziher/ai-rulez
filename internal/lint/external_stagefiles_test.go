package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stagedListing runs a scanner that reports the files it finds below its skill
// directory and returns that listing.
func stagedListing(t *testing.T, p *scannerProject, opts Options) string {
	t.Helper()
	got := scannerFindings(p.run(opts))
	require.Len(t, got, 1, "one skill, one finding")
	_, listing, ok := strings.Cut(got[0].Message, "files=")
	require.True(t, ok, got[0].Message)
	return listing
}

// skillTreeProject is a project whose skill "deploy" keeps files all over its directory.
func skillTreeProject(t *testing.T, keys string) *scannerProject {
	t.Helper()
	p := fakeProject(t, isolationNone, fakeScannerCommand(t, "--list-files", "{skill_dirs}")+"inputs = [\"skills\"]\n"+keys, "deploy")
	for rel, body := range map[string]string{
		"notes.md":                 "root note\n",
		"scripts/run.sh":           "#!/bin/sh\n",
		"references/api.md":        "api\n",
		"rules/extra.md":           "extra\n",
		"templates/deploy.yaml":    "a: 1\n",
		".hidden":                  "hidden\n",
		"evals/case.json":          "{}\n",
		"node_modules/dep/main.js": "x\n",
		"__pycache__/mod.pyc":      "x\n",
		"id_rsa":                   "-----BEGIN OPENSSH PRIVATE KEY-----\n",
		"server.pem":               "pem\n",
	} {
		p.write(".ai-rulez/skills/deploy/"+rel, body)
	}
	return p
}

func TestEveryRegularFileOfASkillDirectoryIsStaged(t *testing.T) {
	// Arrange
	p := skillTreeProject(t, "")

	// Act
	listing := stagedListing(t, p, Options{})

	// Assert: files at the root and in any subdirectory arrive; bundle exclusions and credential-like names do not.
	want := ".hidden,SKILL.md,evals/case.json,notes.md,references/api.md,rules/extra.md,scripts/run.sh,templates/deploy.yaml"
	assert.Equal(t, want, listing)
}

func TestSymlinksAndOversizedFilesAreNotStaged(t *testing.T) {
	// Arrange
	p := skillTreeProject(t, "")
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("secret\n"), 0o644)) //nolint:gosec // a test file
	testutil.SymlinkOrSkip(t, outside, filepath.Join(p.root, ".ai-rulez", "skills", "deploy", "link.md"))
	require.NoError(t, os.WriteFile(filepath.Join(p.root, ".ai-rulez", "skills", "deploy", "huge.bin"), make([]byte, 9<<20), 0o644)) //nolint:gosec // a test file

	// Act
	listing := stagedListing(t, p, Options{})

	// Assert
	assert.NotContains(t, listing, "link.md")
	assert.NotContains(t, listing, "huge.bin")
	assert.Contains(t, listing, "notes.md")
}

func TestStageGlobsAndExcludeSelectSkillFiles(t *testing.T) {
	tests := []struct {
		name string
		keys string
		want string
	}{
		{"stage_globs keeps SKILL.md and the matches", "stage_globs = [\"references/**\", \"notes.md\"]\n", "SKILL.md,notes.md,references/api.md"},
		{"stage_exclude drops the matches", "stage_exclude = [\"scripts/**\", \"evals/**\", \".*\", \"templates/**\", \"rules/**\"]\n", "SKILL.md,notes.md,references/api.md"},
		{"stage_exclude never drops SKILL.md", "stage_exclude = [\"**\"]\n", "SKILL.md"},
		{"exclude wins over globs", "stage_globs = [\"**\"]\nstage_exclude = [\"**/*.sh\", \"evals/**\", \".*\", \"templates/**\", \"rules/**\"]\n", "SKILL.md,notes.md,references/api.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := skillTreeProject(t, tt.keys)

			// Act
			listing := stagedListing(t, p, Options{})

			// Assert
			assert.Equal(t, tt.want, listing)
		})
	}
}

func TestStageGlobsRejectsAMalformedPattern(t *testing.T) {
	// Arrange
	p := skillTreeProject(t, "stage_globs = [\"refs/[a\"]\nstage_exclude = [\"/abs\", \"../up/*\"]\n")

	// Act
	findings := p.run(Options{})

	// Assert
	var messages []string
	for _, f := range ofCode(findings, CodeScannerConfigInvalid) {
		messages = append(messages, f.Message)
	}
	joined := strings.Join(messages, "\n")
	assert.Contains(t, joined, "stage_globs")
	assert.Contains(t, joined, "stage_exclude")
	assert.Empty(t, scannerFindings(findings))
}

func TestDryRunListsTheFilesTheScannerWouldSee(t *testing.T) {
	// Arrange
	p := skillTreeProject(t, "stage_exclude = [\"scripts/**\"]\n")
	var out bytes.Buffer

	// Act
	p.run(Options{Scanner: ScannerOptions{DryRun: true, Out: &out}})

	// Assert
	plan := out.String()
	assert.Contains(t, plan, ".ai-rulez/skills/deploy/notes.md")
	assert.Contains(t, plan, ".ai-rulez/skills/deploy/rules/extra.md")
	assert.NotContains(t, plan, "scripts/run.sh")
	assert.NotContains(t, plan, "node_modules")
}
