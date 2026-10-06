package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMinReleaseAgeKeys(t *testing.T) {
	tests := []struct {
		name    string
		version string
		minAge  string
		wantErr string
	}{
		{name: "days", version: "^1", minAge: "7d"},
		{name: "hours", version: "^1", minAge: "12h"},
		{name: "zero disables", version: "^1", minAge: "0"},
		{name: "unset", version: "^1"},
		{name: "needs version", minAge: "7d", wantErr: "min_release_age needs version"},
		{name: "bad unit", version: "^1", minAge: "7 days", wantErr: "min_release_age is invalid"},
		{name: "negative", version: "^1", minAge: "-1d", wantErr: "min_release_age is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVersionKeys("includes", "shared", "", tt.version, "", false, tt.minAge)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "AR731")
		})
	}
}

func TestLockMinReleaseAgeValidation(t *testing.T) {
	tests := []struct {
		name    string
		lock    *LockConfig
		wantErr string
	}{
		{name: "no lock table"},
		{name: "age and auto source", lock: &LockConfig{MinReleaseAge: "7d", MinReleaseAgeSource: "auto"}},
		{name: "forge source", lock: &LockConfig{MinReleaseAgeSource: "forge"}},
		{name: "first-seen source", lock: &LockConfig{MinReleaseAgeSource: "first-seen"}},
		{name: "commit source", lock: &LockConfig{MinReleaseAgeSource: "commit"}},
		{name: "bad age", lock: &LockConfig{MinReleaseAge: "soon"}, wantErr: "lock min_release_age"},
		{name: "bad source", lock: &LockConfig{MinReleaseAgeSource: "gossip"}, wantErr: "min_release_age_source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Lock: tt.lock}
			err := cfg.validateLock()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMinReleaseAgeDecodesFromTOML(t *testing.T) {
	data := []byte(`
version = "4.0"
name = "p"

[lock]
min_release_age = "3d"
min_release_age_source = "forge"

[[includes]]
name = "shared"
source = "https://github.com/example-org/ai-rules"
version = "^1.2"
min_release_age = "7d"
`)

	cfg, err := DecodeTOMLConfig(data, "config.toml")

	require.NoError(t, err)
	assert.Equal(t, "7d", cfg.Includes[0].VersionSpec().MinReleaseAge)
	assert.Equal(t, "3d", cfg.LockMinReleaseAge())
	assert.Equal(t, AgeSourceForge, cfg.LockMinReleaseAgeSource())
	assert.Equal(t, AgeSourceAuto, (&Config{}).LockMinReleaseAgeSource())
	require.NoError(t, cfg.validateVersionKeys())
	require.NoError(t, cfg.validateLock())
}
