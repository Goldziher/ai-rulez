package judgebench

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalizationRequiresTheQuoteAtTheExpectedPath(t *testing.T) {
	tests := []struct {
		name    string
		finding string
		want    bool
	}{
		{"primary quote", `{"location":{"path":"expected.md"},"quote":"expected evidence"}`, true},
		{"chain quote", `{"location":{"path":"other.md"},"quote":"other evidence","chain":[{"location":{"path":"expected.md"},"quote":"expected evidence"}]}`, true},
		{"quote at another path", `{"location":{"path":"expected.md"},"quote":"other evidence","chain":[{"location":{"path":"other.md"},"quote":"expected evidence"}]}`, false},
		{"quote at another chain step", `{"location":{"path":"other.md"},"quote":"other evidence","chain":[{"location":{"path":"expected.md"},"quote":"other evidence"},{"location":{"path":"third.md"},"quote":"expected evidence"}]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var finding Finding
			require.NoError(t, json.Unmarshal([]byte(tt.finding), &finding))
			expect := Expect{Path: "expected.md", Contains: "expected evidence"}
			assert.Equal(t, tt.want, locates(finding, expect))
		})
	}
}
