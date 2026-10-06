package importer

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// maxMapRead bounds how much of a source file is read to map a finding back.
const maxMapRead = 5 << 20

// origins maps a planned file (path relative to the config directory, domain
// prefix included) to the source paths (relative to the source directory) it
// was built from, so a scan finding in the unwritten tree can be shown where the
// user can act on it.
func origins(plan *Plan, domain string) map[string][]string {
	out := map[string][]string{}
	for i := range plan.Items {
		it := &plan.Items[i]
		for _, f := range it.Files() {
			var srcs []string
			for _, s := range it.Sources {
				if it.Kind == KindSkill {
					// A skill source is the skill directory; its files keep their names.
					s = path.Join(filepath.ToSlash(s), strings.TrimPrefix(f.Path, it.root()+"skills/"+it.Name+"/"))
				}
				srcs = append(srcs, filepath.ToSlash(s))
			}
			out[placed(domain, f.Path)] = srcs
		}
	}
	return out
}

// sourceLocation finds, for line no (1-based) of the planned file content, the
// source file and line it came from: the same text, taking the k-th occurrence
// when the text repeats. ok is false when no source holds the line.
func sourceLocation(srcDir string, sources []string, content []byte, no int) (file string, line int, ok bool) {
	lines := bytes.Split(content, []byte("\n"))
	if no < 1 || no > len(lines) {
		return "", 0, false
	}
	want := bytes.TrimRight(lines[no-1], "\r")
	if len(bytes.TrimSpace(want)) == 0 {
		return "", 0, false
	}
	ordinal := 0
	for _, l := range lines[:no-1] {
		if bytes.Equal(bytes.TrimRight(l, "\r"), want) {
			ordinal++
		}
	}
	for _, src := range sources {
		data, err := readCapped(filepath.Join(srcDir, filepath.FromSlash(src)))
		if err != nil {
			continue
		}
		var hits []int
		for i, l := range bytes.Split(data, []byte("\n")) {
			if bytes.Equal(bytes.TrimRight(l, "\r"), want) {
				hits = append(hits, i+1)
			}
		}
		if len(hits) == 0 {
			continue
		}
		if ordinal < len(hits) {
			return src, hits[ordinal], true
		}
		return src, hits[0], true
	}
	return "", 0, false
}

func readCapped(p string) ([]byte, error) {
	f, err := os.Open(p) //nolint:gosec // a source file the importer already read
	if err != nil {
		return nil, err //nolint:wrapcheck // skipped by the caller
	}
	defer f.Close() //nolint:errcheck // read only
	buf := make([]byte, maxMapRead)
	n, _ := f.Read(buf) //nolint:errcheck // a short read is what was read
	return buf[:n], nil
}
