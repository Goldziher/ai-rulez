package policy

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	hostPool = []string{"github.com", "github.com/example-org", "github.com/example-org/rules", "github.com/other", "*.example.org", "git.example.org", "gitlab.com/*"}
	secHosts = []string{"github.com", "*.example.org", "git.example.org", "example.org", "evil.test"}
	codePool = []string{"AR001", "AR005", "AR008", "AR010", "AR701"}
	tagPool  = []string{"assistant", "human", "tool"}
	orgPool  = []string{"acme", "anthropics", "evil", "github"}
	// budgetPool are load-budget ids the random policies bound.
	budgetPool = []string{"claude-skill-listing", "cursor-rule-lines", "codex-agents-chain"}
)

func pick(rng *rand.Rand, pool []string) []string {
	var out []string
	for _, s := range pool {
		if rng.Intn(3) == 0 {
			out = append(out, s)
		}
	}
	return sortedUnique(out)
}

func randomList(rng *rand.Rand, pool []string) List {
	if rng.Intn(3) == 0 {
		return List{}
	}
	return List{Set: true, Items: pick(rng, pool)}
}

// randomPolicy builds a normalized Policy: lists sorted and unique, empty
// slices nil, so structural equality means semantic equality.
func randomPolicy(rng *rand.Rand) Policy {
	var p Policy
	p.Sources.Allowed = randomList(rng, hostPool)
	p.Sources.Deny = pick(rng, hostPool)
	p.Sources.RequirePinned = rng.Intn(3) == 0
	p.Lint.RequiredCodes = pick(rng, codePool)
	for _, c := range codePool {
		if rng.Intn(3) == 0 {
			if p.Lint.SeverityFloor == nil {
				p.Lint.SeverityFloor = map[string]string{}
			}
			p.Lint.SeverityFloor[c] = []string{"info", "warning", "error"}[rng.Intn(3)]
		}
	}
	p.Lint.Security.AllowedHosts = randomList(rng, secHosts)
	p.Lint.Security.ScanImports = []string{"", "warn", "error"}[rng.Intn(3)]
	p.Lint.Security.DirectiveTags = pick(rng, tagPool)
	p.Lint.Security.TrustedOrgs = randomList(rng, orgPool)
	if rng.Intn(2) == 0 {
		p.Lint.Capability.MaxNetworkCommands = intPtr(rng.Intn(8))
	}
	for _, id := range budgetPool {
		if rng.Intn(3) == 0 {
			if p.Lint.LoadBudgets == nil {
				p.Lint.LoadBudgets = map[string]int{}
			}
			p.Lint.LoadBudgets[id] = 100 * (1 + rng.Intn(10))
		}
	}
	p.Lock = Lock{Enforce: rng.Intn(3) == 0, IncludeOutputs: rng.Intn(3) == 0}
	p.Telemetry.Disabled = rng.Intn(3) == 0
	p.LLM.Disabled = rng.Intn(3) == 0
	p.Guard.Generated = rng.Intn(3) == 0
	p.Governance.Enforce = rng.Intn(3) == 0
	p.Governance.RequireApproval = pick(rng, []string{"remote", "local", "kind:hook", "kind:skill"})
	p.Governance.MinApprovers = rng.Intn(4)
	p.Governance.Approvers = randomList(rng, []string{"a@x.org", "b@x.org", "c@x.org"})
	p.MCP.AllowedCommands = randomList(rng, []string{"npx", "uvx", "node", "bash"})
	p.MCP.DenyTransports = pick(rng, mcpTransports)
	p.Hooks.Forbidden = rng.Intn(3) == 0
	if rng.Intn(2) == 0 {
		p.Signing.RequireVerified = []string{"lock"}
	}
	p.Signing.TLog = []string{"", "optional", "required"}[rng.Intn(3)]
	if rng.Intn(2) == 0 {
		p.Signing.MaxAge = time.Duration(1+rng.Intn(400)) * 24 * time.Hour
	}
	p.Signing.MinHashVersion = rng.Intn(4)
	for _, c := range codePool {
		if rng.Intn(3) == 0 {
			if p.Lint.MaxFindings == nil {
				p.Lint.MaxFindings = map[string]int{}
			}
			p.Lint.MaxFindings[c] = rng.Intn(5)
		}
	}
	p.Lint.NoInlineIgnore = pick(rng, codePool)
	if rng.Intn(2) == 0 {
		p.Sources.MinReleaseAge = time.Duration(1+rng.Intn(60)) * 24 * time.Hour
	}
	for _, kind := range []string{"rule", "skill", "agent"} {
		if rng.Intn(3) == 0 {
			if p.Lint.SizeBudgets == nil {
				p.Lint.SizeBudgets = map[string]SizeBudget{}
			}
			p.Lint.SizeBudgets[kind] = SizeBudget{MaxLines: rng.Intn(3) * 100, MaxTokens: rng.Intn(3) * 1000}
		}
	}
	p.Signing.Trust = randomList(rng, []string{"subject=lock identity=a issuer=i", "subject=lock identity=b issuer=i", "subject=lock identity_regexp=^c$ issuer=i"})
	return p
}

func TestMergeKeyTypes(t *testing.T) {
	list := func(items ...string) List { return List{Set: true, Items: items} }
	tests := []struct {
		name string
		a, b Policy
		want Policy
	}{
		{
			"allowlists intersect by coverage",
			Policy{Sources: Sources{Allowed: list("github.com")}},
			Policy{Sources: Sources{Allowed: list("github.com/example-org", "gitlab.com")}},
			Policy{Sources: Sources{Allowed: list("github.com/example-org")}},
		},
		{
			"an unset allowlist constrains nothing",
			Policy{Sources: Sources{Allowed: list("github.com")}}, Policy{},
			Policy{Sources: Sources{Allowed: list("github.com")}},
		},
		{
			"disjoint allowlists leave nothing allowed",
			Policy{Sources: Sources{Allowed: list("a.com")}},
			Policy{Sources: Sources{Allowed: list("b.com")}},
			Policy{Sources: Sources{Allowed: List{Set: true}}},
		},
		{
			"denylists and required codes union",
			Policy{Sources: Sources{Deny: []string{"a.com"}}, Lint: Lint{RequiredCodes: []string{"AR001"}}},
			Policy{Sources: Sources{Deny: []string{"b.com"}}, Lint: Lint{RequiredCodes: []string{"AR005"}}},
			Policy{Sources: Sources{Deny: []string{"a.com", "b.com"}}, Lint: Lint{RequiredCodes: []string{"AR001", "AR005"}}},
		},
		{
			"severity floors take the higher",
			Policy{Lint: Lint{SeverityFloor: map[string]string{"AR001": "warning", "AR005": "error"}}},
			Policy{Lint: Lint{SeverityFloor: map[string]string{"AR001": "error", "AR005": "info", "AR008": "info"}}},
			Policy{Lint: Lint{SeverityFloor: map[string]string{"AR001": "error", "AR005": "error", "AR008": "info"}}},
		},
		{
			"scan_imports takes the stricter level",
			Policy{Lint: Lint{Security: Security{ScanImports: "warn"}}},
			Policy{Lint: Lint{Security: Security{ScanImports: "error"}}},
			Policy{Lint: Lint{Security: Security{ScanImports: "error"}}},
		},
		{
			"an unset scan_imports never loosens warn",
			Policy{Lint: Lint{Security: Security{ScanImports: "warn"}}}, Policy{},
			Policy{Lint: Lint{Security: Security{ScanImports: "warn"}}},
		},
		{
			"switches only turn on",
			Policy{Lock: Lock{Enforce: true}, Guard: Guard{Generated: true}, Telemetry: Network{Disabled: true}},
			Policy{Lock: Lock{IncludeOutputs: true}, LLM: Network{Disabled: true}, Sources: Sources{RequirePinned: true}},
			Policy{
				Lock: Lock{Enforce: true, IncludeOutputs: true}, Guard: Guard{Generated: true},
				Telemetry: Network{Disabled: true}, LLM: Network{Disabled: true}, Sources: Sources{RequirePinned: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Merge(tt.a, tt.b)
			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, got, Merge(tt.b, tt.a), "merge is commutative")
		})
	}
}

func TestMergeLaws(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 2000; i++ {
		a, b, c := randomPolicy(rng), randomPolicy(rng), randomPolicy(rng)
		assert.Equal(t, Merge(a, b), Merge(b, a), "commutative: %+v %+v", a, b)
		assert.Equal(t, Merge(a, a), Merge(a, Policy{}), "idempotent up to normalization")
		assert.Equal(t, Merge(Merge(a, b), c), Merge(a, Merge(b, c)), "associative: %+v %+v %+v", a, b, c)
		assert.Equal(t, Merge(a, Policy{}), Merge(Policy{}, a), "the zero policy is the identity")
		m := Merge(a, b)
		assert.Equal(t, m, Merge(m, a), "merging a layer already folded in changes nothing")
		assert.Equal(t, m, Merge(m, b))
	}
}

// strictness views: a tighter policy never admits more.
func TestMergeOnlyTightens(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 2000; i++ {
		a, b := randomPolicy(rng), randomPolicy(rng)
		m := Merge(a, b)
		for _, side := range []Policy{a, b} {
			// every allowlist entry that survives was covered by the side that set one
			if side.Sources.Allowed.Set {
				assert.True(t, m.Sources.Allowed.Set)
				for _, item := range m.Sources.Allowed.Items {
					assert.True(t, anyCovers(side.Sources.Allowed.Items, item), "%q escaped %v", item, side.Sources.Allowed.Items)
				}
			}
			assert.Subset(t, m.Sources.Deny, side.Sources.Deny)
			assert.Subset(t, m.Lint.RequiredCodes, side.Lint.RequiredCodes)
			for code, sev := range side.Lint.SeverityFloor {
				assert.GreaterOrEqual(t, severityRank[m.Lint.SeverityFloor[code]], severityRank[sev])
			}
			assert.GreaterOrEqual(t, policyScanRank[m.Lint.Security.ScanImports], policyScanRank[side.Lint.Security.ScanImports])
			assert.Subset(t, m.Lint.Security.DirectiveTags, side.Lint.Security.DirectiveTags)
			if side.Lint.Security.TrustedOrgs.Set {
				assert.True(t, m.Lint.Security.TrustedOrgs.Set)
				assert.Subset(t, side.Lint.Security.TrustedOrgs.Items, m.Lint.Security.TrustedOrgs.Items, "an org escaped the policy list")
			}
			if side.Lint.Capability.MaxNetworkCommands != nil {
				require.NotNil(t, m.Lint.Capability.MaxNetworkCommands)
				assert.LessOrEqual(t, *m.Lint.Capability.MaxNetworkCommands, *side.Lint.Capability.MaxNetworkCommands)
			}
			for id, limit := range side.Lint.LoadBudgets {
				assert.LessOrEqual(t, m.Lint.LoadBudgets[id], limit, "budget %s", id)
				assert.Positive(t, m.Lint.LoadBudgets[id])
			}
			assert.True(t, !side.Lock.Enforce || m.Lock.Enforce)
			assert.True(t, !side.Telemetry.Disabled || m.Telemetry.Disabled)
			if side.MCP.AllowedCommands.Set {
				assert.True(t, m.MCP.AllowedCommands.Set)
				assert.Subset(t, side.MCP.AllowedCommands.Items, m.MCP.AllowedCommands.Items, "a command escaped the allowlist")
			}
			assert.Subset(t, m.MCP.DenyTransports, side.MCP.DenyTransports)
			assert.True(t, !side.Hooks.Forbidden || m.Hooks.Forbidden)
			assert.GreaterOrEqual(t, m.Sources.MinReleaseAge, side.Sources.MinReleaseAge)
			for kind, sb := range side.Lint.SizeBudgets {
				if sb.MaxLines > 0 {
					assert.Positive(t, m.Lint.SizeBudgets[kind].MaxLines)
					assert.LessOrEqual(t, m.Lint.SizeBudgets[kind].MaxLines, sb.MaxLines)
				}
				if sb.MaxTokens > 0 {
					assert.Positive(t, m.Lint.SizeBudgets[kind].MaxTokens)
					assert.LessOrEqual(t, m.Lint.SizeBudgets[kind].MaxTokens, sb.MaxTokens)
				}
			}
			for code, limit := range side.Lint.MaxFindings {
				got, ok := m.Lint.MaxFindings[code]
				assert.True(t, ok, "ceiling of %s was dropped", code)
				assert.LessOrEqual(t, got, limit)
			}
			assert.Subset(t, m.Lint.NoInlineIgnore, side.Lint.NoInlineIgnore)
			assert.Subset(t, m.Signing.RequireVerified, side.Signing.RequireVerified)
			assert.GreaterOrEqual(t, tlogRank[m.Signing.TLog], tlogRank[side.Signing.TLog])
			if side.Signing.MaxAge > 0 {
				assert.Positive(t, m.Signing.MaxAge)
				assert.LessOrEqual(t, m.Signing.MaxAge, side.Signing.MaxAge)
			}
			assert.GreaterOrEqual(t, m.Signing.MinHashVersion, side.Signing.MinHashVersion)
			if side.Signing.Trust.Set {
				assert.True(t, m.Signing.Trust.Set)
				assert.Subset(t, side.Signing.Trust.Items, m.Signing.Trust.Items, "a signer escaped the policy list")
			}
		}
	}
}
