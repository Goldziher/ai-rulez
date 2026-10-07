package lockrun

import (
	"errors"
	"fmt"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
)

func TestExitCode_ScanRefusalsAreFindingsNotFailures(t *testing.T) {
	served := servedRefusal(&config.Config{ConfigDir: "/p/.ai-rulez"}, DynamicResult{
		Problems: []string{"served evil: AR005 pipes a download into a shell"}, Refused: 1,
		Refusals: []mcp.Refusal{{Name: "evil", Code: "AR005", Reason: "pipes a download into a shell"}},
	})
	tests := []struct {
		name string
		res  *Result
		err  error
		want int
	}{
		{"written", &Result{}, nil, ExitOK},
		{"written with unpinned served skills", &Result{Unpinned: []mcp.Refusal{{Name: "evil"}}}, nil, ExitUnpinned},
		{"pre-pin scan refused a tree", &Result{}, prePinRefusal(nil, 1), ExitFindings},
		{"strict refused a served skill", &Result{}, served, ExitFindings},
		{"wrapped findings stay findings", &Result{}, fmt.Errorf("root a: %w", served), ExitFindings},
		{"a tool error", &Result{Unpinned: []mcp.Refusal{{Name: "evil"}}}, oops.Errorf("cannot write"), ExitFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := ExitCode(tt.res, tt.err)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServedRefusal_KeepsTheCommandLineMessageAndTheCodes(t *testing.T) {
	// Arrange
	dyn := DynamicResult{
		Problems: []string{"served evil: AR005 pipes a download into a shell"}, Refused: 1,
		Refusals: []mcp.Refusal{{Name: "evil", View: "role:qa", Code: "AR005", Reason: "pipes a download into a shell"}},
	}

	// Act
	err := servedRefusal(&config.Config{ConfigDir: "/p/.ai-rulez"}, dyn)

	// Assert
	var fe *FindingsError
	assert.True(t, errors.As(err, &fe))
	assert.ErrorIs(t, err, ErrScanRefused)
	assert.Equal(t, "cannot write ai-rulez.lock:\n  served evil: AR005 pipes a download into a shell: "+ErrScanRefused.Error(), err.Error())
	assert.Equal(t, []Finding{{Kind: "served", Name: "evil", View: "role:qa", Code: "AR005", Severity: "error", Message: "pipes a download into a shell"}}, fe.Findings)
	assert.Zero(t, fe.Sources)
}
