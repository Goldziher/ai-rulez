package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pwnVerifier is a command verifier a hostile repository (or pull request)
// ships: running its script leaves a marker file in $MARKER_DIR.
const pwnVerifier = `[[verifiers]]
id = "looks-harmless"
rule = "local"
severity = "error"

[verifiers.require.command]
argv = ["./scripts/check.sh"]

[[verifiers.examples]]
name = "runs"
files = { "a.txt" = "x" }
expect = "pass"
`

const pwnScript = "#!/bin/sh\necho pwned > \"$MARKER_DIR/PWNED\"\nenv > \"$MARKER_DIR/ENV\"\nexit 0\n"

// TestCommandVerifiersNeedAllowExecE2E is the "command verifiers run without
// --allow-exec" attack repo: no command but `verifiers run|test --allow-exec`
// executes a verifier's program, and an imported verifier never does.
func TestCommandVerifiersNeedAllowExecE2E(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		allow    bool
		wantExit int
		wantRun  bool
	}{
		{name: "verifiers run refuses without --allow-exec (AR9H3)", args: []string{"verifiers", "run", "--format", "json"}, wantExit: 1},
		{name: "verifiers test fails its examples without --allow-exec", args: []string{"verifiers", "test"}, wantExit: 2},
		{name: "validate --strict never runs a command", args: []string{"validate", "--strict"}, wantExit: -1},
		{name: "generate never runs a command", args: []string{"generate", "--yes"}},
		{name: "verifiers list never runs a command", args: []string{"verifiers", "list"}},
		{name: "--allow-exec runs it", args: []string{"verifiers", "run", "--allow-exec", "--format", "json"}, wantRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			env := newIsoEnv(t)
			markers := t.TempDir()
			env.set("MARKER_DIR", markers).set("E2E_SECRET_KEY", "must-not-reach-the-verifier").set("GITHUB_TOKEN", "ghp_must-not-reach-the-verifier")
			root := minimalProject(t, "")
			writeTree(t, root, map[string]string{".ai-rulez/verifiers/pwn.toml": pwnVerifier, "a.txt": "x\n"})
			writeExec(t, filepath.Join(root, "scripts", "check.sh"), pwnScript)
			env.commitAll(root, "hostile")
			// The verifier's environment allowlist drops MARKER_DIR; name it so the
			// script can still leave its marker once it is allowed to run.
			writeTree(t, root, map[string]string{".ai-rulez/config.toml": "version = \"4.0\"\nname = \"e2e\"\npresets = [\"claude\"]\ngitignore = false\n\n[verifiers_settings]\ncommand_env = [\"MARKER_DIR\"]\n"})

			// Act
			res := env.run(root, tt.args...)

			// Assert
			if tt.wantExit >= 0 {
				assert.Equal(t, tt.wantExit, res.ExitCode, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			}
			_, err := os.Stat(filepath.Join(markers, "PWNED"))
			assert.Equal(t, tt.wantRun, err == nil, "the verifier program ran: %v", err == nil)
			if tt.wantRun {
				childEnv, rerr := os.ReadFile(filepath.Join(markers, "ENV")) //nolint:gosec // test file
				require.NoError(t, rerr)
				assert.NotContains(t, string(childEnv), "ghp_must-not-reach", "a token never reaches a verifier")
			}
		})
	}
}

// TestCommandVerifierSecretKeyNamesE2E pins RV-SEC-5: an environment variable
// whose name ends in _KEY must not reach a command verifier through the allowlist.
func TestCommandVerifierSecretKeyNamesE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	markers := t.TempDir()
	env.set("MARKER_DIR", markers).set("OPENAI_KEY", "must-not-reach-the-verifier")
	root := minimalProject(t, "\n[verifiers_settings]\ncommand_env = [\"MARKER_DIR\", \"OPENAI_KEY\"]\n")
	writeTree(t, root, map[string]string{".ai-rulez/verifiers/pwn.toml": pwnVerifier, "a.txt": "x\n"})
	writeExec(t, filepath.Join(root, "scripts", "check.sh"), pwnScript)
	env.commitAll(root, "hostile")

	// Act
	res := env.run(root, "verifiers", "run", "--allow-exec")

	// Assert
	childEnv, _ := os.ReadFile(filepath.Join(markers, "ENV")) //nolint:errcheck,gosec // absent when refused
	assert.NotContains(t, string(childEnv), "must-not-reach-the-verifier", "exit %d: %s", res.ExitCode, res.Stderr)
}

// TestImportedCommandVerifierNeverRunsE2E: a command verifier that arrives
// through an include is refused (AR9H2) even with --allow-exec.
func TestImportedCommandVerifierNeverRunsE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	markers := t.TempDir()
	env.set("MARKER_DIR", markers)
	root := minimalProject(t, "\n[[includes]]\nname = \"shared\"\nsource = \"vendor/shared\"\n\n[verifiers_settings]\ncommand_env = [\"MARKER_DIR\"]\n")
	writeTree(t, filepath.Join(root, "vendor", "shared"), map[string]string{
		".ai-rulez/config.toml":      "version = \"4.0\"\nname = \"shared\"\n",
		".ai-rulez/rules/local.md":   "# Shared\n\nshared rule\n",
		"rules/local.md":             "# Shared\n\nshared rule\n",
		"verifiers/pwn.toml":         pwnVerifier,
		".ai-rulez/verifiers/x.toml": pwnVerifier,
	})
	writeExec(t, filepath.Join(root, "scripts", "check.sh"), pwnScript)
	env.commitAll(root, "include")

	// Act
	res := env.run(root, "verifiers", "run", "--allow-exec", "--format", "json")

	// Assert
	_, err := os.Stat(filepath.Join(markers, "PWNED"))
	assert.True(t, os.IsNotExist(err), "an imported command verifier ran: %s %s", res.Stdout, res.Stderr)
	assert.Contains(t, res.Stdout+res.Stderr, "AR9H2", "the imported command verifier is refused, not silently dropped")
}
