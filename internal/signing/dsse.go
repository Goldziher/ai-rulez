package signing

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/samber/oops"
)

// Media and statement types.
const (
	// PayloadTypeInToto is the DSSE payload type of an in-toto statement.
	PayloadTypeInToto = "application/vnd.in-toto+json"
	// StatementType is the in-toto statement type ai-rulez writes and accepts.
	StatementType = "https://in-toto.io/Statement/v1"
	// StatementTypeV01 is the in-toto statement type `cosign attest-blob` writes;
	// it is accepted on verification, never written.
	StatementTypeV01 = "https://in-toto.io/Statement/v0.1"
	// PredicateLock is the predicate type of a lock attestation.
	PredicateLock = "https://github.com/Goldziher/ai-rulez/attestations/lock/v1"
	// MaxBundleBytes bounds how much of a bundle file is read.
	MaxBundleBytes = 1 << 20
)

// Subject is one artifact a statement vouches for.
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Statement is an in-toto statement (v1). Predicate is kept raw so a verifier
// decodes it only for the predicate type it understands.
type Statement struct {
	Type          string          `json:"_type"`
	Subject       []Subject       `json:"subject"`
	PredicateType string          `json:"predicateType"`
	Predicate     json.RawMessage `json:"predicate"`
}

// NewStatement builds a statement for the subjects with predicate encoded as JSON.
func NewStatement(predicateType string, subjects []Subject, predicate any) (*Statement, error) {
	if predicateType == "" || len(subjects) == 0 {
		return nil, oops.Errorf("a statement needs a predicate type and at least one subject")
	}
	raw, err := json.Marshal(predicate)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the statement predicate")
	}
	return &Statement{Type: StatementType, Subject: subjects, PredicateType: predicateType, Predicate: raw}, nil
}

// Marshal encodes the statement as JSON, the payload that gets signed.
func (s *Statement) Marshal() ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the statement")
	}
	return data, nil
}

// ParseStatement decodes a DSSE payload as an in-toto statement (v1, or the v0.1 cosign writes).
func ParseStatement(payload []byte) (*Statement, error) {
	var s Statement
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, wrap(CodeInvalid, err, "the signed payload is not a JSON statement")
	}
	if s.Type != StatementType && s.Type != StatementTypeV01 {
		return nil, Errorf(CodeInvalid, "the signed payload has statement type %q, want %q", s.Type, StatementType)
	}
	if len(s.Subject) == 0 {
		return nil, Errorf(CodeInvalid, "the signed statement has no subject")
	}
	return &s, nil
}

// RequireSubject fails with AR724 unless the statement has a subject whose alg
// digest equals hexDigest. The statement may name several subjects; one match is
// enough, but every digest of that subject must be well formed.
func (s *Statement) RequireSubject(alg, hexDigest string) error {
	for _, sub := range s.Subject {
		if strings.EqualFold(sub.Digest[alg], hexDigest) && hexDigest != "" {
			return nil
		}
	}
	return Errorf(CodeSubjectMismatch, "the signed statement does not cover %s:%s", alg, hexDigest)
}

// DecodePredicate decodes the predicate into v. Unknown fields are ignored so a
// newer writer can add fields without breaking an older verifier; the fields a
// verifier relies on are checked by the caller.
func (s *Statement) DecodePredicate(v any) error {
	if err := json.NewDecoder(bytes.NewReader(s.Predicate)).Decode(v); err != nil {
		return wrap(CodeInvalid, err, "the signed predicate cannot be read")
	}
	return nil
}
