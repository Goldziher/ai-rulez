package verifiers

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const commandHead = "[[verifiers]]\nid = \"cmd\"\nrule = \"database\"\nseverity = \"error\"\n"

func commandSpec(body string) string {
	return commandHead + "when_changed = [\"*.sql\"]\n[verifiers.require.command]\n" + body
}

func runCommandSpec(t *testing.T, toml string, opts Options, settings *config.VerifiersSettings) Result {
	t.Helper()
	cfg := specProject(t, map[string]string{"a.sql": "x", "b.sql": "y"}, toml)
	cfg.VerifiersSettings = settings
	rep := Run(context.Background(), cfg, opts)
	require.Len(t, rep.Results, 1)
	return rep.Results[0]
}

func TestCommandPredicate_RefusedWithoutAllowExec(t *testing.T) {
	// Arrange
	fake := &runner.Fake{}

	// Act
	res := runCommandSpec(t, commandSpec("argv = [\"make\", \"check\"]\n"), Options{Runner: fake}, nil)

	// Assert
	assert.Equal(t, StatusError, res.Status)
	assert.Equal(t, CodeVerifierCommand, res.Code)
	assert.Contains(t, res.Message, "--allow-exec")
	assert.Empty(t, fake.Calls(), "nothing may be started without --allow-exec")
}

func TestCommandPredicate_Outcomes(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		result   runner.Result
		want     Status
		wantCode string
		wantMsg  string
	}{
		{"exit 0 passes", "argv = [\"x\"]\n", runner.Result{Status: runner.StatusOK}, StatusPass, "", ""},
		{"non-zero fails", "argv = [\"x\"]\n", runner.Result{Status: runner.StatusExit, ExitCode: 3, Stderr: []byte("\nlock is stale\nmore\n")}, StatusFail, CodeVerifierFailed, "exited 3, expected 0"},
		{"expect_exit matches", "argv = [\"x\"]\nexpect_exit = 3\n", runner.Result{Status: runner.StatusExit, ExitCode: 3}, StatusPass, "", ""},
		{"timeout is AR9H3", "argv = [\"x\"]\n", runner.Result{Status: runner.StatusTimeout, Timeout: time.Second}, StatusError, CodeVerifierCommand, "timed out"},
		{"missing binary is AR9H3", "argv = [\"x\"]\n", runner.Result{Status: runner.StatusUnavailable, Err: assert.AnError}, StatusError, CodeVerifierCommand, "could not be started"},
		{"start failure is AR9H3", "argv = [\"x\"]\n", runner.Result{Status: runner.StatusError, Err: assert.AnError}, StatusError, CodeVerifierCommand, "did not run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{Handle: func(runner.Spec) runner.Result { return tt.result }}

			// Act
			res := runCommandSpec(t, commandSpec(tt.body), Options{Runner: fake, AllowExec: true}, nil)

			// Assert
			assert.Equal(t, tt.want, res.Status, res.Message)
			assert.Equal(t, tt.wantCode, res.Code)
			assert.Contains(t, res.Message, tt.wantMsg)
		})
	}
}

func TestCommandPredicate_FailureQuotesOutputMasked(t *testing.T) {
	fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("key sk-abcdefghijklmnopqrstuvwxyz rejected\n")}
	}}

	res := runCommandSpec(t, commandSpec("argv = [\"x\"]\n"), Options{Runner: fake, AllowExec: true}, nil)

	require.Len(t, res.Findings, 1)
	assert.NotContains(t, res.Findings[0].Match, "sk-abcdefghijklmnopqrstuvwxyz")
	assert.Contains(t, res.Findings[0].Match, "rejected")
}

func TestCommandPredicate_RunSpecIsHardened(t *testing.T) {
	// Arrange
	fake := &runner.Fake{}
	parent := []string{"PATH=/bin", "HOME=/h", "CI=true", "GITHUB_TOKEN=secret", "MY_FLAG=1", "OTHER=2", "LC_ALL=C"}
	settings := &config.VerifiersSettings{CommandEnv: []string{"MY_FLAG"}, MaxTimeoutS: 10}

	// Act
	res := runCommandSpec(t, commandSpec("argv = [\"make\", \"check\"]\ntimeout_s = 600\n"),
		Options{Runner: fake, AllowExec: true, Environ: parent}, settings)

	// Assert
	require.Equal(t, StatusPass, res.Status, res.Message)
	calls := fake.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"make", "check"}, calls[0].Argv)
	assert.NotEmpty(t, calls[0].Dir)
	assert.Equal(t, 10*time.Second, calls[0].Timeout, "timeout_s is capped by max_timeout_s")
	assert.EqualValues(t, maxCommandOutput, calls[0].MaxOutput)
	assert.Contains(t, calls[0].Env, "MY_FLAG=1")
	assert.Contains(t, calls[0].Env, "CI=true")
	assert.Contains(t, calls[0].Env, "LC_ALL=C")
	assert.NotContains(t, calls[0].Env, "GITHUB_TOKEN=secret")
	assert.NotContains(t, calls[0].Env, "OTHER=2")
}

func TestCommandPredicate_PassFiles(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantArgv  []string
		wantStdin string
	}{
		{"args", "args", []string{"lint", "a.sql", "b.sql"}, ""},
		{"stdin0", "stdin0", []string{"lint"}, "a.sql\x00b.sql\x00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{}

			// Act
			res := runCommandSpec(t, commandSpec("argv = [\"lint\"]\npass_files = \""+tt.mode+"\"\n"), Options{Runner: fake, AllowExec: true}, nil)

			// Assert
			require.Equal(t, StatusPass, res.Status, res.Message)
			call := fake.Calls()[0]
			assert.Equal(t, tt.wantArgv, call.Argv)
			assert.Equal(t, tt.wantStdin, string(call.Stdin))
		})
	}
}

func TestCommandPredicate_NotDoesNotInvertARefusal(t *testing.T) {
	toml := commandHead + "when_changed = [\"*.sql\"]\n[verifiers.require.not.command]\nargv = [\"x\"]\n"

	res := runCommandSpec(t, toml, Options{Runner: &runner.Fake{}}, nil)

	assert.Equal(t, StatusError, res.Status, "a command that did not run is never a pass, even under not")
	assert.Equal(t, CodeVerifierCommand, res.Code)
}

func TestCommandPredicate_RealProcessSeesNoSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	t.Setenv("PLANTED_SECRET", "hunter2")
	toml := commandSpec("argv = [\"sh\", \"-c\", \"test -z \\\"$PLANTED_SECRET\\\"\"]\n")

	res := runCommandSpec(t, toml, Options{AllowExec: true}, nil)

	assert.Equal(t, StatusPass, res.Status, res.Message)
}

func TestCommandPredicate_RealProcessExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}

	res := runCommandSpec(t, commandSpec("argv = [\"sh\", \"-c\", \"echo broken >&2; exit 4\"]\n"), Options{AllowExec: true}, nil)

	assert.Equal(t, StatusFail, res.Status)
	require.Len(t, res.Findings, 1)
	assert.Equal(t, "broken", res.Findings[0].Match)
}

func TestLoadSpecs_CommandValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"valid", "argv = [\"make\"]\n", ""},
		{"empty argv", "argv = []\n", "non-empty argv"},
		{"blank program", "argv = [\" \"]\n", "non-empty argv"},
		{"bad pass_files", "argv = [\"x\"]\npass_files = \"env\"\n", "args or stdin0"},
		{"timeout too long", "argv = [\"x\"]\ntimeout_s = 99999\n", "timeout_s"},
		{"bad expect_exit", "argv = [\"x\"]\nexpect_exit = 300\n", "expect_exit"},
		{"unknown field", "argv = [\"x\"]\nshell = true\n", "invalid TOML"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := specProject(t, nil, commandSpec(tt.body))

			specs, problems := LoadSpecs(cfg)

			if tt.want == "" {
				assert.Empty(t, problems)
				assert.Len(t, specs, 1)
				return
			}
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0].Message, tt.want)
		})
	}
}

func TestLoadSpecs_PassFilesNeedsScope(t *testing.T) {
	toml := commandHead + "[verifiers.require.command]\nargv = [\"x\"]\npass_files = \"args\"\n"

	_, problems := LoadSpecs(specProject(t, nil, toml))

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "needs when_changed")
}

func TestCommandPredicate_RefusedCommandDoesNotMaskADeterministicFailure(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Status
	}{
		{"forbid then command", "[[verifiers.require.all]]\n[verifiers.require.all.forbid]\nregex = \"x\"\n[[verifiers.require.all]]\n[verifiers.require.all.command]\nargv = [\"x\"]\n", StatusFail},
		{"command then forbid", "[[verifiers.require.all]]\n[verifiers.require.all.command]\nargv = [\"x\"]\n[[verifiers.require.all]]\n[verifiers.require.all.forbid]\nregex = \"x\"\n", StatusFail},
		{"passing forbid leaves the refusal", "[[verifiers.require.all]]\n[verifiers.require.all.forbid]\nregex = \"nomatch\"\n[[verifiers.require.all]]\n[verifiers.require.all.command]\nargv = [\"x\"]\n", StatusError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{}

			// Act
			res := runCommandSpec(t, commandHead+"when_changed = [\"*.sql\"]\n"+tt.body, Options{Runner: fake}, nil)

			// Assert
			assert.Equal(t, tt.want, res.Status, res.Message)
			assert.Empty(t, fake.Calls())
			if tt.want == StatusFail {
				assert.Equal(t, CodeVerifierFailed, res.Code)
				assert.NotEmpty(t, res.Findings)
				assert.Contains(t, strings.Join(res.Notes, " "), "--allow-exec")
			} else {
				assert.Equal(t, CodeVerifierCommand, res.Code)
			}
		})
	}
}

func TestCommandPredicate_PassFilesArgsCannotInjectAnOption(t *testing.T) {
	// Arrange
	fake := &runner.Fake{}
	cfg := specProject(t, map[string]string{"--version.sql": "x", "-rf.sql": "y", "ok.sql": "z"},
		commandSpec("argv = [\"lint\"]\npass_files = \"args\"\n"))

	// Act
	rep := Run(context.Background(), cfg, Options{Runner: fake, AllowExec: true})

	// Assert
	require.Len(t, rep.Results, 1)
	require.Equal(t, StatusPass, rep.Results[0].Status, rep.Results[0].Message)
	assert.Equal(t, []string{"lint", "./--version.sql", "./-rf.sql", "ok.sql"}, fake.Calls()[0].Argv)
}
