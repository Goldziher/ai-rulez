package signing

import (
	"errors"
	"fmt"
)

// Codes of signing findings (docs/strict-validation.md, AR720 to AR729). The
// same codes are registered in internal/lint; a test keeps the two equal.
const (
	CodeMissing          = "AR720"
	CodeInvalid          = "AR721"
	CodeSignerNotTrusted = "AR722"
	CodeStale            = "AR723"
	CodeSubjectMismatch  = "AR724"
	CodeRootUnavailable  = "AR725"
	CodeTLogMissing      = "AR726"
	CodeRollback         = "AR727"
	CodeThreshold        = "AR728"
	CodeProvenance       = "AR729"
)

// Names are the registry names of the codes.
var Names = map[string]string{
	CodeMissing:          "signature-missing",
	CodeInvalid:          "signature-invalid",
	CodeSignerNotTrusted: "signer-not-trusted",
	CodeStale:            "signature-stale",
	CodeSubjectMismatch:  "attestation-subject-mismatch",
	CodeRootUnavailable:  "trusted-root-unavailable",
	CodeTLogMissing:      "tlog-proof-missing",
	CodeRollback:         "signature-rollback",
	CodeThreshold:        "signature-threshold-not-met",
	CodeProvenance:       "provenance-invalid",
}

// Error is a failed verification: the AR code and a reason fit for display.
type Error struct {
	Code   string
	Reason string
	Err    error
}

// Errorf builds an Error.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Reason: fmt.Sprintf(format, args...)}
}

// Wrap builds an Error that wraps err.
func Wrap(code string, err error, format string, args ...any) *Error {
	return wrap(code, err, format, args...)
}

func wrap(code string, err error, format string, args ...any) *Error {
	return &Error{Code: code, Reason: fmt.Sprintf(format, args...), Err: err}
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s %s: %s: %v", e.Code, Names[e.Code], e.Reason, e.Err)
	}
	return fmt.Sprintf("%s %s: %s", e.Code, Names[e.Code], e.Reason)
}

func (e *Error) Unwrap() error { return e.Err }

// CodeOf returns the AR code of err, or "" when it is not an *Error.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
