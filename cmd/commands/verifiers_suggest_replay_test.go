package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
)

func TestRenderSuggestion_ShowsTheReplay(t *testing.T) {
	tests := []struct {
		name   string
		replay *verifiers.Replay
		want   string
		absent string
	}{
		{name: "flagged diffs are listed", replay: &verifiers.Replay{Diffs: 8, Flagged: 2, FlaggedCommits: []string{"abc12345 add a TODO"}},
			want: "# replay: would have flagged 2 of 8 merged diff(s) (abc12345 add a TODO)"},
		{name: "a clean replay", replay: &verifiers.Replay{Diffs: 4}, want: "# replay: would have flagged 0 of 4 merged diff(s)"},
		{name: "no replay, no line", replay: nil, absent: "# replay:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			res := &verifiers.SuggestResult{
				Target:    &verifiers.Target{Kind: "rule", ID: "database"},
				Proposals: []verifiers.Proposal{{ID: "no-todo", Examples: "verified", TOML: "[[verifiers]]\n", Replay: tt.replay}},
			}

			// Act
			out := renderSuggestion(res, "", false)

			// Assert
			if tt.want != "" {
				assert.Contains(t, out, tt.want)
			}
			if tt.absent != "" {
				assert.NotContains(t, out, tt.absent)
			}
		})
	}
}
