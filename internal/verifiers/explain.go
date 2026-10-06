package verifiers

import (
	"fmt"
	"io"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// Explain writes what a verifier checks, the rule it enforces and how to fix a
// failure. It reads the declaration only; nothing is evaluated.
func Explain(w io.Writer, cfg *config.Config, name string) error {
	specs, _ := LoadSpecs(cfg)
	for i := range specs {
		if specs[i].ID == name {
			return explainSpec(w, cfg, &specs[i])
		}
	}
	for i := range cfg.Verifiers {
		if !cfg.Verifiers[i].IsSpec() && cfg.Verifiers[i].Name == name {
			return explainLegacy(w, &cfg.Verifiers[i])
		}
	}
	return oops.Hint("Run `ai-rulez verifiers list` to see the declared names.").Errorf("unknown verifier %q", name)
}

func explainSpec(w io.Writer, cfg *config.Config, sp *Spec) error {
	t := resolveTarget(cfg, sp)
	sev := sp.Severity
	if sev == "" {
		sev = severityWarning
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", sanitize(sp.ID), sev)
	if sp.Description != "" {
		fmt.Fprintf(&b, "  %s\n", sanitize(sp.Description))
	}
	fmt.Fprintf(&b, "  enforces: %s %q", t.Kind, sanitize(t.ID))
	if t.Path != "" {
		fmt.Fprintf(&b, " (%s", sanitize(t.Path))
		if t.Line > 0 {
			fmt.Fprintf(&b, ":%d", t.Line)
		}
		b.WriteString(")")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "  declared in: %s\n", sanitize(sp.source))
	if len(sp.WhenChanged) > 0 {
		fmt.Fprintf(&b, "  scope: changed files matching %s\n", strings.Join(sp.WhenChanged, ", "))
		if len(sp.Exclude) > 0 {
			fmt.Fprintf(&b, "  excluding: %s\n", strings.Join(sp.Exclude, ", "))
		}
	} else {
		b.WriteString("  scope: the whole repository (runs on every invocation)\n")
	}
	b.WriteString("  requires:\n")
	describeRequire(&b, sp.Require, "    ")
	if sp.Message != "" {
		fmt.Fprintf(&b, "  message: %s\n", sanitize(sp.Message))
	}
	if sp.Fix != "" {
		fmt.Fprintf(&b, "  fix: %s\n", sanitize(sp.Fix))
	}
	fmt.Fprintf(&b, "  examples: %d (run `ai-rulez verifiers test %s`)\n", len(sp.Examples), sp.ID)
	_, err := io.WriteString(w, b.String())
	return wrapWrite(err)
}

func describeRequire(b *strings.Builder, r *Require, indent string) {
	switch {
	case r.Regex != nil:
		fmt.Fprintf(b, "%sregex `%s` %s\n", indent, r.Regex.Regex, regexScope(r.Regex))
	case r.Forbid != nil:
		fmt.Fprintf(b, "%sforbid `%s` %s\n", indent, r.Forbid.Regex, regexScope(r.Forbid))
	case r.FileExists != nil:
		verb := "exists"
		if r.FileExists.Exists != nil && !*r.FileExists.Exists {
			verb = "does not exist"
		}
		fmt.Fprintf(b, "%sfile %s %s\n", indent, r.FileExists.Path, verb)
	case r.Paired != nil:
		how := "changed: " + r.Paired.RequiresChanged
		if r.Paired.RequiresExists != "" {
			how = "existing: " + r.Paired.RequiresExists
		}
		fmt.Fprintf(b, "%spaired: for each %s, require %s\n", indent, r.Paired.ForEach, how)
	case r.GlobCount != nil:
		fmt.Fprintf(b, "%sglob_count %s min=%s max=%s\n", indent, r.GlobCount.Files, optInt(r.GlobCount.Min), optInt(r.GlobCount.Max))
	case len(r.All) > 0:
		describeGroup(b, "all of", r.All, indent)
	case len(r.Any) > 0:
		describeGroup(b, "any of", r.Any, indent)
	case r.Not != nil:
		fmt.Fprintf(b, "%snot:\n", indent)
		describeRequire(b, r.Not, indent+"  ")
	}
}

func describeGroup(b *strings.Builder, label string, kids []Require, indent string) {
	fmt.Fprintf(b, "%s%s:\n", indent, label)
	for i := range kids {
		describeRequire(b, &kids[i], indent+"  ")
	}
}

func regexScope(p *RegexPred) string {
	in := p.In
	if in == "" {
		in = inSameFile
	}
	if p.Files != "" {
		return "in " + in + " (" + p.Files + ")"
	}
	return "in " + in
}

func optInt(n *int) string {
	if n == nil {
		return "-"
	}
	return itoa(*n)
}

func explainLegacy(w io.Writer, v *config.VerifierConfig) error {
	sev := v.Severity
	if sev == "" {
		sev = severityError
	}
	_, err := fmt.Fprintf(w, "%s (%s)\n  type: %s\n  declared in: config.toml [[verifiers]]\n  scope: the whole repository\n  enforces: no rule or skill is named; add a spec under .ai-rulez/verifiers/ to map failures to one\n",
		sanitize(v.Name), sev, v.Type)
	return wrapWrite(err)
}

// ListRow is one declared verifier in `verifiers list`.
type ListRow struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description,omitempty"`
	// Target is the enforced item as "kind:id", empty for a flat config.toml verifier.
	Target string `json:"target,omitempty"`
	Source string `json:"source"`
	// Invalid carries the AR9H2 reason of a declaration that cannot run.
	Invalid string `json:"invalid,omitempty"`
}

// List returns the declared verifiers: the flat entries of config.toml, then
// the specs, then the invalid declarations.
func List(cfg *config.Config) []ListRow {
	rows := []ListRow{}
	for _, v := range cfg.Verifiers {
		if v.IsSpec() {
			continue
		}
		sev := v.Severity
		if sev == "" {
			sev = severityError
		}
		rows = append(rows, ListRow{Name: v.Name, Type: v.Type, Severity: sev, Description: v.Description, Source: "config.toml"})
	}
	specs, problems := LoadSpecs(cfg)
	for i := range specs {
		sp := &specs[i]
		kind, id := sp.TargetKind()
		sev := sp.Severity
		if sev == "" {
			sev = severityWarning
		}
		rows = append(rows, ListRow{Name: sp.ID, Type: predicateKind(sp.Require), Severity: sev, Description: sp.Description,
			Target: kind + ":" + id, Source: sp.source})
	}
	for _, p := range problems {
		name := p.ID
		if name == "" {
			name = p.File
		}
		rows = append(rows, ListRow{Name: sanitize(name), Type: "invalid", Severity: severityError, Source: p.File, Invalid: sanitize(p.Message)})
	}
	return rows
}
