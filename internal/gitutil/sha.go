package gitutil

// IsCommitSHA reports whether s is a full git object id in lowercase hex: 40
// characters (SHA-1 repositories) or 64 (SHA-256 repositories). An abbreviated
// id, an uppercase id, a branch or a tag name is not one.
func IsCommitSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
