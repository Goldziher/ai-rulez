package usage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeEntries(t *testing.T) {
	a := Entry{ID: "a", EventID: "0000000000000001"}
	b := Entry{ID: "b", EventID: "0000000000000002"}
	legacy := Entry{ID: "a"}
	tests := []struct {
		name           string
		logs           [][]Entry
		want           []Entry
		wantDuplicates int
	}{
		{name: "no logs", want: nil},
		{name: "one log is unchanged", logs: [][]Entry{{a, b}}, want: []Entry{a, b}},
		{name: "a repeated event id counts once", logs: [][]Entry{{a, b}, {a}}, want: []Entry{a, b}, wantDuplicates: 1},
		{name: "a copy within one log also counts once", logs: [][]Entry{{a, a}}, want: []Entry{a}, wantDuplicates: 1},
		{name: "entries without an id are never merged", logs: [][]Entry{{legacy}, {legacy, legacy}}, want: []Entry{legacy, legacy, legacy}},
		{name: "order is log order then file order", logs: [][]Entry{{b}, {a}}, want: []Entry{b, a}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, duplicates := MergeEntries(tt.logs...)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantDuplicates, duplicates)
		})
	}
}
