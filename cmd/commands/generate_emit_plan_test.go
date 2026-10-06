package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckEmitPlanFlags(t *testing.T) {
	tests := []struct {
		name    string
		set     func()
		wantErr string
	}{
		{name: "no plan, nothing to check", set: func() { generateEmitPlan = "" }},
		{name: "plan alone", set: func() {}},
		{name: "plan with a profile and dry-run", set: func() { profile = "backend"; dryRun = true }},
		{name: "watch", set: func() { generateWatch = true }, wantErr: "--watch"},
		{name: "check", set: func() { generateCheck = true }, wantErr: "--check"},
		{name: "user", set: func() { userScope = true }, wantErr: "--user"},
		{name: "recursive", set: func() { recursive = true }, wantErr: "--recursive"},
		{name: "plugin", set: func() { pluginMode = true }, wantErr: "--plugin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetEmitPlanFlags(t)
			generateEmitPlan = "-"
			tt.set()

			// Act
			err := checkEmitPlanFlags()

			// Assert
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "--emit-plan")
		})
	}
}

func resetEmitPlanFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		generateEmitPlan, profile = "", ""
		generateWatch, generateCheck, userScope, recursive, pluginMode, dryRun = false, false, false, false, false, false
	}
	reset()
	t.Cleanup(reset)
}

func TestEmitPlanRunsTheReadOnlyPreflight(t *testing.T) {
	// Arrange: an unknown key, which --strict turns into an error before anything is planned
	cfg := preflightProject(t, preflightBase+"[lock]\nenforc = true\n", "")
	resetEmitPlanFlags(t)
	generateStrict = true
	t.Cleanup(func() { generateStrict = false })

	// Act
	err := emitPlan(context.Background(), cfg, filepath.Join(t.TempDir(), "plan.json"))

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown or invalid")
}

func TestEmitPlanRejectsAnUnknownRole(t *testing.T) {
	// Arrange
	cfg := preflightProject(t, preflightBase, "")
	resetEmitPlanFlags(t)
	generateRole = "nope"
	t.Cleanup(func() { generateRole = "" })
	dest := filepath.Join(t.TempDir(), "plan.json")

	// Act
	err := emitPlan(context.Background(), cfg, dest)

	// Assert
	require.Error(t, err)
	assert.NoFileExists(t, dest)
}

func TestEmitPlanWritesNothingButThePlan(t *testing.T) {
	// Arrange
	cfg := preflightProject(t, preflightBase, "")
	resetEmitPlanFlags(t)
	dest := filepath.Join(t.TempDir(), "plan.json")

	// Act
	err := emitPlan(context.Background(), cfg, dest)

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, dest)
	entries, err := os.ReadDir(cfg.BaseDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.Equal(t, ".ai-rulez", e.Name(), "no output may be written")
	}
}
