package tokens

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCL100KCountMemoMatchesUncachedCount(t *testing.T) {
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20)
	want, err := cl100kOnce().Count(text)
	assert.NoError(t, err)

	first, second := CL100KBase().Count(text), CL100KBase().Count(text)

	assert.Equal(t, want, first)
	assert.Equal(t, want, second)
	_, cached := cl100kCache.get(countKey{encoding: CounterCL100KBase, sum: sha256.Sum256([]byte(text))})
	assert.True(t, cached, "a long text is memoised")
}

func TestCountCacheIsBounded(t *testing.T) {
	c := newCountCache(3)
	keys := make([]countKey, 5)
	for i := range keys {
		keys[i] = countKey{encoding: "e", sum: sha256.Sum256([]byte{byte(i)})}
		c.put(keys[i], i)
	}

	assert.Len(t, c.m, 3)
	_, oldest := c.get(keys[0])
	assert.False(t, oldest, "the oldest entries are evicted first")
	n, ok := c.get(keys[4])
	assert.True(t, ok)
	assert.Equal(t, 4, n)
}
