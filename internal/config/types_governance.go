package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"
)

// GovernanceConfig is the [governance] table: which pinned content needs a
// recorded reviewer approval in ai-rulez.lock (docs/approvals.md). Without
// require_approval nothing needs approval.
type GovernanceConfig struct {
	// RequireApproval selects the content that needs approval: "remote" (includes,
	// installed skills, skill sources and the remote skills the server serves),
	// "local" (every authored item), "all", or "kind:<kind>".
	RequireApproval []string `yaml:"require_approval,omitempty" json:"require_approval,omitempty" toml:"require_approval,omitempty"` //nolint:tagliatelle
	// Exempt lists globs over "kind:domain/id" (or "kind:id") that never need approval.
	Exempt []string `yaml:"exempt,omitempty" json:"exempt,omitempty" toml:"exempt,omitempty"`
	// MinApprovers is how many distinct reviewers must approve one digest. Default 1.
	MinApprovers int `yaml:"min_approvers,omitempty" json:"min_approvers,omitempty" toml:"min_approvers,omitempty"` //nolint:tagliatelle
	// Approvers, when set, is the allowlist of reviewer strings (exact match).
	Approvers []string `yaml:"approvers,omitempty" json:"approvers,omitempty" toml:"approvers,omitempty"`
	// MaxAge is the default lifetime of a new approval ("365d", "720h"); empty means no expiry.
	MaxAge string `yaml:"max_age,omitempty" json:"max_age,omitempty" toml:"max_age,omitempty"` //nolint:tagliatelle
	// Enforce makes `lock --check`, `generate --locked` and the skills server fail on
	// content whose approval is missing, stale, expired or insufficient.
	Enforce bool `yaml:"enforce,omitempty" json:"enforce,omitempty" toml:"enforce,omitempty"`

	// PolicyFloor holds the require_approval selectors an organization policy
	// imposes (docs/policy.md). Runtime only: the policy sets it, never the file.
	// Unlike RequireApproval it is not narrowed by Exempt.
	PolicyFloor []string `yaml:"-" json:"-" toml:"-"`
}

// Approval selectors besides "kind:<kind>".
const (
	ApprovalSelectorRemote = "remote"
	ApprovalSelectorLocal  = "local"
	ApprovalSelectorAll    = "all"
	approvalKindPrefix     = "kind:"
)

// ApprovalKinds are the kinds `kind:<kind>` accepts: the pinned item kinds and
// the remote entry kinds. "mcp_server" selects the pinned MCP server
// declarations (the settings item "mcp-servers").
var ApprovalKinds = []string{
	"rule", "context", "skill", "agent", "command", "check", "hook", "role", "settings", "local-include",
	"include", "installed-skill", "source", "served", "mcp_server",
}

// ParseApprovalSelector validates one require_approval entry and returns the
// kind for a "kind:<kind>" selector ("" for the keywords).
func ParseApprovalSelector(sel string) (kind string, err error) {
	switch sel {
	case ApprovalSelectorRemote, ApprovalSelectorLocal, ApprovalSelectorAll:
		return "", nil
	}
	if k, ok := strings.CutPrefix(sel, approvalKindPrefix); ok && slices.Contains(ApprovalKinds, k) {
		return k, nil
	}
	return "", fmt.Errorf("invalid selector %q (use remote, local, all or kind:<%s>)", sel, strings.Join(ApprovalKinds, "|"))
}

// ParseApprovalMaxAge parses a max_age: whole days ("365d") or a Go duration ("720h").
func ParseApprovalMaxAge(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid max_age %q (use a positive number of days such as \"365d\", or a duration such as \"720h\")", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid max_age %q (use a positive number of days such as \"365d\", or a duration such as \"720h\")", s)
	}
	return d, nil
}

func (c *Config) validateGovernance() error {
	g := c.Governance
	if g == nil {
		return nil
	}
	for _, sel := range g.RequireApproval {
		if _, err := ParseApprovalSelector(sel); err != nil {
			return oops.With("field", "governance.require_approval").Wrap(err)
		}
	}
	if g.MinApprovers < 0 {
		return oops.With("field", "governance.min_approvers").Errorf("min_approvers must not be negative")
	}
	if g.MaxAge != "" {
		if _, err := ParseApprovalMaxAge(g.MaxAge); err != nil {
			return oops.With("field", "governance.max_age").Wrap(err)
		}
	}
	return nil
}
