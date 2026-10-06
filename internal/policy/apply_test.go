package policy

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

func layer(origin string, p Policy) Layer {
	return Layer{Origin: origin, Path: "/policy/" + origin + ".toml", Digest: "sha256:" + origin, Policy: p}
}

func boolPtr(b bool) *bool { return &b }

func testConfig(t *testing.T, body string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o644))
	return &config.Config{ConfigDir: dir, ConfigFile: "config.toml"}
}

func codes(out *config.PolicyOutcome) []string {
	var got []string
	for _, v := range out.Violations {
		got = append(got, v.Code+" "+v.Key)
	}
	return got
}

func TestApplySeveritiesAndRequiredCodes(t *testing.T) {
	policy := Policy{Lint: Lint{
		RequiredCodes: []string{"AR001"},
		SeverityFloor: map[string]string{"AR008": "warning", "AR005": "error"},
	}}
	tests := []struct {
		name         string
		lint         config.LintConfig
		wantViol     []string
		wantSeverity map[string]string
		wantIgnore   []string
	}{
		{"untouched repo has no violation", config.LintConfig{}, nil, nil, nil},
		{
			"severity below the floor is raised and reported",
			config.LintConfig{Severity: map[string]string{"AR008": "info"}},
			[]string{"AR740 lint.severity.AR008"}, map[string]string{"AR008": "warning"}, nil,
		},
		{
			"off below the floor is raised and reported",
			config.LintConfig{Severity: map[string]string{"path-missing": "off", "AR005": "off"}},
			[]string{"AR740 lint.severity.AR005"}, map[string]string{"AR005": "error", "path-missing": "off"}, nil,
		},
		{
			"a stricter severity is accepted",
			config.LintConfig{Severity: map[string]string{"AR008": "error"}},
			nil, map[string]string{"AR008": "error"}, nil,
		},
		{
			"a required code cannot be turned off",
			config.LintConfig{Severity: map[string]string{"AR001": "off"}},
			[]string{"AR744 lint.severity.AR001"}, map[string]string{}, nil,
		},
		{
			"a required code can be demoted but not removed",
			config.LintConfig{Severity: map[string]string{"AR001": "info"}},
			nil, map[string]string{"AR001": "info"}, nil,
		},
		{
			"required and floored codes cannot be ignored",
			config.LintConfig{Ignore: []string{"secret-detected", "AR008", "AR701"}},
			[]string{"AR740 lint.ignore", "AR744 lint.ignore"}, nil, []string{"AR701"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "name = \"x\"\n[lint.severity]\nAR008 = \"info\"\n")
			lc := tt.lint
			cfg.Lint = &lc
			res := Resolve([]Layer{layer("managed", policy)})
			// Act
			out := res.Apply(cfg).Outcome
			// Assert
			assert.ElementsMatch(t, tt.wantViol, codes(out))
			if tt.wantSeverity != nil {
				assert.Equal(t, tt.wantSeverity, cfg.Lint.Severity)
			}
			assert.Equal(t, tt.wantIgnore, cfg.Lint.Ignore)
			assert.Equal(t, "error", out.SeverityFloor["AR005"])
			assert.Equal(t, []string{"AR001"}, out.RequiredCodes)
		})
	}
}

func TestApplyViolationCarriesOriginFileAndLine(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n\n[lint.severity]\nAR001 = \"info\"\nAR008 = \"off\"\n")
	cfg.Lint = &config.LintConfig{Severity: map[string]string{"AR008": "off"}}
	res := Resolve([]Layer{layer("managed", Policy{Lint: Lint{SeverityFloor: map[string]string{"AR008": "warning"}}})})
	// Act
	v := res.Apply(cfg).Outcome.Violations
	// Assert
	require.Len(t, v, 1)
	assert.Equal(t, "AR740", v[0].Code)
	assert.Equal(t, 5, v[0].Line)
	assert.Equal(t, filepath.Join(cfg.ConfigDir, "config.toml"), v[0].File)
	assert.Equal(t, "managed", v[0].Origin)
	assert.Contains(t, v[0].Message, "below the policy floor")
}

func TestApplySourcesAllowAndDeny(t *testing.T) {
	policy := Policy{Sources: Sources{
		Allowed: List{Set: true, Items: []string{"github.com/example-org", "*.example.org"}},
		Deny:    []string{"git.example.org/banned"},
	}}
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.Includes = []config.IncludeConfig{
		{Name: "ok", Source: "https://github.com/example-org/rules.git"},
		{Name: "evil", Source: "https://github.com/other-org/rules"},
		{Name: "local", Source: "../shared"},
		{Name: "ssh", Source: "git@git.example.org:team/rules.git"},
		{Name: "banned", Source: "https://git.example.org/banned/rules"},
	}
	cfg.InstalledSkills = []config.InstalledSkillConfig{
		{Name: "good", Source: "https://github.com/example-org/skills"},
		{Name: "bad", Source: "git@evil.test:x/y.git"},
	}
	cfg.SkillSources = []config.SkillSourceConfig{
		{Name: "src", URL: "https://github.com/example-org/s"},
		{Name: "src2", URL: "https://github.com/typo-org/s"},
	}
	res := Resolve([]Layer{layer("env", policy)})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	var kept []string
	for _, i := range cfg.Includes {
		kept = append(kept, i.Name)
	}
	assert.Equal(t, []string{"ok", "local", "ssh"}, kept, "a disallowed source is not loaded")
	require.Len(t, cfg.InstalledSkills, 1)
	require.Len(t, cfg.SkillSources, 1)
	assert.ElementsMatch(t, []string{
		"AR745 sources.allowed_hosts", "AR745 sources.allowed_hosts", "AR745 sources.allowed_hosts", "AR745 sources.deny_hosts",
	}, codes(out))
	for _, v := range out.Violations {
		assert.Equal(t, "env", v.Origin)
	}
}

func TestApplyEmptyAllowlistRefusesEveryRemote(t *testing.T) {
	cfg := testConfig(t, "")
	cfg.Includes = []config.IncludeConfig{{Name: "a", Source: "https://github.com/x/y"}, {Name: "l", Source: "./local"}}
	res := Resolve([]Layer{layer("flag", Policy{Sources: Sources{Allowed: List{Set: true}}})})
	out := res.Apply(cfg).Outcome
	require.Len(t, cfg.Includes, 1)
	assert.Equal(t, "l", cfg.Includes[0].Name)
	assert.Len(t, out.Violations, 1)
}

func TestApplySecurityHosts(t *testing.T) {
	policy := Policy{Lint: Lint{Security: Security{AllowedHosts: List{Set: true, Items: []string{"github.com", "*.example.org"}}}}}
	tests := []struct {
		name     string
		repo     []string
		want     []string
		wantViol int
		accepted bool
	}{
		{"unset takes the policy list", nil, []string{"github.com", "*.example.org"}, 0, false},
		{"covered entries are kept (narrowed)", []string{"git.example.org"}, []string{"git.example.org"}, 0, true},
		{"uncovered entries are dropped and reported", []string{"git.example.org", "evil.test"}, []string{"git.example.org"}, 1, true},
		{"nothing covered falls back to the policy list", []string{"evil.test"}, []string{"github.com", "*.example.org"}, 1, false},
		{"a wider wildcard is not provably inside", []string{"*.org"}, []string{"github.com", "*.example.org"}, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t, "[lint.security]\nallowed_hosts = []\n")
			if tt.repo != nil {
				cfg.Lint = &config.LintConfig{Security: &config.LintSecurity{AllowedHosts: tt.repo}}
			}
			res := Resolve([]Layer{layer("managed", policy)}).Apply(cfg)
			assert.Equal(t, tt.want, cfg.Lint.Security.AllowedHosts)
			assert.Len(t, res.Outcome.Violations, tt.wantViol)
			assert.Equal(t, tt.accepted, len(res.Accepted) > 0)
		})
	}
}

func TestApplyEmptySecurityPolicyListAllowsNoHost(t *testing.T) {
	cfg := testConfig(t, "")
	cfg.Lint = &config.LintConfig{Security: &config.LintSecurity{AllowedHosts: []string{"github.com"}}}
	res := Resolve([]Layer{layer("managed", Policy{Lint: Lint{Security: Security{AllowedHosts: List{Set: true}}}})})
	out := res.Apply(cfg).Outcome
	assert.Equal(t, []string{noHostSentinel}, cfg.Lint.Security.AllowedHosts, "an empty lint list would switch the check off")
	assert.Len(t, out.Violations, 1)
}

func TestApplyScanImports(t *testing.T) {
	tests := []struct {
		name, policy, repo, want string
		wantViol                 int
	}{
		{"unset under error is enforced silently", "error", "", "error", 0},
		{"unset under warn stays default", "warn", "", "", 0},
		{"off is raised and reported", "warn", "off", "warn", 1},
		{"warn under error is raised and reported", "error", "warn", "error", 1},
		{"equal is accepted", "error", "error", "error", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t, "")
			cfg.Lint = &config.LintConfig{Security: &config.LintSecurity{ScanImports: tt.repo}}
			res := Resolve([]Layer{layer("managed", Policy{Lint: Lint{Security: Security{ScanImports: tt.policy}}})}).Apply(cfg)
			assert.Equal(t, tt.want, cfg.Lint.Security.ScanImports)
			assert.Len(t, res.Outcome.Violations, tt.wantViol)
		})
	}
}

func TestApplyLockAndSwitches(t *testing.T) {
	tests := []struct {
		name        string
		policy      Policy
		lock        *config.LockConfig
		wantEnforce *bool
		wantViol    int
	}{
		{"unset is enforced silently", Policy{Lock: Lock{Enforce: true}}, nil, boolPtr(true), 0},
		{"false is raised and reported", Policy{Lock: Lock{Enforce: true}}, &config.LockConfig{Enforce: boolPtr(false)}, boolPtr(true), 1},
		{"require_pinned enforces the lock", Policy{Sources: Sources{RequirePinned: true}}, &config.LockConfig{Enforce: boolPtr(false)}, boolPtr(true), 1},
		{"true is accepted", Policy{Lock: Lock{Enforce: true}}, &config.LockConfig{Enforce: boolPtr(true)}, boolPtr(true), 0},
		{"no lock policy leaves it alone", Policy{}, &config.LockConfig{Enforce: boolPtr(false)}, boolPtr(false), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t, "[lock]\nenforce = false\n")
			cfg.Lock = tt.lock
			res := Resolve([]Layer{layer("managed", tt.policy)}).Apply(cfg)
			if tt.wantEnforce == nil {
				assert.Nil(t, cfg.Lock)
			} else {
				assert.Equal(t, *tt.wantEnforce, *cfg.Lock.Enforce)
			}
			assert.Len(t, res.Outcome.Violations, tt.wantViol)
		})
	}
}

func TestApplyIncludeOutputsNetworksAndGuard(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "[guard]\nversion = \"1\"\n")
	cfg.Lock = &config.LockConfig{IncludeOutputs: boolPtr(false)}
	cfg.Telemetry = &config.TelemetryConfig{AllowNetwork: true}
	cfg.LLM = &llm.Config{AllowNetwork: true}
	cfg.Guard = &config.GuardConfig{Version: "1"}
	res := Resolve([]Layer{layer("managed", Policy{
		Lock: Lock{IncludeOutputs: true}, Telemetry: Network{Disabled: true}, LLM: Network{Disabled: true}, Guard: Guard{Generated: true},
	})})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	assert.ElementsMatch(t, []string{
		"AR740 lock.include_outputs", "AR740 telemetry.allow_network", "AR740 llm.allow_network", "AR740 guard.generated",
	}, codes(out))
	assert.True(t, *cfg.Lock.IncludeOutputs)
	assert.False(t, cfg.Telemetry.AllowNetwork)
	assert.False(t, cfg.LLM.AllowNetwork)
	assert.True(t, cfg.Guard.Generated)
}

func TestApplyGuardUnsetIsForcedSilently(t *testing.T) {
	cfg := testConfig(t, "name = \"x\"\n")
	res := Resolve([]Layer{layer("managed", Policy{Guard: Guard{Generated: true}})}).Apply(cfg)
	assert.True(t, cfg.Guard.Generated)
	assert.Empty(t, res.Outcome.Violations)
}

// A policy that constrains nothing leaves the configuration byte for byte alone.
func TestEmptyPolicyChangesNothing(t *testing.T) {
	cfg := testConfig(t, "")
	cfg.Lint = &config.LintConfig{Severity: map[string]string{"AR001": "off"}, Ignore: []string{"AR008"}, Security: &config.LintSecurity{AllowedHosts: []string{"x.test"}}}
	cfg.Includes = []config.IncludeConfig{{Name: "a", Source: "https://evil.test/x"}}
	before := *cfg.Lint
	res := Resolve([]Layer{layer("managed", Policy{})}).Apply(cfg)
	assert.Empty(t, res.Outcome.Violations)
	assert.Equal(t, before.Severity, cfg.Lint.Severity)
	assert.Equal(t, before.Ignore, cfg.Lint.Ignore)
	assert.Equal(t, []string{"x.test"}, cfg.Lint.Security.AllowedHosts)
	assert.Len(t, cfg.Includes, 1)
	assert.Nil(t, cfg.Lock)
}

func TestProvenance(t *testing.T) {
	// Arrange
	layers := []Layer{
		layer("flag", Policy{Lint: Lint{SeverityFloor: map[string]string{"AR008": "error"}}, Lock: Lock{Enforce: true}}),
		layer("managed", Policy{
			Lint:    Lint{SeverityFloor: map[string]string{"AR008": "warning", "AR005": "error"}, RequiredCodes: []string{"AR001"}},
			Sources: Sources{Allowed: List{Set: true, Items: []string{"github.com"}}},
			Lock:    Lock{Enforce: true},
		}),
		layer("env", Policy{Lint: Lint{RequiredCodes: []string{"AR005"}}, Sources: Sources{Allowed: List{Set: true, Items: []string{"github.com"}}}}),
	}
	// Act
	got := Resolve(layers).Provenance
	// Assert
	assert.Equal(t, "flag", got["lint.severity_floor.AR008"], "the layer that raised it")
	assert.Equal(t, "managed", got["lint.severity_floor.AR005"])
	assert.Equal(t, "flag", got["lock.enforce"], "the strongest anchor wins a tie")
	assert.Equal(t, "managed+env", got["lint.required_codes"])
	assert.Equal(t, "managed+env", got["sources.allowed_hosts"])
}

// Monotonicity: a tighter policy never yields fewer violations on the same
// repository, and its effective configuration is at least as strict.
func TestApplyIsMonotonic(t *testing.T) {
	rng := rand.New(rand.NewSource(216))
	for i := 0; i < 400; i++ {
		base, extra := randomPolicy(rng), randomPolicy(rng)
		tight := Merge(base, extra)
		seed := rng.Int63()
		cfgBase, cfgTight := randomRepoConfig(t, seed), randomRepoConfig(t, seed)
		vBase := Resolve([]Layer{layer("managed", base)}).Apply(cfgBase).Outcome.Violations
		vTight := Resolve([]Layer{layer("managed", base), layer("flag", extra)}).Apply(cfgTight).Outcome.Violations
		assert.Equal(t, tight, Resolve([]Layer{layer("managed", base), layer("flag", extra)}).Policy)
		counts := func(vs []config.PolicyViolation) map[string]int {
			m := map[string]int{}
			for _, v := range vs {
				key := v.Key
				if v.Code == "AR745" {
					key = "sources" // one finding per refused source, whichever list refused it
				}
				m[key]++
			}
			return m
		}
		cb, ct := counts(vBase), counts(vTight)
		for key, n := range cb {
			assert.GreaterOrEqual(t, ct[key], n, "tighter policy lost a violation of %s (seed %d)", key, seed)
		}
		// effective strictness
		for _, code := range codePool {
			assert.GreaterOrEqual(t, sevOf(cfgTight, code), sevOf(cfgBase, code), "%s severity (seed %d)", code, seed)
		}
		assert.True(t, !enforced(cfgBase) || enforced(cfgTight))
		assert.LessOrEqual(t, len(cfgTight.Includes), len(cfgBase.Includes))
		assert.GreaterOrEqual(t, scanRankOf(cfgTight), scanRankOf(cfgBase))
	}
}

func randomRepoConfig(t *testing.T, seed int64) *config.Config {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	cfg := &config.Config{}
	lc := &config.LintConfig{Severity: map[string]string{}, Security: &config.LintSecurity{}}
	for _, c := range codePool {
		switch rng.Intn(4) {
		case 0:
			lc.Severity[c] = []string{"off", "info", "warning", "error"}[rng.Intn(4)]
		case 1:
			lc.Ignore = append(lc.Ignore, c)
		}
	}
	lc.Security.AllowedHosts = pick(rng, secHosts)
	lc.Security.ScanImports = []string{"", "off", "warn", "error"}[rng.Intn(4)]
	cfg.Lint = lc
	if rng.Intn(2) == 0 {
		cfg.Lock = &config.LockConfig{Enforce: boolPtr(rng.Intn(2) == 0)}
	}
	for i, h := range []string{"github.com/example-org/rules", "github.com/other/x", "https://git.example.org/a/b", "gitlab.com/q/r"} {
		if rng.Intn(2) == 0 {
			src := h
			if i != 2 {
				src = "https://" + h
			}
			cfg.Includes = append(cfg.Includes, config.IncludeConfig{Name: h, Source: src})
		}
	}
	return cfg
}

// sevOf is the effective strictness of a code: 0 for off or ignored.
func sevOf(cfg *config.Config, code string) int {
	for _, c := range cfg.Lint.Ignore {
		if c == code {
			return 0
		}
	}
	if v, ok := cfg.Lint.Severity[code]; ok {
		return severityRank[v]
	}
	return 4 // unset keeps the rule's own severity
}

func enforced(cfg *config.Config) bool {
	return cfg.Lock != nil && cfg.Lock.Enforce != nil && *cfg.Lock.Enforce
}

func scanRankOf(cfg *config.Config) int { return scanImportsRank[cfg.Lint.Security.ScanImports] }
