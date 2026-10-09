package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalRun_BuiltinGraderRefusesAModelItCannotPriceBeforeTheRunnerStarts(t *testing.T) {
	// Arrange: the user config switches the network on for a model the price table does not list.
	resetEvalFlags(t)
	resetGraderFlags(t)
	graderProject(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("LOCAL_GRADER_KEY", "not-a-real-key")
	require.NoError(t, os.MkdirAll(filepath.Join(xdg, "ai-rulez"), 0o750))
	userConfig := "[llm]\nprovider = \"openai\"\nmodel = \"mystery-9\"\nallow_network = true\napi_key_env = \"LOCAL_GRADER_KEY\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(xdg, "ai-rulez", "config.toml"), []byte(userConfig), 0o600))
	marker := filepath.Join(t.TempDir(), "started")
	evalFlags.runnerCommand = "touch " + marker
	evalFlags.grader, evalFlags.allowLLM = evals.GraderBuiltin, true

	// Act
	_, err := runEval(evalRunCmd, nil)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "no price is known for model")
	assert.NoFileExists(t, marker, "the runner was never started")

	// And with the cap dropped the grader may start.
	evalFlags.graderMaxCost = 0
	_, err = runEval(evalRunCmd, nil)
	assert.NotContains(t, errString(err), "no price is known")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
