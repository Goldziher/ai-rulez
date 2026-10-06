package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
)

// TestApprovalCodesMatchThePackage keeps the codes declared here equal to the ones
// internal/approval reports, because lint must not import it.
func TestApprovalCodesMatchThePackage(t *testing.T) {
	assert.Equal(t, approval.CodeMissing, CodeApprovalMissing)
	assert.Equal(t, approval.CodeStale, CodeApprovalStale)
	assert.Equal(t, approval.CodeExpired, CodeApprovalExpired)
	assert.Equal(t, approval.CodeUnauthorized, CodeApproverUnauthorized)
	assert.Equal(t, approval.CodeInsufficient, CodeApprovalInsufficient)
	assert.Equal(t, approval.CodeOrphan, CodeApprovalOrphan)
}
