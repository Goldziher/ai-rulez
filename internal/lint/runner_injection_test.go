package lint

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	proc "github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestDelegatePluginValidateThroughAnInjectedRunner(t *testing.T) {
	tests := []struct {
		name         string
		answer       proc.Result
		wantFindings int
	}{
		{
			name: "the reported error becomes a finding",
			answer: proc.Result{Status: proc.StatusExit, ExitCode: 1,
				Stdout: []byte(`{"manifest":{"errors":[{"path":"name","message":"bad name"}],"warnings":[]},"contents":[]}`)},
			wantFindings: 1,
		},
		{name: "a missing binary leaves the built-in checks alone", answer: proc.Result{Status: proc.StatusUnavailable}},
		{name: "unparseable output is ignored", answer: proc.Result{Status: proc.StatusOK, Stdout: []byte("not json")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			fake := &proc.Fake{Handle: func(proc.Spec) proc.Result { return tt.answer }}
			r := &runner{cfg: &config.Config{}, tree: &Tree{Top: dir}, docs: map[string]doc{}, host: ambient.Host{Runner: fake}}
			r.resolveSettings()

			// Act
			r.delegatePluginValidate(dir)

			// Assert
			calls := fake.Calls()
			require.Len(t, calls, 1)
			assert.Equal(t, []string{"claude", "plugin", "validate", dir, "--json"}, calls[0].Argv)
			assert.Len(t, r.findings, tt.wantFindings)
		})
	}
}

func TestScannerTodayUsesTheInjectedHost(t *testing.T) {
	tests := []struct {
		name string
		opt  ScannerOptions
		host ambient.Host
		want string
	}{
		{"the flag wins", ScannerOptions{Today: "2031-02-03"}, ambient.Host{Env: ambient.MapEnv{Vars: map[string]string{TodayEnv: "2030-01-02"}}}, "2031-02-03"},
		{"the injected environment is read", ScannerOptions{}, ambient.Host{Env: ambient.MapEnv{Vars: map[string]string{TodayEnv: "2030-01-02"}}}, "2030-01-02"},
		{"the injected clock is the fallback", ScannerOptions{}, ambient.Host{Env: ambient.MapEnv{}, Clock: ambient.Fixed(time.Date(2040, 5, 6, 23, 0, 0, 0, time.UTC))}, "2040-05-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.opt.today(tt.host))
		})
	}
}

func TestWithCwdSetsTheDisplayBase(t *testing.T) {
	// Arrange
	r := &runner{}

	// Act
	WithCwd("/work")(r)
	WithHost(ambient.Host{Clock: ambient.Fixed(time.Unix(0, 0))})(r)

	// Assert
	assert.Equal(t, "/work", r.cwd)
	assert.True(t, r.clock().Equal(time.Unix(0, 0)))
}
