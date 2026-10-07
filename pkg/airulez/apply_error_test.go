package airulez

import (
	"errors"
	"fmt"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// TestApplyErrorTellsARefusalFromAFailure is RV-ENGINE-8: a refusal to
// overwrite a file ai-rulez cannot prove it wrote arrived as CodeApply with no
// sentinel, indistinguishable from an I/O error without matching the message.
func TestApplyErrorTellsARefusalFromAFailure(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
		refused  bool
	}{
		{name: "a refusal", err: oops.With("files", []string{"CLAUDE.md"}).Wrapf(config.ErrOutputRefused, "refusing to overwrite 1 existing file(s)"), wantCode: CodeRefused, refused: true},
		{name: "a wrapped refusal", err: fmt.Errorf("apply: %w", oops.Wrapf(config.ErrOutputRefused, "refusing")), wantCode: CodeRefused, refused: true},
		{name: "an I/O failure", err: errors.New("write CLAUDE.md: no space left on device"), wantCode: CodeApply},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := applyError(tt.err)

			// Assert
			assert.Equal(t, tt.wantCode, got.Code)
			assert.Equal(t, tt.refused, errors.Is(got, ErrRefused))
			assert.ErrorIs(t, got, tt.err)
		})
	}
}
