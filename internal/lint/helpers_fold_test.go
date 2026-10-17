package lint

import (
	"strings"
	"testing"
)

// TestIndexFoldASCIIAgreesWithTheReference compares indexFoldASCII with
// strings.Index over the lower-cased text.
func TestIndexFoldASCIIAgreesWithTheReference(t *testing.T) {
	texts := []string{
		"", "a", "auto", "AUTO", "The AUTHOR runs Auto-Approve", "no match here", "xxxxxxxxxxauto", "autoauto", "aut", "a",
		"DON'T ask", "Do Not", "end with wit", "WITHOUT", "wiTHOUt asking", "ſkip", "skiP skip", strings.Repeat("a", 5000) + "skip",
		strings.Repeat("S", 5000), strings.Repeat("sk", 3000) + "ip skip",
	}
	stems := []string{"auto", "don", "do not", "without", "skip", "s", "a", "wit", "approve", "-", "0", "ip"}
	for _, text := range texts {
		for _, stem := range stems {
			want := strings.Index(strings.ToLower(text), stem)
			if strings.ContainsRune(text, 'ſ') {
				continue // ToLower leaves the long s alone, as the function does
			}
			if got := indexFoldASCII(text, stem); got != want {
				t.Errorf("indexFoldASCII(%.30q, %q) = %d, want %d", text, stem, got, want)
			}
		}
	}
}
