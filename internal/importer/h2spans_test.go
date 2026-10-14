package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{"leading BOM", "\ufeff## A\n\na\n## B\nb\n", []H2Span{{Header: "A", Start: 0, End: 3}, {Header: "B", Start: 3, End: 6}}},
		{"CRLF", "# T\r\n\r\n## A\r\n\r\na\r\n", []H2Span{{Header: "", Start: 0, End: 2}, {Header: "A", Start: 2, End: 6}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := H2Spans(tt.text)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			itemCount := len(splitH2(normalizeText(tt.text)))
			assert.Equal(t, itemCount, len(got), "one span per item of a split read")
		})
	}
}

func TestH2SpansRefusesALoneCarriageReturn(t *testing.T) {
	// Act
	spans, err := H2Spans("# T\r\r## A\ra\r")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line endings")
	assert.Nil(t, spans)
}
