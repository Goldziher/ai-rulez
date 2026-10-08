// Package signing signs and verifies ai-rulez attestations with Sigstore
// (docs/signing.md). An attestation is a Sigstore bundle holding a DSSE envelope
// over an in-toto statement; the first user is `ai-rulez sign --lock`, which
// signs the lock subject printed by `lock --subject`; `sign --bundle`, `--skill`
// and `--sbom` sign the other subjects.
//
// # Package split
//
// This package holds the verification policy: statements, trust sets, k-of-n
// thresholds, freshness, rollback state and the lock, approval and artifact
// checks. It imports no Sigstore, Fulcio, Rekor or KMS SDK. The cryptographic
// work (bundle verification, trusted roots, key and keyless signing, KMS) is in
// internal/signing/sigstore, which the command installs once at startup with
// signing.UseBackend(sigstore.New()). Without a backend every cryptographic check
// fails closed with AR721. Library code (pkg/airulez, govview, mcp) must not
// import internal/signing/sigstore; tests/archlint enforces it.
//
// # Reusable DSSE helper API
//
// Later features (approval signatures, SBOM signing, signed policies, bundle
// signing) sign their own statements through the same calls; none of them needs
// to touch sigstore-go.
//
// Build a statement:
//
//	st, err := signing.NewStatement(predicateType, []signing.Subject{
//		{Name: "policy.toml", Digest: map[string]string{"sha256": hex}},
//	}, predicate) // predicate is any JSON-encodable value
//
// Sign it with a Signer and get the bundle JSON (a Sigstore bundle v0.3):
//
//	signer, err := sigstore.LoadKeySigner(pemBytes, password) // key mode, offline
//	signer, err := sigstore.NewKeylessSigner(sigstore.KeylessOptions{IDToken: tok}) // Fulcio + Rekor
//	bundleJSON, err := signing.SignStatement(ctx, signer, st)
//
// Only in-toto statements are signed (DSSE payload type application/vnd.in-toto+json):
// sigstore-go verifies nothing else. A feature picks its own predicate type URI
// and predicate struct; the lock's is PredicateLock with LockPredicate.
//
// Verify cryptographically with a Verifier (the trust anchors) and decide who
// may sign with a TrustSet:
//
//	v := signing.Verifier{TrustedRoot: root, Keys: pubKeys, TLog: signing.TLogRequired}
//	res, err := v.Verify(bundleJSON)          // *signing.Error with an AR code on failure
//	err = trust.Check(res, "lock", time.Now()) // AR722 when the signer is not trusted
//	err = res.Statement.RequireSubject("sha256", hex) // AR724 when the digest differs
//	err = signing.CheckFresh(res, claimedAt, maxAge, now) // AR723
//
// The Result carries the verified payload, the signer (key id, or certificate
// identity and issuer), and the time a transparency log or timestamp authority
// observed the signature (zero and Weak when there is none). Callers compare
// the statement subject with a digest they recompute themselves; a valid
// signature alone says only who signed, never that the content matches.
//
// Rollback detection is a per-user high-water mark kept outside the repository
// and authenticated with an HMAC (OpenState, State.Check, State.Advance).
//
// # Other subjects
//
// Plugin bundles, published skills and SBOM files use the same calls through
// TreeStatement and SBOMStatement (what to sign), VerifyArtifact (how a set of
// attestation files is judged, with k-of-n thresholds) and PrepareArtifactCheck
// (the [signing] policy for one subject). ProvenanceStatement writes SLSA v1
// provenance for a bundle, and sigstore.LoadKMSSigner signs with a KMS key URI.
//
// Errors returned by Verify, Check and the lock helpers are *Error values whose
// Code is one of AR720 to AR729.
package signing
