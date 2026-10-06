package sbom

import (
	"strings"
	"time"

	"github.com/samber/oops"
)

// Output formats.
const (
	FormatCycloneDX = "cyclonedx"
	FormatSPDXJSON  = "spdx-json"
)

// Which files of an item are listed (with their plain SHA-256).
const (
	FilesNone   = "none"
	FilesSkills = "skills"
	FilesAll    = "all"
)

// NormalizeFormat maps a --format value to FormatCycloneDX or FormatSPDXJSON.
func NormalizeFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", FormatCycloneDX:
		return FormatCycloneDX, nil
	case FormatSPDXJSON, "spdx":
		return FormatSPDXJSON, nil
	}
	return "", oops.Hint("use cyclonedx or spdx-json").Errorf("unknown --format %q", format)
}

// Signature is the outcome of verifying the lock attestation (`--verify`).
type Signature struct {
	// Status is SignatureVerified, SignatureAbsent or SignatureInvalid.
	Status string
	// Code is the AR72x code of an invalid attestation.
	Code string
	// Signer and Issuer say who signed (a key id, or a certificate identity and issuer).
	Signer, Issuer string
	// SignedAt is when a log or timestamp authority saw the signature (zero when none did).
	SignedAt time.Time
}

// Statuses of a Signature.
const (
	SignatureVerified = "verified"
	SignatureAbsent   = "absent"
	SignatureInvalid  = "invalid"
)

// Options select what Build lists. The zero value is the default document.
type Options struct {
	// Files is FilesNone (default), FilesSkills or FilesAll.
	Files string
	// IncludeOutputs adds the generated output files, with their output digest.
	IncludeOutputs bool
	// Profile and Role narrow the document to a profile's domains and a role's items.
	Profile, Role string
	// NoApprovals leaves the approval status out; RedactReviewers replaces reviewer
	// identities with a salted hash.
	NoApprovals, RedactReviewers bool
	// RedactKey, when set, keys the reviewer hash (HMAC-SHA-256): without it the
	// salt is public and a reviewer identity can be confirmed by guessing it.
	RedactKey string
	// Signature, when set, is recorded on the project component.
	Signature *Signature
	// Timestamp, when not zero, is written as the document time (CycloneDX
	// metadata.timestamp, SPDX creationInfo.created).
	Timestamp time.Time
	// Now is the clock approval expiry is judged by; the zero value judges nothing expired.
	Now time.Time
}

func (o Options) files() string {
	if o.Files == "" {
		return FilesNone
	}
	return o.Files
}

func (o Options) validate() error {
	switch o.files() {
	case FilesNone, FilesSkills, FilesAll:
		return nil
	}
	return oops.Hint("use none, skills or all").Errorf("unknown --files %q", o.Files)
}

// scopeKey identifies the slice of the project the document describes, so two
// documents of one tree that list different things get different serial numbers.
// The default document has no key.
func (o Options) scopeKey() string {
	var parts []string
	if f := o.files(); f != FilesNone {
		parts = append(parts, "files="+f)
	}
	if o.Profile != "" {
		parts = append(parts, "profile="+o.Profile)
	}
	if o.Role != "" {
		parts = append(parts, "role="+o.Role)
	}
	if o.IncludeOutputs {
		parts = append(parts, "outputs")
	}
	if o.NoApprovals {
		parts = append(parts, "no-approvals")
	}
	if o.RedactReviewers {
		parts = append(parts, "redact-reviewers")
	}
	return strings.Join(parts, ";")
}
