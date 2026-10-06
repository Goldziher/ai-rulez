package skillsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestItemFromSkill(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want Item
	}{
		{"full", "---\nname: deploy\ndescription: Ship it\ntriggers: [go live, ship]\nkeywords: a, b\n---\n# Body\ntext\n",
			Item{ID: "deploy", Body: "# Body\ntext", Doc: Doc{Name: "deploy", Description: "Ship it", Triggers: []string{"go live", "ship"}, Keywords: []string{"a", "b"}}}},
		{"name and description fall back", "---\nfoo: bar\n---\nbody", Item{ID: "dir-id", Body: "body", Doc: Doc{Name: "dir-id", Description: "dir-id"}}},
		{"no frontmatter", "just text", Item{ID: "dir-id", Body: "just text", Doc: Doc{Name: "dir-id", Description: "dir-id"}}},
		{"crlf", "---\r\nname: x\r\ndescription: d\r\n---\r\nb", Item{ID: "x", Body: "b", Doc: Doc{Name: "x", Description: "d"}}},
		{"unclosed", "---\nname: x\n", Item{ID: "dir-id", Body: "---\nname: x\n", Doc: Doc{Name: "dir-id", Description: "dir-id"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ItemFromSkill("dir-id", "", []byte(tt.in))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEmbedText_FieldsAndBody(t *testing.T) {
	t.Parallel()
	it := &Item{Body: "the body text", Doc: Doc{Name: "n", Description: "d  with   spaces", Triggers: []string{"t1", "t2"}, Keywords: []string{"k1", "k2"}}}
	assert.Equal(t, "name: n\ndescription: d with spaces\ntriggers: t1; t2\nkeywords: k1, k2", EmbedText(it, Config{}))
	assert.Equal(t, "name: n", EmbedText(it, Config{Fields: []string{FieldName}}))
	assert.Equal(t, "name: n\nbody: the b", EmbedText(it, Config{Fields: []string{FieldName}, IndexBody: true, BodyChars: 5}))
	assert.Equal(t, "name: n", EmbedText(&Item{Doc: Doc{Name: "n"}}, Config{}), "empty fields are left out")
	assert.Equal(t, TextDigest("x"), TextDigest("x"))
	assert.NotEqual(t, TextDigest("x"), TextDigest("y"))
}

func TestCapQuery(t *testing.T) {
	t.Parallel()
	long := make([]rune, 3000)
	for i := range long {
		long[i] = 'é'
	}
	got := capQuery(string(long))
	assert.LessOrEqual(t, len(got), maxQueryEmbedBytes)
	assert.True(t, len(got) > 0)
	assert.Equal(t, "short", capQuery("short"))
}
