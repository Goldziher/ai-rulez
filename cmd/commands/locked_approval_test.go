package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitApprovalLines(t *testing.T) {
	tests := []struct {
		name          string
		lines         []string
		wantApprovals []string
		wantDrift     []string
	}{
		{"none", nil, nil, nil},
		{
			name:          "approval change and policy problem are approvals",
			lines:         []string{"approval: AR710 missing: rule:x requires approval", "AR710 approval-missing: no lock"},
			wantApprovals: []string{"approval: AR710 missing: rule:x requires approval", "AR710 approval-missing: no lock"},
		},
		{
			name:          "a content change is drift",
			lines:         []string{"changed rule:x", "approval: AR710 missing: rule:y"},
			wantApprovals: []string{"approval: AR710 missing: rule:y"},
			wantDrift:     []string{"changed rule:x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			approvals, drift := splitApprovalLines(tt.lines)

			// Assert
			assert.Equal(t, tt.wantApprovals, approvals)
			assert.Equal(t, tt.wantDrift, drift)
		})
	}
}
