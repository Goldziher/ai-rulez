// Package contentlock computes and compares the content pins stored in
// ai-rulez.lock: sha256 digests of every authored item (rules, skills with their
// resources, agents, commands, context, hooks, roles and the settings sources),
// of the generated outputs, of remote includes, installed skills, skill sources
// and OKF includes (DigestDir) and of served skills, plus one digest over the
// whole set. It is the only hashing scheme in the lock.
//
// The scheme is specified in docs/lockfile.md and frozen by the test vectors in
// hash_test.go. In short, every digest is a SHA-256 over length-prefixed fields
// that start with a domain-separation label ("ai-rulez/<kind>/v1"), files are
// hashed from the bytes on disk (never from the frontmatter-stripped content the
// loader keeps), paths are sorted and "/"-separated, and nothing depends on a
// timestamp, a map order or the operating system.
//
// The package reads configuration types but never imports the generator: the
// caller renders the outputs and passes them in.
package contentlock

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Algorithm prefixes every digest in the lock.
const Algorithm = "sha256"

// textExt are the extensions whose CRLF line endings are normalized to LF:
// documents and data files only. Scripts (.sh, .py, .js, ...) are hashed byte
// for byte, because a CRLF in a shell script is a different program.
var textExt = map[string]bool{
	".md": true, ".markdown": true, ".mdc": true, ".mdx": true, ".txt": true,
	".toml": true, ".yaml": true, ".yml": true, ".json": true, ".jsonc": true,
}

// label is the domain-separation label of a tree of kind, "ai-rulez/<kind>/v1".
func label(kind string) string { return "ai-rulez/" + kind + "/v1" }

// File modes recorded in the digest. Only the executable bit is significant.
const (
	ModeRegular    = "100644"
	ModeExecutable = "100755"
)

// IsTextPath reports whether p is hashed with line-ending normalization.
func IsTextPath(p string) bool { return textExt[strings.ToLower(path.Ext(p))] }

// TextExtensions returns the normalized extensions, sorted.
func TextExtensions() []string {
	out := make([]string, 0, len(textExt))
	for e := range textExt {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// Leaf is one file of a tree.
type Leaf struct {
	// Path is relative to the tree root and "/"-separated.
	Path string
	// Mode is ModeRegular or ModeExecutable.
	Mode string
	Data []byte
}

// ModeFor returns ModeExecutable when the owner execute bit is set. Git records
// only that bit (100755 versus 100644), so group and other execute bits are
// ignored: a checkout then digests the same wherever the umask put them.
func ModeFor(perm uint32) string {
	if perm&0o100 != 0 {
		return ModeExecutable
	}
	return ModeRegular
}

func lp(buf *bytes.Buffer, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	buf.Write(n[:])
	buf.Write(b)
}

func lps(buf *bytes.Buffer, s string) { lp(buf, []byte(s)) }

func u64(buf *bytes.Buffer, v int) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(v)) //nolint:gosec // counts are non-negative
	buf.Write(n[:])
}

// NormalizeText converts CRLF line endings to LF. A lone CR is kept.
func NormalizeText(data []byte) []byte {
	if bytes.IndexByte(data, '\r') < 0 {
		return data
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// leafDigest hashes one file:
//
//	sha256( lp("ai-rulez/file/v1") || lp(path) || lp(mode) || lp(data) )
//
// where data is line-ending normalized for the text extensions, and lp(x) is the
// 8-byte big-endian length of x followed by x.
func leafDigest(l Leaf) [sha256.Size]byte {
	data := l.Data
	if IsTextPath(l.Path) {
		data = NormalizeText(data)
	}
	var buf bytes.Buffer
	lps(&buf, label("file"))
	lps(&buf, l.Path)
	lps(&buf, l.Mode)
	lp(&buf, data)
	return sha256.Sum256(buf.Bytes())
}

// TreeDigest returns the digest of a set of files of one kind:
//
//	sha256( lp("ai-rulez/<kind>/v1") || u64(n) || leafDigest(f1) || ... || leafDigest(fn) )
//
// with the files sorted by path (bytewise). Paths must be unique, relative,
// "/"-separated and free of "." and ".." segments.
func TreeDigest(kind string, leaves []Leaf) (string, error) {
	entries := make([]leafSum, len(leaves))
	for i, l := range leaves {
		entries[i] = leafSum{path: l.Path, mode: l.Mode, sum: leafDigest(l)}
	}
	return combineLeaves(kind, entries)
}

// leafSum is the digest of one file together with what validation needs.
type leafSum struct {
	path string
	mode string
	sum  [sha256.Size]byte
}

// combineLeaves folds per-file digests into the tree digest of kind. It is the
// one place the tree layout lives, shared by the in-memory and streaming paths.
func combineLeaves(kind string, entries []leafSum) (string, error) {
	sorted := append([]leafSum(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	var buf bytes.Buffer
	lps(&buf, label(kind))
	u64(&buf, len(sorted))
	for i, l := range sorted {
		if err := validLeafPath(l.path); err != nil {
			return "", err
		}
		if l.mode != ModeRegular && l.mode != ModeExecutable {
			return "", oops.With("path", l.path).Errorf("invalid file mode %q", l.mode)
		}
		if i > 0 && sorted[i-1].path == l.path {
			return "", oops.With("path", l.path).Errorf("duplicate path in %s tree", kind)
		}
		buf.Write(l.sum[:])
	}
	sum := sha256.Sum256(buf.Bytes())
	return Algorithm + ":" + hex.EncodeToString(sum[:]), nil
}

func validLeafPath(p string) error {
	if p == "" || strings.Contains(p, `\`) || strings.HasPrefix(p, "/") {
		return oops.With("path", p).Errorf("invalid path in a digest tree")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return oops.With("path", p).Errorf("invalid path in a digest tree")
		}
	}
	return nil
}

// Entry is one row of the top-level tree digest.
type Entry struct {
	Kind   string
	Key    string
	Digest string
}

// TopDigest returns the top-level digest over all pinned entries:
//
//	sha256( lp("ai-rulez/tree/v1") || u64(n) || { lp(kind) || lp(key) || lp(digest) }... )
//
// with the entries sorted by (kind, key).
func TopDigest(entries []Entry) string {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Kind != sorted[j].Kind {
			return sorted[i].Kind < sorted[j].Kind
		}
		return sorted[i].Key < sorted[j].Key
	})
	var buf bytes.Buffer
	lps(&buf, label("tree"))
	u64(&buf, len(sorted))
	for _, e := range sorted {
		lps(&buf, e.Kind)
		lps(&buf, e.Key)
		lps(&buf, e.Digest)
	}
	sum := sha256.Sum256(buf.Bytes())
	return Algorithm + ":" + hex.EncodeToString(sum[:])
}

func describe(kind, domain, id string) string {
	if domain != "" {
		return fmt.Sprintf("%s %s/%s", kind, domain, id)
	}
	return fmt.Sprintf("%s %s", kind, id)
}
