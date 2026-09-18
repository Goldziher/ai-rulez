package gitignore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const block = BeginMarker + "\n.claude/CLAUDE.md\n" + EndMarker + "\n"

func TestReplaceFencedBlock(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		newBlock string
		want     string
	}{
		{
			name:     "splices in place, entries after the fence stay after it",
			content:  "node_modules/\n\n" + BeginMarker + "\nold.md\n" + EndMarker + "\n\n*.log\ndist/\n",
			newBlock: block,
			want:     "node_modules/\n\n" + block + "\n*.log\ndist/\n",
		},
		{
			name:     "no fence appends the block",
			content:  "node_modules/\n",
			newBlock: block,
			want:     "node_modules/\n\n" + block,
		},
		{
			name:     "empty block drops the fence and closes the gap",
			content:  "node_modules/\n\n" + BeginMarker + "\nold.md\n" + EndMarker + "\n\n*.log\n",
			newBlock: "",
			want:     "node_modules/\n\n*.log\n",
		},
		{
			name:     "empty block on a fence-only file empties the file",
			content:  block,
			newBlock: "",
			want:     "",
		},
		{
			name:     "unterminated fence consumes the rest of the file",
			content:  "node_modules/\n\n" + BeginMarker + "\nold.md\n",
			newBlock: block,
			want:     "node_modules/\n\n" + block,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReplaceFencedBlock(tt.content, tt.newBlock)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, got, ReplaceFencedBlock(got, tt.newBlock), "not idempotent")
		})
	}
}

func TestPatternsOutsideFence(t *testing.T) {
	content := "node_modules/\n# a comment\n\n" + BeginMarker + "\n.claude/CLAUDE.md\n" + EndMarker + "\n*.log\n"
	assert.Equal(t, map[string]bool{"node_modules/": true, "*.log": true}, PatternsOutsideFence(content))
}
