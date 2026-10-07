package ard

import (
	"regexp"
	"strings"

	"github.com/samber/oops"
)

// URNPrefix is the scheme and namespace identifier of every ARD identifier
// (Appendix C; ADR-0009 renamed the NID from "ai" to "air").
const URNPrefix = "urn:air:"

// maxFQDNLength is the longest DNS name in presentation form (RFC 1035 2.3.4).
const maxFQDNLength = 253

var (
	// dnsLabelRe is one RFC 1123 host name label: 1-63 letters, digits or
	// hyphens, neither first nor last a hyphen.
	dnsLabelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	// segmentRe is a namespace or name segment, as the schema pattern allows.
	segmentRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	allDigits = regexp.MustCompile(`^\d+$`)
)

// Identifier is a parsed ARD discovery identifier,
// urn:air:<publisher>:<namespace>:<name>.
type Identifier struct {
	Publisher string
	// Namespace holds zero or more colon-separated segments (ADR-0007 allows
	// none and allows several).
	Namespace string
	Name      string
}

// String renders the identifier.
func (id Identifier) String() string {
	if id.Namespace == "" {
		return URNPrefix + id.Publisher + ":" + id.Name
	}
	return URNPrefix + id.Publisher + ":" + id.Namespace + ":" + id.Name
}

// NewIdentifier builds and validates urn:air:<publisher>:<namespace>:<name>.
// Unlike the grammar, it requires a namespace: ai-rulez always publishes one.
func NewIdentifier(publisher, namespace, name string) (Identifier, error) {
	if namespace == "" {
		return Identifier{}, oops.Errorf("ARD namespace is empty")
	}
	id := Identifier{Publisher: publisher, Namespace: namespace, Name: name}
	if err := id.validate(); err != nil {
		return Identifier{}, err
	}
	return id, nil
}

// ParseIdentifier parses and validates an identifier against Appendix C: the
// schema pattern ^urn:air:[a-zA-Z0-9.-]+(:[a-zA-Z0-9._-]+)+$ plus the rule that
// the publisher is a fully qualified domain name.
func ParseIdentifier(s string) (Identifier, error) {
	rest, ok := strings.CutPrefix(s, URNPrefix)
	if !ok {
		return Identifier{}, oops.Errorf("identifier %q does not start with %q", s, URNPrefix)
	}
	parts := strings.Split(rest, ":")
	if len(parts) < 2 {
		return Identifier{}, oops.Errorf("identifier %q has no name after the publisher", s)
	}
	for _, seg := range parts[1 : len(parts)-1] {
		if !segmentRe.MatchString(seg) { // checked here: an empty segment would vanish in the join
			return Identifier{}, oops.Errorf("identifier %q: namespace segment %q must be letters, digits, '.', '_' or '-'", s, seg)
		}
	}
	id := Identifier{
		Publisher: parts[0],
		Namespace: strings.Join(parts[1:len(parts)-1], ":"),
		Name:      parts[len(parts)-1],
	}
	if err := id.validate(); err != nil {
		return Identifier{}, oops.Wrapf(err, "identifier %q", s)
	}
	return id, nil
}

func (id Identifier) validate() error {
	if err := ValidatePublisher(id.Publisher); err != nil {
		return err
	}
	if id.Namespace != "" {
		for _, seg := range strings.Split(id.Namespace, ":") {
			if !segmentRe.MatchString(seg) {
				return oops.Errorf("namespace segment %q must be letters, digits, '.', '_' or '-'", seg)
			}
		}
	}
	if !segmentRe.MatchString(id.Name) {
		return oops.Errorf("name %q must be letters, digits, '.', '_' or '-'", id.Name)
	}
	return nil
}

// ValidatePublisher checks that a publisher is a fully qualified domain name
// (compared case-insensitively, as DNS is): at least two RFC 1123 labels, no trailing dot, at most 253
// characters and a top-level label that is not all digits (so not an IP
// address). Appendix C and the URN naming guide reject "localhost"; the
// reserved "agent.localhost" and "example.com" are valid placeholders.
func ValidatePublisher(publisher string) error {
	switch {
	case publisher == "":
		return oops.Errorf("ARD publisher is empty")
	case len(publisher) > maxFQDNLength:
		return oops.Errorf("publisher %q is longer than %d characters", publisher, maxFQDNLength)
	}
	labels := strings.Split(strings.ToLower(publisher), ".")
	if len(labels) < 2 {
		return oops.Errorf("publisher %q is not a fully qualified domain name", publisher)
	}
	for _, l := range labels {
		if !dnsLabelRe.MatchString(l) {
			return oops.Errorf("publisher %q has an invalid DNS label %q", publisher, l)
		}
	}
	if allDigits.MatchString(labels[len(labels)-1]) {
		return oops.Errorf("publisher %q is an IP address, not a domain name", publisher)
	}
	return nil
}
