package improve

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

// lookIn finds only the named tools, under /usr/bin.
func lookIn(tools ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, t := range tools {
			if t == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
}

func TestExecute_IsolationWrapsTheOptimizerInTheSandbox(t *testing.T) {
	tests := []struct {
		name        string
		egress      []string
		wantNetwork bool
	}{
		{"no egress denies the network", nil, false},
		{"declared egress allows the network", []string{"api.example.com"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root, configDir := project(t)
			var seen []runner.Spec
			opt := optimizer(t, func(dir string, _ *OptimizerRequest, spec runner.Spec) {
				seen = append(seen, spec)
				appendSkill(t, dir, "\nGOOD advice.\n")
			})
			o := baseOptions(root, configDir, goodEval(), opt)
			o.MaxRounds, o.Isolation, o.Egress = 1, sandbox.ModeRequire, tt.egress
			o.Sandbox = sandbox.New("darwin", lookIn("sandbox-exec")).WithRunner(&runner.Fake{})
			plan := mustPrepare(t, &o)

			// Act
			report, err := plan.Execute(context.Background())

			// Assert
			require.NoError(t, err)
			assert.Equal(t, StatusAccepted, report.Status)
			require.Len(t, seen, 1)
			argv := seen[0].Argv
			assert.Equal(t, "/usr/bin/sandbox-exec", argv[0])
			assert.Equal(t, "opt", argv[len(argv)-1], "the optimizer is the last word of the confined command")
			profile := strings.Join(argv, " ")
			assert.Equal(t, tt.wantNetwork, !strings.Contains(profile, "(deny network*)"), profile)
			assert.Contains(t, profile, "W0=", "the workspace is a writable directory")
			require.NotNil(t, report.Isolation)
			assert.Equal(t, IsolationReport{Mode: "require", Backend: "sandbox-exec", Confined: true, NoNetwork: !tt.wantNetwork, NoWrites: true}, *report.Isolation)
			assertReportSchema(t, report)
			assert.Contains(t, plan.Summary(), "sandbox-exec sandbox")
		})
	}
}

func TestPrepare_Isolation(t *testing.T) {
	tests := []struct {
		name     string
		mode     sandbox.Mode
		sb       *sandbox.Sandbox
		wantErr  string
		wantWarn bool
		confined bool
	}{
		{"none never confines", sandbox.ModeNone, nil, "", false, false},
		{"unset means none", "", nil, "", false, false},
		{"require refuses without a backend", sandbox.ModeRequire, sandbox.New("plan9", lookIn()), CodeIsolationUnavailable, false, false},
		{"auto warns and runs unconfined without a backend", sandbox.ModeAuto, sandbox.New("plan9", lookIn()), "", true, false},
		{"auto confines when a backend works", sandbox.ModeAuto, sandbox.New("darwin", lookIn("sandbox-exec")).WithRunner(&runner.Fake{}), "", false, true},
		{"require refuses a backend that cannot confine", sandbox.ModeRequire, sandbox.New("darwin", lookIn("sandbox-exec")).WithRunner(&runner.Fake{Handle: func(runner.Spec) runner.Result {
			return runner.Result{Status: runner.StatusExit, ExitCode: 71, Stderr: []byte("sandbox_apply: Operation not permitted")}
		}}), CodeIsolationUnavailable, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root, configDir := project(t)
			o := baseOptions(root, configDir, goodEval(), &runner.Fake{})
			o.Isolation, o.Sandbox = tt.mode, tt.sb

			// Act
			plan, err := Prepare(context.Background(), &o)

			// Assert
			if tt.wantErr != "" {
				var refusal *Refusal
				require.ErrorAs(t, err, &refusal)
				assert.Equal(t, tt.wantErr, refusal.Code)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.confined, plan.isolation.Confined)
			hasWarn := false
			for _, w := range plan.Warnings {
				hasWarn = hasWarn || strings.Contains(w, CodeIsolationUnavailable)
			}
			assert.Equal(t, tt.wantWarn, hasWarn, "%v", plan.Warnings)
		})
	}
}

func TestSummary_IsolationLineStatesWhatTheBackendEnforces(t *testing.T) {
	tests := []struct {
		name    string
		sb      *sandbox.Sandbox
		egress  []string
		want    []string
		notWant []string
	}{
		{"sandbox-exec confines writes and network", sandbox.New("darwin", lookIn("sandbox-exec")).WithRunner(&runner.Fake{}), nil,
			[]string{"writes only inside", "no network", "reads are not restricted", "report-signing key"}, []string{"NOT"}},
		{"declared egress", sandbox.New("darwin", lookIn("sandbox-exec")).WithRunner(&runner.Fake{}), []string{"api.example.com"},
			[]string{"network allowed (egress declared)"}, []string{"no network", "NOT"}},
		{"unshare confines the network only", sandbox.New("linux", lookIn("unshare")).WithRunner(&runner.Fake{}), nil,
			[]string{"writes are NOT confined", "no network"}, []string{"writes only inside"}},
		{"unshare with egress blocks nothing", sandbox.New("linux", lookIn("unshare")).WithRunner(&runner.Fake{}), []string{"api.example.com"},
			[]string{"writes are NOT confined", "network allowed (egress declared)"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root, configDir := project(t)
			o := baseOptions(root, configDir, goodEval(), &runner.Fake{})
			o.Isolation, o.Sandbox, o.Egress = sandbox.ModeAuto, tt.sb, tt.egress
			plan := mustPrepare(t, &o)

			// Act
			text := plan.Summary()

			// Assert
			require.True(t, plan.isolation.Confined, "%v", plan.Warnings)
			for _, w := range tt.want {
				assert.Contains(t, text, w)
			}
			for _, w := range tt.notWant {
				assert.NotContains(t, text, w)
			}
		})
	}
	t.Run("none says what stays open", func(t *testing.T) {
		root, configDir := project(t)
		o := baseOptions(root, configDir, goodEval(), &runner.Fake{})
		text := mustPrepare(t, &o).Summary()
		assert.Contains(t, text, "report-signing key")
	})
}
