package signing

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// maxCosignatures bounds how many bundle files one subject may carry: the
// primary and its co-signatures.
const maxCosignatures = 16

// signerID identifies a signer for counting: a key by fingerprint, a certificate
// by identity and issuer. Two bundles by one signer count once.
func signerID(s SignerInfo) string {
	if s.Kind == KindKeyless {
		return "keyless:" + s.Identity + "@" + s.Issuer
	}
	return "key:" + s.KeyID
}

// signerKey identifies the person behind a signer when counting toward a
// threshold: a certificate identity, or the reviewer a trust entry for subject
// names for a key (reviewer = "..."), so one person holding several keys, or a
// key and a keyless identity, counts once. A key no entry names counts as itself.
func (t TrustSet) signerKey(s SignerInfo, subject, source string) string {
	if s.Kind == KindKeyless {
		return "person:" + strings.ToLower(strings.TrimSpace(s.Identity))
	}
	for i := range t.Entries {
		e := &t.Entries[i]
		if e.Reviewer != "" && e.appliesToSource(subject, source) && e.matches(s) {
			return "person:" + strings.ToLower(strings.TrimSpace(e.Reviewer))
		}
	}
	return signerID(s)
}

// pickSigners keeps the first accepted item of each distinct signer, as idOf
// names it. It returns the kept items when there are at least threshold of them
// (threshold below 1 counts as 1). With none accepted it returns the first
// error, so a single failing signature reports its own code; with some but too
// few it is AR728.
func pickSigners[T any](items []T, errs []error, idOf func(T) string, threshold int) ([]T, error) {
	threshold = max(threshold, 1)
	var kept []T
	seen := map[string]bool{}
	for _, it := range items {
		id := idOf(it)
		if !seen[id] {
			seen[id] = true
			kept = append(kept, it)
		}
	}
	if len(kept) >= threshold {
		return kept, nil
	}
	if len(kept) == 0 {
		for _, err := range errs {
			if err != nil {
				return nil, err
			}
		}
		return nil, Errorf(CodeMissing, "no attestation to verify")
	}
	rejected := ""
	for _, err := range errs {
		if err != nil {
			rejected = "; another signature was rejected: " + err.Error()
			break
		}
	}
	return nil, Errorf(CodeThreshold, "%d distinct trusted signers signed it and [signing] thresholds asks for %d%s", len(kept), threshold, rejected)
}

// BundleFiles lists the attestation files of one subject: path (the primary, when
// it exists) followed by its co-signatures, X.2.sigstore.json, X.3.sigstore.json
// ... for a primary X.sigstore.json, in numeric order. Nothing found is AR720.
func BundleFiles(path string) ([]string, error) {
	var out []string
	if _, err := os.Stat(path); err == nil {
		out = append(out, path)
	}
	extras, err := cosignatureFiles(path)
	if err != nil {
		return nil, err
	}
	out = append(out, extras...)
	if len(out) == 0 {
		return nil, Errorf(CodeMissing, "no attestation at %s", path)
	}
	if len(out) > maxCosignatures {
		return nil, Errorf(CodeInvalid, "%s has more than %d signature files", path, maxCosignatures)
	}
	return out, nil
}

// cosignatureFiles finds path's numbered siblings, sorted by number.
func cosignatureFiles(path string) ([]string, error) {
	stem, ok := strings.CutSuffix(filepath.Base(path), bundleSuffix)
	if !ok {
		return nil, nil
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil //nolint:nilnil,nilerr // an unreadable directory has no co-signatures; the primary read reports the problem
	}
	type numbered struct {
		n    int
		path string
	}
	var found []numbered
	for _, e := range entries {
		mid, ok := strings.CutPrefix(e.Name(), stem+".")
		if !ok {
			continue
		}
		mid, ok = strings.CutSuffix(mid, bundleSuffix)
		if !ok || !isDigits(mid) {
			continue
		}
		n, err := strconv.Atoi(mid)
		if err != nil {
			continue
		}
		found = append(found, numbered{n, filepath.Join(dir, e.Name())})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].n < found[j].n })
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.path
	}
	return out, nil
}

// NextCosignaturePath returns the first unused X.<n>.sigstore.json (n from 2) for
// a primary X.sigstore.json: where `sign --append` writes.
func NextCosignaturePath(path string) string {
	stem, ok := strings.CutSuffix(path, bundleSuffix)
	if !ok {
		stem = path
	}
	for n := 2; ; n++ {
		p := stem + "." + strconv.Itoa(n) + bundleSuffix
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
	}
}
