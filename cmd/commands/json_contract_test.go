package commands

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

// reportCommands are the commands of the reports work package: each one that can
// print JSON must have a published schema (schema.JSONContracts), so a new report
// cannot ship without its contract.
var reportCommands = []string{
	"validate", "scan", "doctor", "verify", "lock", "tokens", "cost", "catalog", "sbom", "scanners", "roles",
	"publish", "update", "sign", "export", "okf", "verifiers", "eval",
}

// noContractYet are report commands whose JSON has no schema of its own; each
// names the reason.
var noContractYet = map[string]string{
	"verifiers calibrate":     "the calibration record is documented in docs/verifiers.md; its schema is the record file's",
	"verifiers suggest":       "the proposals follow the llm response; documented in docs/verifiers.md",
	"eval calibrate-estimate": "documented in docs/evals.md",
	"eval import":             "the import report shares the convert report schema (schema/convert-report.schema.json)",
	"okf import":              "the import report is documented in docs/okf.md",
}

// jsonCommandPaths collects the runnable commands that accept --format json,
// whether the flag is their own or inherited from a group.
func jsonCommandPaths(cmd *cobra.Command, prefix string, out *[]string) {
	for _, c := range cmd.Commands() {
		path := strings.TrimSpace(prefix + " " + c.Name())
		if c.Runnable() {
			f := c.Flags().Lookup("format")
			if f == nil {
				f = c.InheritedFlags().Lookup("format")
			}
			if f != nil && slices.Contains(f.Annotations[formatValuesAnnotation], formatJSON) {
				*out = append(*out, path)
			}
		}
		jsonCommandPaths(c, path, out)
	}
}

func TestEveryReportCommandWithJSONHasAContract(t *testing.T) {
	// Arrange
	var paths []string
	jsonCommandPaths(RootCmd, "", &paths)
	require.NotEmpty(t, paths)
	contracted := func(path string) bool {
		for _, c := range schema.JSONContracts {
			if c.Command == path || strings.HasPrefix(c.Command, path+" ") {
				return true
			}
		}
		return false
	}

	for _, path := range paths {
		if !slices.Contains(reportCommands, strings.Fields(path)[0]) {
			continue
		}
		// Act and assert
		if reason, ok := noContractYet[path]; ok {
			assert.NotEmpty(t, reason)
			continue
		}
		assert.True(t, contracted(path), "%q prints JSON but schema.JSONContracts has no entry for it", path)
	}
}

func TestEveryJSONContractNamesACommand(t *testing.T) {
	for _, c := range schema.JSONContracts {
		cmd, _, err := RootCmd.Find(strings.Fields(strings.SplitN(c.Command, " --", 2)[0]))
		require.NoError(t, err, c.Command)
		assert.NotEqual(t, RootCmd, cmd, "%q is not a command", c.Command)
	}
}
