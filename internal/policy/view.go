package policy

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// ReportSchemaVersion is the schema_version of the --show-policy JSON.
const ReportSchemaVersion = 1

// LayerView is a layer as `validate --show-policy` shows it.
type LayerView struct {
	Origin string `json:"origin"`
	Source string `json:"source"`
	Name   string `json:"name,omitempty"`
	Digest string `json:"digest"`
}

// Overrides summarizes what the repository did to the policy.
type Overrides struct {
	Accepted []string `json:"accepted"`
	Rejected int      `json:"rejected"`
}

// Report is the machine-readable effective policy (schema/policy-effective.schema.json).
type Report struct {
	SchemaVersion int                      `json:"schema_version"`
	Layers        []LayerView              `json:"layers"`
	Effective     map[string]any           `json:"effective"`
	Provenance    map[string]string        `json:"provenance"`
	Overrides     Overrides                `json:"overrides"`
	Violations    []config.PolicyViolation `json:"violations"`
}

// BuildReport describes the policy and what it did to one configuration. A nil
// resolved policy gives an empty report; a nil result leaves overrides empty.
func BuildReport(r *Resolved, res *Result) Report {
	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		Layers:        []LayerView{},
		Effective:     map[string]any{},
		Provenance:    map[string]string{},
		Overrides:     Overrides{Accepted: []string{}},
		Violations:    []config.PolicyViolation{},
	}
	if r == nil {
		return rep
	}
	for _, l := range r.Layers {
		rep.Layers = append(rep.Layers, LayerView{Origin: l.Origin, Source: l.Path, Name: l.Name, Digest: l.Digest})
	}
	rep.Effective = r.Policy.Tree()
	for k, v := range r.Provenance {
		rep.Provenance[k] = v
	}
	if res != nil && res.Outcome != nil {
		rep.Violations = append(rep.Violations, res.Outcome.Violations...)
		rep.Overrides.Accepted = append(rep.Overrides.Accepted, res.Accepted...)
		rep.Overrides.Rejected = len(res.Outcome.Violations)
	}
	return rep
}

// Tree is the policy in the shape of a policy file: only constrained keys appear.
func (p Policy) Tree() map[string]any {
	root := map[string]any{}
	table := func(path ...string) map[string]any {
		m := root
		for _, k := range path {
			next, ok := m[k].(map[string]any)
			if !ok {
				next = map[string]any{}
				m[k] = next
			}
			m = next
		}
		return m
	}
	if p.Sources.Allowed.Set {
		table("sources")["allowed_hosts"] = nonNil(p.Sources.Allowed.Items)
	}
	if len(p.Sources.Deny) > 0 {
		table("sources")["deny_hosts"] = p.Sources.Deny
	}
	if p.Sources.RequirePinned {
		table("sources")["require_pinned"] = true
	}
	if len(p.Lint.RequiredCodes) > 0 {
		table("lint")["required_codes"] = p.Lint.RequiredCodes
	}
	if len(p.Lint.SeverityFloor) > 0 {
		floor := table("lint", "severity_floor")
		for k, v := range p.Lint.SeverityFloor {
			floor[k] = v
		}
	}
	if p.Lint.Security.AllowedHosts.Set {
		table("lint", "security")["allowed_hosts"] = nonNil(p.Lint.Security.AllowedHosts.Items)
	}
	if p.Lint.Security.ScanImports != "" {
		table("lint", "security")["scan_imports"] = p.Lint.Security.ScanImports
	}
	if p.Lock.Enforce {
		table("lock")["enforce"] = true
	}
	if p.Lock.IncludeOutputs {
		table("lock")["include_outputs"] = true
	}
	if p.Telemetry.Disabled {
		table("telemetry")["allow_network"] = false
	}
	if p.LLM.Disabled {
		table("llm")["allow_network"] = false
	}
	if p.Guard.Generated {
		table("guard")["generated"] = true
	}
	g := p.Governance
	if g.Enforce {
		table("governance")["enforce"] = true
	}
	if len(g.RequireApproval) > 0 {
		table("governance")["require_approval"] = g.RequireApproval
	}
	if g.MinApprovers > 0 {
		table("governance")["min_approvers"] = g.MinApprovers
	}
	if g.Approvers.Set {
		table("governance")["approvers"] = nonNil(g.Approvers.Items)
	}
	return root
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// WriteJSON writes the report as indented JSON.
func (rep Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// WriteText writes the report for people.
func (rep Report) WriteText(w io.Writer) {
	if len(rep.Layers) == 0 {
		fmt.Fprintln(w, "policy: none (no --policy, AI_RULEZ_POLICY or managed policy file)")
		return
	}
	fmt.Fprintf(w, "policy: %d layer%s\n", len(rep.Layers), plural(len(rep.Layers)))
	width := 0
	for _, l := range rep.Layers {
		width = max(width, len(l.Source))
	}
	for _, l := range rep.Layers {
		name := ""
		if l.Name != "" {
			name = "  (" + l.Name + ")"
		}
		fmt.Fprintf(w, "  %-8s  %-*s  %s%s\n", l.Origin, width, l.Source, shortDigest(l.Digest), name)
	}
	fmt.Fprintln(w, "effective (origin in brackets)")
	flat := flatten(rep.Effective)
	keyWidth, valueWidth := 0, 0
	for _, e := range flat {
		keyWidth = max(keyWidth, len(e.key))
		valueWidth = max(valueWidth, len(e.value))
	}
	for _, e := range flat {
		fmt.Fprintf(w, "  %-*s = %-*s [%s]\n", keyWidth, e.key, valueWidth, e.value, rep.Provenance[provenanceKey(e.key)])
	}
	if len(flat) == 0 {
		fmt.Fprintln(w, "  (the policy constrains nothing)")
	}
	if len(rep.Overrides.Accepted) > 0 {
		fmt.Fprintf(w, "repo overrides accepted: %s\n", strings.Join(rep.Overrides.Accepted, "; "))
	}
	fmt.Fprintf(w, "repo overrides rejected: %d\n", rep.Overrides.Rejected)
	for _, v := range rep.Violations {
		fmt.Fprintf(w, "  %s %s:%d  %s\n", v.Code, v.File, max(v.Line, 1), v.Message)
	}
}

type flatEntry struct{ key, value string }

// flatten renders the effective tree as sorted "a.b.c = value" entries; the
// severity floor is flattened one code per line so each has its own origin.
func flatten(tree map[string]any) []flatEntry {
	var out []flatEntry
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			full := k
			if prefix != "" {
				full = prefix + "." + k
			}
			switch v := m[k].(type) {
			case map[string]any:
				walk(full, v)
			case []string:
				out = append(out, flatEntry{full, quoteList(v)})
			case string:
				out = append(out, flatEntry{full, v})
			default:
				out = append(out, flatEntry{full, fmt.Sprint(v)})
			}
		}
	}
	walk("", tree)
	return out
}

// provenanceKey is the provenance key of a flattened entry (they coincide).
func provenanceKey(key string) string { return key }

func shortDigest(d string) string {
	if i := strings.Index(d, ":"); i >= 0 && len(d) > i+13 {
		return d[:i+13] + "…"
	}
	return d
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
