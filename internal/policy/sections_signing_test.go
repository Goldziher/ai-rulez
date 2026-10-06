package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const (
	ciIdentity = "https://github.com/example-org/ai-config/.github/workflows/release.yml@refs/heads/main"
	ciIssuer   = "https://token.actions.githubusercontent.com"
)

var ciKey = "subject=lock identity=" + ciIdentity + " issuer=" + ciIssuer

func TestParseSigning(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[signing]\nrequire_verified = [\"lock\", \"lock\"]\ntlog = \"Required\"\nmax_age = \"180d\"\nmin_hash_version = 2\n" +
		"[[signing.trust]]\nsubject = \"lock\"\nidentity = \"" + ciIdentity + "\"\nissuer = \"" + ciIssuer + "\"\n"

	// Act
	_, p, err := Parse("p.toml", []byte(body))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"lock"}, p.Signing.RequireVerified)
	assert.Equal(t, "required", p.Signing.TLog)
	assert.Equal(t, 180*24*time.Hour, p.Signing.MaxAge)
	assert.Equal(t, 2, p.Signing.MinHashVersion)
	assert.Equal(t, List{Set: true, Items: []string{ciKey}}, p.Signing.Trust)
}

func TestParseSigningForbidRepoIdentitiesWithoutListTrustsNobody(t *testing.T) {
	// Act
	_, p, err := Parse("p.toml", []byte("policy_version = 1\n[signing]\nallow_repo_identities = false\n"))
	// Assert
	require.NoError(t, err)
	assert.Equal(t, List{Set: true}, p.Signing.Trust)
}

func TestParseSigningRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"unknown subject", "policy_version = 1\n[signing]\nrequire_verified = [\"policy\"]\n", "not a subject"},
		{"tlog off is no floor", "policy_version = 1\n[signing]\ntlog = \"off\"\n", "tlog"},
		{"bad max_age", "policy_version = 1\n[signing]\nmax_age = \"soon\"\n", "max_age"},
		{"negative hash version", "policy_version = 1\n[signing]\nmin_hash_version = -1\n", "min_hash_version"},
		{"identity without issuer", "policy_version = 1\n[[signing.trust]]\nidentity = \"a@b.c\"\n", "needs an issuer"},
		{"both identity forms", "policy_version = 1\n[[signing.trust]]\nidentity = \"a\"\nidentity_regexp = \"^a$\"\nissuer = \"i\"\n", "not both"},
		{"unanchored regexp", "policy_version = 1\n[[signing.trust]]\nidentity_regexp = \"a.*\"\nissuer = \"i\"\n", "anchored"},
		{"key files have no place in a policy", "policy_version = 1\n[[signing.trust]]\nkey_file = \"k.pem\"\n", "unknown key"},
		{"empty entry", "policy_version = 1\n[[signing.trust]]\nissuer = \"i\"\n", "needs identity"},
		{"whitespace in identity", "policy_version = 1\n[[signing.trust]]\nidentity = \"a b\"\nissuer = \"i\"\n", "whitespace"},
		{"allowing repo identities contradicts a listed set", "policy_version = 1\n[signing]\nallow_repo_identities = true\n[[signing.trust]]\nidentity = \"a\"\nissuer = \"i\"\n", "contradicts"},
		{"unknown subject on trust", "policy_version = 1\n[[signing.trust]]\nsubject = \"sbom\"\nidentity = \"a\"\nissuer = \"i\"\n", "only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, _, err := Parse("p.toml", []byte(tt.body))
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestTrustKeyRoundTrips(t *testing.T) {
	for _, in := range []config.SigningTrust{
		{Identity: ciIdentity, Issuer: ciIssuer},
		{Subject: "lock", IdentityRegexp: "^https://x/.*$", Issuer: ciIssuer},
	} {
		// Act
		got := trustFromKey(trustKey(in))
		// Assert
		in.Subject = "lock"
		assert.Equal(t, in, got)
	}
}

func TestMergeSigning(t *testing.T) {
	a := Policy{Signing: Signing{
		RequireVerified: []string{"lock"}, TLog: "optional", MaxAge: 30 * 24 * time.Hour, MinHashVersion: 1,
		Trust: List{Set: true, Items: []string{"x", "y"}},
	}}
	b := Policy{Signing: Signing{TLog: "required", MaxAge: 7 * 24 * time.Hour, MinHashVersion: 2, Trust: List{Set: true, Items: []string{"y", "z"}}}}

	got := Merge(a, b)

	assert.Equal(t, []string{"lock"}, got.Signing.RequireVerified)
	assert.Equal(t, "required", got.Signing.TLog, "the stricter log mode")
	assert.Equal(t, 7*24*time.Hour, got.Signing.MaxAge, "the shorter age")
	assert.Equal(t, 2, got.Signing.MinHashVersion, "the newer hash scheme")
	assert.Equal(t, List{Set: true, Items: []string{"y"}}, got.Signing.Trust, "trusted signers intersect")
	assert.Equal(t, got, Merge(b, a))
	assert.Equal(t, a.Signing, Merge(a, Policy{}).Signing)
}

func TestApplySigning(t *testing.T) {
	pol := Signing{
		RequireVerified: []string{"lock"}, TLog: "required", MaxAge: 90 * 24 * time.Hour, MinHashVersion: 2,
		Trust: List{Set: true, Items: []string{ciKey}},
	}
	tests := []struct {
		name     string
		repo     *config.SigningConfig
		want     config.SigningConfig
		wantViol []string
	}{
		{
			"a repository without [signing] gets the policy",
			nil,
			config.SigningConfig{ // an unset tlog already derives "required" from the identity signer
				Require: []string{"lock"}, MaxAge: "90d", MinHashVersion: 2,
				Trust: []config.SigningTrust{{Subject: "lock", Identity: ciIdentity, Issuer: ciIssuer}},
			},
			nil,
		},
		{
			"weaker explicit values are raised and reported, own signers are dropped",
			&config.SigningConfig{
				TLog: "off", MaxAge: "365d", MinHashVersion: 1, Identity: "evil@x.org", Issuer: "https://issuer.example",
			},
			config.SigningConfig{
				Require: []string{"lock"}, TLog: "required", MaxAge: "90d", MinHashVersion: 2,
				Trust: []config.SigningTrust{{Subject: "lock", Identity: ciIdentity, Issuer: ciIssuer}},
			},
			[]string{"AR740 signing.max_age", "AR740 signing.min_hash_version", "AR740 signing.tlog", "AR740 signing.trust"},
		},
		{
			"stricter values and the policy's own signer are accepted",
			&config.SigningConfig{
				Require: []string{"lock"}, TLog: "required", MaxAge: "30d", MinHashVersion: 3,
				Trust: []config.SigningTrust{{Identity: ciIdentity, Issuer: ciIssuer, ValidUntil: "2030-01-01"}},
			},
			config.SigningConfig{
				Require: []string{"lock"}, TLog: "required", MaxAge: "30d", MinHashVersion: 3,
				Trust: []config.SigningTrust{{Subject: "lock", Identity: ciIdentity, Issuer: ciIssuer, ValidUntil: "2030-01-01"}},
			},
			nil,
		},
		{
			"a key file is not a policy signer",
			&config.SigningConfig{KeyFile: "keys/own.pem"},
			config.SigningConfig{
				Require: []string{"lock"}, TLog: "required", MaxAge: "90d", MinHashVersion: 2,
				Trust: []config.SigningTrust{{Subject: "lock", Identity: ciIdentity, Issuer: ciIssuer}},
			},
			[]string{"AR740 signing.trust"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "name = \"x\"\n[signing]\ntlog = \"off\"\nmax_age = \"365d\"\nmin_hash_version = 1\nidentity = \"evil@x.org\"\nkey_file = \"keys/own.pem\"\n")
			cfg.Signing = tt.repo
			res := Resolve([]Layer{layer("managed", Policy{Signing: pol})})
			// Act
			out := res.Apply(cfg).Outcome
			// Assert
			require.NotNil(t, cfg.Signing)
			assert.Equal(t, tt.want, *cfg.Signing)
			assert.ElementsMatch(t, tt.wantViol, codes(out))
		})
	}
}

func TestApplySigningEmptyTrustListTrustsNobody(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.Signing = &config.SigningConfig{Identity: "a@b.c", Issuer: "https://i"}
	res := Resolve([]Layer{layer("managed", Policy{Signing: Signing{Trust: List{Set: true}}})})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	assert.Empty(t, cfg.Signing.SigningTrustEntries())
	assert.Equal(t, []string{"AR740 signing.trust"}, codes(out))
}

func TestApplySigningUnsetTLogKeepsAStricterDerivedMode(t *testing.T) {
	// Arrange: an identity signer derives "required", which already meets "optional".
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.Signing = &config.SigningConfig{Identity: "a@b.c", Issuer: "https://i"}
	res := Resolve([]Layer{layer("managed", Policy{Signing: Signing{TLog: "optional"}})})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	assert.Empty(t, cfg.Signing.TLog)
	assert.Empty(t, out.Violations)
}

func TestFormatAgeRoundTrips(t *testing.T) {
	for _, d := range []time.Duration{24 * time.Hour, 90 * 24 * time.Hour, 36 * time.Hour, 90 * time.Minute} {
		// Act
		got, err := config.ParseApprovalMaxAge(formatAge(d))
		// Assert
		require.NoError(t, err)
		assert.Equal(t, d, got)
	}
}
