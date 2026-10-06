package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestParseScannerPolicy(t *testing.T) {
	body := "policy_version = 1\n[lint.scanner_policy]\npreset = \"Strict\"\nrequired = [\" cisco \", \"agnix\", \"cisco\"]\nfail_on = \"warning\"\n" +
		"isolation = \"require\"\nallow_egress = []\n"

	_, p, err := Parse("p.toml", []byte(body))

	require.NoError(t, err)
	assert.Equal(t, ScannerPolicy{Preset: "strict", Required: []string{"agnix", "cisco"}, FailOn: "warning", Isolation: "require", AllowEgress: List{Set: true}}, p.Lint.ScannerPolicy)
}

func TestParseScannerPolicyRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"unknown preset", "[lint.scanner_policy]\npreset = \"loud\"\n", "preset"},
		{"bad fail_on", "[lint.scanner_policy]\nfail_on = \"fatal\"\n", "fail_on"},
		{"bad isolation", "[lint.scanner_policy]\nisolation = \"maybe\"\n", "isolation"},
		{"empty required", "[lint.scanner_policy]\nrequired = [\"\"]\n", "required"},
		{"empty allow_egress entry", "[lint.scanner_policy]\nallow_egress = [\" \"]\n", "allow_egress"},
		{"unknown key", "[lint.scanner_policy]\nbaseline = \"x\"\n", "unknown key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse("p.toml", []byte("policy_version = 1\n"+tt.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestMergeScannerPolicy(t *testing.T) {
	a := Policy{Lint: Lint{ScannerPolicy: ScannerPolicy{Preset: "baseline", Required: []string{"a"}, FailOn: "error", Isolation: "auto", AllowEgress: List{Set: true, Items: []string{"x", "y"}}}}}
	b := Policy{Lint: Lint{ScannerPolicy: ScannerPolicy{Preset: "strict", Required: []string{"b"}, FailOn: "warning", Isolation: "require", AllowEgress: List{Set: true, Items: []string{"y", "z"}}}}}

	got := Merge(a, b)

	assert.Equal(t, ScannerPolicy{Preset: "strict", Required: []string{"a", "b"}, FailOn: "warning", Isolation: "require", AllowEgress: List{Set: true, Items: []string{"y"}}}, got.Lint.ScannerPolicy)
	assert.Equal(t, got, Merge(b, a), "commutative")
	assert.Equal(t, got, Merge(got, got), "idempotent")
	assert.Equal(t, a.Lint.ScannerPolicy, Merge(a, Policy{}).Lint.ScannerPolicy, "unset constrains nothing")
}

func TestApplyScannerPolicy(t *testing.T) {
	pol := Policy{Lint: Lint{ScannerPolicy: ScannerPolicy{Preset: "strict", Required: []string{"cisco"}, FailOn: "warning", Isolation: "auto", AllowEgress: List{Set: true, Items: []string{"snyk"}}}}}
	tests := []struct {
		name     string
		repo     *config.LintScannerPolicy
		want     config.LintScannerPolicy
		wantKeys []string
		accepted bool
	}{
		{
			name: "unset takes the policy silently", repo: nil,
			want: config.LintScannerPolicy{Preset: "strict", Required: []string{"cisco"}, FailOn: "warning", Isolation: "auto", AllowEgress: []string{"snyk"}},
		},
		{
			name:     "weaker values are clamped and reported",
			repo:     &config.LintScannerPolicy{Preset: "off", FailOn: "error", Isolation: "none", Required: []string{"agnix"}, AllowEgress: []string{"snyk", "evil"}},
			want:     config.LintScannerPolicy{Preset: "strict", Required: []string{"agnix", "cisco"}, FailOn: "warning", Isolation: "auto", AllowEgress: []string{"snyk"}},
			wantKeys: []string{"lint.scanner_policy.allow_egress", "lint.scanner_policy.fail_on", "lint.scanner_policy.isolation", "lint.scanner_policy.preset"},
		},
		{
			name:     "stricter values are accepted",
			repo:     &config.LintScannerPolicy{Preset: "strict", FailOn: "info", Isolation: "require", AllowEgress: []string{}},
			want:     config.LintScannerPolicy{Preset: "strict", Required: []string{"cisco"}, FailOn: "info", Isolation: "require", AllowEgress: []string{}},
			accepted: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "[lint.scanner_policy]\npreset = \"off\"\n")
			cfg.Lint = &config.LintConfig{ScannerPolicy: tt.repo}
			// Act
			res := Resolve([]Layer{layer("managed", pol)}).Apply(cfg)
			// Assert
			assert.Equal(t, tt.want, *cfg.Lint.ScannerPolicy)
			var keys []string
			for _, v := range res.Outcome.Violations {
				assert.Equal(t, "AR740", v.Code)
				assert.Equal(t, "managed", v.Origin)
				keys = append(keys, v.Key)
			}
			assert.ElementsMatch(t, tt.wantKeys, keys)
			assert.Equal(t, tt.accepted, len(res.Accepted) > 0)
		})
	}
}

func TestApplyEmptyAllowEgressPolicyForbidsEverySender(t *testing.T) {
	cfg := testConfig(t, "")
	cfg.Lint = &config.LintConfig{ScannerPolicy: &config.LintScannerPolicy{AllowEgress: []string{"snyk"}}}
	pol := Policy{Lint: Lint{ScannerPolicy: ScannerPolicy{AllowEgress: List{Set: true}}}}

	res := Resolve([]Layer{layer("managed", pol)}).Apply(cfg)

	require.NotNil(t, cfg.Lint.ScannerPolicy.AllowEgress, "set but empty: a nil list would mean the flag alone decides")
	assert.Empty(t, cfg.Lint.ScannerPolicy.AllowEgress)
	assert.Len(t, res.Outcome.Violations, 1)
}

func TestScannerPolicyShowsInThePolicyView(t *testing.T) {
	pol := Policy{Lint: Lint{ScannerPolicy: ScannerPolicy{Preset: "baseline", Required: []string{"agnix"}, FailOn: "warning", Isolation: "require", AllowEgress: List{Set: true}}}}
	res := Resolve([]Layer{layer("flag", pol)})
	flat := map[string]string{}
	for _, e := range flatten(res.Policy.Tree()) {
		flat[e.key] = e.value
	}
	assert.Equal(t, "baseline", flat["lint.scanner_policy.preset"])
	assert.Equal(t, `["agnix"]`, flat["lint.scanner_policy.required"])
	assert.Equal(t, "warning", flat["lint.scanner_policy.fail_on"])
	assert.Equal(t, "require", flat["lint.scanner_policy.isolation"])
	assert.Equal(t, `[]`, flat["lint.scanner_policy.allow_egress"])
	for key := range flat {
		if len(key) > 20 && key[:20] == "lint.scanner_policy." {
			assert.Equal(t, "flag", res.Provenance[key], "origin of %s", key)
		}
	}
}
