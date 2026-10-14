package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestH2SpansMatchTheItemsOfASplitRead(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []H2Span
	}{
		{"no heading", "# Title\n\nText.\n", []H2Span{{Header: "", Start: 0, End: 4}}},
		{"blank preamble is no section", "\n\n## A\n\nText\n", []H2Span{{Header: "A", Start: 2, End: 6}}},
		{
			"preamble and two sections",
			"# T\n\n## A\n\na\n\n## B\n\nb\n",
			[]H2Span{{Header: "", Start: 0, End: 2}, {Header: "A", Start: 2, End: 6}, {Header: "B", Start: 6, End: 10}},
		},
		{"heading in a fence", "## A\n\n```\n## B\n```\n", []H2Span{{Header: "A", Start: 0, End: 6}}},
		{"heading without a body", "## A\n## B\nb\n", []H2Span{{Header: "A", Start: 0, End: 1}, {Header: "B", Start: 1, End: 4}}},
		{"third level heading does not split", "## A\n\n### B\n", []H2Span{{Header: "A", Start: 0, End: 4}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := H2Spans(tt.text)

			// Assert
			assert.Equal(t, tt.want, got)
			itemCount := len(splitH2(tt.text))
			assert.Equal(t, itemCount, len(got), "one span per item of a split read")
		})
	}
}
