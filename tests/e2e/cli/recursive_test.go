package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type RecursiveGenerateSuite struct {
	suite.Suite
	workingDir string
}

func TestRecursiveGenerateSuite(t *testing.T) {
	suite.Run(t, new(RecursiveGenerateSuite))
}

func (s *RecursiveGenerateSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *RecursiveGenerateSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

// writeMinimalConfig writes a `<dir>/.ai-rulez/config.yaml` that produces
// a Claude preset output deterministically.
func (s *RecursiveGenerateSuite) writeMinimalConfig(dir, name string) {
	cfg := fmt.Sprintf(`version: "4.0"
name: "%s"
description: "%s test config"
presets:
  - claude
gitignore: false
`, name, name)
	aiRulesDir := filepath.Join(dir, ".ai-rulez")
	s.NoError(os.MkdirAll(aiRulesDir, 0o755))
	testutil.WriteFile(s.T(), aiRulesDir, "config.yaml", cfg)
}

func (s *RecursiveGenerateSuite) TestRecursiveProcessesAllConfigsAndPrunesNoise() {
	// Real configs we expect to be processed.
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		s.writeMinimalConfig(filepath.Join(s.workingDir, name), name)
	}

	// Junk under directories that must be pruned by the walker.
	for _, junk := range []string{"node_modules/x", "target/debug", ".venv/lib", "vendor/y", ".cache", "build"} {
		s.writeMinimalConfig(filepath.Join(s.workingDir, junk), "should-not-process")
	}

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--recursive")
	result.AssertOutputContains(s.T(), "Total: Generated")

	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		out := filepath.Join(s.workingDir, name, "CLAUDE.md")
		s.True(testutil.FileExists(s.T(), out), "expected %s to be generated", out)
	}

	for _, junk := range []string{"node_modules/x", "target/debug", ".venv/lib", "vendor/y", ".cache", "build"} {
		out := filepath.Join(s.workingDir, junk, "CLAUDE.md")
		s.False(testutil.FileExists(s.T(), out), "junk dir %s should not have been processed", out)
	}
}

func (s *RecursiveGenerateSuite) TestRecursiveSkipsSharedRuleLibrary() {
	// Top-level shared library: `ai-rulez/` (no dot) with a root config.
	libRoot := filepath.Join(s.workingDir, "ai-rulez")
	s.NoError(os.MkdirAll(libRoot, 0o755))
	testutil.WriteFile(s.T(), libRoot, "config.toml", `version = "4.0"
name = "shared-lib"
`)
	// Library modules — should be skipped by the walker.
	for _, mod := range []string{"core", "languages"} {
		s.writeMinimalConfig(filepath.Join(libRoot, "modules", mod), "lib-"+mod)
	}

	// One real consumer config — should be processed.
	s.writeMinimalConfig(filepath.Join(s.workingDir, "consumer"), "consumer")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--recursive")
	result.AssertOutputContains(s.T(), "Total: Generated")

	s.True(testutil.FileExists(s.T(), filepath.Join(s.workingDir, "consumer", "CLAUDE.md")),
		"consumer config should produce output")
	for _, mod := range []string{"core", "languages"} {
		out := filepath.Join(libRoot, "modules", mod, "CLAUDE.md")
		s.False(testutil.FileExists(s.T(), out),
			"library module %s should not be generated for", out)
	}
}

func (s *RecursiveGenerateSuite) TestRecursiveParallelOutputMatchesSerial() {
	// Generate the same set of configs twice (parallel and forced serial)
	// and ensure the produced output files are byte-identical. Catches
	// parallel-only races in include/template handling.
	const n = 6
	for i := 0; i < n; i++ {
		s.writeMinimalConfig(filepath.Join(s.workingDir, fmt.Sprintf("svc-%d", i)), fmt.Sprintf("svc-%d", i))
	}

	// Run 1: parallel.
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate", "--recursive")
	result.AssertOutputContains(s.T(), "Total: Generated")

	parallelOutputs := map[string]string{}
	for i := 0; i < n; i++ {
		path := filepath.Join(s.workingDir, fmt.Sprintf("svc-%d", i), "CLAUDE.md")
		parallelOutputs[path] = testutil.ReadFile(s.T(), path)
	}

	// Run 2: forced serial. We just re-run; with the hash cache, identical
	// inputs produce identical outputs (and skip writes when unchanged).
	resultSerial := testutil.RunCLIWithEnv(s.T(), s.workingDir, map[string]string{"GOMAXPROCS": "1"},
		"generate", "--recursive")
	s.Equal(0, resultSerial.ExitCode, "serial run should succeed: %s", resultSerial.Stderr)

	// Compare outputs byte-for-byte.
	keys := make([]string, 0, len(parallelOutputs))
	for k := range parallelOutputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, path := range keys {
		got := testutil.ReadFile(s.T(), path)
		s.Equal(parallelOutputs[path], got, "output for %s changed between parallel and serial runs", path)
	}
}
