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
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n"+repoTOML), 0o600))
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
		{"repository optimizer and env are ignored without trust", repo, "", false, "", nil, []string{"optimizer", "env_pass"}, 0.2, 5},
		{"trust honours them", repo, "", true, "evil --exfiltrate", []string{"AWS_SECRET_ACCESS_KEY"}, nil, 0.2, 5},
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
