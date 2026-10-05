// Package contentlock computes and compares the content pins stored in
// ai-rulez.lock: sha256 digests of every authored item (rules, skills with their
// resources, agents, commands, context, hooks, roles and the settings sources) and
// of the generated outputs, plus one digest over the whole set.
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

// Domain-separation labels. The version suffix changes only with lockfile.HashVersion.
const (
	labelFile = "ai-rulez/file/v1"
	labelTree = "ai-rulez/tree/v1"
)

// kindLabel is the label of a tree of files of one item kind.
func kindLabel(kind string) string { return "ai-rulez/" + kind + "/v1" }

// File modes recorded in the digest. Only the executable bit is significant.
const (
	ModeRegular    = "100644"
	ModeExecutable = "100755"
)

// textExtensions are the file extensions whose line endings are normalized
// (CRLF to LF) before hashing, so a checkout with autocrlf does not change a
// digest. Every other file is hashed byte for byte. The list is part of the
// scheme: changing it means a new hash_version.
var textExtensions = map[string]bool{
	".md": true, ".markdown": true, ".mdc": true, ".mdx": true, ".txt": true,
	".toml": true, ".yaml": true, ".yml": true, ".json": true, ".jsonc": true,
	".sh": true, ".bash": true, ".zsh": true, ".py": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true,
}

// IsTextPath reports whether p is hashed with line-ending normalization.
func IsTextPath(p string) bool { return textExtensions[strings.ToLower(path.Ext(p))] }

// TextExtensions returns the normalized extensions, sorted.
func TextExtensions() []string {
	out := make([]string, 0, len(textExtensions))
	for e := range textExtensions {
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

// ModeFor returns ModeExecutable when any execute bit is set.
func ModeFor(perm uint32) string {
	if perm&0o111 != 0 {
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
	lps(&buf, labelFile)
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
	sorted := append([]Leaf(nil), leaves...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var buf bytes.Buffer
	lps(&buf, kindLabel(kind))
	u64(&buf, len(sorted))
	for i, l := range sorted {
		if err := validLeafPath(l.Path); err != nil {
			return "", err
		}
		if l.Mode != ModeRegular && l.Mode != ModeExecutable {
			return "", oops.With("path", l.Path).Errorf("invalid file mode %q", l.Mode)
		}
		if i > 0 && sorted[i-1].Path == l.Path {
			return "", oops.With("path", l.Path).Errorf("duplicate path in %s tree", kind)
		}
		d := leafDigest(l)
		buf.Write(d[:])
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
	lps(&buf, labelTree)
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
