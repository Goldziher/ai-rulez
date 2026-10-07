package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const improveCases = `schema_version: 1
cases:
  - id: t1
    prompt: Deploy billing to staging
    expect_trigger: true
    assertions: [{type: contains, value: OK}]
  - id: t2
    prompt: Deploy search to staging
    expect_trigger: true
    assertions: [{type: contains, value: OK}]
  - id: h1
    prompt: Ship the api to staging
    expect_trigger: true
    tags: [holdout]
    assertions: [{type: contains, value: OK}]
  - id: h2
    prompt: Push web to staging
    expect_trigger: true
    tags: [holdout]
    assertions: [{type: contains, value: OK}]
  - id: h3
    prompt: What is the capital of France?
    expect_trigger: false
    tags: [holdout]
`

// improveRunner is an eval command runner whose cases pass only when the skill
// it is given says IMPROVED, so a candidate that adds the word wins.
const improveRunner = `#!/bin/sh
req=$(cat)
dir=$(printf '%s' "$req" | sed -n 's/.*"dir": *"\([^"]*\)".*/\1/p' | head -1)
ok=NO
if grep -q IMPROVED "$dir/SKILL.md" 2>/dev/null; then ok=OK; fi
ids=$(printf '%s' "$req" | grep -o '"id": *"[a-z0-9.-]*"' | sed 's/.*"\([^"]*\)"$/\1/' | grep -v '^deploy$')
out='{"version":1,"results":['
sep=
for id in $ids; do
  trig=true; case $id in h3) trig=false;; esac
  out="$out$sep{\"case\":\"$id\",\"arm\":\"with\",\"triggered\":$trig,\"output\":\"$ok\",\"cost_usd\":0.001}"
  sep=,
done
echo "$out],\"cost_usd\":0.001}"
`

// Optimizers: one rewrites the skill so it wins, one changes nothing, and one
// tries to read the held-out cases and leak them into the skill.
const (
	optimizerImprove = `#!/bin/sh
cat > /dev/null
sed 's/Run make deploy./IMPROVED: say OK./' deploy/SKILL.md > deploy/SKILL.md.new && mv deploy/SKILL.md.new deploy/SKILL.md
echo '{"version":1,"summary":"say OK","changed":["SKILL.md"],"cost_usd":0.01}'
`
	optimizerNoop = `#!/bin/sh
cat > /dev/null
echo '{"version":1,"summary":"nothing","changed":[],"cost_usd":0}'
`
)

func improveProject(t *testing.T, env *isoEnv) (root, tools string) {
	t.Helper()
	root = minimalProject(t, "")
	writeTree(t, root, map[string]string{".ai-rulez/skills/deploy/evals/deploy.eval.yaml": improveCases})
	env.commitAll(root, "init")
	tools = t.TempDir()
	writeExec(t, filepath.Join(tools, "runner.sh"), improveRunner)
	writeExec(t, filepath.Join(tools, "improve.sh"), optimizerImprove)
	writeExec(t, filepath.Join(tools, "noop.sh"), optimizerNoop)
	return root, tools
}

type improveReport struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
	Rounds []struct {
		Decision string `json:"decision"`
	} `json:"rounds"`
}

func TestImproveRunE2E(t *testing.T) {
	tests := []struct {
		name       string
		optimizer  string
		extra      []string
		mutate     func(t *testing.T, root string)
		wantExit   int
		wantStatus string
	}{
		{name: "a winning candidate is accepted", optimizer: "improve.sh", wantStatus: "accepted"},
		{name: "a candidate that changes nothing is not accepted", optimizer: "noop.sh", wantExit: 2, wantStatus: "no-candidate"},
		{name: "--dry-run runs nothing", optimizer: "improve.sh", extra: []string{"--dry-run"}},
		{name: "--max-cost is required", optimizer: "improve.sh", extra: []string{"--max-cost", "0"}, wantExit: 1},
		{
			name: "uncommitted eval changes are refused", optimizer: "improve.sh",
			mutate: func(t *testing.T, root string) {
				writeTree(t, root, map[string]string{".ai-rulez/skills/deploy/evals/deploy.eval.yaml": improveCases + "  - id: t3\n    prompt: x\n    expect_trigger: true\n"})
			},
			wantExit: 1,
		},
		{name: "an unknown skill is refused", optimizer: "improve.sh", extra: []string{"--max-rounds", "1"}, wantExit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			root, tools := improveProject(t, env)
			if tt.mutate != nil {
				tt.mutate(t, root)
			}
			skill := "deploy"
			if tt.name == "an unknown skill is refused" {
				skill = "nope"
			}
			args := append([]string{"improve", "run", skill, "--with", filepath.Join(tools, tt.optimizer),
				"--runner-command", filepath.Join(tools, "runner.sh"), "--max-cost", "1", "--runs", "1", "--yes", "--format", "json"}, tt.extra...)
			original, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"))
			require.NoError(t, err)

			// Act
			res := env.run(root, args...)

			// Assert
			require.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			after, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"))
			require.NoError(t, err)
			assert.Equal(t, string(original), string(after), "run never touches the authored skill")
			if tt.wantStatus == "" {
				return
			}
			requireJSONDoc(t, res)
			var rep improveReport
			require.NoError(t, json.Unmarshal([]byte(res.Stdout), &rep))
			assert.Equal(t, tt.wantStatus, rep.Status)
			assert.NotContains(t, res.Stdout, "Ship the api to staging", "held-out prompts never reach the report the optimizer could read")
		})
	}
}

// TestImproveShowApplyCleanE2E: an accepted run is shown, applied into the
// authored skill without a commit, refused when another user's key signed it,
// and cleaned.
func TestImproveShowApplyCleanE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root, tools := improveProject(t, env)
	run := env.run(root, "improve", "run", "deploy", "--with", filepath.Join(tools, "improve.sh"),
		"--runner-command", filepath.Join(tools, "runner.sh"), "--max-cost", "1", "--runs", "1", "--yes", "--format", "json")
	require.Equal(t, 0, run.ExitCode, run.Stderr)
	var rep improveReport
	require.NoError(t, json.Unmarshal([]byte(run.Stdout), &rep))
	head := env.git(root, "rev-parse", "HEAD")

	// Act
	show := env.run(root, "improve", "show", rep.RunID, "--format", "json")
	foreign := newIsoEnv(t).run(root, "improve", "apply", rep.RunID, "--yes")
	apply := env.run(root, "improve", "apply", rep.RunID, "--yes")
	clean := env.run(root, "improve", "clean", rep.RunID)

	// Assert
	require.Equal(t, 0, show.ExitCode, show.Stderr)
	report, ok := requireJSONDoc(t, show)["report"].(map[string]any)
	require.True(t, ok, show.Stdout)
	assert.Equal(t, rep.RunID, report["run_id"])
	assert.NotEqual(t, 0, foreign.ExitCode, "a run signed by another user's key is refused: %s", foreign.Stdout)
	require.Equal(t, 0, apply.ExitCode, apply.Stderr)
	skill, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(skill), "IMPROVED: say OK.")
	assert.Equal(t, head, env.git(root, "rev-parse", "HEAD"), "apply never commits")
	require.Equal(t, 0, clean.ExitCode, clean.Stderr)
	_, err = os.Stat(filepath.Join(root, ".ai-rulez", "local", "improve", rep.RunID))
	assert.True(t, os.IsNotExist(err), "clean removes the saved run")
}

// TestImprovePRUsesAFakeGhE2E: improve pr pushes a branch from an isolated
// worktree to the origin remote and opens the PR through gh, which here is a
// fake that must resolve first on PATH.
func TestImprovePRUsesAFakeGhE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	fakes := env.fakeTool("gh", "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$(dirname \"$0\")/gh.log\"\necho https://github.com/example/repo/pull/1\n")
	root, tools := improveProject(t, env)
	origin := filepath.Join(t.TempDir(), "origin.git")
	env.git(root, "init", "-q", "--bare", origin)
	env.git(root, "remote", "add", "origin", origin)
	env.git(root, "push", "-q", "origin", "main")
	run := env.run(root, "improve", "run", "deploy", "--with", filepath.Join(tools, "improve.sh"),
		"--runner-command", filepath.Join(tools, "runner.sh"), "--max-cost", "1", "--runs", "1", "--yes", "--format", "json")
	require.Equal(t, 0, run.ExitCode, run.Stderr)
	var rep improveReport
	require.NoError(t, json.Unmarshal([]byte(run.Stdout), &rep))
	status := env.git(root, "status", "--porcelain", "--untracked-files=no")

	// Act
	pr := env.run(root, "improve", "pr", rep.RunID, "--yes", "--draft", "--format", "json")

	// Assert
	require.Equal(t, 0, pr.ExitCode, "stdout: %s\nstderr: %s", pr.Stdout, pr.Stderr)
	requireJSONDoc(t, pr)
	logged, err := os.ReadFile(filepath.Join(fakes, "gh.log"))
	require.NoError(t, err)
	assert.Contains(t, string(logged), "pr\ncreate\n")
	branches := env.git(origin, "branch", "--list")
	assert.Contains(t, branches, "ai-rulez/improve/deploy-", "the branch was pushed to origin")
	assert.Equal(t, status, env.git(root, "status", "--porcelain", "--untracked-files=no"), "the checkout is untouched")
	assert.False(t, strings.Contains(env.git(root, "log", "--oneline", "-1"), "improve"), "nothing is committed on the user's branch")
}

// TestImproveResolvesARelativeOptimizerFromTheWorkingDirectoryE2E covers MAN-1:
// --with ./optimize.sh (and the documented `--with 'python optimize.py'`) is
// resolved from the directory the command ran in, not the throwaway workspace.
func TestImproveResolvesARelativeOptimizerFromTheWorkingDirectoryE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root, tools := improveProject(t, env)
	writeExec(t, filepath.Join(root, "optimize.sh"), optimizerImprove)
	env.commitAll(root, "optimizer")

	// Act
	res := env.run(root, "improve", "run", "deploy", "--with", "./optimize.sh",
		"--runner-command", filepath.Join(tools, "runner.sh"), "--max-cost", "1", "--runs", "1", "--yes", "--format", "json")

	// Assert
	require.Equal(t, 0, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	var rep improveReport
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &rep))
	assert.Equal(t, "accepted", rep.Status)
}
