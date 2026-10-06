package contentlock

// ScopeApproval is content that needs a reviewer approval ([governance]) and
// does not have one that applies to its current digest. Change is Added when no
// approval exists, Changed when the digest, an expiry or the approver set stops
// matching; Old is the approved digest of a stale approval and New the current one.
const ScopeApproval = "approval"

func init() { scopeOrder[ScopeApproval] = 5 }
