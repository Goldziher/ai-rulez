// Package policy implements the tighten-only organization policy (docs/policy.md):
// discovery from anchors outside the repository, a per-key merge algebra that
// can only restrict, and the application of the result to a loaded
// configuration, which clamps what runs and reports every loosening attempt.
package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// Version is the policy file format this build reads.
const Version = 1

// maxPolicyBytes bounds a policy file.
const maxPolicyBytes = 256 << 10

// Severities a policy may require, lowest first. "off" is not a floor.
var severityRank = map[string]int{"info": 1, "warning": 2, "error": 3}

// scanImports levels, lowest first. An unset repository value ranks between
// warn and error: it keeps every finding's own severity.
var scanImportsRank = map[string]int{"off": 0, "warn": 1, "": 2, "error": 3}

// List is an allowlist. Set distinguishes "no constraint" (not set) from
// "nothing is allowed" (set and empty).
type List struct {
	Set   bool
	Items []string
}

// Policy is the normalized content of one policy file, or the merge of several.
// The zero value constrains nothing and is the identity of Merge.
type Policy struct {
	Sources    Sources
	Lint       Lint
	Lock       Lock
	Telemetry  Network
	LLM        Network
	Guard      Guard
	Governance Governance
	MCP        MCP
	Hooks      Hooks
	Signing    Signing
}

// Governance governs [governance] (docs/approvals.md): the approval floor the
// repository can raise but not lower.
type Governance struct {
	// Enforce makes missing approvals fail lock --check, generate --locked and the skills server.
	Enforce bool
	// RequireApproval lists selectors whose content always needs approval; the
	// repository's exempt list cannot remove them.
	RequireApproval []string
	// MinApprovers is the fewest distinct reviewers a digest needs.
	MinApprovers int
	// Approvers, when set, is the only list of reviewers that count (lower-cased).
	Approvers List
}

// Sources governs where remote content may come from.
type Sources struct {
	// Allowed lists host patterns every include, installed skill and skill
	// source must fall under.
	Allowed List
	// Deny lists host patterns that are never allowed.
	Deny []string
	// RequirePinned makes the lock enforced, so an unpinned remote is an error.
	RequirePinned bool
	// MinReleaseAge is the youngest a tag may be to be adopted: the floor under
	// [lock] min_release_age and each source's own. 0 for no floor.
	MinReleaseAge time.Duration
}

// Lint governs the strict-validation settings.
type Lint struct {
	// RequiredCodes lists rule codes that may be neither turned off nor ignored.
	RequiredCodes []string
	// SeverityFloor maps a rule code to the lowest severity it may have.
	SeverityFloor map[string]string
	Security      Security
	// Capability governs [lint.capability].
	Capability Capability
	// LoadBudgets maps a load-budget id to the largest limit the repository may
	// set for it (the lower of the layers wins).
	LoadBudgets map[string]int
	// ScannerPolicy governs [lint.scanner_policy] (scanners.go).
	ScannerPolicy ScannerPolicy
	// SizeBudgets maps a content kind to the largest size budget the repository
	// may set for it ([lint.budgets.<kind>]).
	SizeBudgets map[string]SizeBudget
	// MaxFindings maps a rule code to the most findings of it a run may have: one
	// more is AR749. The lower of the layers wins; 0 allows none.
	MaxFindings map[string]int
	// NoInlineIgnore lists rule codes whose inline ignore comments are not
	// honored. Layers union.
	NoInlineIgnore []string
}

// Capability governs [lint.capability].
type Capability struct {
	// MaxNetworkCommands is the largest max_network_commands the repository may
	// set; nil for no constraint.
	MaxNetworkCommands *int
}

// Security governs [lint.security].
type Security struct {
	// AllowedHosts bounds the repository's lint.security.allowed_hosts.
	AllowedHosts List
	// ScanImports is the weakest scan_imports level the repository may use:
	// "warn" or "error", "" for no constraint.
	ScanImports string
	// DirectiveTags are element names the repository's directive_tags always
	// include (it can only add more).
	DirectiveTags []string
	// TrustedOrgs bounds the repository's trusted_orgs: it may name only these
	// (lower-cased).
	TrustedOrgs List
}

// Lock governs [lock].
type Lock struct {
	Enforce        bool
	IncludeOutputs bool
}

// Network forbids a network feature (telemetry export, LLM calls) outright.
type Network struct{ Disabled bool }

// Guard governs [guard].
type Guard struct{ Generated bool }

// Layer is one loaded policy file.
type Layer struct {
	// Origin is how the layer was found: "flag", "env" or "managed".
	Origin string
	// Path is the file the layer was read from.
	Path string
	// Name is the policy's own name, if it has one.
	Name string
	// Note says how the layer was obtained when that is not plain: a cached copy
	// standing in for an unreachable URL.
	Note string
	// Digest is "sha256:" and the hex SHA-256 of the file, CRLF normalized.
	Digest string
	// Policy is the parsed content of this file alone (its extends are separate layers).
	Policy Policy
	// Extends lists the policies this one extends, as shown to people.
	Extends []string
	// extendsRaw is the extends list as written.
	extendsRaw []string
	// key is the identity of the policy (its URL or absolute path).
	key string
}

// fileDoc is the TOML form of a policy file. Pointers tell "unset" from the
// zero value; unknown keys are an error because a typo must not loosen.
type fileDoc struct {
	PolicyVersion *int         `toml:"policy_version"`
	Name          string       `toml:"name"`
	Extends       []string     `toml:"extends"`
	Sources       *fileSources `toml:"sources"`
	Lint          *fileLint    `toml:"lint"`
	Lock          *struct {
		Enforce        *bool `toml:"enforce"`
		IncludeOutputs *bool `toml:"include_outputs"`
	} `toml:"lock"`
	Telemetry *fileNetwork `toml:"telemetry"`
	LLM       *fileNetwork `toml:"llm"`
	Guard     *struct {
		Generated *bool `toml:"generated"`
	} `toml:"guard"`
	Governance *fileGovernance `toml:"governance"`
	MCP        *fileMCP        `toml:"mcp"`
	Hooks      *fileHooks      `toml:"hooks"`
	Signing    *fileSigning    `toml:"signing"`
}

type fileGovernance struct {
	Enforce         *bool     `toml:"enforce"`
	RequireApproval []string  `toml:"require_approval"`
	MinApprovers    *int      `toml:"min_approvers"`
	Approvers       *[]string `toml:"approvers"`
}

type fileSources struct {
	AllowedHosts  *[]string `toml:"allowed_hosts"`
	DenyHosts     []string  `toml:"deny_hosts"`
	RequirePinned *bool     `toml:"require_pinned"`
	MinReleaseAge string    `toml:"min_release_age"`
}

type fileLint struct {
	RequiredCodes []string          `toml:"required_codes"`
	SeverityFloor map[string]string `toml:"severity_floor"`
	Security      *fileSecurity     `toml:"security"`
	Capability    *struct {
		MaxNetworkCommands *int `toml:"max_network_commands"`
	} `toml:"capability"`
	LoadBudgets    map[string]int            `toml:"load_budgets"`
	ScannerPolicy  *fileScannerPolicy        `toml:"scanner_policy"`
	Budgets        map[string]fileSizeBudget `toml:"budgets"`
	MaxFindings    map[string]int            `toml:"max_findings"`
	NoInlineIgnore []string                  `toml:"no_inline_ignore"`
}

type fileSecurity struct {
	AllowedHosts  *[]string `toml:"allowed_hosts"`
	ScanImports   string    `toml:"scan_imports"`
	DirectiveTags []string  `toml:"directive_tags"`
	TrustedOrgs   *[]string `toml:"trusted_orgs"`
}

type fileNetwork struct {
	AllowNetwork *bool `toml:"allow_network"`
}

// ParseError is a policy that cannot be used (AR743).
type ParseError struct {
	Path string
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: policy %s: %s", lint.CodePolicyInvalid, e.Path, e.Msg)
}

// Parse reads one policy file's content, for a policy that does not extend
// another: extends is resolved when policies are loaded (Discover), not here. path
// is only used in messages.
func Parse(path string, data []byte) (name string, p Policy, err error) {
	d, err := parseDoc(path, data)
	if err != nil {
		return "", Policy{}, err
	}
	if len(d.extends) > 0 {
		return "", Policy{}, &ParseError{Path: path, Msg: "extends is resolved when the policy is loaded (--policy, AI_RULEZ_POLICY), not by Parse"}
	}
	return d.name, d.policy, nil
}

// parsedDoc is one policy file read: its own content and the extends it names.
type parsedDoc struct {
	name    string
	policy  Policy
	extends []string
}

// parseDoc reads one policy file. Unknown keys, a missing or newer
// policy_version and bad values are *ParseError (AR743).
func parseDoc(path string, data []byte) (parsedDoc, error) {
	var p Policy
	fail := func(format string, args ...any) (parsedDoc, error) {
		return parsedDoc{}, &ParseError{Path: path, Msg: fmt.Sprintf(format, args...)}
	}
	var doc fileDoc
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if derr := dec.Decode(&doc); derr != nil {
		var strict *toml.StrictMissingError
		if errors.As(derr, &strict) {
			var keys []string
			for _, d := range strict.Errors {
				keys = append(keys, strings.Join(d.Key(), "."))
			}
			return fail("unknown key(s) %s (this build reads policy_version %d; a newer policy needs a newer ai-rulez)", strings.Join(keys, ", "), Version)
		}
		return fail("%v", derr)
	}
	if doc.PolicyVersion == nil {
		return fail("policy_version is required (use %d)", Version)
	}
	if *doc.PolicyVersion != Version {
		return fail("policy_version %d is not supported by this build (it reads %d); upgrade ai-rulez", *doc.PolicyVersion, Version)
	}
	if len(doc.Extends) > maxExtendsPerFile {
		return fail("extends lists %d policies; at most %d", len(doc.Extends), maxExtendsPerFile)
	}
	for _, e := range doc.Extends {
		if strings.TrimSpace(e) == "" || strings.ContainsAny(e, "\r\n\x00") {
			return fail("extends: %q is not a policy path or URL", e)
		}
	}
	for _, step := range []func() error{
		func() error { return p.Sources.fromDoc(doc.Sources) },
		func() error { return p.Lint.fromDoc(doc.Lint) },
		func() error { return p.Governance.fromDoc(doc.Governance) },
		func() error { return p.MCP.fromDoc(doc.MCP) },
		func() error { return p.Hooks.fromDoc(doc.Hooks) },
		func() error { return p.Signing.fromDoc(doc.Signing) },
	} {
		if err := step(); err != nil {
			return fail("%v", err)
		}
	}
	if doc.Lock != nil {
		p.Lock.Enforce = doc.Lock.Enforce != nil && *doc.Lock.Enforce
		p.Lock.IncludeOutputs = doc.Lock.IncludeOutputs != nil && *doc.Lock.IncludeOutputs
	}
	if doc.Telemetry != nil {
		p.Telemetry.Disabled = doc.Telemetry.AllowNetwork != nil && !*doc.Telemetry.AllowNetwork
	}
	if doc.LLM != nil {
		p.LLM.Disabled = doc.LLM.AllowNetwork != nil && !*doc.LLM.AllowNetwork
	}
	if doc.Guard != nil {
		p.Guard.Generated = doc.Guard.Generated != nil && *doc.Guard.Generated
	}
	return parsedDoc{name: doc.Name, policy: p, extends: doc.Extends}, nil
}

func (g *Governance) fromDoc(d *fileGovernance) error {
	if d == nil {
		return nil
	}
	g.Enforce = d.Enforce != nil && *d.Enforce
	for _, sel := range d.RequireApproval {
		if _, err := config.ParseApprovalSelector(sel); err != nil {
			return fmt.Errorf("governance.require_approval: %w", err)
		}
	}
	g.RequireApproval = sortedUnique(d.RequireApproval)
	if d.MinApprovers != nil {
		if *d.MinApprovers < 0 {
			return fmt.Errorf("governance.min_approvers: %d must not be negative", *d.MinApprovers)
		}
		g.MinApprovers = *d.MinApprovers
	}
	if d.Approvers != nil {
		items := make([]string, 0, len(*d.Approvers))
		for _, a := range *d.Approvers {
			if n := approval.NormalizeReviewer(a); n != "" {
				items = append(items, n)
			}
		}
		g.Approvers = List{Set: true, Items: sortedUnique(items)}
	}
	return nil
}

func (s *Sources) fromDoc(d *fileSources) error {
	if d == nil {
		return nil
	}
	if d.AllowedHosts != nil {
		items, err := normalizeAll("sources.allowed_hosts", *d.AllowedHosts)
		if err != nil {
			return err
		}
		s.Allowed = List{Set: true, Items: items}
	}
	deny, err := normalizeAll("sources.deny_hosts", d.DenyHosts)
	if err != nil {
		return err
	}
	s.Deny = deny
	s.RequirePinned = d.RequirePinned != nil && *d.RequirePinned
	if d.MinReleaseAge != "" {
		age, err := parseMinReleaseAge(d.MinReleaseAge)
		if err != nil {
			return err
		}
		s.MinReleaseAge = age
	}
	return nil
}

func (l *Lint) fromDoc(d *fileLint) error {
	if d == nil {
		return nil
	}
	req, err := normalizeCodes("lint.required_codes", d.RequiredCodes)
	if err != nil {
		return err
	}
	l.RequiredCodes = req
	for key, sev := range d.SeverityFloor {
		code, ok := lint.ResolveCode(key)
		if !ok {
			return fmt.Errorf("lint.severity_floor: unknown rule %q (the policy is newer than this ai-rulez, or the name is wrong)", key)
		}
		sev = strings.ToLower(strings.TrimSpace(sev))
		if _, ok := severityRank[sev]; !ok {
			return fmt.Errorf("lint.severity_floor.%s: %q is not a severity (use info, warning or error)", key, sev)
		}
		if l.SeverityFloor == nil {
			l.SeverityFloor = map[string]string{}
		}
		if cur, dup := l.SeverityFloor[code]; !dup || severityRank[sev] > severityRank[cur] {
			l.SeverityFloor[code] = sev
		}
	}
	if d.Capability != nil && d.Capability.MaxNetworkCommands != nil {
		if *d.Capability.MaxNetworkCommands < 0 {
			return fmt.Errorf("lint.capability.max_network_commands: %d must not be negative", *d.Capability.MaxNetworkCommands)
		}
		n := *d.Capability.MaxNetworkCommands
		l.Capability.MaxNetworkCommands = &n
	}
	known := lint.LoadBudgetIDs()
	for id, limit := range d.LoadBudgets {
		if !slices.Contains(known, id) {
			return fmt.Errorf("lint.load_budgets: unknown limit %q (the policy is newer than this ai-rulez, or the name is wrong; known: %s)", id, strings.Join(known, ", "))
		}
		if limit < 1 {
			return fmt.Errorf("lint.load_budgets.%s: %d must be positive", id, limit)
		}
		if l.LoadBudgets == nil {
			l.LoadBudgets = map[string]int{}
		}
		l.LoadBudgets[id] = limit
	}
	if err := l.ScannerPolicy.fromDoc(d.ScannerPolicy); err != nil {
		return err
	}
	budgets, err := parseSizeBudgets(d.Budgets)
	if err != nil {
		return err
	}
	l.SizeBudgets = budgets
	if l.NoInlineIgnore, err = normalizeCodes("lint.no_inline_ignore", d.NoInlineIgnore); err != nil {
		return err
	}
	for key, limit := range d.MaxFindings {
		code, ok := lint.ResolveCode(key)
		if !ok {
			return fmt.Errorf("lint.max_findings: unknown rule %q (the policy is newer than this ai-rulez, or the name is wrong)", key)
		}
		if limit < 0 {
			return fmt.Errorf("lint.max_findings.%s: %d must not be negative", key, limit)
		}
		if l.MaxFindings == nil {
			l.MaxFindings = map[string]int{}
		}
		if cur, dup := l.MaxFindings[code]; !dup || limit < cur {
			l.MaxFindings[code] = limit
		}
	}
	return l.Security.fromDoc(d.Security)
}

func (s *Security) fromDoc(d *fileSecurity) error {
	if d == nil {
		return nil
	}
	if d.AllowedHosts != nil {
		items, err := normalizeAll("lint.security.allowed_hosts", *d.AllowedHosts)
		if err != nil {
			return err
		}
		for _, it := range items {
			if strings.Contains(it, "/") {
				return fmt.Errorf("lint.security.allowed_hosts: %q has a path; this list names hosts only", it)
			}
		}
		s.AllowedHosts = List{Set: true, Items: items}
	}
	level := strings.ToLower(strings.TrimSpace(d.ScanImports))
	if level != "" && level != "warn" && level != "error" {
		return fmt.Errorf("lint.security.scan_imports: %q is not allowed in a policy (use warn or error)", d.ScanImports)
	}
	s.ScanImports = level
	for _, tag := range d.DirectiveTags {
		if !lint.ValidDirectiveTag(tag) {
			return fmt.Errorf("lint.security.directive_tags: %q is not an element name", tag)
		}
		s.DirectiveTags = append(s.DirectiveTags, strings.TrimSpace(tag))
	}
	s.DirectiveTags = sortedUnique(s.DirectiveTags)
	if d.TrustedOrgs != nil {
		orgs := make([]string, 0, len(*d.TrustedOrgs))
		for _, o := range *d.TrustedOrgs {
			if o = strings.ToLower(strings.TrimSpace(o)); o == "" {
				return fmt.Errorf("lint.security.trusted_orgs: an entry is empty")
			}
			orgs = append(orgs, o)
		}
		s.TrustedOrgs = List{Set: true, Items: sortedUnique(orgs)}
	}
	return nil
}

func normalizeAll(key string, in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		n, err := normalizePattern(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out = append(out, n)
	}
	return sortedUnique(out), nil
}

func normalizeCodes(key string, in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		code, ok := lint.ResolveCode(raw)
		if !ok {
			return nil, fmt.Errorf("%s: unknown rule %q (the policy is newer than this ai-rulez, or the name is wrong)", key, raw)
		}
		out = append(out, code)
	}
	return sortedUnique(out), nil
}

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	w := 0
	for i, s := range out {
		if i == 0 || s != out[w-1] {
			out[w] = s
			w++
		}
	}
	return out[:w]
}

// digest is the identity of a policy file: SHA-256 of its bytes with CRLF
// normalized to LF.
func digest(data []byte) string {
	sum := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
