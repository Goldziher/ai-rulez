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
		{
			"indented ATX with a tab and closing hashes",
			"## A\na\n   ##\tB ###\nb\n",
			[]H2Span{{Header: "A", Start: 0, End: 2}, {Header: "B", Start: 2, End: 5}},
		},
		{
			"setext H2 with a multiline title",
			"## A\na\n\nSecond\nsection\n-------\nb\n",
			[]H2Span{{Header: "A", Start: 0, End: 3}, {Header: "Second section", Start: 3, End: 8}},
		},
		{
			"headings in indented code, HTML, fences and blockquotes",
			"## A\n\n    ## code\n\n<!--\n## html\n-->\n\n~~~\n   ## fence\n~~~\n\n> ## quote\n## B\n",
			[]H2Span{{Header: "A", Start: 0, End: 13}, {Header: "B", Start: 13, End: 15}},
		},
		{
			"empty ATX headings still mark boundaries",
			"## A\na\n##\n## ###",
			[]H2Span{{Header: "A", Start: 0, End: 2}, {Start: 2, End: 3}, {Start: 3, End: 4}},
		},
		{
			"frontmatter delimiter is not a Setext underline",
			"---\r\ntitle: Demo\r\n---\r\n## A\r\n",
			[]H2Span{{Start: 0, End: 3}, {Header: "A", Start: 3, End: 5}},
		},
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
