package publish

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestBuildAggregate_HoldsTheIndexAndEmitterFilesAndVerifiesBySums(t *testing.T) {
	x := Extras{
		Channel: "stable", Pin: pinFor(claudeIndex), PinCommit: commit40,
		Emit: &EmitRequest{Names: []string{"port"}, Experimental: true},
		Plugins: []emit.Plugin{{
			Name: "acme", Version: "1.4.0",
			Files: []emit.File{{Path: "skills/deploy/SKILL.md", Data: []byte("---\nname: deploy\n---\n")}},
		}},
	}

	d, err := BuildAggregate("acme-market", x)

	require.NoError(t, err)
	assert.Contains(t, d.Files, "marketplace/stable/.claude-plugin/marketplace.json")
	assert.Contains(t, d.Files, "emit/port/index.json")
	assert.Equal(t, "acme-market", d.Plan.Name)
	assert.NotEmpty(t, d.Warnings)
	schemaValid(t, "publish-plan.schema.json", d.Files[PlanFile])
	dir := filepath.Join(t.TempDir(), "aggregate")
	require.NoError(t, d.Write(dir))
	res, err := VerifySums(dir)
	require.NoError(t, err)
	assert.True(t, res.OK(), "%v", res.Problems)
	appendTo(t, filepath.Join(dir, "emit", "port", "index.json"), " ")
	res, err = VerifySums(dir)
	require.NoError(t, err)
	assert.False(t, res.OK())
}

func TestBuildAggregate_NeedsSomethingToAggregate(t *testing.T) {
	_, err := BuildAggregate("m", Extras{})

	var pe *Error
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, CodeConfig, pe.Code)
}

func TestPreviousTag(t *testing.T) {
	describe := []string{"git", "-C", "/p", "describe", "--tags", "--abbrev=0"}
	tests := []struct {
		name    string
		exclude string
		result  runner.Result
		want    string
		argv    []string
	}{
		{"closest tag", "v1.4.0", runner.Result{Status: runner.StatusOK, Stdout: []byte("v1.3.0\n")}, "v1.3.0",
			append(append([]string{}, describe...), "--exclude", "v1.4.0", "HEAD")},
		{"no tags", "", runner.Result{Status: runner.StatusExit, ExitCode: 128}, "",
			append(append([]string{}, describe...), "HEAD")},
		{"option-looking tag is dropped", "", runner.Result{Status: runner.StatusOK, Stdout: []byte("--force\n")}, "",
			append(append([]string{}, describe...), "HEAD")},
		{"unsafe exclude is not passed", "-x", runner.Result{Status: runner.StatusOK, Stdout: []byte("v1\n")}, "v1",
			append(append([]string{}, describe...), "HEAD")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &runner.Fake{Handle: func(runner.Spec) runner.Result { return tt.result }}

			got := PreviousTag(context.Background(), fake, "/p", tt.exclude)

			assert.Equal(t, tt.want, got)
			require.Len(t, fake.Calls(), 1)
			assert.Equal(t, tt.argv, fake.Calls()[0].Argv)
		})
	}
}
