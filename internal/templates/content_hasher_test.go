package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContentHasherMatchesHashContent(t *testing.T) {
	big := strings.Repeat("0123456789abcdef", 1<<14) // several BLAKE3 chunks
	for _, whole := range []string{"", "schema=1\n", big} {
		h := NewContentHasher()
		for i := 0; i < len(whole); i += 777 {
			h.WriteString(whole[i:min(i+777, len(whole))])
		}
		assert.Equal(t, HashContent(whole), h.Sum(), "len %d", len(whole))
	}
	h := NewContentHasher()
	h.WriteString("a")
	_, err := h.Write([]byte("b"))
	assert.NoError(t, err)
	assert.NoError(t, h.WriteByte('c'))
	assert.Equal(t, HashContent("abc"), h.Sum())
}
