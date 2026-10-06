package review

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// fingerprint hashes the parts of a finding that survive an edit that only
// moves lines: the code, the item, the dimension and the evidence codes.
func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:16])
}
