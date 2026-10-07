// Package ard builds and validates Agentic Resource Discovery manifests
// (ard.json, https://agenticresourcediscovery.org/spec/).
//
// It implements ARD spec v0.91, which is still a proposal, against the entry
// schema vendored in schema/ and pinned by SchemaCommit. It knows nothing about
// ai-rulez configuration: callers map their skills, MCP servers and plugins to
// a Model. Build is deterministic: it never reads the clock, the environment or
// the file system, so the same Model always yields the same bytes.
//
// Build never emits trustManifest. Section 4.5.1 requires its identity to be a
// credential whose trust domain is the identifier's publisher, and section
// 4.5.2 leaves verification to a trust framework the manifest names. ai-rulez
// signs with Sigstore keyless DSSE bundles, whose identity is a CI workflow or
// OIDC account, not a credential issued by the publisher domain, and no trust
// framework has been chosen; an emitted manifest would claim a binding no
// verifier can check. The pinned schema also spells the term "TrustManifest"
// while the spec text says "trustManifest", so the term itself is unsettled.
package ard

// SpecVersion is the ARD spec version this package implements.
const SpecVersion = "0.91"

// Severity of a finding.
type Severity string

// Severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Rules of the findings this package reports. They are stable names; the
// command layer maps them to AR codes.
const (
	// RuleSchema: the manifest does not validate against the vendored schema.
	RuleSchema = "ard-schema"
	// RuleIdentifier: an identifier breaks the Appendix C URN grammar (its
	// publisher is not an FQDN) or is used by two entries.
	RuleIdentifier = "ard-identifier"
	// RuleQueries: representativeQueries is missing or holds fewer than 2 or
	// more than 5 queries (spec section 4.2 and D.2: a warning, not an error).
	RuleQueries = "ard-representative-queries"
)

// Finding is one problem in a manifest or in a Model.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	// Identifier is the entry's URN, when the finding concerns one entry.
	Identifier string `json:"identifier,omitempty"`
	// Path is a JSON pointer into the manifest, when known.
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// HasErrors reports whether any finding is an error.
func HasErrors(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}
