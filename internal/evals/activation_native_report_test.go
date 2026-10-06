package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustActivation(t *testing.T, opts *ActivationOptions) *ActivationReport {
	t.Helper()
	report, err := RunActivation(context.Background(), opts)
	require.NoError(t, err)
	return report
}

func TestNativeReportMatchesTheSchema(t *testing.T) {
	// Arrange: a real run, a replay, a dry run and a run with an errored prompt.
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "eval-activation.v1.schema.json"))
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	cfg := activationProject(t)
	runner := scripted(map[string]map[string]int{
		"fires": {"deploy-staging": 5}, "stolen": {"deploy-staging": 5}, "fires.near-miss-1": {firedNone: 5}, "unrelated": {firedNone: 5},
	})
	store := NewStore()
	opts := nativeOptions(cfg, runner)
	opts.Store = store
	reports := map[string]*ActivationReport{}
	reports["ran"] = mustActivation(t, opts)
	reports["replay"] = mustActivation(t, opts)
	dry := nativeOptions(cfg, runner)
	dry.DryRun = true
	reports["dry"] = mustActivation(t, dry)
	broken := nativeRunner{&fakeRunner{fn: func(req *Request) (*Response, error) {
		return &Response{Version: ProtocolVersion, Results: []Result{{Case: req.Cases[0].ID, Arm: ArmWith, Error: "boom"}}}, nil
	}}}
	reports["errored"] = mustActivation(t, nativeOptions(cfg, broken))

	for name, report := range reports {
		t.Run(name, func(t *testing.T) {
			// Act
			var out bytes.Buffer
			require.NoError(t, report.Write(&out, FormatJSON))

			// Assert
			result := compiled.Validate(out.Bytes())
			if !result.IsValid() {
				details, _ := json.Marshal(result.Errors) //nolint:errcheck // diagnostics only
				t.Fatalf("report does not match the schema: %s\n%s", details, out.String())
			}
		})
	}
	assert.Equal(t, RunCached, reports["replay"].Skills[0].Status)
}

func TestNativeReportMarkdown(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	runner := scripted(map[string]map[string]int{
		"fires": {"deploy-staging": 3, "release-notes": 2}, "stolen": {"release-notes": 5}, "fires.near-miss-1": {"deploy-staging": 1, firedNone: 4}, "unrelated": {firedNone: 5},
	})
	report := mustActivation(t, nativeOptions(cfg, runner))
	var out bytes.Buffer

	// Act
	require.NoError(t, report.Write(&out, FormatMarkdown))

	// Assert
	text := out.String()
	assert.Contains(t, text, "runner `fake`")
	assert.Contains(t, text, "5 runs per prompt")
	assert.Contains(t, text, "fired in 3 of 5 runs")
	assert.Contains(t, text, "(borderline)")
	assert.Contains(t, text, "most fired: `release-notes`")
	assert.Contains(t, text, "### Confusion")
	assert.Contains(t, text, "| Run recall | Cost |", "a native table has no rank columns")
	assert.NotContains(t, text, "recall@1")
}
