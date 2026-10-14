package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCombineDedupesNotices(t *testing.T) {
	tests := []struct {
		name    string
		reports []*Report
		want    []string
	}{
		{
			name: "the same notice from two roots is kept once, in first-seen order",
			reports: []*Report{
				{Root: "a", Notices: []string{"x.md: not linted (reason)", "y.md: not linted (other)"}},
				{Root: "b", Notices: []string{"x.md: not linted (reason)"}},
			},
			want: []string{"x.md: not linted (reason)", "y.md: not linted (other)"},
		},
		{
			name:    "distinct notices stay",
			reports: []*Report{{Root: "a", Notices: []string{"a"}}, {Root: "b", Notices: []string{"b"}}},
			want:    []string{"a", "b"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Combine(tc.reports).Notices)
		})
	}
}
