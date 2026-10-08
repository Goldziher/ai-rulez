package signing

// ReviewerHinter is a Signer that can say, before anything is signed or logged,
// which reviewer identity its signature will carry (the identity ApprovalReviewer
// reads back from the bundle). `approve --sign` uses it to check the reviewer
// allowlist before a keyless signature is written to a public transparency log.
type ReviewerHinter interface {
	// ExpectedReviewer is the identity of the signature to come; false when it
	// cannot be told from what the signer holds.
	ExpectedReviewer() (string, bool)
}
