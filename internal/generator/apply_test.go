package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const applyConfig = `version = "5.0"
name = "apply"
presets = ["claude", "cursor", "codex"]
gitignore = true
`

func newApplyGenerator(t *testing.T, dir string) *Generator {
	t.Helper()
	return NewGenerator(loadPlanConfig(t, dir, nil))
}

func TestPlanThenApplyWritesWhatGenerateWrites(t *testing.T) {
	// Arrange: two identical projects.
	direct := planProject(t, applyConfig)
	split := planProject(t, applyConfig)

	// Act
	written, err := newApplyGenerator(t, direct).GenerateFiles("")
	require.NoError(t, err)
	g := newApplyGenerator(t, split)
	plan, err := g.Plan("")
	require.NoError(t, err)
	res, err := g.Apply(plan, DiskApplier)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, written, res.Written)
	assert.NotEmpty(t, plan.Outputs)
	assert.Equal(t, planTree(t, direct), planTree(t, split))
}

func TestPlanWritesNothing(t *testing.T) {
	// Arrange
	dir := planProject(t, applyConfig)
	before := planTree(t, dir)
	g := newApplyGenerator(t, dir)

	// Act
	plan, err := g.Plan("")

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, plan.Outputs)
	assert.Equal(t, before, planTree(t, dir))
}

func TestAppliers(t *testing.T) {
	tests := []struct {
		name  string
		after func(t *testing.T, dir string, g *Generator, p *RunPlan)
	}{
		{
			name: "dry run lists the writes and changes nothing",
			after: func(t *testing.T, dir string, g *Generator, p *RunPlan) {
				before := planTree(t, dir)
				res, err := g.Apply(p, DryRunApplier)
				require.NoError(t, err)
				assert.Equal(t, "profile: default", res.Lines[0])
				assert.Contains(t, res.Lines, "write-file: CLAUDE.md")
				assert.Equal(t, before, planTree(t, dir))
			},
		},
		{
			name: "check reports every file missing before the first generate",
			after: func(t *testing.T, dir string, g *Generator, p *RunPlan) {
				res, err := g.Apply(p, CheckApplier)
				require.NoError(t, err)
				require.NotEmpty(t, res.Drift)
				for _, d := range res.Drift {
					assert.Equal(t, DriftMissing, d.Kind, d.Path)
				}
			},
		},
		{
			name: "describe reports the plan document",
			after: func(t *testing.T, dir string, g *Generator, p *RunPlan) {
				res, err := g.Apply(p, DescribeApplier)
				require.NoError(t, err)
				require.NotNil(t, res.Document)
				assert.Equal(t, PlanSchema, res.Document.Schema)
				assert.NotEmpty(t, res.Document.Files)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := planProject(t, applyConfig)
			g := newApplyGenerator(t, dir)
			plan, err := g.Plan("")
			require.NoError(t, err)

			// Act and assert
			tt.after(t, dir, g, plan)
		})
	}
}

func TestCheckApplierIsCleanAfterTheDiskApplier(t *testing.T) {
	// Arrange
	dir := planProject(t, applyConfig)
	_, err := newApplyGenerator(t, dir).GenerateFiles("")
	require.NoError(t, err)
	g := newApplyGenerator(t, dir)
	plan, err := g.Plan("")
	require.NoError(t, err)

	// Act
	res, err := g.Apply(plan, CheckApplier)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, res.Drift)
}

func TestApplyRefusesAPlanFromAnotherGenerator(t *testing.T) {
	// Arrange
	dir := planProject(t, applyConfig)
	plan, err := newApplyGenerator(t, dir).Plan("")
	require.NoError(t, err)

	// Act
	_, err = newApplyGenerator(t, dir).Apply(plan, DiskApplier)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not made by this generator")
	_, err = newApplyGenerator(t, dir).Apply(nil, DiskApplier)
	require.Error(t, err)
}
