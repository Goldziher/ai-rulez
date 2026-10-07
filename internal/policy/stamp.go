package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
)

// maxStampBytes bounds how much of a policy file the stamp hashes; a policy
// file is far smaller (maxPolicyBytes), so a larger file is stamped by its size.
const maxStampBytes = maxPolicyBytes + 1

// localFiles lists the local files a resolved policy depends on: the local
// anchors (which may appear or vanish between loads), every layer read from a
// file, and the signature sidecar next to each.
func localFiles(o DiscoverOptions, layers []Layer) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || strings.Contains(p, "://") || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p, p+SidecarSuffix)
	}
	for _, raw := range []string{o.Flag, o.envPolicy()} {
		if ref, err := ParseRef(raw, ""); raw != "" && err == nil && !ref.Remote {
			add(ref.Location)
		}
	}
	for _, p := range managedPaths(o) {
		add(p)
	}
	for i := range layers {
		add(layers[i].Path)
	}
	sort.Strings(out)
	return out
}

// fileStamp identifies the content of the files: size, modification time and
// SHA-256 of each, or "-" for a file that does not exist. An edited policy file
// gets a new stamp even when its size and mtime happen to be unchanged.
func fileStamp(files []string) string {
	var b strings.Builder
	for _, p := range files {
		b.WriteString(p)
		b.WriteByte('=')
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			b.WriteString("-\x00")
			continue
		}
		fmt.Fprintf(&b, "%d:%d:", info.Size(), info.ModTime().UnixNano())
		if info.Size() <= maxStampBytes {
			if data, rerr := os.ReadFile(p); rerr == nil { //nolint:gosec // a policy anchor or layer file chosen by the operator
				sum := sha256.Sum256(data)
				b.WriteString(hex.EncodeToString(sum[:]))
			}
		}
		b.WriteByte(0)
	}
	return b.String()
}
