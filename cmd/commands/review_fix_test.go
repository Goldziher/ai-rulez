package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const (
	fixSkillPath = ".ai-rulez/skills/deploy/SKILL.md"
	fixSkillBody = "---\nname: deploy\ndescription: Helps with deployments\n---\nRun the deploy script and check the logs.\n"
	fixedDesc    = "Deploy a build to staging when asked to ship; not for rollbacks or production releases"
)

func reviewGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := gitutil.CommandNoContext("", append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.email=t@example.test", "-c", "user.name=t"}, args...)...) //nolint:gosec // test
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// fixProject is a committed project with one vague skill and a fake that judges "Helps with" as a
// failing trigger and writes a good description when asked to fix it.
func fixProject(t *testing.T) *fakeModel {
	t.Helper()
	judgedProject(t, "")
	reviewFlags.semantic = false
	require.NoError(t, reviewFixCmd.Flags().Set("max-cost", "0"))
	t.Cleanup(func() { reviewFixCmd.Flags().Lookup("max-cost").Changed = false })
	require.NoError(t, os.RemoveAll(".ai-rulez/skills/release"))
	require.NoError(t, os.RemoveAll(".ai-rulez/skills/leak"))
	require.NoError(t, os.WriteFile(fixSkillPath, []byte(fixSkillBody), 0o644)) //nolint:gosec // the mode is asserted below
	require.NoError(t, os.Chmod(fixSkillPath, 0o644))
	fixFlags.model, fixFlags.judgeModel, fixFlags.content, fixFlags.out = "fixer-model", "", "full", ""
	fixFlags.apply, fixFlags.allowSame, fixFlags.patch, fixFlags.finding, fixFlags.format = false, false, "", "", formatText
	t.Cleanup(func() {
		fixFlags.model, fixFlags.apply, fixFlags.patch, fixFlags.out = "", false, "", ""
		reviewFlags.content = ""
	})
	f := &fakeModel{}
	f.decide = func(_, desc, dim string) string {
		if dim == "trigger-quality" && strings.Contains(desc, "Helps with") {
			return "fail"
		}
		return "pass"
	}
	f.fixer = func(llm.ChatRequest) string {
		b, _ := json.Marshal(map[string]any{"edits": []map[string]string{{"old": "Helps with deployments", "new": fixedDesc}}, "note": "named the trigger and a non-trigger"})
		return string(b)
	}
	useFakeModel(t, f)
	reviewGit(t, "init", "-q")
	reviewGit(t, "add", "-A")
	reviewGit(t, "commit", "-q", "-m", "one")
	return f
}

func TestReviewFixProposesAPatchAndNeverWritesWithoutApply(t *testing.T) {
	// Arrange
	fixProject(t)
	var out bytes.Buffer

	// Act
	exit, err := runFix(reviewFixCmd, nil, &out)

	// Assert
	require.NoError(t, err)
	assert.Zero(t, exit)
	assert.Contains(t, out.String(), "# digest: sha256:")
	assert.Contains(t, out.String(), "+description: "+fixedDesc)
	data, rerr := os.ReadFile(fixSkillPath)
	require.NoError(t, rerr)
	assert.Equal(t, fixSkillBody, string(data), "nothing is written without --apply")
}

func TestReviewFixApplyIsIdempotentAndKeepsTheFileMode(t *testing.T) {
	// Arrange
	f := fixProject(t)
	fixFlags.apply = true

	// Act
	exit, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})
	callsAfterApply := f.calls
	var again, summary bytes.Buffer
	reviewFixCmd.SetErr(&summary)
	t.Cleanup(func() { reviewFixCmd.SetErr(nil) })
	exit2, err2 := runFix(reviewFixCmd, nil, &again)

	// Assert
	require.NoError(t, err)
	assert.Zero(t, exit)
	data, rerr := os.ReadFile(fixSkillPath)
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "description: "+fixedDesc)
	info, serr := os.Stat(fixSkillPath)
	require.NoError(t, serr)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "the write keeps the file's mode")
	require.NoError(t, err2)
	assert.Zero(t, exit2)
	assert.Empty(t, again.String(), "a second run proposes nothing: the empty patch")
	assert.Contains(t, summary.String(), "nothing to fix")
	assert.Equal(t, callsAfterApply, f.calls, "and calls no model: the rated-better text is cached")
}

func TestReviewFixApplyRefusesAFileWithUncommittedChanges(t *testing.T) {
	// Arrange
	fixProject(t)
	fixFlags.apply = true
	// The proposal is made on the committed text; the file is then edited before --apply writes.
	orig := reviewClientFactory
	edited := false
	reviewClientFactory = func(lc llm.Config, opts llm.Options) (llm.Client, error) {
		c, err := orig(lc, opts)
		if !edited {
			edited = true
			require.NoError(t, os.WriteFile(fixSkillPath, []byte(fixSkillBody+"\nA late edit.\n"), 0o644)) //nolint:gosec // test
		}
		return c, err
	}
	t.Cleanup(func() { reviewClientFactory = orig })

	// Act
	_, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})

	// Assert
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "stale patch") || strings.Contains(err.Error(), "uncommitted"), err.Error())
	data, rerr := os.ReadFile(fixSkillPath)
	require.NoError(t, rerr)
	assert.NotContains(t, string(data), fixedDesc)
}

func TestReviewFixFromAPatchChecksTheDigestAndTheGitState(t *testing.T) {
	// Arrange: write a patch, then apply it
	fixProject(t)
	patchFile := filepath.Join(t.TempDir(), "fix.patch")
	fixFlags.out = patchFile
	_, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})
	require.NoError(t, err)
	fixFlags.out = ""
	fixFlags.patch = patchFile

	t.Run("a clean file takes the patch only with --apply", func(t *testing.T) {
		_, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})
		require.NoError(t, err)
		data, _ := os.ReadFile(fixSkillPath)
		assert.Equal(t, fixSkillBody, string(data), "verification alone writes nothing")

		fixFlags.apply = true
		var out, summary bytes.Buffer
		reviewFixCmd.SetErr(&summary)
		t.Cleanup(func() { reviewFixCmd.SetErr(nil) })
		_, err = runFix(reviewFixCmd, nil, &out)
		require.NoError(t, err)
		data, _ = os.ReadFile(fixSkillPath)
		assert.Contains(t, string(data), fixedDesc)
		assert.Contains(t, summary.String(), "ai-rulez lock")
	})

	t.Run("a file that changed since is a stale patch", func(t *testing.T) {
		require.NoError(t, os.WriteFile(fixSkillPath, []byte(fixSkillBody+"\nedited\n"), 0o644)) //nolint:gosec // test
		fixFlags.apply = true

		_, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "stale patch")
	})
}

func TestReviewFixModelRules(t *testing.T) {
	t.Run("no fixer model", func(t *testing.T) {
		fixProject(t)
		fixFlags.model = ""

		_, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no fixer model")
	})

	t.Run("the fixer must differ from the judge", func(t *testing.T) {
		fixProject(t)
		fixFlags.model = "model"

		_, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "the fixer and the judge are both model")
	})

	t.Run("--allow-same-model lifts it", func(t *testing.T) {
		fixProject(t)
		fixFlags.model, fixFlags.allowSame = "model", true

		_, err := runFix(reviewFixCmd, nil, &bytes.Buffer{})

		require.NoError(t, err)
	})
}

func TestReviewFixSkipsContentThatIsNotAuthored(t *testing.T) {
	// Arrange: the machine-local overlay is not committed content
	fixProject(t)
	require.NoError(t, os.RemoveAll(".ai-rulez/skills"))
	require.NoError(t, os.MkdirAll(".ai-rulez/local/skills/mine", 0o755))
	require.NoError(t, os.WriteFile(".ai-rulez/local/skills/mine/SKILL.md", []byte(strings.Replace(fixSkillBody, "name: deploy", "name: mine", 1)), 0o600))
	var out bytes.Buffer

	// Act
	_, err := runFix(reviewFixCmd, nil, &out)

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "+description:")
}

// patchFor writes a patch that adds a line to the top of rel, with the digest of the file as it is.
func patchFor(t *testing.T, rel, line string) string {
	t.Helper()
	data, err := os.ReadFile(rel)
	require.NoError(t, err)
	orig := string(data)
	patched := line + "\n" + orig
	p := &rv.FixProposal{Item: "x", Path: rel, Digest: rv.TextDigest(orig), Verified: true, Patch: rv.UnifiedDiff(rel, orig, patched)}
	rubric, lerr := rv.Load(".ai-rulez", "")
	require.NoError(t, lerr)
	file := filepath.Join(t.TempDir(), "evil.patch")
	require.NoError(t, os.WriteFile(file, []byte(rv.RenderPatch([]*rv.FixProposal{p}, rubric, "f", "j")), 0o600))
	return file
}

func TestReviewFixPatchOnlyTouchesAuthoredItems(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		setup func(t *testing.T)
	}{
		{"the config file", ".ai-rulez/config.toml", nil},
		{"the lock file", ".ai-rulez/ai-rulez.lock", func(t *testing.T) {
			require.NoError(t, os.WriteFile(".ai-rulez/ai-rulez.lock", []byte("# lock\n"), 0o600))
		}},
		{"a script", ".ai-rulez/hooks/run.sh", func(t *testing.T) {
			require.NoError(t, os.MkdirAll(".ai-rulez/hooks", 0o755))
			require.NoError(t, os.WriteFile(".ai-rulez/hooks/run.sh", []byte("echo hi\n"), 0o700))
		}},
		{"a calibration record", ".ai-rulez/calibration/skill-quality.builtin.json", func(t *testing.T) {
			require.NoError(t, os.MkdirAll(".ai-rulez/calibration", 0o755))
			require.NoError(t, os.WriteFile(".ai-rulez/calibration/skill-quality.builtin.json", []byte("{}\n"), 0o600))
		}},
		{"the machine-local overlay in another case", ".ai-rulez/Local/skills/mine/SKILL.md", func(t *testing.T) {
			require.NoError(t, os.MkdirAll(".ai-rulez/Local/skills/mine", 0o755))
			require.NoError(t, os.WriteFile(".ai-rulez/Local/skills/mine/SKILL.md", []byte(strings.Replace(fixSkillBody, "name: deploy", "name: mine", 1)), 0o600))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fixProject(t)
			if tt.setup != nil {
				tt.setup(t)
			}
			reviewGit(t, "add", "-A")
			reviewGit(t, "commit", "-q", "--allow-empty", "-m", "more")
			before, err := os.ReadFile(tt.path)
			require.NoError(t, err)
			fixFlags.patch, fixFlags.apply = patchFor(t, tt.path, "# injected"), true

			// Act
			_, err = runFix(reviewFixCmd, nil, &bytes.Buffer{})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not an authored item")
			after, rerr := os.ReadFile(tt.path)
			require.NoError(t, rerr)
			assert.Equal(t, string(before), string(after))
		})
	}
}

func TestRequireCleanInGitSeesAnUncommittedChangeThroughASymlinkedPath(t *testing.T) {
	// Arrange: a repository reached through a link, as /var is /private/var on macOS
	real := t.TempDir()
	real, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	link := filepath.Join(t.TempDir(), "alias")
	testutil.SymlinkOrSkip(t, real, link)
	file := filepath.Join(real, "item.md")
	require.NoError(t, os.WriteFile(file, []byte("one\n"), 0o600))
	t.Chdir(real)
	reviewGit(t, "init", "-q")
	reviewGit(t, "add", "item.md")
	reviewGit(t, "commit", "-q", "-m", "one")
	require.NoError(t, os.WriteFile(file, []byte("two\n"), 0o600))

	// Act
	err = requireCleanInGit(filepath.Join(link, "item.md"))

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}
