package signing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// The lint registry cannot import this package, so the codes are declared twice;
// this keeps the two equal.
func TestCodesMatchTheLintRegistry(t *testing.T) {
	want := map[string]string{
		lint.CodeSignatureMissing: CodeMissing, lint.CodeSignatureInvalid: CodeInvalid, lint.CodeSignerNotTrusted: CodeSignerNotTrusted,
		lint.CodeSignatureStale: CodeStale, lint.CodeAttestationSubject: CodeSubjectMismatch, lint.CodeTrustedRootMissing: CodeRootUnavailable,
		lint.CodeTLogProofMissing: CodeTLogMissing, lint.CodeSignatureRollback: CodeRollback,
	}
	registered := map[string]string{}
	for _, r := range lint.Rules() {
		registered[r.Code] = r.Name
	}
	assert.Len(t, Names, len(want))
	for lintCode, code := range want {
		assert.Equal(t, lintCode, code)
		assert.Equal(t, Names[code], registered[code], "name of %s", code)
	}
}
