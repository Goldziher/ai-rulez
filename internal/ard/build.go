package ard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/samber/oops"
)

// Kind is the kind of resource an entry describes.
type Kind string

// Kinds ai-rulez publishes.
const (
	KindSkill     Kind = "skill"
	KindMCPServer Kind = "mcp-server"
	KindPlugin    Kind = "plugin"
)

// Media types of the entry `type` term (spec section 3.3).
const (
	// MediaTypeSkill is the skill type of the spec's own skill example
	// (section 4.4). The conformance tool at the pinned commit lists
	// text/markdown; profile="urn:air:agent-skills" and
	// application/agent-skills+zip instead and reports this one as a valid
	// extension type (info, not a warning); Model.MediaTypes can switch.
	MediaTypeSkill = "application/ai-skill+md"
	// MediaTypeMCPServer is the MCP server card type (ADR-0008, which renamed
	// application/mcp-server+json).
	MediaTypeMCPServer = "application/mcp-server-card+json"
	// DefaultMediaTypePlugin is used for plugins because neither the spec nor
	// its conformance tool lists a plugin type, and agent-plugins.org defines
	// none. It sits in the vendor tree (RFC 6838 section 3.2) under ai-rulez's
	// own prefix, which the conformance tool accepts as an extension type.
	// Model.MediaTypes overrides it once a registered type exists.
	DefaultMediaTypePlugin = "application/vnd.ai-rulez.plugin+json"
)

var defaultMediaTypes = map[Kind]string{
	KindSkill:     MediaTypeSkill,
	KindMCPServer: MediaTypeMCPServer,
	KindPlugin:    DefaultMediaTypePlugin,
}

// Model is what Build publishes: one publisher, one namespace, and the
// resources to list under them.
type Model struct {
	// Publisher is the FQDN the identifiers are anchored to (Appendix C).
	Publisher string
	// Namespace is the identifier segment between publisher and name; it may
	// hold colon-separated sub-segments.
	Namespace string
	// UpdatedAt is the default updatedAt for resources without their own,
	// such as the release time. The zero value omits the term. Build never
	// reads the clock.
	UpdatedAt time.Time
	// MediaTypes overrides the default media type of a kind.
	MediaTypes map[Kind]string
	Resources  []Resource
}

// Resource is one skill, MCP server or plugin.
type Resource struct {
	Kind Kind
	// Name is the last identifier segment: letters, digits, '.', '_' or '-'.
	Name string
	// DisplayName defaults to Name.
	DisplayName string
	Description string
	Tags        []string
	Version     string
	// UpdatedAt overrides Model.UpdatedAt; the zero value inherits it.
	UpdatedAt time.Time
	// URL (an absolute https URL) or Data (the artifact inline): exactly one.
	URL  string
	Data map[string]any
	// RepresentativeQueries are kept in order, deduplicated and capped at 5.
	// DeriveQueries builds them from skill frontmatter and eval cases.
	RepresentativeQueries []string
	Capabilities          []string
}

// entry is the serialised form; the field order is the output order.
// trustManifest is never written: see the package documentation.
type entry struct {
	Identifier            string         `json:"identifier"`
	DisplayName           string         `json:"displayName"`
	Type                  string         `json:"type"`
	URL                   string         `json:"url,omitempty"`
	Data                  map[string]any `json:"data,omitzero"`
	Description           string         `json:"description,omitempty"`
	Tags                  []string       `json:"tags,omitempty"`
	Capabilities          []string       `json:"capabilities,omitempty"`
	RepresentativeQueries []string       `json:"representativeQueries,omitempty"`
	Version               string         `json:"version,omitempty"`
	UpdatedAt             string         `json:"updatedAt,omitempty"`
}

type manifest struct {
	Entries []entry `json:"entries"`
}

// Build renders the ard.json manifest for m. The output is deterministic:
// entries are sorted by identifier, tags and capabilities are sorted and
// deduplicated, timestamps are UTC RFC 3339 and come only from m. Problems
// that make an entry invalid (a bad identifier, url and data both or neither
// set, a non-https url, a duplicate identifier) are returned together as the
// error; problems that only hurt discovery (fewer than 2 or more than 5
// representative queries) are findings. The output is checked against the
// vendored schema before it is returned.
func Build(m Model) ([]byte, []Finding, error) {
	out, findings, errs, err := render(m)
	if err != nil {
		return nil, nil, err
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	check, err := Validate(out)
	if err != nil {
		return nil, nil, err
	}
	if HasErrors(check) {
		return nil, nil, oops.Errorf("ard: built manifest fails the schema: %+v", check)
	}
	return out, findings, nil
}

// Check reports every problem of m as a finding, without stopping at the first:
// an invalid identifier or entry and a duplicate identifier are errors, a
// missing or out-of-range representativeQueries is a warning, and a manifest
// that renders is validated against the vendored schema. It is what
// `validate --strict` runs; Build fails on the same errors.
func Check(m Model) ([]Finding, error) {
	out, findings, errs, err := render(m)
	if err != nil {
		return nil, err
	}
	for _, e := range errs {
		f := Finding{Rule: RuleEntry, Severity: SeverityError, Message: e.Error()}
		var re *ruleError
		if errors.As(e, &re) {
			f.Rule, f.Identifier = re.rule, re.identifier
		}
		findings = append(findings, f)
	}
	if len(errs) == 0 {
		check, err := Validate(out)
		if err != nil {
			return nil, err
		}
		for _, f := range check {
			if f.Rule != RuleQueries { // render reported those already
				findings = append(findings, f)
			}
		}
	}
	sortFindings(findings)
	return findings, nil
}

// ruleError is a problem that makes an entry invalid, tagged with its rule.
type ruleError struct {
	rule       string
	identifier string
	err        error
}

func (e *ruleError) Error() string { return e.err.Error() }
func (e *ruleError) Unwrap() error { return e.err }

// render builds the entries; errs are per-entry problems, err is a problem with
// the model itself (an unusable media type override).
func render(m Model) (out []byte, findings []Finding, errs []error, err error) {
	types, err := mediaTypes(m.MediaTypes)
	if err != nil {
		return nil, nil, nil, err
	}
	var (
		entries = make([]entry, 0, len(m.Resources))
		byID    = map[string]string{}
	)
	for _, r := range m.Resources {
		e, f, err := buildEntry(m, types, r)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if prev, dup := byID[e.Identifier]; dup {
			errs = append(errs, &ruleError{rule: RuleIdentifier, identifier: e.Identifier,
				err: oops.Errorf("%s is the identifier of more than one resource (%s and %s)", e.Identifier, prev, r.Kind)})
			continue
		}
		byID[e.Identifier] = string(r.Kind)
		entries = append(entries, e)
		findings = append(findings, f...)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Identifier < entries[j].Identifier })
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Identifier < findings[j].Identifier })
	if len(errs) > 0 {
		return nil, findings, errs, nil
	}
	out, err = encode(manifest{Entries: entries})
	return out, findings, nil, err
}

func mediaTypes(overrides map[Kind]string) (map[Kind]string, error) {
	types := make(map[Kind]string, len(defaultMediaTypes))
	for k, v := range defaultMediaTypes {
		types[k] = v
	}
	for k, v := range overrides {
		if _, known := defaultMediaTypes[k]; !known {
			return nil, oops.Errorf("media type override for unknown kind %q", k)
		}
		if strings.TrimSpace(v) == "" || !strings.Contains(v, "/") {
			return nil, oops.Errorf("media type override %q for %s is not a type/subtype", v, k)
		}
		types[k] = strings.TrimSpace(v)
	}
	return types, nil
}

func buildEntry(m Model, types map[Kind]string, r Resource) (entry, []Finding, error) {
	mediaType, known := types[r.Kind]
	if !known {
		return entry{}, nil, oops.Errorf("resource %q has unknown kind %q (want skill, mcp-server or plugin)", r.Name, r.Kind)
	}
	id, err := NewIdentifier(m.Publisher, m.Namespace, r.Name)
	if err != nil {
		return entry{}, nil, &ruleError{rule: RuleIdentifier, err: oops.Wrapf(err, "%s %q", r.Kind, r.Name)}
	}
	ident := id.String()
	if (r.URL == "") == (r.Data == nil) {
		return entry{}, nil, &ruleError{rule: RuleEntry, identifier: ident,
			err: oops.Errorf("%s: exactly one of url or data is required (spec section 4.3)", ident)}
	}
	if r.URL != "" {
		if err := checkURL(r.URL); err != nil {
			return entry{}, nil, &ruleError{rule: RuleEntry, identifier: ident, err: oops.Wrapf(err, "%s", ident)}
		}
	}
	e := entry{
		Identifier:   ident,
		DisplayName:  firstNonEmpty(strings.TrimSpace(r.DisplayName), r.Name),
		Type:         mediaType,
		URL:          r.URL,
		Data:         r.Data,
		Description:  strings.TrimSpace(r.Description),
		Tags:         sortedSet(r.Tags),
		Capabilities: sortedSet(r.Capabilities),
		Version:      strings.TrimSpace(r.Version),
	}
	if ts := r.UpdatedAt; !ts.IsZero() || !m.UpdatedAt.IsZero() {
		if ts.IsZero() {
			ts = m.UpdatedAt
		}
		e.UpdatedAt = ts.UTC().Format(time.RFC3339)
	}
	queries := normalizeQueries(r.RepresentativeQueries)
	var findings []Finding
	switch {
	case len(queries) > MaxQueries:
		findings = append(findings, Finding{Rule: RuleQueries, Severity: SeverityWarning, Identifier: ident,
			Message: fmt.Sprintf("%d representativeQueries; kept the first %d", len(queries), MaxQueries)})
		queries = queries[:MaxQueries]
	case len(queries) < MinQueries:
		var raw any // a typed nil slice would not read as "missing"
		if len(queries) > 0 {
			raw = anySlice(queries)
		}
		f, _ := queryFinding(raw)
		f.Identifier = ident
		findings = append(findings, f)
	}
	e.RepresentativeQueries = queries
	return e, findings, nil
}

// checkURL requires an absolute https URL with a host: the manifest is served
// over HTTPS and v5 rejects plain-http remotes.
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return oops.Wrapf(err, "url %q", raw)
	}
	if u.Scheme != "https" || u.Host == "" {
		return oops.Errorf("url %q must be an absolute https URL", raw)
	}
	return nil
}

// encode writes two-space indented JSON with a trailing newline and without
// HTML escaping, so <, > and & in descriptions stay readable.
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, oops.Wrapf(err, "encoding the ARD manifest")
	}
	return buf.Bytes(), nil
}

func sortedSet(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func anySlice(in []string) []any {
	if in == nil {
		return nil
	}
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
