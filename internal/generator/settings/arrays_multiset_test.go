package settings

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

func rawStrings(values ...string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(values))
	for _, v := range values {
		raw, _ := json.Marshal(v)
		out = append(out, raw)
	}
	return out
}

func rawsToStrings(raws []json.RawMessage) []string {
	out := make([]string, 0, len(raws))
	for _, raw := range raws {
		var s string
		_ = json.Unmarshal(raw, &s)
		out = append(out, s)
	}
	return out
}

func TestPlanElements(t *testing.T) {
	claimOf := func(values ...any) []jsonmerge.Claim {
		return []jsonmerge.Claim{{Path: []string{"list"}, Elements: values}}
	}
	tests := []struct {
		name        string
		previous    []jsonmerge.Claim
		existing    []string
		ours        []string
		wantValue   []string
		wantClaimed []string
	}{
		{
			name:      "a hand-written identical element is not claimed",
			existing:  []string{"x"},
			ours:      []string{"x", "y"},
			wantValue: []string{"x", "y"}, wantClaimed: []string{"y"},
		},
		{
			name:     "a regeneration keeps claiming what it added",
			previous: claimOf("y"), existing: []string{"x", "y"}, ours: []string{"y"},
			wantValue: []string{"x", "y"}, wantClaimed: []string{"y"},
		},
		{
			name:     "a hand-written copy beside a claimed one is claimed once",
			previous: claimOf("y"), existing: []string{"y", "y"}, ours: []string{"y"},
			wantValue: []string{"y", "y"}, wantClaimed: []string{"y"},
		},
		{
			name:     "dropping a rule removes one copy and keeps the hand-written one",
			previous: claimOf("y"), existing: []string{"y", "y"}, ours: []string{"z"},
			wantValue: []string{"y", "z"}, wantClaimed: []string{"z"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			value, claimed := planElements(tt.previous, rawStrings(tt.existing...), rawStrings(tt.ours...))

			// Assert
			assert.Equal(t, tt.wantValue, rawsToStrings(value))
			assert.Equal(t, tt.wantClaimed, rawsToStrings(claimed))
		})
	}
}
