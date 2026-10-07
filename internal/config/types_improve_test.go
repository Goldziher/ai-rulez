package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fptr(v float64) *float64 { return &v }
func iptr(v int) *int         { return &v }

func TestImproveConfig_Validate(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ImproveConfig
		want string
	}{
		{"nil", nil, ""},
		{"empty", &ImproveConfig{}, ""},
		{"all valid", &ImproveConfig{HoldoutTag: "ho", HoldoutFraction: fptr(0.4), MinHoldoutCases: 5, MinGain: fptr(0), MaxRegressions: iptr(1), Runs: 3, MaxSkillGrowth: 1.5, Isolation: "require", EnvPass: []string{"FOO"}}, ""},
		{"fraction one", &ImproveConfig{HoldoutFraction: fptr(1)}, "holdout_fraction"},
		{"too few min cases", &ImproveConfig{MinHoldoutCases: 2}, "min_holdout_cases"},
		{"gain above one", &ImproveConfig{MinGain: fptr(1.5)}, "min_gain"},
		{"negative regressions", &ImproveConfig{MaxRegressions: iptr(-1)}, "max_regressions"},
		{"negative rounds", &ImproveConfig{MaxRounds: -1}, "max_rounds"},
		{"growth too big", &ImproveConfig{MaxSkillGrowth: 5}, "max_skill_growth"},
		{"growth below one", &ImproveConfig{MaxSkillGrowth: 0.5}, "max_skill_growth"},
		{"bad isolation", &ImproveConfig{Isolation: "docker"}, "isolation"},
		{"bad env name", &ImproveConfig{EnvPass: []string{"A=B"}}, "env_pass"},
		{"tag with space", &ImproveConfig{HoldoutTag: "a b"}, "holdout_tag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			problems := tt.cfg.Validate()

			// Assert
			if tt.want == "" {
				assert.Empty(t, problems)
				return
			}
			require.NotEmpty(t, problems)
			assert.Contains(t, problems[0], tt.want)
		})
	}
}

func loadImprove(t *testing.T, repoTOML, userTOML string) *Config {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"+repoTOML), 0o600))
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if userTOML != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(xdg, "ai-rulez"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(xdg, "ai-rulez", UserConfigFileName), []byte(userTOML), 0o600))
	}
	cfg, err := LoadConfig(context.Background(), dir, WithoutRemote())
	require.NoError(t, err)
	return cfg
}

func TestResolveImprove(t *testing.T) {
	const repo = "[improve]\nmin_gain = 0.2\nmax_rounds = 5\noptimizer = \"evil --exfiltrate\"\nenv_pass = [\"AWS_SECRET_ACCESS_KEY\"]\n"
	tests := []struct {
		name          string
		repo, user    string
		trust         bool
		wantOptimizer string
		wantEnvPass   []string
		wantIgnored   []string
		wantGain      float64
		wantRounds    int
	}{
		{"repository optimizer and env are ignored without trust", repo, "", false, "", nil, []string{"optimizer", "env_pass"}, 0.2, 0},
		{"trust honors them", repo, "", true, "evil --exfiltrate", []string{"AWS_SECRET_ACCESS_KEY"}, nil, 0.2, 5},
		{"user scope wins and needs no trust", repo, "[improve]\noptimizer = \"mine\"\nmax_rounds = 2\nenv_pass = [\"MY_VAR\"]\n", false, "mine", []string{"MY_VAR"}, []string{"optimizer", "env_pass"}, 0.2, 2},
		{"no table", "", "", false, "", nil, nil, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := loadImprove(t, tt.repo, tt.user)

			// Act
			res, err := cfg.ResolveImprove(tt.trust, nil)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantOptimizer, res.Effective.Optimizer)
			assert.Equal(t, tt.wantEnvPass, res.Effective.EnvPass)
			assert.Equal(t, tt.wantIgnored, res.IgnoredRepoKeys)
			assert.Equal(t, tt.wantRounds, res.Effective.MaxRounds)
			if tt.wantGain != 0 {
				require.NotNil(t, res.Effective.MinGain)
				assert.InDelta(t, tt.wantGain, *res.Effective.MinGain, 1e-9)
			}
		})
	}
}

func TestResolveImprove_RejectsAnInvalidUserTable(t *testing.T) {
	// Arrange
	cfg := loadImprove(t, "", "[improve]\nisolation = \"docker\"\n")

	// Act
	_, err := cfg.ResolveImprove(false, nil)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "isolation")
}

func TestLoadImproveTableRoundTripsThroughTheWriter(t *testing.T) {
	// Arrange
	cfg := loadImprove(t, "[improve]\nholdout_tag = 'hold'\nmin_gain = 0\nmax_regressions = 0\n", "")
	require.NotNil(t, cfg.Improve)
	require.NotNil(t, cfg.Improve.MinGain, "an explicit zero is kept distinct from unset")

	// Act
	out, err := MarshalTOML(cfg)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(out), "[improve]")
	assert.Contains(t, string(out), "holdout_tag = 'hold'")
	assert.Contains(t, string(out), "min_gain = 0.0")
}

func TestResolveImprove_ARepositoryTableMayOnlyTightenTheGate(t *testing.T) {
	tests := []struct {
		name        string
		repo, user  string
		trust       bool
		wantDropped []string
		check       func(t *testing.T, e *ImproveConfig)
	}{
		{"looser values are dropped", "[improve]\nmin_gain = 0\nmax_regressions = 2\nholdout_fraction = 0.1\nmax_skill_growth = 2\n", "", false,
			[]string{"min_gain", "max_regressions", "holdout_fraction", "max_skill_growth"}, func(t *testing.T, e *ImproveConfig) {
				assert.Nil(t, e.MinGain)
				assert.Nil(t, e.MaxRegressions)
				assert.Nil(t, e.HoldoutFraction)
				assert.Zero(t, e.MaxSkillGrowth)
			}},
		{"fewer runs and larger loop budgets are dropped", "[improve]\nruns = 1\nmax_rounds = 50\nmax_holdout_evals = 50\n", "", false,
			[]string{"runs", "max_rounds", "max_holdout_evals"}, func(t *testing.T, e *ImproveConfig) {
				assert.Zero(t, e.Runs)
				assert.Zero(t, e.MaxRounds)
				assert.Zero(t, e.MaxHoldoutEvals)
			}},
		{"more runs and smaller loop budgets are kept", "[improve]\nruns = 5\nmax_rounds = 2\nmax_holdout_evals = 1\n", "", false,
			nil, func(t *testing.T, e *ImproveConfig) {
				assert.Equal(t, 5, e.Runs)
				assert.Equal(t, 2, e.MaxRounds)
				assert.Equal(t, 1, e.MaxHoldoutEvals)
			}},
		{"stricter values are kept", "[improve]\nmin_gain = 0.3\nmax_regressions = 0\nholdout_fraction = 0.5\nmax_skill_growth = 1.1\nrequire_ci_above_zero = true\nmin_holdout_cases = 6\n", "", false,
			nil, func(t *testing.T, e *ImproveConfig) {
				require.NotNil(t, e.MinGain)
				assert.InDelta(t, 0.3, *e.MinGain, 1e-9)
				assert.InDelta(t, 1.1, e.MaxSkillGrowth, 1e-9)
				assert.True(t, e.RequireCIAboveZero)
				assert.Equal(t, 6, e.MinHoldoutCases)
			}},
		{"the defaults themselves are allowed", "[improve]\nmin_gain = 0.05\nmax_regressions = 0\nholdout_fraction = 0.3\nmax_skill_growth = 1.25\n", "", false,
			nil, func(t *testing.T, e *ImproveConfig) { require.NotNil(t, e.MinGain) }},
		{"trust-repo-optimizer lets looser values through", "[improve]\nmin_gain = 0\nmax_regressions = 2\n", "", true,
			nil, func(t *testing.T, e *ImproveConfig) {
				require.NotNil(t, e.MinGain)
				assert.Zero(t, *e.MinGain)
				require.NotNil(t, e.MaxRegressions)
				assert.Equal(t, 2, *e.MaxRegressions)
			}},
		{"the user config may loosen without trust", "[improve]\nmin_gain = 0.3\n", "[improve]\nmin_gain = 0\nmax_regressions = 1\n", false,
			nil, func(t *testing.T, e *ImproveConfig) {
				require.NotNil(t, e.MinGain)
				assert.Zero(t, *e.MinGain)
				assert.Equal(t, 1, *e.MaxRegressions)
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := loadImprove(t, tt.repo, tt.user)

			// Act
			res, err := cfg.ResolveImprove(tt.trust, nil)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantDropped, res.LoosenedRepoKeys)
			tt.check(t, &res.Effective)
		})
	}
}
