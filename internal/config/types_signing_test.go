package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSigning(t *testing.T) {
	const issuer = "https://token.actions.githubusercontent.com"
	tests := []struct {
		name string
		in   *SigningConfig
		want string
	}{
		{"absent", nil, ""},
		{"key shorthand", &SigningConfig{Require: []string{"lock"}, KeyFile: "keys/release.pub", MaxAge: "30d"}, ""},
		{"identity shorthand", &SigningConfig{Identity: "me", Issuer: issuer, TLog: "required"}, ""},
		{"full trust entry", &SigningConfig{Trust: []SigningTrust{{IdentityRegexp: `^https://github\.com/org/.+$`, Issuer: issuer, ValidFrom: "2026-01-01", ValidUntil: "2027-01-01"}}}, ""},
		{"unknown subject to require", &SigningConfig{Require: []string{"bundle"}, KeyFile: "k"}, "invalid subject"},
		{"require without a signer", &SigningConfig{Require: []string{"lock"}}, "needs a trusted signer"},
		{"bad tlog", &SigningConfig{TLog: "always"}, "invalid tlog"},
		{"bad max_age", &SigningConfig{MaxAge: "forever"}, "max_age"},
		{"negative hash version", &SigningConfig{MinHashVersion: -1}, "min_hash_version"},
		{"absolute key path", &SigningConfig{KeyFile: "/etc/keys/release.pub"}, "inside the project"},
		{"key path escapes", &SigningConfig{KeyFile: "../release.pub"}, "inside the project"},
		{"trusted root escapes", &SigningConfig{TrustedRoot: "../../root.json"}, "inside the project"},
		{"identity without issuer", &SigningConfig{Identity: "me"}, "go together"},
		{"identity and key", &SigningConfig{Identity: "me", Issuer: issuer, KeyFile: "k"}, "not both"},
		{"unanchored regexp", &SigningConfig{Trust: []SigningTrust{{IdentityRegexp: "https://github.com/org/.*", Issuer: issuer}}}, "AR722"},
		{"regexp missing end anchor", &SigningConfig{Trust: []SigningTrust{{IdentityRegexp: "^https://github.com/org/.*", Issuer: issuer}}}, "anchored"},
		{"identity and regexp", &SigningConfig{Trust: []SigningTrust{{Identity: "a", IdentityRegexp: "^a$", Issuer: issuer}}}, "not both"},
		{"entry without issuer", &SigningConfig{Trust: []SigningTrust{{Identity: "a"}}}, "needs an issuer"},
		{"empty entry", &SigningConfig{Trust: []SigningTrust{{}}}, "needs identity"},
		{"key entry with issuer", &SigningConfig{Trust: []SigningTrust{{KeyFile: "k", Issuer: issuer}}}, "no issuer"},
		{"entry subject", &SigningConfig{Trust: []SigningTrust{{Subject: "nonsense", KeyFile: "k"}}}, "invalid subject"},
		{"served is not a trust subject", &SigningConfig{Trust: []SigningTrust{{Subject: "served", KeyFile: "k"}}}, "invalid subject"},
		{"bundle trust entry", &SigningConfig{Trust: []SigningTrust{{Subject: "bundle", KeyFile: "k"}}}, ""},
		{"sbom trust entry", &SigningConfig{Trust: []SigningTrust{{Subject: "sbom", KeyFile: "k"}}}, ""},
		{"require served", &SigningConfig{Require: []string{"served"}, KeyFile: "k"}, ""},
		{"require skill with a skill entry", &SigningConfig{Require: []string{"skill"}, Trust: []SigningTrust{{Subject: "skill", Source: "shared", KeyFile: "k"}}}, ""},
		{"require skill without a skill entry", &SigningConfig{Require: []string{"skill"}, KeyFile: "k"}, "subject = \"skill\""},
		{"source on a lock entry", &SigningConfig{Trust: []SigningTrust{{Source: "shared", KeyFile: "k"}}}, "source scopes"},
		{"threshold of two with two entries", &SigningConfig{Thresholds: map[string]int{"lock": 2}, Trust: []SigningTrust{{KeyFile: "a"}, {KeyFile: "b"}}}, ""},
		{"threshold of two with one entry", &SigningConfig{Thresholds: map[string]int{"lock": 2}, KeyFile: "a"}, "needs at least 2"},
		{"threshold of two with a pattern", &SigningConfig{Thresholds: map[string]int{"lock": 2}, Trust: []SigningTrust{{IdentityRegexp: "^a.*$", Issuer: issuer}}}, ""},
		{"threshold below one", &SigningConfig{Thresholds: map[string]int{"lock": 0}, KeyFile: "a"}, "at least 1"},
		{"threshold for an unknown subject", &SigningConfig{Thresholds: map[string]int{"served": 2}, KeyFile: "a"}, "invalid subject"},
		{"empty builder", &SigningConfig{Builders: []string{" "}}, "builder id"},
		{"bad date", &SigningConfig{Trust: []SigningTrust{{KeyFile: "k", ValidFrom: "soon"}}}, "valid_from"},
		{"window reversed", &SigningConfig{Trust: []SigningTrust{{KeyFile: "k", ValidFrom: "2027-01-01", ValidUntil: "2026-01-01"}}}, "before valid_from"},
		{"tlog off with an identity", &SigningConfig{TLog: "off", Identity: "me", Issuer: issuer}, "only works with keys"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Config{Signing: tt.in}).validateSigning()

			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestSigningTLogDefault(t *testing.T) {
	tests := []struct {
		name string
		in   *SigningConfig
		want string
	}{
		{"explicit", &SigningConfig{TLog: "optional", KeyFile: "k"}, "optional"},
		{"keys only", &SigningConfig{KeyFile: "k"}, "off"},
		{"key entries only", &SigningConfig{Trust: []SigningTrust{{KeyFile: "k"}}}, "off"},
		{"identity shorthand", &SigningConfig{Identity: "me", Issuer: "i"}, "required"},
		{"mixed", &SigningConfig{KeyFile: "k", Trust: []SigningTrust{{Identity: "me", Issuer: "i"}}}, "required"},
		{"nothing configured", &SigningConfig{}, "required"},
		{"absent", nil, "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.SigningTLog())
		})
	}
}

func TestSigningTrustEntriesFillsDefaultSubject(t *testing.T) {
	s := &SigningConfig{Identity: "me", Issuer: "i", Trust: []SigningTrust{{KeyFile: "k"}, {Subject: "lock", KeyFile: "k2"}}}

	entries := s.SigningTrustEntries()

	require.Len(t, entries, 3)
	for _, e := range entries {
		assert.Equal(t, SigningSubjectLock, e.Subject)
	}
	assert.True(t, s.Requires("lock") == false)
	assert.True(t, (&SigningConfig{Require: []string{"lock"}}).Requires("lock"))
	assert.Nil(t, (*SigningConfig)(nil).SigningTrustEntries())
}

func TestParseSigningTime(t *testing.T) {
	from, err := ParseSigningTime("2026-01-01", false)
	require.NoError(t, err)
	until, err := ParseSigningTime("2026-01-01", true)
	require.NoError(t, err)
	rfc, err := ParseSigningTime("2026-03-04T05:06:07Z", true)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), from)
	assert.True(t, until.After(time.Date(2026, 1, 1, 23, 59, 59, 0, time.UTC)), "a date bound covers its whole day")
	assert.True(t, until.Before(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), rfc)
	_, err = ParseSigningTime("01/02/2026", false)
	assert.Error(t, err)
}

func TestMergeConfigDocs_LocalOverlayCannotAddASigner(t *testing.T) {
	shared := map[string]any{"name": "p", "signing": map[string]any{"require": []any{"lock"}, "key_file": "keys/release.pub"}}
	local := map[string]any{"signing": map[string]any{"key_file": "keys/mine.pub", "require": []any{}}}

	merged, warnings, err := MergeConfigDocs(shared, local)

	require.NoError(t, err)
	assert.Equal(t, shared["signing"], merged["signing"])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "[signing]")
}

func TestSigningLoadsFromTOML(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte("version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n[signing]\nrequire = [\"lock\"]\nmax_age = \"90d\"\n[[signing.trust]]\nidentity = \"me\"\nissuer = \"i\"\n"), "config.toml")

	require.NoError(t, err)
	require.NotNil(t, cfg.Signing)
	assert.Equal(t, []string{"lock"}, cfg.Signing.Require)
	assert.Equal(t, "90d", cfg.Signing.MaxAge)
	require.Len(t, cfg.Signing.Trust, 1)
	assert.Equal(t, "me", cfg.Signing.Trust[0].Identity)
	assert.NoError(t, cfg.validateSigning())
}
